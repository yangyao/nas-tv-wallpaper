# Native Mac photo frame

An independent AppKit + SwiftUI macOS application. No WebKit, web page,
Chromium, browser extension, JavaScript runtime or third-party dependency.
The existing NAS service and web frame remain unchanged.

## Build and install

Requires Xcode or its command-line tools, macOS 13 or later.

```sh
swift test
bash build-app.sh
ditto 'dist/照片相框.app' '/Applications/照片相框.app'
open '/Applications/照片相框.app'
```

The bundle is locally ad-hoc signed, not Developer ID signed or notarized.
Build on the target Mac; distributing downloaded binaries to other Macs may
require Developer ID signing/notarization. The Swift package opens in Xcode too.

On first launch, enter the NAS service origin in Settings, such as
`https://photos.example.com:8443`. Use the LAN reverse proxy's valid HTTPS
certificate. No credentials or device addresses are compiled into the app.
If macOS requests local-network access, allow it for this app.

The native application has a separate bundle ID (`com.yangyao.photoframe`) from
any installed Chrome web app. It does not uninstall or modify that web app.

## Playback

- Ten seconds per photograph, with a crossfade and a softened same-photo backdrop.
- Left/right selects a prepared photograph. Missing next photograph keeps the
  current scene unchanged, with no notification or queued navigation.
- Down, or moving the pointer over the bottom of the active window, reveals the
  thumbnail strip. Up hides it. Controls hide after five seconds of inactivity.
- Space pauses/resumes, F toggles fullscreen. Escape hides controls first, then
  exits fullscreen. Command-comma opens Settings.
- Settings are the only text-entry surface. Clicking/dragging the photograph
  does not navigate, export, open a context menu or trigger a browser action.
- Playback continues when another application has focus. Network retries never
  activate the window, request fullscreen, or show an error dialog.
- Window position/size use macOS frame restoration. Fullscreen and multi-monitor
  placement remain under the user's control; no automatic screen reassignment.

## Data and limits

Uses the existing LAN-only `/frame/api/batch` and `/frame/photos/{id}` endpoints.
The native client supplies the required matching Origin, without weakening the
server's web protections. Redirects to another origin are rejected and TLS uses
normal system certificate verification.

Serial downloads prepare at most twenty future photographs. At most sixty
normalized JPEGs are retained, each at most 2 MiB and fitting 2560 × 1440, with
EXIF orientation applied once. Originals are temporary downloads, not a second
permanent photo library. Sources over 40 MiB or 40 megapixels are rejected.
Only thumbnails and the current/crossfading full-size photograph remain decoded.

Local cache and selected-photo history live in
`~/Library/Application Support/PhotoFrame`; the endpoint is in macOS preferences.
This cache contains private photos, is excluded from backups, and is not placed
in source control. A fresh offline launch restores the last prepared scene.
The maximum normal image cache is about 120 MiB plus one incoming original
(at most 40 MiB from the supported NAS sampler), thumbnails and metadata.
The cache is not encrypted separately from the user's disk; FileVault and macOS
account permissions apply. System sleep/locking remain unchanged.

## Verification

`swift test` covers bounded history, unavailable navigation without queued steps,
endpoint validation, orphan cleanup, server separation and offline disk restore.
Cache regression tests also cover interleaved prepare/save, stale snapshots and
a next image disappearing after startup: automatic playback skips the bad entry
and continues without a black frame. Prepared images stay pinned until their
timeline commit, and obsolete snapshots cannot evict newer committed files.

The application menu includes **诊断信息…**, opened only on request. It reports
play/pause state, controls, cache counts, missing files, recovery count and a
numeric network error, with no server address, photo IDs, photo content or token.
Use its copy button when reporting a problem; it never opens automatically.

Build the app and verify real NAS playback, keyboard navigation, fullscreen and
continued playback while Chrome is used for development. Use synthetic photos
for automated fixtures; never commit family photos or screenshots.
