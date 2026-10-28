#!/usr/bin/env python3
"""Toggle production and versioned publication using existing key-based SSH."""

import argparse
import json
import pathlib
import re
import shlex
import subprocess

NAS_SCRIPT = r"""set -eu
TASK_APP=$1
TASK_PKG=$2
TASK_ACTION=$3
TASK_WORKFLOW=$4
case "$TASK_WORKFLOW" in ""|*[!a-zA-Z0-9_.-]*) echo "Invalid workflow" >&2; exit 2;; esac
if [ "$TASK_ACTION" = status ]; then
 /bin/grep -E '^NAS_PHOTO_(OUTPAINT|BATCH|PUBLISH|MAX_AGE)' "$TASK_APP/server.env" || true
 exit 0
fi
if [ "$TASK_ACTION" = on ]; then TASK_VALUE=1; else TASK_VALUE=0; fi
cp -p "$TASK_APP/server.env" "$TASK_APP/server.env.before-outpaint-control"
awk -v value="$TASK_VALUE" -v workflow="$TASK_WORKFLOW" '
 /^NAS_PHOTO_OUTPAINT=/ {$0="NAS_PHOTO_OUTPAINT=1"; queue=1}
 /^NAS_PHOTO_OUTPAINT_AUTOFILL=/ {$0="NAS_PHOTO_OUTPAINT_AUTOFILL="value; fill=1}
 /^NAS_PHOTO_OUTPAINT_SERVE=/ {$0="NAS_PHOTO_OUTPAINT_SERVE="value; serve=1}
 /^NAS_PHOTO_BATCH_PUBLISH=/ {$0="NAS_PHOTO_BATCH_PUBLISH=1"; batches=1}
 /^NAS_PHOTO_PUBLISH_WORKFLOW=/ {$0="NAS_PHOTO_PUBLISH_WORKFLOW="workflow; recipe=1}
 /^NAS_PHOTO_MAX_AGE_YEARS=/ {$0="NAS_PHOTO_MAX_AGE_YEARS=10"; age=1}
 {print}
 END {if(!queue)print "NAS_PHOTO_OUTPAINT=1"; if(!fill)print "NAS_PHOTO_OUTPAINT_AUTOFILL="value; if(!serve)print "NAS_PHOTO_OUTPAINT_SERVE="value; if(!batches)print "NAS_PHOTO_BATCH_PUBLISH=1"; if(!recipe)print "NAS_PHOTO_PUBLISH_WORKFLOW="workflow; if(!age)print "NAS_PHOTO_MAX_AGE_YEARS=10"}
' "$TASK_APP/server.env" > "$TASK_APP/server.env.outpaint-tmp"
chmod 600 "$TASK_APP/server.env.outpaint-tmp"
mv "$TASK_APP/server.env.outpaint-tmp" "$TASK_APP/server.env"
export NAS_PHOTO_APP_DIR="$TASK_APP" NAS_PHOTO_BINARY="$TASK_PKG/bin/nas-photos"
"$TASK_PKG/nas-service.sh" stop
"$TASK_PKG/nas-service.sh" start
"$TASK_PKG/nas-service.sh" status
"""
MAC_SCRIPT = """import json,pathlib,sys
root=pathlib.Path(sys.argv[1]);action=sys.argv[2];p=root/'outpaint-config.json'
c=json.loads(p.read_text())
if c.get('engine')!='flux-fill':raise SystemExit('Refusing to toggle a different engine')
sys.path.insert(0,str(root/'outpaint'))
from flux_worker import recipe_key
if action!='status':
 c['enabled']=action=='on'
 tmp=p.with_suffix('.tmp');tmp.write_text(json.dumps(c,indent=2));tmp.chmod(0o600);tmp.replace(p)
print(json.dumps({'enabled':c.get('enabled',True),'start_hour':c.get('start_hour'),'end_hour':c.get('end_hour'),'max_per_day':c.get('max_per_day'),'workflow':recipe_key(c)}))
"""


def remote(target, command, script):
    if not re.fullmatch(r"[a-zA-Z0-9_][a-zA-Z0-9_.@:-]*", target):
        raise ValueError("Invalid SSH target")
    result = subprocess.run(
        ["ssh", "-o", "BatchMode=yes", target, shlex.join(command)],
        input=script,
        text=True,
        check=True,
        capture_output=True,
    )
    return result.stdout


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--config", type=pathlib.Path, required=True)
    p.add_argument("action", choices=["on", "off", "status"])
    a = p.parse_args()
    c = json.loads(a.config.expanduser().read_text())
    for key in ["nas_runtime_dir", "nas_package_dir", "mac_root"]:
        if (
            not pathlib.PurePosixPath(c[key]).is_absolute()
            or ".." in pathlib.PurePosixPath(c[key]).parts
        ):
            p.error("Absolute normalized paths required")

    def mac(action):
        return json.loads(
            remote(
                c["mac_ssh"],
                [c["mac_root"] + "/.venv/bin/python", "-", c["mac_root"], action],
                MAC_SCRIPT,
            )
        )

    info = mac("off" if a.action == "off" else "status")
    print(
        remote(
            c["nas_ssh"],
            [
                "/bin/sh",
                "-s",
                "--",
                c["nas_runtime_dir"],
                c["nas_package_dir"],
                a.action,
                info["workflow"],
            ],
            NAS_SCRIPT,
        ),
        end="",
    )
    if a.action == "on":
        info = mac("on")
    print("Mac generator:", json.dumps(info))


if __name__ == "__main__":
    main()
