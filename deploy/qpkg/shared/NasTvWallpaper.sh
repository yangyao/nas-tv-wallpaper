#!/bin/sh
set -eu
ACTION=${1:-status}
case "$ACTION" in start|stop|restart|status) ;; *) echo "usage: $0 start|stop|restart|status" >&2; exit 2;; esac
INSTALL_PATH=$(/sbin/getcfg NasTvWallpaper Install_Path -f /etc/config/qpkg.conf)
[ -n "$INSTALL_PATH" ] && [ -x "$INSTALL_PATH/service-user.sh" ] || exit 1
CONF=/etc/config/nas-tv-wallpaper.conf
SERVICE_USER=$(/sbin/getcfg Service User -f "$CONF")
export NAS_PHOTO_APP_DIR=$(/sbin/getcfg Service RuntimeDir -f "$CONF")
[ -n "$SERVICE_USER" ] && [ -n "$NAS_PHOTO_APP_DIR" ] || { echo "Configure $CONF first" >&2; exit 1; }
case "$NAS_PHOTO_APP_DIR" in /*) ;; *) echo "RuntimeDir must be absolute" >&2; exit 1;; esac
if [ "$ACTION" = start ]; then
 ENABLE=$(/sbin/getcfg NasTvWallpaper Enable -d FALSE -f /etc/config/qpkg.conf)
 [ "$ENABLE" = TRUE ] || exit 0
fi
exec "$INSTALL_PATH/bin/qpkg-runner" "$SERVICE_USER" "$INSTALL_PATH/service-user.sh" "$ACTION" "$INSTALL_PATH"
