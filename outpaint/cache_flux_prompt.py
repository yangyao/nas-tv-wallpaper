#!/usr/bin/env python3
"""Cache text conditioning with ComfyUI's own nodes, then release encoders."""

import argparse
import hashlib
import json
import pathlib
import shutil
import time
import requests


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--comfy-root", type=pathlib.Path, required=True)
    ap.add_argument("--prompt", required=True)
    ap.add_argument("--url", default="http://127.0.0.1:8188")
    a = ap.parse_args()
    root = a.comfy_root.expanduser().resolve()
    name = "flux-text-" + hashlib.sha256(a.prompt.encode()).hexdigest()[:16]
    folder = root / "output" / "conditioning"
    graph = {
        "34": {
            "class_type": "DualCLIPLoaderGGUF",
            "inputs": {
                "clip_name1": "clip_l.safetensors",
                "clip_name2": "t5-v1_1-xxl-encoder-Q5_K_M.gguf",
                "type": "flux",
            },
        },
        "23": {
            "class_type": "CLIPTextEncode",
            "inputs": {"clip": ["34", 0], "text": a.prompt},
        },
        "50": {
            "class_type": "SaveConditioning",
            "inputs": {
                "conditioning": ["23", 0],
                "filename_prefix": "conditioning/" + name,
            },
        },
    }
    http = requests.Session()
    http.trust_env = False
    r = http.post(a.url + "/prompt", json={"prompt": graph}, timeout=60)
    r.raise_for_status()
    ident = r.json()["prompt_id"]
    print("Encoding " + ident, flush=True)
    began = time.monotonic()
    while time.monotonic() - began < 1200:
        r = http.get(a.url + "/history/" + ident, timeout=30)
        r.raise_for_status()
        history = r.json().get(ident)
        if history:
            if history.get("status", {}).get("status_str") != "success":
                raise RuntimeError(json.dumps(history))
            files = sorted(
                folder.glob(name + "_*.safetensors"), key=lambda p: p.stat().st_mtime
            )
            if not files:
                raise RuntimeError("Completed without saved conditioning")
            target = root / "models" / "embeddings" / (name + ".safetensors")
            target.parent.mkdir(exist_ok=True, parents=True)
            shutil.copyfile(files[-1], target)
            target.chmod(0o600)
            target.with_suffix(".json").write_text(
                json.dumps(
                    {
                        "prompt": a.prompt,
                        "workflow": graph,
                        "history": history,
                        "seconds": time.monotonic() - began,
                    },
                    indent=2,
                )
            )
            target.with_suffix(".json").chmod(0o600)
            print("Cached " + target.name, flush=True)
            return
        time.sleep(3)
    raise TimeoutError("Encoding timeout; check history before retrying")


if __name__ == "__main__":
    main()
