# Optional local outpainting (experimental)

**The current SD 1.5 image quality is not production-ready.** Pipeline operation is validated, but the private pilot showed inconsistent scene extension and joins. See [MODEL-EVALUATION.md](MODEL-EVALUATION.md) before enabling automatic serving.

The NAS owns a bounded persistent queue and result cache. A Mac pulls jobs, calls a dedicated local ComfyUI instance, restores the full-resolution source centre and detected faces while softly blending only the background border, and uploads a 1920×1080 JPEG. Outputs enter **review**, never automatic approval. Originals remain read-only. No photo is sent to a cloud inference API.

## NAS feature flags

Add to the private runtime `server.env`, then restart the service:

```sh
NAS_PHOTO_OUTPAINT=1
NAS_PHOTO_OUTPAINT_AUTOFILL=0
NAS_PHOTO_OUTPAINT_SERVE=0
```

All are off by default. `AUTOFILL=1` allows bounded random sampling when a claim finds no queued work; it does not scan the archive. `SERVE=1` makes approved derivatives available through normal photo GET/HEAD routes, with landscape dimensions. Keep it off while evaluating quality. Disabling either the queue or serving restores normal original-photo behaviour without deleting results.

Queue and JPEGs live under the service data directory's `outpaint/`, maximum 200 jobs. Job IDs deduplicate source ID + workflow version + prompt. Leases last 30 minutes, maximum three attempts; failed jobs wait at least an hour before retry. When full, generation pauses; this initial version does not automatically delete reviewed images. Disable the service and privately back up/archive its outpaint directory before starting a fresh pool. Result uploads must be JPEG, 1920×1080 and at most 4 MiB; NAS re-encodes them to strip metadata.

Authenticated endpoints use the existing Bearer token:

- `GET /v1/outpaint/jobs`: status list.
- `POST /v1/outpaint/jobs`: enqueue a known sampled `photo_id`, `workflow` and optional `prompt`.
- `POST /v1/outpaint/claim`: `{worker, workflow}`; 204 means no eligible work was found. Autofill tries one bounded sample and leases an eligible portrait in the same request.
- `GET /v1/outpaint/jobs/{id}/input`: cropped sharp foreground of the already resized NAS cache, not the original file or its metadata.
- `PUT /v1/outpaint/jobs/{id}/result`: JPEG body plus `X-Worker-ID`; valid lease required.
- `GET /v1/outpaint/jobs/{id}/result`: preview derivative.
- `POST /v1/outpaint/jobs/{id}/decision`: `{status: approved|rejected}`. Pending/failed jobs may be rejected to skip bad sources.
- `POST /v1/outpaint/jobs/{id}/resume`: renew a failed/current job owned by the same worker, within the attempt budget. For saved generations, `recover.py --config ... JOB_ID` downloads existing Comfy history, reapplies composition and uploads without queuing another GPU task.
- `POST /v1/outpaint/jobs/{id}/fail`: worker failure, `X-Worker-ID` required.

## Mac setup

Use a dedicated directory such as `~/.local/share/nas-tv-wallpaper/` with:

```text
ComfyUI/              official source checkout
.venv/                Python 3.12 + ComfyUI requirements
outpaint/             this folder
models/               checkpoint, downloaded separately
outpaint-config.json  private copy of config.example.json, mode 600
```

ComfyUI is a separate GPL dependency: https://github.com/Comfy-Org/ComfyUI . The tested checkout is `f49e3b466f637cf0c1120614a7dbcf02ec2805e3`. For this fixed headless workflow the optional workflow-template media packs and embedded docs can be omitted from requirements. Keep the frontend/core dependencies. The tested Mac environment pins `torch==2.12.0`, `torchvision==0.27.0`, and `setuptools==81.0.0`; newer default versions are not assumed compatible. Never install arbitrary custom nodes to run this workflow.

The initial model is Stable Diffusion 1.5 Inpainting, mirrored as safetensors at https://huggingface.co/webui/stable-diffusion-inpainting . Download `sd-v1-5-inpainting.safetensors` (4,265,203,873 bytes), verify SHA-256 `0ec8f8585b104417a8c34a9fbcc1e922a70b8c15490ec4553087f01c8cf33673`, then link it into `ComfyUI/models/checkpoints/`. See the model card and CreativeML Open RAIL-M license before use. Model files are not bundled or committed.

