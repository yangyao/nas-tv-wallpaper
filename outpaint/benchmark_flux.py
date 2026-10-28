#!/usr/bin/env python3
"""Local-only FLUX Fill benchmark; never uploads to the NAS or approves a photo."""

import argparse
import hashlib
import json
import pathlib
import time
import requests
from PIL import Image, ImageOps


def workflow(image, width, model, prompt, seed, steps):
    left = ((1024 - width) // 16) * 8
    right = 1024 - width - left
    return {
        "34": {
            "class_type": "DualCLIPLoaderGGUF",
            "inputs": {
                "clip_name1": "clip_l.safetensors",
                "clip_name2": "t5-v1_1-xxl-encoder-Q5_K_M.gguf",
                "type": "flux",
            },
        },
        "31": {"class_type": "UnetLoaderGGUF", "inputs": {"unet_name": model}},
        "23": {
            "class_type": "CLIPTextEncode",
            "inputs": {"clip": ["34", 0], "text": prompt},
        },
        "32": {"class_type": "VAELoader", "inputs": {"vae_name": "ae.safetensors"}},
        "46": {
            "class_type": "ConditioningZeroOut",
            "inputs": {"conditioning": ["23", 0]},
        },
        "26": {
            "class_type": "FluxGuidance",
            "inputs": {"conditioning": ["23", 0], "guidance": 30.0},
        },
        "39": {
            "class_type": "DifferentialDiffusion",
            "inputs": {"model": ["31", 0], "strength": 1.0},
        },
        "17": {"class_type": "LoadImage", "inputs": {"image": image}},
        "44": {
            "class_type": "ImagePadForOutpaint",
            "inputs": {
                "image": ["17", 0],
                "left": left,
                "top": 0,
                "right": right,
                "bottom": 0,
                "feathering": 24,
            },
        },
        "38": {
            "class_type": "InpaintModelConditioning",
            "inputs": {
                "positive": ["26", 0],
                "negative": ["46", 0],
                "vae": ["32", 0],
                "pixels": ["44", 0],
                "mask": ["44", 1],
                "noise_mask": False,
            },
        },
        "3": {
            "class_type": "KSampler",
            "inputs": {
                "model": ["39", 0],
                "positive": ["38", 0],
                "negative": ["38", 1],
                "latent_image": ["38", 2],
                "seed": seed,
                "steps": steps,
                "cfg": 1.0,
                "sampler_name": "euler",
                "scheduler": "normal",
                "denoise": 1.0,
            },
        },
        "8": {
            "class_type": "VAEDecode",
            "inputs": {"samples": ["3", 0], "vae": ["32", 0]},
        },
        "9": {
            "class_type": "SaveImage",
            "inputs": {"images": ["8", 0], "filename_prefix": "flux-benchmark"},
        },
    }


def execution_seconds(history):
    timestamps = {
        event: data.get("timestamp")
        for event, data in history.get("status", {}).get("messages", [])
        if event in ("execution_start", "execution_success")
    }
    start, end = timestamps.get("execution_start"), timestamps.get("execution_success")
    return (end - start) / 1000 if start is not None and end is not None else None


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("source", type=pathlib.Path)
    ap.add_argument("--output", type=pathlib.Path, required=True)
    ap.add_argument("--url", default="http://127.0.0.1:8188")
    ap.add_argument("--model", default="flux1-fill-dev-Q4_K_S.gguf")
    ap.add_argument("--prompt", required=True)
    ap.add_argument("--seed", type=int, default=164211176398261)
    ap.add_argument("--steps", type=int, default=20)
    ap.add_argument(
        "--conditioning", help="Cached conditioning filename from cache_flux_prompt.py"
    )
    ap.add_argument("--timeout", type=int, default=7200)
    ap.add_argument(
        "--comfy-root",
        type=pathlib.Path,
        help="Local ComfyUI root, required with cached conditioning",
    )
    args = ap.parse_args()
    cache_metadata = None
    if args.conditioning:
        if (
            not args.comfy_root
            or pathlib.Path(args.conditioning).name != args.conditioning
        ):
            ap.error("cached conditioning requires --comfy-root and a plain filename")
        cache_file = (
            args.comfy_root.expanduser() / "models" / "embeddings" / args.conditioning
        )
        cache_metadata = json.loads(cache_file.with_suffix(".json").read_text())
        if cache_metadata["prompt"] != args.prompt:
            ap.error("cached conditioning does not match the requested prompt")
        cache_metadata["sha256"] = hashlib.sha256(cache_file.read_bytes()).hexdigest()
    if (args.output / "prompt-id.txt").exists():
        raise FileExistsError(
            "Existing prompt in output folder; recover its history or choose another folder"
        )
    args.output.mkdir(parents=True, exist_ok=True, mode=0o700)
    if cache_metadata:
        (args.output / "conditioning.json").write_text(
            json.dumps(cache_metadata, indent=2)
        )
    image = ImageOps.exif_transpose(Image.open(args.source)).convert("RGB")
    width = max(8, round(image.width * 576 / image.height / 8) * 8)
    if width >= 1024:
        raise ValueError("benchmark expects a portrait source")
    image = image.resize((width, 576), Image.Resampling.LANCZOS)
    source = args.output / "input.png"
    image.save(source)
    http = requests.Session()
    http.trust_env = False
    with source.open("rb") as f:
        r = http.post(
            args.url + "/upload/image",
            files={
                "image": ("flux-benchmark-" + args.output.name + ".png", f, "image/png")
            },
            data={"overwrite": "true"},
            timeout=30,
        )
    r.raise_for_status()
    upload = r.json()
    name = (
        upload.get("subfolder", "") + "/" if upload.get("subfolder") else ""
    ) + upload["name"]
    graph = workflow(name, width, args.model, args.prompt, args.seed, args.steps)
    if args.conditioning:
        del graph["34"]
        graph["23"] = {
            "class_type": "ConditioningLoader",
            "inputs": {"conditioning_name": args.conditioning},
        }
    (args.output / "parameters.json").write_text(
        json.dumps(vars(args), default=str, indent=2)
    )
    (args.output / "workflow.json").write_text(json.dumps(graph, indent=2))
    start = time.monotonic()
    r = http.post(args.url + "/prompt", json={"prompt": graph}, timeout=60)
    if not r.ok:
        raise RuntimeError(r.text)
    ident = r.json()["prompt_id"]
    (args.output / "prompt-id.txt").write_text(ident)
    print("Submitted " + ident, flush=True)
    while time.monotonic() - start < args.timeout:
        r = http.get(args.url + "/history/" + ident, timeout=30)
        r.raise_for_status()
        history = r.json().get(ident)
        if history:
            (args.output / "history.json").write_text(json.dumps(history, indent=2))
            if history.get("status", {}).get("status_str") == "error":
                raise RuntimeError(json.dumps(history["status"]))
            images = history.get("outputs", {}).get("9", {}).get("images", [])
            if images:
                r = http.get(args.url + "/view", params=images[0], timeout=60)
                r.raise_for_status()
                (args.output / "raw.png").write_bytes(r.content)
                elapsed = time.monotonic() - start
                (args.output / "timing.json").write_text(
                    json.dumps(
                        {
                            "seconds": elapsed,
                            "execution_seconds": execution_seconds(history),
                            "size": [1024, 576],
                            "model": args.model,
                            "steps": args.steps,
                            "seed": args.seed,
                        },
                        indent=2,
                    )
                )
                print(
                    json.dumps(
                        {"seconds": elapsed, "result": str(args.output / "raw.png")}
                    ),
                    flush=True,
                )
                return
        time.sleep(5)
    raise TimeoutError(
        "Benchmark still running; prompt-id.txt allows recovery without resubmission"
    )


if __name__ == "__main__":
    main()
