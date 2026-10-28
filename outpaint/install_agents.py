#!/usr/bin/env python3
"""Install user LaunchAgents; never changes system power/sleep settings."""

import argparse, os, pathlib, plistlib, subprocess

p = argparse.ArgumentParser()
p.add_argument("--root", type=pathlib.Path, required=True)
p.add_argument("--config", type=pathlib.Path, required=True)
p.add_argument("--worker", action="store_true")
a = p.parse_args()
root = a.root.expanduser().resolve()
config = a.config.expanduser().resolve()
agents = pathlib.Path.home() / "Library/LaunchAgents"
agents.mkdir(parents=True, exist_ok=True)
logs = root / "logs"
logs.mkdir(exist_ok=True)
python = str(root / ".venv/bin/python")
domain = "gui/" + str(os.getuid())
services = {
    "com.nastv.comfy": {
        "ProgramArguments": [
            python,
            str(root / "ComfyUI/main.py"),
            "--listen",
            "127.0.0.1",
            "--port",
            "8188",
            "--cpu-vae",
            "--preview-method",
            "none",
            "--offline",
        ],
        "WorkingDirectory": str(root / "ComfyUI"),
        "KeepAlive": True,
        "RunAtLoad": True,
        "EnvironmentVariables": {
            "PYTORCH_ENABLE_MPS_FALLBACK": "1",
            "HF_HUB_OFFLINE": "1",
            "TRANSFORMERS_OFFLINE": "1",
        },
    }
}
if a.worker:
    services["com.nastv.outpaint-worker"] = {
        "ProgramArguments": [
            python,
            str(root / "outpaint/worker.py"),
            "--config",
            str(config),
            "--count",
            "1",
        ],
        "WorkingDirectory": str(root),
        "StartInterval": 1800,
        "RunAtLoad": False,
    }
for label, body in services.items():
    body.update(
        Label=label,
        StandardOutPath=str(logs / (label + ".log")),
        StandardErrorPath=str(logs / (label + ".error.log")),
    )
    path = agents / (label + ".plist")
    path.write_bytes(plistlib.dumps(body))
    path.chmod(0o600)
    subprocess.run(
        ["launchctl", "bootout", domain, str(path)],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    subprocess.run(["launchctl", "bootstrap", domain, str(path)], check=True)
    print("Installed", label)
