# NAS TV Wallpaper

Android TV launcher with opt-in, locally cached NAS photo wallpaper, plus a small Go photo service. Based on [Gathin23/cleanyourtv](https://github.com/Gathin23/cleanyourtv); upstream documentation is preserved in [docs/UPSTREAM_README.md](docs/UPSTREAM_README.md). Original MIT license and third-party notices are retained.

## How it works

- NAS photos are **off by default**. On the TV, open the gear menu → 照片背景（NAS）, enter a private LAN URL and a generated access token, then enable.
- The NAS returns small random batches using bounded directory reads and camera EXIF checks. No recursive archive scan or whole-library index.
- JPEG only for now. Conservative metadata and filename filters reject many screenshots/downloads, but are not semantic photo recognition. HEIC and images lacking camera metadata are skipped.
- Recent selections are remembered to reduce repeats; copies at different paths can still repeat.
- Each requested photo becomes a 1920×1080 JPEG with the complete photo centred over a softened backdrop. Originals are read-only; exported metadata is stripped.
- TV wallpaper changes every two minutes: **landscape originals display individually; portrait/square originals pair side by side**. Pairing excludes landscapes and refits the complete photo foreground over the soft backdrop. If there is no second portrait, show one complete image.
- Layout uses EXIF-oriented original dimensions, not the padded 16:9 cache frame. Dimensions are cached locally; an authenticated HEAD request can hydrate older image caches without redownloading images or reading original JPEGs. Unknown dimensions safely use single-image display until metadata is available.
- TV downloads only while Home is visible. After six hours, returning Home requests another batch. No scheduled wakeups or downloads during video playback.
- The first three downloads can appear immediately. Later batches normally need at least ten completed images (or the entire shorter batch) before switching atomically. Failed requests retain the cache and retry no sooner than 30 minutes later.
- Disabling restores normal local wallpaper. The existing clock screensaver does not use the NAS photo cache.

NAS sampling is limited to 100 metadata candidates, 32 directory chunks of 256 entries, roughly ten seconds per batch, and two photos per selected directory. Directory windows and recent-selection state are bounded. Image conversion concurrency is one; service launch uses GOMAXPROCS=1 and a 192 MiB soft memory target. NAS image cache is approximately 200 MiB, TV cache approximately 100 MiB.

## Optional Mac outpainting

A fixed local ComfyUI workflow and thin pull worker can precompute landscape derivatives. NAS queue/result storage are opt-in, approved results require a separate serving switch, and originals remain read-only. See [outpaint/README.md](outpaint/README.md) for setup, model provenance, review and rollback.

## Slideshow status page

Open `http://<NAS-LAN-IP>:8791/manage` on the home network. This is a small read-only page for the TV desktop slideshow: current finished photos, available batches and task status. Details are collapsed by default. Expand a batch record to view its photos and click a thumbnail for a full-size preview. Galleries load when expanded, and open batches stay expanded during the visible page’s 30-second refresh.

The page needs no login. It accepts only direct private-network or loopback peers; forwarded headers do not grant access. TV and worker APIs still require their existing Bearer token, including all mutations. Do not expose this service through an internet-facing proxy. The page does not expose source paths or credentials, and its bounded thumbnail cache does not scan the archive.

## Repository

- `backend/`: Go standard-library photo server; Go 1.20+.
- `launcher/`: Android TV launcher and photo cache; JDK 17, Android SDK 34.
- `deploy/`: standalone/QNAP service scripts, example configuration and packaging.
- Other folders are inherited CleanYourTV tools, examples and add-ons; see upstream docs for their usage.

## Standalone NAS service

Build on a computer with Go (change GOARCH for your NAS):

```sh
cd backend
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o nas-photos .
```

As a non-root NAS user with read access to the photo share, prepare a runtime directory:

```sh
mkdir -p "$HOME/.local/share/nas-tv-wallpaper/bin"
chmod 700 "$HOME/.local/share/nas-tv-wallpaper"
```

Copy the binary into `bin/`, `deploy/nas-service.sh` into the runtime directory, and `deploy/server.env.example` as `server.env`. Edit `server.env` to set:

```sh
NAS_PHOTO_TOKEN='REPLACE_WITH_A_RANDOM_TOKEN'
NAS_PHOTO_ROOT='/share/Photos'
NAS_PHOTO_LISTEN='192.168.1.10:8791'
```

Generate a token with `openssl rand -hex 32`. Use your actual private LAN IP, not the example. Protect `server.env` with `chmod 600`, make the service script executable, then run it with `start`, `stop` or `status`. `NAS_PHOTO_APP_DIR` overrides the runtime directory; `NAS_PHOTO_BINARY` overrides the executable. The standalone script does not configure boot startup.

The backend deliberately accepts only private LAN/loopback bind addresses. Photo routes require a Bearer token. This is a trusted-LAN HTTP service, not an internet-facing server: do not expose it by port forwarding. The token travels over HTTP and must not be reused elsewhere.

Health check (use your address):

```sh
wget -qO- http://192.168.1.10:8791/health
```

Expected response: `{"ok":true}`. `/health` exposes no photo information. Authenticated endpoints: `POST /v1/batch` (`count` and optional `exclude` IDs), `GET /v1/photos/{id}`, and `HEAD /v1/photos/{id}`. GET/HEAD return `X-Photo-Width` and `X-Photo-Height` for the EXIF-oriented original. HEAD reads only bounded saved selection history, with no image conversion. There is no arbitrary file/path endpoint.

## QNAP QPKG

ARM64 packaging sources are included. Public configuration differs from earlier personal builds; no personal binaries are distributed in this repository.

Before installation:

1. Choose an existing **non-root** NAS user that can read your photo share. Prepare its runtime directory and private `server.env` as above. The directory and runtime files must belong to that user.
2. Copy `deploy/qpkg/qpkg.conf.example` to `/etc/config/nas-tv-wallpaper.conf`. Edit `[Service] User` and `RuntimeDir` for your setup. Keep this file root-owned and mode 600; it is read as INI configuration, not executed as shell code.
3. Stop any standalone photo service using the same port before starting QPKG.

Build with Go, Python 3, SSH access to a QNAP NAS and an [official QDK](https://github.com/qnap-dev/QDK) 2.5.3 checkout:

```sh
QDK_SOURCE=/path/to/QDK \
NAS=nas-user@nas-host \
REMOTE=/share/homes/nas-user/.cache/nas-tv-wallpaper-qpkg \
./deploy/build-qpkg.sh
```

The remote build directory must end in `/nas-tv-wallpaper-qpkg` and is disposable: its `package/` and `QDK/` subdirectories are replaced during builds. QDK is staged in that user directory, not installed into QTS. Outputs go to gitignored `dist/`. The transfer command uses macOS tar options; on other build hosts adapt those options to the installed tar.

Upload the generated package to your NAS, then via an administrator SSH session install it (replace VERSION):

```sh
sudo /bin/sh /share/Public/NasTvWallpaper_VERSION_arm_64.qpkg
```

Keep **NAS TV Wallpaper** enabled in App Center for QTS-managed startup/shutdown. This is an unsigned community package; App Center may require allowing unsigned applications. The small Linux `qpkg-runner` drops to the configured user's UID/GID and supplementary groups before running service scripts. Photo processing does not run as root and needs no Go, Python, Docker or su on the installed NAS.

Check registration and process ownership:

```sh
/sbin/getcfg NasTvWallpaper Enable -f /etc/config/qpkg.conf
/sbin/getcfg NasTvWallpaper Install_Path -f /etc/config/qpkg.conf
ps | grep '[n]as-photos'
```

App Center can start/stop/disable the package. Runtime settings and data are outside the QPKG directory and survive package updates/removal. To revert to standalone, disable QPKG first, then start your standalone script. Never run both on one port.

## Android build and rollback

```sh
./launcher/gradlew -p launcher testReleaseUnitTest lintRelease assembleRelease
```

The existing build uses your local Android debug signing configuration. Keep your own signing key private and backed up. Independently built APKs may have different signatures and cannot necessarily replace another installation. This fork retains versionCode 9 for same-version local rollback; use versionName and Git tags to distinguish source versions. Before installing, preserve your own known-good APK and configuration.

`deploy/launcher-control.py --tv IP:5555 on|off|refresh|status` controls the optional feature through ADB. For APK rollback, use `rollback --apk /path/to/your-backup.apk`. The provider requires shell/WRITE_SECURE_SETTINGS privileges. Do not turn off or revoke ADB access as part of feature toggling.

## Validation and limits

Go race tests and Android unit tests/lint/release compilation are the automated checks. The earlier personal deployment validated QNAP SSH package installation, non-root service operation, health, startup/shutdown link creation, TV cache fallback, feature disable and same-signature APK rollback. The configurable public QPKG is a subsequent implementation; installation and reboot startup need validation on the destination system. Landscape single-image display was verified on an Android TV deployment. Portrait selection and complete-foreground geometry have unit coverage; verify both layouts on the destination TV.

Do not commit runtime files, tokens, photos, device dumps or signing keys. Generated packages and local backups are ignored. This project is not affiliated with QNAP, Sony, Google or Apple.

The optional NAS integration now supports [published, immutable wallpaper batches](docs/PUBLISHED-WALLPAPERS.md): at most ten retained versions, complete-download switching on TV, and EXIF capture-date filtering to the most recent ten years. TV has no dependency on the AI worker and never substitutes raw photos in published mode. QNAP package 1.3.0 and TV client 3.1-nas.2 implement this protocol.
