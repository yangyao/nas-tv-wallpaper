# Random photo frame

The frame is an internal mode of the existing launcher APK. It has no launcher
icon, home tile or additional desktop button. `photoframe` is an Android library
module; NAS selection stays in the existing Go service and repository.

## Remote and presentation

- Back on Home enters the frame; Back in the frame exits. Home returns Home.
- Left/right selects the previous/next **ready** photograph. Order stays stable.
- Down reveals the thumbnail filmstrip; Up or OK hides it. It fades away after
  4.5 seconds without navigation. Up to seven nearby ready photos are visible.
- OK while the strip is hidden toggles automatic playback (12 seconds per photo).
  Media play/pause keys work too. Navigation restarts the playback interval.
- No captions, counters, loading notices, spinners or error dialogs. Missing next
  photo means no movement, with no deferred navigation after the download.
- Originals fit the display without cropping. Only filmstrip thumbnails crop.
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