`workflow.json` contains only builtin nodes: load checkpoint/input → text conditioning → masked VAE → KSampler → VAE decode → save. Working canvas 896×504, 24 steps, deterministic seed derived from job ID. The entire source rectangle is protected by the input mask, with mask growth disabled; only the newly added outer canvas is generated. After inference the worker reinstates the sharper source at final resolution. With the optional local Apple Vision helper, a narrow background border is feathered and expanded detected face rectangles are fully protected. If detection is unavailable or returns no faces, it falls back to the strict original rectangle. Detection can miss faces and generated outer content can contain mistakes or unwanted people; review remains necessary. Original files are never overwritten.

Configure a private LAN NAS URL and token in `outpaint-config.json`. Bind ComfyUI only to localhost; run it offline, without partner/API nodes. Do not expose its raw API to the internet. The worker bypasses environment proxies for local ComfyUI/NAS requests.

On macOS with Command Line Tools, compile the optional on-device face helper and set `face_helper` in the private worker config:

```sh
xcrun swiftc -O outpaint/face_boxes.swift -o outpaint/face-boxes
```

Apple Vision runs locally; no image is uploaded for face detection. Helper binaries are gitignored. GPU/model inference still uses ComfyUI.

Run a pilot:

```sh
.venv/bin/python outpaint/worker.py --config outpaint-config.json --count 3
```

A filesystem lock prevents overlapping workers; daily attempts (including failures) default to six. Processing times include model loading and source fetch. A dedicated instance is required because timeout recovery interrupts its current generation. Private work folders contain original/prepared/generated/final images, workflow, prompt ID and timing for diagnosis. They are never uploaded to GitHub.

Install user LaunchAgents after validating the environment:

```sh
.venv/bin/python outpaint/install_agents.py \
  --root "$HOME/.local/share/nas-tv-wallpaper" \
  --config "$HOME/.local/share/nas-tv-wallpaper/outpaint-config.json" --worker
```

ComfyUI runs at user login, localhost port 8188. Worker polls every 30 minutes, within config `start_hour`/`end_hour`, one job per run. Mac must be awake and that user logged in; installation does not change system sleep settings or wake the Mac. Use `launchctl bootout gui/$(id -u) ~/Library/LaunchAgents/com.nastv.outpaint-worker.plist` to stop scheduling. Logs are under the dedicated root's `logs/`.

Review using `admin.py --config ... list`, `approve JOB_ID` or `reject JOB_ID`. Approval alone does not affect TV while NAS `SERVE=0`. If serving is enabled later, existing TV caches may continue showing the earlier original until refreshed; no immediate TV cache invalidation is performed.

## Checks

Go queue tests cover auth/feature gates, deduplication, exclusive leases, invalid upload, approval and persistence. `python -m unittest discover -s outpaint -p 'test_*.py'` checks masking and that the full-resolution source centre is preserved. Actual visual quality and MPS timings need representative private samples; do not infer quality from successful generation alone.


## Observed pilot

On an M4 / 16 GiB Mac, six 896×504 / 24-step samples took roughly 69–76 seconds each including load/save (GPU generation plus postprocessing; cached-generation recovery was also verified without resubmitting GPU work). First installation/model download is separate and can be slow. Go race tests, fixed-node availability, MPS execution and composition/face-helper integration tests passed. Face-protected edge blending improved visible seams; generated scenery and geometry remain imperfect. No personal samples or credentials are published. Keep automatic sampling and TV serving off until the results meet your needs.

## Dedicated FLUX Fill quality benchmark

This is a separate manual experiment, **not** a change to the scheduled SD 1.5 worker or NAS delivery. Keep automatic sampling and serving disabled until the actual results pass review. Inputs and generated media must stay outside the checkout.

