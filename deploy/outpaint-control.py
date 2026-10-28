#!/usr/bin/env python3
"""Enable/disable the optional Mac generator and NAS delivery over existing SSH."""

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
if [ "$TASK_ACTION" = status ]; then
 /bin/grep '^NAS_PHOTO_OUTPAINT' "$TASK_APP/server.env" || true
 exit 0
fi
if [ "$TASK_ACTION" = on ]; then TASK_VALUE=1; else TASK_VALUE=0; fi
cp -p "$TASK_APP/server.env" "$TASK_APP/server.env.before-outpaint-control"
awk -v value="$TASK_VALUE" '
 /^NAS_PHOTO_OUTPAINT=/ {$0="NAS_PHOTO_OUTPAINT=1"; queue=1}
 /^NAS_PHOTO_OUTPAINT_AUTOFILL=/ {$0="NAS_PHOTO_OUTPAINT_AUTOFILL="value; fill=1}
 /^NAS_PHOTO_OUTPAINT_SERVE=/ {$0="NAS_PHOTO_OUTPAINT_SERVE="value; serve=1}
 {print}
 END {if(!queue)print "NAS_PHOTO_OUTPAINT=1"; if(!fill)print "NAS_PHOTO_OUTPAINT_AUTOFILL="value; if(!serve)print "NAS_PHOTO_OUTPAINT_SERVE="value}
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
if action!='status':
 c['enabled']=action=='on'
 tmp=p.with_suffix('.tmp');tmp.write_text(json.dumps(c,indent=2));tmp.chmod(0o600);tmp.replace(p)
print('Mac generator enabled:',c.get('enabled',True),'night window:',c.get('start_hour'),'-',c.get('end_hour'),'daily limit:',c.get('max_per_day'))
"""


def remote(target, command, script):
    if not re.fullmatch(r"[a-zA-Z0-9_][a-zA-Z0-9_.@:-]*", target):
        raise ValueError("Invalid SSH target")
    subprocess.run(
        ["ssh", "-o", "BatchMode=yes", target, shlex.join(command)],
        input=script,
        text=True,
        check=True,
    )


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

    def mac():
        remote(
            c["mac_ssh"],
            [c["mac_root"] + "/.venv/bin/python", "-", c["mac_root"], a.action],
            MAC_SCRIPT,
        )

    def nas():
        remote(
            c["nas_ssh"],
            [
                "/bin/sh",
                "-s",
                "--",
                c["nas_runtime_dir"],
                c["nas_package_dir"],
                a.action,
            ],
            NAS_SCRIPT,
        )

    # Stop new generation before disabling serving; enable serving before generation.
    if a.action == "off":
        mac()
        nas()
    else:
        nas()
        mac()


if __name__ == "__main__":
    main()
