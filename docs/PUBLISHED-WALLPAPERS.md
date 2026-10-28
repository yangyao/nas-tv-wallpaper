# Published wallpaper batches

NAS owns task inventory, publication and downloads. Mac is an offline producer; TV only understands completed, versioned photo batches. No AI request is made by the television.

## Fixed prepared pool

Enable `NAS_PHOTO_BATCH_PUBLISH=1` and `NAS_PHOTO_BATCH_POOL=1`. The one-minute NAS maintenance task prepares a pool of **ten independent batches of exactly twenty approved images**. Mac claims a bounded six-task queue, uses the selected light treatment and the two people checks, and uploads completed JPEGs. No recursive archive scan is introduced.

- **Bootstrap:** keep producing without a time gate until ten full batches exist. Nineteen good images are staging, not a finished batch. A newly completed batch is committed immediately; incomplete or rejected results never fill a slot.
- **Steady state:** two hours after the last prepared batch, production resumes for the next twenty fresh approved images. Only after all new assets and the atomic index are durable is the oldest batch replaced. Failure keeps the previous ten. The interval is not a deletion deadline.
- **TV selection:** independently rotate through valid prepared batches every two hours. Preparing a reserve batch does not immediately interrupt the currently selected batch. TV still polls every fifteen minutes while Home is visible and downloads no raw originals.
- **Distinct inventory:** active batches do not share a JPEG. Assigned ready results are recorded as used, and the source IDs of active pool photos are excluded from sampling. This does not guarantee that two different source files are not visual copies.
- **Bounded storage:** the queue and fingerprint cache remain at most 200 records; active published JPEGs are independent of queue eviction. Unassigned recent approved results are protected from eviction while staging a batch. Only published assets with no pool/bridge reference are collected. A failed atomic write never evicts the old batch.
- **Migration:** previous rolling snapshots are consolidated into one optional **transition batch**. It remains playable and viewable, but does not count towards ten full batches. Already approved photos can be used once in the first full pool batch; no regeneration is required. The transition asset references remain available for cached/in-flight TV requests.
- **Persistence:** `pool_version`, current selection/time, last preparation time, staging fingerprints and used markers live in the existing private `index.json`. Restarting does not reset the pool or generate duplicate batches. Once the index is migrated, the backend recognises pool mode even if its enabling environment line is omitted.

The management page distinguishes `成品池 N / 10` from `下一批已完成 M / 20` and labels the old transition batch separately. Every full or transition batch can be expanded to view its photos. Hardware throughput, selection rejection and Mac sleep can delay filling; the application does not wake the Mac or parallelise GPU jobs. With approximately seven to eight minutes per generated candidate, 200 accepted photos require at least about a day of generation, plus rejected candidates. A high optional daily attempt budget avoids spending a small budget on fast no-person rejections; all generation is local and single-task.

Without pool mode, the legacy rolling-snapshot publication logic is retained for existing installations. It is not a ten-batch prepared pool.

## Capture dates

`NAS_PHOTO_MAX_AGE_YEARS=10` is the default. The cutoff moves daily, using EXIF `DateTimeOriginal` and calendar dates in the NAS local timezone. For example, on 2026-10-28 the inclusive cutoff is 2016-10-28. Missing, malformed and future capture dates are excluded. File-copy time and modification time never substitute for capture time.

Filtering applies to sampled candidates, queued-job claims and publication, including existing ready results. A batch also declares a hard `valid_until` derived from its earliest photo's age limit. Unlike the soft two-hour refresh TTL, this hard validity prevents the TV from showing an age-ineligible batch indefinitely during an outage. After hard validity, the client uses another still-valid cached batch or its built-in/local wallpaper.

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
NAS_PHOTO_BATCH_POOL=1
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

Also check that the existing runtime `server.env` explicitly contains `NAS_PHOTO_ROOT`, `NAS_PHOTO_LISTEN`, and the existing `NAS_PHOTO_TOKEN`. Some old personal builds kept only the token in that file and compiled the root/listen settings into their startup script. Migrate the existing root and LAN listen address into `server.env`, preserving the token and all feature flags; keep mode 600. Do not assume that a running legacy service proves this file already has every required setting.

With both configuration files complete, upgrade the uploaded package manually:

```sh
sudo /bin/sh /share/Public/NasTvWallpaper_1.3.0_arm_64.qpkg
```

Runtime settings, photos and published data live outside the package and survive upgrades. For a new installation, configure the service user/runtime directory first, following the existing QPKG instructions. Do not disable TV debugging as part of installation or rollback.

Validation covers conditional manifests, hashes, strict legacy compatibility, empty pools, soft expiry, failed atomic publication, restart persistence, ten-version retention/GC, concurrent readers, capture-date boundary/leap-day rules, and Android protocol checks. Runtime status is available through the existing `nas-status` content-provider call, including protocol, batch ID, complete/pending counts and hard validity; no token is returned.

### Pool rollout and rollback

Before first migration, stop NAS briefly and back up the runtime `server.env`, `data/published/index.json`, and installed backend binary to a private backup directory. Enable the pool flag and start the new backend; check `/manage` for separate pool/staging counts and the transition batch. Preserve existing tokens, recent-photo filtering and people checks. To roll back to a pre-pool binary, restore its matching pre-pool index and configuration together while stopped; simply removing the environment line does not undo a persisted migration. Keep the backup private. The live application-directory scripts and binaries can be updated as their service owner; this does not require disabling TV debugging.