Install the pinned ComfyUI-GGUF extension from https://github.com/city96/ComfyUI-GGUF (tested source revision `6ea2651e7df66d7585f6ffee804b20e92fb38b8a`) into ComfyUI's `custom_nodes`. It requires `gguf` and `protobuf` in ComfyUI's Python environment. Model licenses remain applicable, including FLUX.1 dev's noncommercial terms; quantization does not change them.

Download approximately 10.8 GB of model assets, with per-range resume and mandatory whole-file SHA-256 verification:

```sh
python download_assets.py --root /path/to/ComfyUI/models
```

`--mirror` can select an HTTPS Hugging Face mirror. An explicit asset URL in the manifest takes precedence. Downloading public model weights does not send inference inputs to that provider.

Start ComfyUI bound to `127.0.0.1`. On Apple Silicon, keep its normal MPS memory limits; do not disable high-watermark protection. A 16 GB machine requires actual memory and latency testing, even with quantized weights.

```sh
python benchmark_flux.py /private/path/original.jpg \
  --output /private/path/flux-comparison \
  --prompt 'A natural photograph of a family in a residential courtyard, daylight, coherent surroundings.'
```

The benchmark follows [ComfyUI's official Fill outpaint template](https://docs.comfy.org/tutorials/flux/flux-1-fill-dev): FLUX guidance 30, differential diffusion strength 1, inpaint conditioning with `noise_mask=false`, 24-pixel feathering, Euler/normal, CFG 1, 20 steps. Only the model loaders use GGUF. Input is resized to 576 pixels high and padded horizontally to 1024 pixels. The generated PNG is saved unchanged: no original-region paste, face correction, or seam blending. Inspect subject identity as well as backgrounds. This raw output is a quality test, not a guarantee of unchanged source pixels.

The private output contains input, exact API workflow, prompt ID, execution history, timing and raw image. If interrupted, retrieve the saved prompt's history rather than resubmitting and spending another generation. Model asset hashes are recorded in `flux-assets.json`. The selected seed enables repeatable comparisons within the same software/hardware stack; cross-device bitwise identity is not promised.

For constrained RAM, `cache_flux_prompt.py --comfy-root /path/to/ComfyUI --prompt '...'` uses built-in `SaveConditioning` to store text encoder output under `models/embeddings`. Restart the idle ComfyUI process to release text encoder weights, then pass `--conditioning <reported-filename> --comfy-root /path/to/ComfyUI` to the benchmark with the **same** prompt. The benchmark checks the cached prompt and records its hash and encoding workflow. This preserves conditioning values while separating encoder and diffusion model residency. No third-party conditioning serializer is needed.

Reference benchmark environment: ComfyUI `f49e3b466f637cf0c1120614a7dbcf02ec2805e3`, Python 3.12.13, PyTorch 2.12.0, torchvision 0.27.0, macOS 26.5.1, Apple M4 with 16 GiB unified memory. Optional GGUF dependencies are pinned in `requirements-flux.txt`. CPU text encoding and CPU VAE keep MPS memory available for diffusion. The core text-conditioning save/load nodes must be present; older ComfyUI versions may not include them.

Generate an offline comparison after a successful run:

```sh
python compare_flux.py --source /private/original.jpg \
  --previous /private/previous-result.jpg --current /private/flux/raw.png \
  --output /private/compare.html
```

The source is shown with plain padding. FLUX output is shown raw; the previous SD 1.5 pipeline may include original-region compositing. Differences therefore compare the actual pipelines, not an isolated checkpoint substitution. Neither HTML nor images belong in the public repository.

## Optional local softening of generated margins

`soften_outpaint.py` is an offline postprocessing experiment; the worker and TV delivery do not call it automatically. It reuses existing generated output and requires no new diffusion run:

```sh
python soften_outpaint.py --source /private/original.jpg \
  --generated /private/flux/raw.png --prepared /private/flux/input.png \
  --output /private/flux/softened.png
```

The output is 1920×1080. Default Gaussian radius is 10 output pixels (the selected light setting), with a 96-pixel source-edge feather and 260-pixel outward ramp. Use `--radius 16` for moderate softening, or 28/44 for the older stronger variants. A Gaussian pyramid varies blur smoothly by distance from the original image boundary. The original source is restored in the centre; only its side edges are feathered. The geometry follows the benchmark's exact eight-pixel-aligned padding, including rounded portrait widths. Parameters are saved beside the image.

This suppresses invented lettering and fine background detail while retaining broad colours and shapes. It is a spatial margin treatment, not depth estimation or true optical bokeh: generated extensions of a subject crossing the original frame are also softened. Do not interpret it as reconstruction of the real scene. Private sample previews remain outside the public checkout.

## Scheduled FLUX Fill + selected light treatment

`worker.py` now accepts `engine: "flux-fill"`; the legacy `sd15` engine remains available for rollback. Start from `config.flux.example.json` (disabled and manual review by default). Prepare the exact configured generic prompt with `cache_flux_prompt.py` once, then release the encoder before scheduling generation. The dedicated ComfyUI service must bind to localhost, and existing models must match the verified manifest.

The selected recipe is radius **10**, feather **96**, outward ramp **260**, 20 FLUX Fill sampling steps, and an unchanged original centre. The workflow key includes a hash of model, prompt and recipe so parameter changes create new jobs instead of reusing old processed output. The accepted manually generated examples can be imported with their original provenance; subsequent jobs use the cached generic prompt.

Set `enabled: true` only after configuring the NAS endpoint/token. `auto_approve: false` keeps new results in manual review; `true` explicitly opts into approving newly generated results for the selected recipe. Rejected results are never automatically reapproved. NAS switches `NAS_PHOTO_OUTPAINT=1`, `NAS_PHOTO_OUTPAINT_AUTOFILL=1` and `NAS_PHOTO_OUTPAINT_SERVE=1` independently enable the queue, bounded automatic source sampling and serving. New installs remain off. No Android APK change is needed for the already installed adaptive NAS launcher: landscape outputs carry 1920×1080 headers and display singly, while unprocessed portraits retain the paired layout.

Schedule: one job per 30-minute LaunchAgent interval, only 02:00–06:00 in the example, maximum six attempts per local calendar day. Mac must be awake and the user's LaunchAgent session available. A busy ComfyUI queue is skipped before claiming a NAS lease. Normal tasks finish inside the 30-minute lease; configured generation timeout is 20 minutes, capped at 25. Errors preserve the original-photo fallback. A saved prompt/raw image/completed result is reused on retries rather than submitting another GPU task. `recover.py` supports both output nodes. Model weights are released after each attempt.

Storage is bounded: Mac retains 40 completed diagnostic folders and cleans its successful Comfy input/output files. NAS holds at most 200 jobs/results; with automatic sampling enabled, the oldest approved/rejected entry rotates out when needed. Pending, processing and unreviewed jobs are protected. No original files are changed or deleted, and no recursive archive scan is added.

### Switching off or rolling back

Copy `deploy/outpaint-control.example.json` to a private path and fill in SSH targets and installation paths. The control program uses existing key-based SSH, never puts tokens on command lines, and does not touch TV debug settings:

```sh
python3 deploy/outpaint-control.py --config /private/control.json status
python3 deploy/outpaint-control.py --config /private/control.json off
python3 deploy/outpaint-control.py --config /private/control.json on
```

`off` disables new Mac worker invocations and NAS automatic sampling/AI serving. An in-flight generation can finish, but its result is hidden while serving is off. Already downloaded TV images remain until the next successful batch refresh; use the existing `launcher-control.py refresh` when the TV is reachable to request a refresh on Home. Offline TV behaviour and local-wallpaper fallback are preserved. `on` restores NAS sampling/serving, then enables the scheduled Mac worker; it does not start an immediate heavy job. For model rollback, restore the previous private worker configuration and leave serving off until its results have been reviewed.

## Versioned publication supersedes mixed original/AI delivery

Production now uses the [published-batch protocol](../docs/PUBLISHED-WALLPAPERS.md). NAS maintains the task inventory and up to ten immutable completed batch versions. TV downloads only complete publications and retains its previous valid batch on producer/network failures. No raw-photo fill is used in this mode. The former rollback description above applies only to legacy mode with published batches disabled; the normal `off` action now pauses delivery and keeps completed client cache.

Capture-date filtering defaults to the rolling last ten years, with unknown/future dates excluded. NAS publication filters existing results as well as new candidates. Control `on` synchronizes the exact derived Mac recipe key to the NAS publisher and enables this strict mode. No additional generation was needed for the initial three-image publication.

### Faster continuous production (optional)

To populate the retained slideshow batches sooner, set `start_hour: 0`, `end_hour: 24` and `max_per_day: 96` in the private worker configuration. Install with `install_agents.py --root <runtime> --config <private-config> --worker --interval 300 --count 10`. This runs up to ten jobs sequentially per invocation, with a filesystem lock preventing overlap; the daily limit includes failures. Generation remains single-task, not parallel GPU work. The Mac must be awake, and macOS Local Network permission must allow the background worker to reach NAS. Verify a task actually enters processing after installation; an SSH health check alone does not verify LaunchAgent permissions.

NAS publication is independent: it publishes changed completed batches every two hours and retains the latest ten. These are rolling snapshots and may share photos, not ten disjoint sets of twenty. TV checks for changes every fifteen minutes while Home is visible; a producer outage retains the last valid cache.

### Reject generated people before publication

For local FLUX production, compile the optional Apple Vision helper on the Mac:

```sh
cd outpaint
xcrun swiftc -O people_boxes.swift -o people-boxes
```

In the private worker configuration set `people_guard: true` and `people_detector` to that executable's absolute path. This setting is off in the example. The helper uses local [face and human rectangle detection](https://developer.apple.com/documentation/vision/vndetecthumanrectanglesrequest); images are not sent to a remote model.

The worker checks both the unsoftened generation and final JPEG, including contextual crops of each side margin. A sufficiently large face substantially outside the source rectangle blocks upload and approval. Small distant faces, back views and body-only detections are allowed. The v2 heuristic requires a face height of at least 8% of the frame (about 86 pixels at 1080p), width of at least 2.5%, confidence of at least 0.25 and at least 20% horizontal overlap with the new margin. It checks visual prominence, not identity; a large blurred face may be caught in the raw generation. Edge-cropped original faces substantially completed by AI can also be rejected intentionally. It writes private `people-check.json` evidence. Cached results must pass the same gate; missing raw images, helper failure or invalid responses block publication. Automatically approving old review tasks by status alone is disabled when this gate is enabled.

Detected prominent margin faces yield `unwanted_people`: NAS permanently rejects that task rather than retrying the same cached generation. Automatic sampling then supplies another source. Other detection failures remain failed tasks with bounded retry. The gate does not modify the original light-softening recipe or add stronger blur. Keep the NAS backend updated before enabling it so this reason code is accepted. Manual authenticated administrative decisions remain available.

This detector is a conservative filter, not proof that a result contains no invented person. Stylised, tiny, occluded or unusual figures can be missed; original edge subjects may be over-rejected. Recheck retained publications when first enabling the policy: rejection affects future selection but does not automatically revoke an existing retained batch or a TV's offline cache. Retire affected batches and let NAS publish a checked replacement. TV refresh occurs while Home is visible.

### Prefer family photos containing people

Set `require_source_person: true` in the private FLUX configuration to exclude originals in which the local detector finds neither a face (confidence ≥0.25) nor an upper body (≥0.35). It shares the configured `people_detector` helper. This switch is independent of the large-margin-face guard and remains off in the example configuration.

The worker checks the EXIF-oriented original before preparing or submitting any GPU work and checks it again before uploading even a cached result. Back views and faces too small for the large-face rejection rule can qualify. It does not identify family members or require a front-facing portrait. A private `source-people-check.json` records the result. `no_source_person` permanently rejects that task, allowing NAS sampling to choose another photo. Missing originals, detector failure or malformed responses fail closed instead of passing an image. Automatic review-status approval is disabled when either people policy is enabled. Existing approved photos/batches need a one-time original-photo audit and retirement of affected publications, as with the margin policy.

People detection can miss tiny, occluded or unusual subjects, so an exclusion means “no person detected,” not proof that a photograph is empty. This only examines the bounded sampled queue; it does not index the photo archive. Set `require_source_person: false` to allow scenery again without changing the margin-face rule.
