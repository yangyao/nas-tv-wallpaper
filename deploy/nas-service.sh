#!/bin/sh
set -eu
APP_DIR=${NAS_PHOTO_APP_DIR:-$HOME/.local/share/nas-tv-wallpaper}
BINARY=${NAS_PHOTO_BINARY:-$APP_DIR/bin/nas-photos}
running(){
 [ -f "$APP_DIR/service.pid" ] || return 1
 pid=$(cat "$APP_DIR/service.pid")
 case "$pid" in ""|*[!0-9]*) return 1;; esac
 [ -r "/proc/$pid/cmdline" ] && tr '\000' ' ' < "/proc/$pid/cmdline" | grep -Fq "$BINARY"
}
case "${1:-status}" in
 start)
  if [ -f "$APP_DIR/service.pid" ]; then
   pid=$(cat "$APP_DIR/service.pid")
   if running; then exit 0; fi
  fi
  set -a
  . "$APP_DIR/server.env"
  set +a
  mkdir -p "$APP_DIR/data"
  export GOMAXPROCS=1 GOMEMLIMIT=192MiB
  : "${NAS_PHOTO_ROOT:?Set NAS_PHOTO_ROOT in server.env}"
  : "${NAS_PHOTO_LISTEN:?Set NAS_PHOTO_LISTEN to a private LAN IP:port}"
  setsid "$BINARY" --root "$NAS_PHOTO_ROOT" --data "$APP_DIR/data" --listen "$NAS_PHOTO_LISTEN" >> "$APP_DIR/server.log" 2>&1 </dev/null &
  echo $! > "$APP_DIR/service.pid"
  sleep 2
  running || { echo "NAS photo service failed to start; inspect $APP_DIR/server.log" >&2; exit 1; }
  ;;
 stop)
  if [ -f "$APP_DIR/service.pid" ]; then
   pid=$(cat "$APP_DIR/service.pid")
   if running; then
    kill "$pid"
    n=0
    while running && [ "$n" -lt 20 ]; do sleep 1; n=$((n+1)); done
    if running; then echo "Service did not stop" >&2; exit 1; fi
   fi
   rm -f "$APP_DIR/service.pid"
  fi
  ;;
 status)
  if running; then echo running; else echo stopped; exit 1; fi
  ;;
 *) echo 'usage: nas-service.sh start|stop|status' >&2; exit 2 ;;
esac
