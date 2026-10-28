# Published wallpaper batches

NAS owns task inventory, publication and downloads. Mac is an offline producer; TV only understands completed, versioned photo batches. No AI request is made by the television.

## Production and publication

- With `NAS_PHOTO_BATCH_PUBLISH=1`, a resident one-minute NAS maintenance loop inspects the bounded result queue, publishes ready results and maintains up to six pending jobs for the configured workflow. It never recursively indexes the photo archive.
- Mac's existing night schedule claims those jobs, produces the selected light treatment, and uploads completed JPEGs. Only approved results matching `NAS_PHOTO_PUBLISH_WORKFLOW` enter publication.
- Each batch contains 1–20 completed images. Startup can publish a smaller batch; missing capacity is never filled with raw photos. The target is at most ten retained batches, filled gradually rather than manufacturing ten duplicates at startup.
- New approved content triggers publication; otherwise a six-hour **soft TTL** requests rotation. If the available completed batch would be identical, its version remains unchanged and clients keep their cache; no duplicate publication is manufactured. A failed producer or failed publication retains the previous complete batch. The new batch's assets are written first, then one atomically replaced, persisted index publishes the whole batch.
- Images use immutable content-derived addresses and SHA-256 hashes. Batches may reuse the same image without copying it on NAS. Only files no longer referenced by any of the ten retained batches are collected. Result-queue eviction does not break a retained published batch.
- Unchanged result files are checked by size/mtime and are not read/hashed every minute. Queue size remains at most 200. Mac diagnostic retention remains 40 completed folders.

## Capture dates

`NAS_PHOTO_MAX_AGE_YEARS=10` is the default. The cutoff moves daily, using EXIF `DateTimeOriginal` and calendar dates in the NAS local timezone. For example, on 2026-10-28 the inclusive cutoff is 2016-10-28. Missing, malformed and future capture dates are excluded. File-copy time and modification time never substitute for capture time.

Filtering applies to sampled candidates, queued-job claims and publication, including existing ready results. A batch also declares a hard `valid_until` derived from its earliest photo's age limit. Unlike the soft six-hour refresh TTL, this hard validity prevents the TV from showing an age-ineligible batch indefinitely during an outage. After hard validity, the client uses another still-valid cached batch or its built-in/local wallpaper.

## TV API

All routes require the existing bearer token and private-LAN endpoint.

- `GET /v2/wallpapers/latest` returns `batch_id`, `published_at`, soft `expires_at`, hard `valid_until`, and `photos` with ID, immutable URL, exact byte length, SHA-256 and dimensions. `If-None-Match: "<batch_id>"` returns 304 when unchanged.
- `GET /v2/wallpapers/assets/<sha256>.jpg` returns an immutable 1920×1080 JPEG. The asset must belong to a retained publication and satisfy the capture-date policy.
- No completed/eligible batch, or delivery paused: 503, meaning **keep valid local cache**. There is no raw-photo substitute.
- In published mode legacy `/v1/batch` and `/v1/photos/...` also expose only published assets. Exclusions may be relaxed to reuse completed images instead of filling with raw photos. Old original-image IDs do not bypass this restriction.

Client 3.1-nas.2 checks for a version every 15 minutes while Home is visible, including if Home remains open. It schedules no wakeups in standby. It checks the manifest, byte count, SHA-256 and TV dimensions, resumes staging after interruptions, and can reuse verified local immutable images. **Every** item must be complete before the current-batch pointer changes. A partial new batch never becomes visible, including on first installation.

Current and previous complete batches survive failed refreshes. The local disk budget is 192 MiB, sufficient for current, previous and staging batches at the maximum protocol sizes. Old legacy cache is not silently accepted because it could contain unprocessed originals. Public settings and images remain opt-in; app upgrades preserve the external theme and user preferences.

## Configuration and operation

In the existing NAS runtime `server.env`:

```sh
NAS_PHOTO_OUTPAINT=1
NAS_PHOTO_OUTPAINT_AUTOFILL=1
NAS_PHOTO_OUTPAINT_SERVE=1
NAS_PHOTO_BATCH_PUBLISH=1
NAS_PHOTO_PUBLISH_WORKFLOW='RECIPE_KEY_FROM_MAC'
NAS_PHOTO_MAX_AGE_YEARS=10
```

The control tool now derives the recipe key from Mac configuration and synchronizes it to NAS when enabling; adjusting a recipe does not silently publish results from the old workflow. `off` stops new generation/automatic sampling and pauses delivery. TV keeps valid completed cache; it does **not** fall back to raw photos in published mode.

```sh
python3 deploy/outpaint-control.py --config /private/control.json status
python3 deploy/outpaint-control.py --config /private/control.json off
python3 deploy/outpaint-control.py --config /private/control.json on
```

Older personal 1.0.1 packages may have compiled-in user/runtime settings and no `/etc/config/nas-tv-wallpaper.conf`. Before upgrading, create a private service configuration using the **existing** service user and runtime directory (the directory already containing `server.env`); do not switch to a new empty runtime directory. `deploy/qpkg/qpkg.conf.example` shows the format. After placing the correctly filled file on NAS, install it as root:

```sh
sudo cp /share/Public/nas-tv-wallpaper.conf /etc/config/nas-tv-wallpaper.conf
sudo chmod 600 /etc/config/nas-tv-wallpaper.conf
```

With that configuration present, upgrade the uploaded package manually:

```sh
sudo /bin/sh /share/Public/NasTvWallpaper_1.3.0_arm_64.qpkg
```

Runtime settings, photos and published data live outside the package and survive upgrades. For a new installation, configure the service user/runtime directory first, following the existing QPKG instructions. Do not disable TV debugging as part of installation or rollback.

Validation covers conditional manifests, hashes, strict legacy compatibility, empty pools, soft expiry, failed atomic publication, restart persistence, ten-version retention/GC, concurrent readers, capture-date boundary/leap-day rules, and Android protocol checks. Runtime status is available through the existing `nas-status` content-provider call, including protocol, batch ID, complete/pending counts and hard validity; no token is returned.
