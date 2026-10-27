#!/bin/sh
# Run from a Mac with Go, SSH access to the target NAS and the official QDK checkout.
set -eu
ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
QDK_SOURCE=${QDK_SOURCE:-$ROOT/private-backup/QDK}
NAS=${NAS:?Set NAS=user@nas-host}
REMOTE=${REMOTE:?Set REMOTE to an absolute NAS build directory ending in /nas-tv-wallpaper-qpkg}
case "$REMOTE" in /*/nas-tv-wallpaper-qpkg) ;; *) echo "Invalid REMOTE build directory" >&2; exit 1;; esac
case "$REMOTE" in *[!a-zA-Z0-9_./-]*|*..*) echo "Unsafe REMOTE path" >&2; exit 1;; esac
STAGE="$ROOT/private-backup/qpkg-build"
[ -f "$QDK_SOURCE/shared/bin/qbuild" ] || { echo 'Set QDK_SOURCE to an official qnap-dev/QDK checkout' >&2; exit 1; }
python3 - "$STAGE" <<'PY_CLEAN'
import pathlib,shutil,sys
p=pathlib.Path(sys.argv[1])
if p.exists(): shutil.rmtree(p)
PY_CLEAN
mkdir -p "$STAGE/package/arm_64/bin" "$STAGE/QDK"
cp -R "$ROOT/deploy/qpkg/." "$STAGE/package/"
cp "$ROOT/deploy/nas-service.sh" "$STAGE/package/shared/"
cp "$ROOT/LICENSE" "$STAGE/package/shared/"
cp -R "$QDK_SOURCE/shared/." "$STAGE/QDK/"
(cd "$ROOT/backend" && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o "$STAGE/package/arm_64/bin/nas-photos" .)
(cd "$ROOT/backend" && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o "$STAGE/package/arm_64/bin/qpkg-runner" ./cmd/qpkg-runner)
# Build QDK without installing it into QTS. Only the private staging copy is adapted.
python3 - "$STAGE/QDK" "$REMOTE" <<'PY'
import pathlib,sys
q=pathlib.Path(sys.argv[1]); remote=sys.argv[2]
p=q/'bin/qbuild'; s=p.read_text(); s=s.replace('IS_NAS="TRUE"','IS_NAS="FALSE"'); p.write_text(s)
(q/'qdk.conf').write_text('QDK_VERSION=2.5.3\nQDK_PATH="'+remote+'/QDK"\n')
PY
ssh "$NAS" "mkdir -p '$REMOTE'; rm -rf '$REMOTE/package' '$REMOTE/QDK'"
COPYFILE_DISABLE=1 tar --no-xattrs --no-mac-metadata -C "$STAGE" -czf - QDK package | ssh "$NAS" "tar -xzf - -C '$REMOTE'"
ssh "$NAS" "cd '$REMOTE/package' && QDK_PATH='$REMOTE/QDK' /bin/bash '$REMOTE/QDK/bin/qbuild' --build-arch arm_64"
mkdir -p "$ROOT/dist"
ssh "$NAS" "cd '$REMOTE/package/build' && tar -czf - ." | tar -xzf - -C "$ROOT/dist"
