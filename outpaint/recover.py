#!/usr/bin/env python3
"""Finish a persisted Comfy generation without submitting another GPU task."""

import argparse, json, pathlib, time, io, re
from PIL import Image
from worker import make_worker

p = argparse.ArgumentParser()
p.add_argument("--config", required=True)
p.add_argument("id")
a = p.parse_args()
if not re.fullmatch(r"[0-9a-f]{32}", a.id):
    p.error("invalid job ID")
w = make_worker(json.loads(pathlib.Path(a.config).expanduser().read_text()))
folder = w.directory / a.id
metadata = json.loads((folder / "job.json").read_text())
job = metadata["job"]
start = time.monotonic()
r = w.http.get(w.comfy + "/history/" + metadata["prompt_id"], timeout=20)
r.raise_for_status()
history = r.json().get(metadata["prompt_id"])
images = (
    history.get("outputs", {}).get(getattr(w, "output_node", "8"), {}).get("images", [])
    if history
    else []
)
if not images:
    raise SystemExit("Generation is not complete; nothing was submitted or changed")
r = w.http.get(w.comfy + "/view", params=images[0], timeout=20)
r.raise_for_status()
generated = Image.open(io.BytesIO(r.content))
generated.load()
generated.save(folder / "generated.png")
job = w.nas(
    "POST", "jobs/" + a.id + "/resume", headers={"X-Worker-ID": w.identity}
).json()
with Image.open(folder / "original.jpg") as source:
    w.finalize(job, folder, generated, source, start)
# Keep recovery time distinct from original GPU generation time.
timing = json.loads((folder / "timing.json").read_text())
timing["recovered_without_generation"] = True
times = {
    name: data.get("timestamp")
    for name, data in history.get("status", {}).get("messages", [])
    if isinstance(data, dict)
}
if times.get("execution_success") and times.get("execution_start"):
    timing["generation_seconds"] = (
        times["execution_success"] - times["execution_start"]
    ) / 1000
(folder / "timing.json").write_text(json.dumps(timing, indent=2))
