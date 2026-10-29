# Random photo frame

The frame is an internal mode of the existing launcher APK. It has no launcher
icon, home tile or additional desktop button. `photoframe` is an Android library
module; NAS selection stays in the existing Go service and repository.

## Remote and presentation

- Back on Home enters the frame; Back in the frame exits. Home returns Home.
- Left/right selects the previous/next **ready** photograph. Order stays stable.
- The selection outline glides with a gentle spring-like settle; thumbnails
  enlarge smoothly and the strip scrolls continuously as selection moves.
  Cache replenishment preserves the current visual position.
- Down reveals the thumbnail filmstrip; Up or OK hides it. It fades away after
  4.5 seconds without navigation. Up to seven nearby ready photos are visible.
- OK while the strip is hidden toggles automatic playback (10 seconds per photo).
  Media play/pause keys work too. Navigation restarts the playback interval.
- No captions, counters, loading notices, spinners or error dialogs. Missing next
  photo means no movement, with no deferred navigation after the download.
- Originals fit the display without cropping. The surrounding area uses a
  center-cropped copy of the same image, softly blurred, desaturated and darkened.
  This transient 128×72 texture is prepared on the display decoder thread; no AI,
  extra network requests or persistent image files are involved. Foreground and
  background crossfade together. Only the backdrop and filmstrip thumbnails crop.
- The glass uses photograph colour, a small preblurred backdrop, translucent tint,
  rim light and a white selection outline. It avoids expensive live full-screen
  blur and respects the launcher's animation/reduced-motion preferences.

The entry can be switched off in NAS photo settings using “桌面按返回进入相框”.
The existing NAS enable switch also controls frame network access. When disabled,
local wallpaper seeds are available without contacting the NAS.

## Data flow and bounds

1. Restore the previous frame session from app-private cache. If unavailable, use
   an existing local wallpaper while downloading. The opening window is translucent
   until a decoded photograph is ready, preserving Home behind it.
2. POST `/v1/frame/batch` with count <=20 and recently held photo IDs. This uses
   a separate bounded sampler/history, not the AI jobs or published wallpaper pool.
3. GET `/v1/frame/photos/{id}` with the same configured bearer credential. The NAS
   streams the original JPEG without decoding or generating thumbnail files.
4. TV downloads one file at a time, corrects EXIF orientation, fits it within
   1920×1080 and stores a display JPEG (<=3 MiB) plus a small thumbnail. Only after
   both are usable does it enter the filmstrip.
5. Prefetch up to 20 future photos; request another batch when five or fewer remain.
   Keep at most 60 timeline entries and at most 40 earlier entries. Evict oldest
   cache files as the timeline advances. Temporary source <=40 MiB; retained
   display data <=180 MiB plus small thumbnails. No downloads while stopped.

Small libraries may repeat after recent exclusions exhaust candidates, but avoid
immediately repeating the current photograph. Random directory sampling is not a
uniform whole-library shuffle and does not guarantee discovery of every photo.
There is no archive import, recursive inventory, or permanent thumbnail library.
The original source is never modified.

First version supports `.jpg`/`.jpeg` only, up to 40 MiB and 100 megapixels.
HEIC/RAW/video conversion is not implemented. Hidden, screenshot, download and
thumbnail directories remain excluded; directory traversal is limited to five
levels, 32 directory reads of 256 entries and 100 metadata inspections per pass.
Unlike curated wallpaper, this stream does not require camera EXIF, people or a
capture date within ten years. It does not run AI or promise a people-only stream.
JPEG orientation comes from the NAS metadata response, not filename conventions.

## Upgrade and rollback

Install the new launcher APK and upgrade the NAS backend together. Old servers
lack `/v1/frame/*`; cached/local photos remain available and network retries are
silent. Existing wallpaper endpoints, AI worker configuration and published
batches are unchanged. Replacing the backend binary uses the existing deployment
process; QPKG source is in `deploy/qpkg`.

Turning off the frame entry restores the old Back behavior without downgrading
anything. Disabling NAS access restores local-only behavior. Android may reclaim
cache storage; a missing cache is rebuilt lazily. A source/token change isolates
and removes the old source's frame cache on next entry.

Validation: Go race tests, Android JVM tests, lint and release build. Physical
TV focus, Home interception and GPU appearance must also be checked on the target
firmware before claiming device verification.

## Deployment verification

Launcher 3.2-frame.1 and backend 1.5.0 were checked together on the target
Android TV: NAS originals populated the filmstrip, next/previous returned the
identical image, and both Back and Home returned to the launcher. Existing AI
published batches and worker jobs remained available after the service restart.
The backend binary was replaced under the existing QPKG with a rollback copy;
QNAP App Center can still display the older package installer version until a
full QPKG upgrade. Runtime configuration and photo data were retained.

## Idle photo playback

With NAS photos and frame mode enabled, leaving Home focused for five minutes
starts photo playback with the controls hidden. NAS photo settings offer five
minutes, ten minutes, or off. Every remote interaction restarts the countdown;
opening another app, settings, or a dialog cancels it. Returning Home starts a
fresh interval. Direction, OK, Back or Menu dismiss an automatically started
frame without also activating the item behind it. Manual frame browsing keeps
its existing controls. Off retains any explicitly configured legacy idle clock.

Home keeps its own window awake only while this photo countdown is enabled, so
Android's screen timeout does not pre-empt it. Photo playback also keeps its
window awake; power-button standby remains system-controlled. This is an app
photo screensaver, not system sleep. No global screen timeout, dream component
or debugging setting is changed. The existing system dream remains available
outside the launcher.

Filmstrip styling uses a lightly translucent white pane and thin white rim,
without the dark outer slab, fake shadow, or opaque gray tint.

Launcher 3.2-frame.4 was verified on the TV: uninterrupted Home idle entered
photo playback at the five-minute deadline (observed at 302 seconds by polling);
OK dismissed the automatic frame back to Home, while Down still opened the
filmstrip in a manually entered frame. Countdown reset/cancel/off and ten-minute
deadlines are covered by JVM tests; the full ten-minute wait was not repeated
on the device. Android unit tests, lint and release build passed.
