# Security model

This is a trusted-private-LAN photo service, not an internet-facing application. Do not port-forward it. The backend binds only a private/loopback IP, requires a long Bearer token on photo routes, and exposes no arbitrary-file endpoint. HTTP does not encrypt that token; use it only on networks you trust and never reuse credentials.

Use a dedicated non-root account with read access only to the intended photo share. Protect runtime configuration and tokens; never upload photos, configuration, signing keys or device dumps to an issue. QPKG configuration in /etc/config/nas-tv-wallpaper.conf must be administrator-owned. The runtime server.env is shell configuration executed as the service user, so only that user and administrators should be able to edit it.

Before deployment review the scripts and keep your own backup APK/signing key. Community builds are unsigned by QNAP and independently built Android APKs can have different signing certificates. Package enablement/configuration is not proof of boot-time success; verify on your hardware.

To report a vulnerability, use GitHub private vulnerability reporting if available on this repository; otherwise contact the maintainer without including secrets or personal media in a public issue.
