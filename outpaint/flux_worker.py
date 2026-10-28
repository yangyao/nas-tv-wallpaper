#!/usr/bin/env python3
"""Bounded NAS worker: cached FLUX Fill inference followed by local light softening."""

import hashlib
import io
import json
import pathlib
import re
import time
from PIL import Image, ImageOps
from worker import Worker, atomic_json
from benchmark_flux import workflow, execution_seconds
from soften_outpaint import soften
from people_guard import check_result, NewPeopleDetected

VERSION = "flux-fill-light-v1"
DEFAULT_PROMPT = "Extend the photograph naturally, maintaining the existing surroundings, lighting, perspective and colors. A coherent, unobtrusive continuation of the environment."


def recipe_settings(config):
    return {
        "version": VERSION,
        "model": config.get("flux_model", "flux1-fill-dev-Q4_K_S.gguf"),
        "prompt": config.get("flux_prompt", DEFAULT_PROMPT),
        "radius": config.get("soften_radius", 10),
        "feather": config.get("soften_feather", 96),
        "ramp": config.get("soften_ramp", 260),
        "steps": 20,
    }


def recipe_key(config):
    digest = hashlib.sha256(
        json.dumps(recipe_settings(config), sort_keys=True).encode()
    ).hexdigest()[:10]
    return VERSION + "-" + digest


