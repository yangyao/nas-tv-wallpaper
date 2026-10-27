# NAS TV Wallpaper

Android TV launcher fork and bounded, read-only Go photo service. Original CleanYourTV guidance is retained in docs/UPSTREAM_AGENTS.md; optional LAN integration is intentional.

- Keep NAS integration off by default. Preserve offline cache, feature disable and local-wallpaper fallback.
- Never recursively index photo archives. Bound sampling, metadata reads, image work and cache sizes.
- Never commit tokens, signing keys, runtime configuration, private photos or device screenshots.
- All usernames, device addresses and data paths must be configurable. Examples must use generic values.
- Preserve upstream licenses and attribution. Do not change live devices or disable ADB during development without an explicit request.
- Run Go race tests and Android unit tests/lint/build for relevant changes. Document device verification limits.
