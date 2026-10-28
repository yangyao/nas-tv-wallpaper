#!/usr/bin/env python3
"""Dedicated one-at-a-time ComfyUI worker. Runtime media/config stay private."""

import argparse, datetime, fcntl, io, json, pathlib, time
import subprocess
import re
import requests
from PIL import Image, ImageFilter, ImageOps, ImageDraw

Image.MAX_IMAGE_PIXELS = 40_000_000
VERSION = "sd15-inpaint-896-v3"


def prepare(source, width=896, height=504):
    source = ImageOps.exif_transpose(source).convert("RGB")
    scale = min(width / source.width, height / source.height)
    size = (max(1, round(source.width * scale)), max(1, round(source.height * scale)))
    original = source.resize(size, Image.Resampling.LANCZOS)
    x, y = (width - size[0]) // 2, (height - size[1]) // 2
    canvas = source.resize((width, height), Image.Resampling.BILINEAR).filter(
        ImageFilter.GaussianBlur(24)
    )
    canvas.paste(original, (x, y))
    alpha = Image.new("L", (width, height), 0)
    alpha.paste(Image.new("L", size, 255), (x, y))
    canvas = canvas.convert("RGBA")
    canvas.putalpha(alpha)
    return canvas, original, (x, y)


def finish(generated, source, faces=None):
    result = generated.convert("RGB").resize((1920, 1080), Image.Resampling.LANCZOS)
    source = ImageOps.exif_transpose(source).convert("RGB")
    scale = min(1920 / source.width, 1080 / source.height)
    size = (max(1, round(source.width * scale)), max(1, round(source.height * scale)))
    original = source.resize(size, Image.Resampling.LANCZOS)
    position = ((1920 - size[0]) // 2, (1080 - size[1]) // 2)
    if not faces:
        result.paste(original, position)
        return result
    # Soften only the background border of the original rectangle. Face boxes are fully opaque.
    mask = Image.new("L", size, 255)
    draw = ImageDraw.Draw(mask)
    feather = min(72, size[0] // 5, size[1] // 5)
    for edge in range(feather):
        alpha = round(255 * edge / feather)
        if position[0] > 0:
            draw.line((edge, 0, edge, size[1] - 1), fill=alpha)
            draw.line(
                (size[0] - 1 - edge, 0, size[0] - 1 - edge, size[1] - 1), fill=alpha
            )
        if position[1] > 0:
            draw.line((0, edge, size[0] - 1, edge), fill=alpha)
            draw.line(
                (0, size[1] - 1 - edge, size[0] - 1, size[1] - 1 - edge), fill=alpha
            )
    for x, y, w, h in faces:
        pad = 0.25
        rect = (
            max(0, round((x - w * pad) * size[0])),
            max(0, round((y - h * pad) * size[1])),
            min(size[0] - 1, round((x + w * (1 + pad)) * size[0])),
            min(size[1] - 1, round((y + h * (1 + pad)) * size[1])),
        )
        draw.rectangle(rect, fill=255)
    result.paste(original, position, mask)
    return result


def atomic_json(path, value):
    tmp = path.with_suffix(".tmp")
    tmp.write_text(json.dumps(value, indent=2))
    tmp.chmod(0o600)
    tmp.replace(path)


class Worker:
    def __init__(self, config):
        self.c = config
        self.base = config["nas_url"].rstrip("/")
        self.comfy = config.get("comfy_url", "http://127.0.0.1:8188").rstrip("/")
        self.http = requests.Session()
        self.http.trust_env = False
        self.headers = {"Authorization": "Bearer " + config["token"]}
        self.directory = pathlib.Path(config["work_dir"]).expanduser()
        self.directory.mkdir(parents=True, exist_ok=True)
        self.directory.chmod(0o700)
        self.identity = config.get("worker_id", "mac-worker")
        self.workflow = config.get("workflow_version", VERSION)

    def nas(self, method, path, **kwargs):
        headers = {**self.headers, **kwargs.pop("headers", {})}
        r = self.http.request(
            method,
            self.base + "/v1/outpaint/" + path,
            headers=headers,
            timeout=60,
            **kwargs,
        )
        r.raise_for_status()
        return r

    def process(self, job):
        ident = job["id"]
        if not isinstance(ident, str) or not re.fullmatch(r"[0-9a-f]{32}", ident):
            raise ValueError("invalid job ID")
        start = time.monotonic()
        folder = self.directory / ident
        folder.mkdir(exist_ok=True)
        cached = folder / "result.jpg"
        if cached.exists():
            with Image.open(cached) as im:
                if im.size != (1920, 1080):
                    raise ValueError("invalid local cached result")
            with cached.open("rb") as f:
                self.nas(
                    "PUT",
                    "jobs/" + ident + "/result",
                    headers={
                        "X-Worker-ID": self.identity,
                        "Content-Type": "image/jpeg",
                    },
                    data=f,
                )
            print(
                json.dumps({"id": ident, "status": "review", "reused": True}),
                flush=True,
            )
            return
        data = self.nas("GET", "jobs/" + ident + "/input").content
        if len(data) > 4 * 1024 * 1024:
            raise ValueError("source exceeds limit")
        source = Image.open(io.BytesIO(data))
        source.load()
        source.convert("RGB").save(folder / "original.jpg", quality=95)
        canvas, original, pos = prepare(source)
        canvas.save(folder / "input.png")
        with (folder / "input.png").open("rb") as f:
            r = self.http.post(
                self.comfy + "/upload/image",
                files={"image": (ident + ".png", f, "image/png")},
                data={"type": "input", "overwrite": "false"},
                timeout=30,
            )
            r.raise_for_status()
            upload = r.json()
        workflow = json.loads(
            pathlib.Path(__file__).with_name("workflow.json").read_text()
        )
        workflow["1"]["inputs"]["ckpt_name"] = self.c.get(
            "checkpoint", "sd-v1-5-inpainting.safetensors"
        )
        workflow["2"]["inputs"]["image"] = (
            upload.get("subfolder", "") + "/" if upload.get("subfolder") else ""
        ) + upload["name"]
        if job.get("prompt"):
            workflow["3"]["inputs"]["text"] = job["prompt"]
        workflow["6"]["inputs"]["seed"] = int(ident[:12], 16)
        workflow["8"]["inputs"]["filename_prefix"] = "outpaint/" + ident
        r = self.http.post(
            self.comfy + "/prompt",
            json={"prompt": workflow, "client_id": self.identity},
            timeout=30,
        )
        r.raise_for_status()
        prompt_id = r.json()["prompt_id"]
        atomic_json(
            folder / "job.json",
            {"job": job, "prompt_id": prompt_id, "workflow": workflow},
        )
        deadline = time.monotonic() + self.c.get("timeout_seconds", 1200)
        while time.monotonic() < deadline:
            r = self.http.get(self.comfy + "/history/" + prompt_id, timeout=15)
            r.raise_for_status()
            history = r.json().get(prompt_id)
            if history:
                if history.get("status", {}).get("status_str") == "error":
                    raise RuntimeError("ComfyUI workflow failed")
                images = history.get("outputs", {}).get("8", {}).get("images", [])
                if images:
                    r = self.http.get(
                        self.comfy + "/view", params=images[0], timeout=30
                    )
                    r.raise_for_status()
                    generated = Image.open(io.BytesIO(r.content))
                    generated.load()
                    generated.save(folder / "generated.png")
                    if generated.size != canvas.size:
                        raise ValueError("unexpected dimensions")
                    self.finalize(job, folder, generated, source, start)
                    return
            time.sleep(2)
        self.http.post(self.comfy + "/interrupt", json={}, timeout=10)
        raise TimeoutError("generation deadline exceeded")

    def finalize(self, job, folder, generated, source, start):
        ident = job["id"]
        faces = None
        helper = self.c.get("face_helper")
        if helper:
            try:
                detected = subprocess.run(
                    [
                        str(pathlib.Path(helper).expanduser()),
                        str(folder / "original.jpg"),
                    ],
                    capture_output=True,
                    check=True,
                    timeout=30,
                )
                faces = json.loads(detected.stdout)
            except (OSError, subprocess.SubprocessError, ValueError):
                faces = None  # Strict original rectangle if the local detector is unavailable.
        result = finish(generated, source, faces)
        result.save(folder / "result.jpg", quality=93)
        with (folder / "result.jpg").open("rb") as f:
            self.nas(
                "PUT",
                "jobs/" + ident + "/result",
                headers={
                    "X-Worker-ID": self.identity,
                    "Content-Type": "image/jpeg",
                },
                data=f,
            )
        elapsed = round(time.monotonic() - start, 2)
        atomic_json(folder / "timing.json", {"seconds": elapsed, "status": "review"})
        print(
            json.dumps({"id": ident, "status": "review", "seconds": elapsed}),
            flush=True,
        )

    def run(self, count):
        statefile = self.directory / "state.json"
        state = json.loads(statefile.read_text()) if statefile.exists() else {}
        day = datetime.date.today().isoformat()
        if state.get("day") != day:
            state = {"day": day, "completed": 0, "attempted": 0}
        for _ in range(count):
            if state.get("attempted", 0) >= self.c.get("max_per_day", 6):
                break
            reply = self.nas(
                "POST",
                "claim",
                json={"worker": self.identity, "workflow": self.workflow},
            )
            if reply.status_code == 204:
                break
            job = reply.json()
            state["attempted"] = state.get("attempted", 0) + 1
            atomic_json(statefile, state)
            try:
                self.process(job)
                state["completed"] = state.get("completed", 0) + 1
            except Exception as error:
                print(
                    json.dumps(
                        {
                            "id": job["id"],
                            "status": "failed",
                            "error_type": type(error).__name__,
                        }
                    ),
                    flush=True,
                )
                try:
                    self.nas(
                        "POST",
                        "jobs/" + job["id"] + "/fail",
                        headers={"X-Worker-ID": self.identity},
                    )
                except requests.RequestException:
                    pass
            finally:
                try:
                    self.http.post(
                        self.comfy + "/free",
                        json={"unload_models": True, "free_memory": True},
                        timeout=20,
                    )
                except requests.RequestException:
                    pass
            atomic_json(statefile, state)


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--config", required=True)
    p.add_argument("--count", type=int, default=1)
    a = p.parse_args()
    config = json.loads(pathlib.Path(a.config).expanduser().read_text())
    worker = Worker(config)
    if (
        not config.get("start_hour", 0)
        <= datetime.datetime.now().hour
        < config.get("end_hour", 24)
    ):
        return
    with (worker.directory / "worker.lock").open("w") as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            return
        worker.run(max(1, min(a.count, 10)))


if __name__ == "__main__":
    main()