class FluxWorker(Worker):
    output_node = "9"

    def __init__(self, config):
        super().__init__(config)
        self.recipe = recipe_settings(config)
        if not (
            0 <= self.recipe["radius"] <= 100
            and 1 <= self.recipe["feather"] <= 160
            and 1 <= self.recipe["ramp"] <= 600
        ):
            raise ValueError("invalid softening settings")
        self.workflow = recipe_key(config)
        self.cache = (
            pathlib.Path(config["comfy_root"]).expanduser()
            / "models"
            / "embeddings"
            / config["flux_conditioning"]
        )
        if (
            pathlib.Path(config["flux_conditioning"]).name
            != config["flux_conditioning"]
        ):
            raise ValueError("conditioning must be a plain filename")
        if (
            json.loads(self.cache.with_suffix(".json").read_text())["prompt"]
            != self.recipe["prompt"]
        ):
            raise ValueError("cached text conditioning differs from configured prompt")
        if not self.cache.is_file():
            raise ValueError("missing cached conditioning")

    def run(self, count):
        queue = self.http.get(self.comfy + "/queue", timeout=15)
        queue.raise_for_status()
        queue = queue.json()
        if queue["queue_running"] or queue["queue_pending"]:
            print("ComfyUI busy; skipping this interval", flush=True)
            return
        if self.c.get("auto_approve", False) and not self.c.get("people_guard", False):
            # Retry a decision if the image upload succeeded but its decision request failed.
            for job in self.nas("GET", "jobs").json()["jobs"]:
                if (
                    job["workflow"] == self.workflow
                    and job["status"] == "review"
                    and job.get("worker") == self.identity
                ):
                    self.approve(job["id"])
        super().run(count)
        self.prune()

    def approve(self, ident):
        if self.c.get("auto_approve", False):
            self.nas("POST", "jobs/" + ident + "/decision", json={"status": "approved"})

    def process(self, job):
        ident = job["id"]
        if not isinstance(ident, str) or not re.fullmatch(r"[0-9a-f]{32}", ident):
            raise ValueError("invalid job ID")
        if job["workflow"] != self.workflow:
            raise ValueError("wrong workflow")
        start = time.monotonic()
        folder = self.directory / ident
        folder.mkdir(exist_ok=True, mode=0o700)
        if (folder / "result.jpg").exists():
            with Image.open(folder / "result.jpg") as result:
                if result.size != (1920, 1080):
                    raise ValueError("invalid cached result")
            self.publish(job, folder)
            return
        source_path = folder / "original.jpg"
        if not source_path.exists():
            data = self.nas("GET", "jobs/" + ident + "/input").content
            if len(data) > 4 * 1024 * 1024:
                raise ValueError("source exceeds limit")
            with Image.open(io.BytesIO(data)) as source:
                ImageOps.exif_transpose(source).convert("RGB").save(
                    source_path, quality=95
                )
        with Image.open(source_path) as im:
            source = ImageOps.exif_transpose(im).convert("RGB")
        if source.width > source.height:
            raise ValueError("source is no longer portrait")
        width = max(8, round(source.width * 576 / source.height / 8) * 8)
        prepared = source.resize((width, 576), Image.Resampling.LANCZOS)
        prepared.save(folder / "input.png")
        generated_path = folder / "generated.png"
        if generated_path.exists():
            with Image.open(generated_path) as generated:
                self.finalize(job, folder, generated, source, start)
            return
        metadata_path = folder / "job.json"
        if metadata_path.exists():
            metadata = json.loads(metadata_path.read_text())
            if metadata.get("recipe") != self.recipe:
                raise ValueError("cached generation recipe mismatch")
        else:
            if job.get("prompt") and job["prompt"] != self.recipe["prompt"]:
                raise ValueError("job prompt needs matching cached conditioning")
            with (folder / "input.png").open("rb") as f:
                r = self.http.post(
                    self.comfy + "/upload/image",
                    files={"image": (ident + ".png", f, "image/png")},
                    data={"type": "input", "overwrite": "true"},
                    timeout=30,
                )
            r.raise_for_status()
            uploaded = r.json()
            name = (
                uploaded.get("subfolder", "") + "/" if uploaded.get("subfolder") else ""
            ) + uploaded["name"]
            graph = workflow(
                name,
                width,
                self.recipe["model"],
                self.recipe["prompt"],
                int(ident[:12], 16),
                20,
            )
            del graph["34"]
            graph["23"] = {
                "class_type": "ConditioningLoader",
                "inputs": {"conditioning_name": self.cache.name},
            }
            graph["9"]["inputs"]["filename_prefix"] = "outpaint/" + ident
            r = self.http.post(
                self.comfy + "/prompt",
                json={"prompt": graph, "client_id": self.identity},
                timeout=30,
            )
            r.raise_for_status()
            metadata = {
                "job": job,
                "prompt_id": r.json()["prompt_id"],
                "workflow": graph,
                "recipe": self.recipe,
            }
            atomic_json(metadata_path, metadata)
        deadline = time.monotonic() + min(self.c.get("timeout_seconds", 1200), 1500)
        while time.monotonic() < deadline:
            r = self.http.get(
                self.comfy + "/history/" + metadata["prompt_id"], timeout=20
            )
            r.raise_for_status()
            history = r.json().get(metadata["prompt_id"])
            if history:
                atomic_json(folder / "history.json", history)
                if history.get("status", {}).get("status_str") == "error":
                    raise RuntimeError(
                        "ComfyUI workflow failed; persisted for diagnosis"
                    )
                images = (
                    history.get("outputs", {})
                    .get(self.output_node, {})
                    .get("images", [])
                )
                if images:
                    r = self.http.get(
                        self.comfy + "/view", params=images[0], timeout=60
                    )
                    r.raise_for_status()
                    with Image.open(io.BytesIO(r.content)) as generated:
                        if generated.size != (1024, 576):
                            raise ValueError("unexpected generated dimensions")
                        generated.save(generated_path)
                        self.finalize(job, folder, generated, source, start)
                    self.clean_comfy(
                        name=metadata["workflow"]["17"]["inputs"]["image"],
                        output=images[0],
                    )
                    return
            time.sleep(5)
        # Only interrupt the prompt we own, never an unrelated manual run.
        r = self.http.get(self.comfy + "/queue", timeout=15)
        r.raise_for_status()
        if any(item[1] == metadata["prompt_id"] for item in r.json()["queue_running"]):
            self.http.post(
                self.comfy + "/interrupt", json={}, timeout=15
            ).raise_for_status()
        raise TimeoutError(
            "generation deadline; saved prompt can be recovered without resubmitting"
        )

    def finalize(self, job, folder, generated, source, start):
        with Image.open(folder / "input.png") as prepared:
            prepared_size = prepared.size
        result = soften(
            source,
            generated,
            prepared_size,
            self.recipe["radius"],
            self.recipe["feather"],
            self.recipe["ramp"],
        )
        tmp = folder / "result.tmp"
        result.save(tmp, format="JPEG", quality=93)
        tmp.replace(folder / "result.jpg")
        atomic_json(folder / "recipe.json", self.recipe)
        self.publish(job, folder)
        history_path = folder / "history.json"
        atomic_json(
            folder / "timing.json",
            {
                "seconds": round(time.monotonic() - start, 2),
                "execution_seconds": execution_seconds(
                    json.loads(history_path.read_text())
                )
                if history_path.exists()
                else None,
                "status": "approved" if self.c.get("auto_approve", False) else "review",
            },
        )
        print(
            json.dumps(
                {
                    "id": job["id"],
                    "status": "approved"
                    if self.c.get("auto_approve", False)
                    else "review",
                    "cached_result": str(folder / "result.jpg"),
                }
            ),
            flush=True,
        )

    def publish(self, job, folder):
        if self.c.get("people_guard", False):
            helper = self.c.get("people_detector")
            if not helper:
                raise ValueError("people detector must be configured")
            # Always inspect cached/recovered results too. Missing raw images,
            # detector failures or malformed replies must never upload/approve.
            report = check_result(folder, helper)
            atomic_json(folder / "people-check.json", report)
            if not report["passed"]:
                raise NewPeopleDetected("people found in generated margins")
        with (folder / "result.jpg").open("rb") as f:
            self.nas(
                "PUT",
                "jobs/" + job["id"] + "/result",
                headers={"X-Worker-ID": self.identity, "Content-Type": "image/jpeg"},
                data=f,
            )
        self.approve(job["id"])

    def clean_comfy(self, name, output):
        root = pathlib.Path(self.c["comfy_root"]).expanduser().resolve()
        for base, relative in [
            (root / "input", name),
            (
                root / "output",
                str(
                    pathlib.PurePosixPath(output.get("subfolder", ""))
                    / output["filename"]
                ),
            ),
        ]:
            candidate = (base / relative).resolve()
            if candidate.is_relative_to(base) and candidate.is_file():
                candidate.unlink()

    def prune(self):
        # NAS owns the durable results. Keep at most 40 completed local diagnostic folders.
        jobs = self.nas("GET", "jobs").json()["jobs"]
        statuses = {j["id"]: j["status"] for j in jobs}
        folders = sorted(
            [
                p
                for p in self.directory.iterdir()
                if p.is_dir()
                and statuses.get(p.name) in (None, "approved", "rejected")
                and (p / "result.jpg").is_file()
                and re.fullmatch(r"[0-9a-f]{32}", p.name)
            ],
            key=lambda p: p.stat().st_mtime,
            reverse=True,
        )
        import shutil

        for folder in folders[40:]:
            shutil.rmtree(folder)
