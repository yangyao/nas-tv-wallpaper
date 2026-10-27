#!/bin/sh
set -eu
ACTION=$1
PACKAGE_DIR=$2
[ "$(id -u)" != 0 ] || { echo "Service must run as a non-root user" >&2; exit 1; }
: "${NAS_PHOTO_APP_DIR:?RuntimeDir is required}"
export NAS_PHOTO_BINARY="$PACKAGE_DIR/bin/nas-photos"
case "$ACTION" in
 start|stop|status) exec "$PACKAGE_DIR/nas-service.sh" "$ACTION";;
 restart) "$PACKAGE_DIR/nas-service.sh" stop; exec "$PACKAGE_DIR/nas-service.sh" start;;
 *) exit 2;;
esac
