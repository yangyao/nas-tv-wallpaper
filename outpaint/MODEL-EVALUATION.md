# Prototype quality assessment

The current SD 1.5 workflow proves local MPS generation, persisted jobs, recovery and review-only delivery. **Its visual quality is not an accepted production baseline.** Broad expansion produced inconsistent scenery and obvious joins; face-protected border blending helps joins but does not solve scene understanding. Do not interpret successful generation as acceptable wallpaper.

Primary references checked:

- IOPaint's Expander demonstration uses PowerPaint, rather than promising identical outpainting quality from every supported checkpoint: https://www.iopaint.com/models . PowerPaint's own examples and task-specific outpainting mode: https://github.com/open-mmlab/PowerPaint .
- ComfyUI's basic outpaint tutorial explicitly includes feathering for smooth boundaries: https://docs.comfy.org/tutorials/basic/outpaint . Its dedicated FLUX Fill example and downloadable workflow: https://docs.comfy.org/tutorials/flux/flux-1-fill-dev .
- FLUX.1 Fill dev is a 12B model specifically trained for filling/extending images; its model card also acknowledges colour shifts and edge artefacts: https://huggingface.co/black-forest-labs/FLUX.1-Fill-dev . Full-precision model and encoder requirements are substantial; performance/memory on a 16 GiB Apple Silicon Mac must be measured, not assumed. Quantized/offloaded variants need their own validation.
- BFL's newer Outpainting service advertises coherent continuation without seams or detailed prompts: https://bfl.ai/blog/outpainting-extend-any-image-in-any-direction . This is a hosted API, not a promise that the older local Fill dev checkpoint reproduces those demonstrations.

Next quality gate: run one unchanged representative portrait through an official dedicated-fill workflow before extending the scheduler. Check seams, perspective, subject identity and unintended added people, then test at least two different scene types. Keep model/workflow/seed and processing time with every comparison. Selected official examples establish possible quality on those inputs, not a success rate on arbitrary family photographs.

During the initial pilot, generated output was review-only and automatic sampling/TV serving were off. The selected light-treatment deployment is described below. No family photos were uploaded to a cloud inference service. Cloud demonstrations are references only. Keep the existing single-landscape/paired-portrait wallpaper fallback while evaluating another local model.

## FLUX Fill manual comparison

The dedicated-fill benchmark uses the official node chain and parameters, substituting GGUF loaders and caching the text conditioning with ComfyUI's built-in safetensors save/load nodes. The cached prompt is checked and hashed. Model asset hashes, software revisions, and reproduction commands are in `flux-assets.json` and README.

On an Apple M4 with 16 GiB unified memory, the Q4_K_S Fill model with Q5_K_M T5 encoder ran a 1024×576, 20-step portrait expansion without an out-of-memory error. The first residential-courtyard example took **441 seconds of ComfyUI execution** (including model loading). Sampling itself took about 411 seconds. Text encoding was performed separately before unloading the encoder. This is practical for precomputation, not live TV requests.

In that first example the apartment buildings, trees and ground continue much more coherently than the SD 1.5 prototype's large blank walls. However, distant people and small ground objects still show malformed detail. The output is raw and uncomposited: source identity and colour must be checked because Fill still decodes/regenerates image content. Good background continuation does not prove pixel preservation. This is a quantized local model result, not a measurement of full-precision or newer hosted model limits.

These are deliberately selected manual scene prompts. They do not establish the success rate of unattended processing with a generic prompt. Do not enable automated TV delivery solely on the basis of these selected examples. Actual media and side-by-side pages remain private and are not included in this repository.

The second car-repair-shop example completed in **444 seconds of execution**, separately from time spent waiting in the queue. The roof canopy, pavement and parked cars continue coherently, but newly invented Chinese signage is visibly incorrect. Both examples improve large-scale continuation over the earlier prototype; neither is artefact-free. Original landscape/paired-portrait delivery and all review gates remain unchanged.


## Selected light recipe deployment

Following review of the actual samples, the selected treatment uses radius 10, source-edge feather 96 and outward ramp 260. The original central pixels are restored before JPEG encoding. Two reviewed examples were imported without another diffusion run. A third, newly sampled portrait was processed end to end with the **generic cached prompt**, completed in 435 seconds of ComfyUI execution, softened, uploaded and visually checked. This verifies the automatic integration on one new input; it does not establish an overall photo-library success rate.

The deployed opt-in configuration enables scheduled generation and automatic approval for this recipe. Public example configuration remains disabled with manual review by default. The NAS was tested both ways: serving enabled returned 1920×1080 outpaint headers and valid JPEGs in the batch pool; serving disabled returned the original photo's dimensions and omitted the outpaint variant. Serving was restored after verification. Existing Android adaptive-layout support requires no APK update. Live TV display could not be checked because its ADB connection timed out; debug settings were not changed.

## 2026-10-28: background-only prompt and lower guidance pilot

A difficult edge-cropped group portrait was tested locally with the same source, seed, 20 steps and existing light-softening settings. A scene-specific prompt asked for empty background continuation and no additional people. Guidance 30 and guidance 20 both introduced people in the margins; the lower-guidance sample introduced additional figures. Neither was uploaded or approved. This single-source test does not measure a population failure rate, but these settings did not solve the observed failure. Production generation remains disabled pending a different approach. Private sources, outputs and prompt cache stay outside the repository. `benchmark_flux.py --guidance` enables repeatable local comparisons; its default remains 30.

## 2026-10-28: narrow generated margins with soft outer fields

Local-only pilot on the same difficult portrait: `benchmark_flux.py --margin 64` generated a 560×576 canvas around a 432×576 prepared source, rather than filling a 1024×576 frame. Same background-only conditioning, seed, 20 steps and guidance 30. Generation took approximately 305 seconds. Visual inspection of this sample found no additional distinct people; the model completed edge-cropped portions of existing subjects. This is not a guarantee for other photos.

`limited_outpaint.py` composes a 1920×1080 preview with 32 or 64 generated pixels retained per side (60 or 120 output pixels). Outer areas use coarse, non-generative vertical colour fields rather than mirrored faces. The original centre is restored; a configurable side-edge feather blends into the narrow AI margins. With the selected 64-pixel feather, only the original's edge transition zones are blended. The visual tradeoff is less continuous scenery and more abstract outer backgrounds; narrow generation still invents details immediately beyond the original boundary. No pilot output was uploaded to NAS and production remains disabled. The scripts are experimental and are not wired into the production worker.

Follow-up preview: the same narrow generated image was recomposed with `--margin 64 --feather 32 --softness 0.5`. This halves the local blur and edge transition width, and increases the vertical colour-field sampling from 12 to 24 rows. It adds no new generated content. The outermost background remains abstract because there is no scene detail to recover there. Original defaults and production settings are unchanged.

## 2026-10-28: reject margin people instead of hiding them

A local Apple Vision face/upper-body detector was applied to ten existing full-width FLUX outputs. Both raw generations and softened JPEGs were inspected, with full-frame and contextual side-margin views. Seven passed; three were flagged, including the reported group portrait and two outputs with detected figures in generated scenery. Flag counts include repeated detections across views and are not person counts. The flagged sources were excluded from the live retained publications; seven checked images form the replacement batch. Production resumed with the original light-softening recipe and a pre-upload detection gate. There was no new model generation for this retrospective evaluation. This small, non-labelled sample establishes that the known failure is caught, not a general false-positive or false-negative rate.


## 2026-10-28: allow distant people, reject prominent margin faces

Policy `margin-large-face-v2` stops treating body detections alone or small faces as a rejection. Face boxes use full-frame normalised coordinates even when detected in margin crops, with minimum height 0.08 and width 0.025. The previous confidence and margin-overlap safeguards remain. A retrospective evaluation of twelve existing full-width outputs passed eleven and rejected the reported group portrait. The two previously flagged scenery examples passed this relaxed gate. These are screening decisions on existing images, not newly generated variants or a measured general error rate. A private LAN preview shows one allowed distant-person example and the rejected prominent-face example; none of those private assets are committed. Existing rejected queue decisions are not automatically undone by changing the policy. A worker already running keeps its imported policy until the next scheduled invocation.

## 2026-10-28: require a person in the original photograph

Fifteen saved originals were inspected with the local detector. Three had no detected face/upper body (including equipment and tree-detail samples) and were excluded; this audit also included an in-flight original. The remaining twelve had detections, independently of the margin-face result. Approved count after both rules and prior queue decisions was nine. A replacement publication retired the affected retained batches. The live worker now uses `require_source_person: true` and the v2 large-face margin rule. The pre-GPU check rejects unsuitable originals without generating a new image; cached outputs are checked too. This sample is not a measured detection error rate.

## 2026-10-28: hosted FLUX.2 Klein 4B instruction editing pilot

One previously problematic edge-cropped group portrait was submitted through Black Forest Labs' official Hugging Face ZeroGPU Space for FLUX.2 Klein 4B, using free quota only. Settings: Distilled, four steps, guidance 1, seed 42, output 1024×576, prompt upsampling off. The prompt requested a 16:9 background expansion with the same three original subjects, unchanged faces/clothes/relative positions, no crop/stretch, and no additional prominent faces or people.

Visual inspection found three subjects with no additional prominent stranger, a plausible extended outdoor background and no added softening. However, the model redrew/reframed the photo, including visible changes to facial, hand and clothing details. It did not preserve original pixels despite the prompt. The private preview compares the untouched original with the hosted output; JPEG conversion was the only export processing. No output was added to the TV pool. Hosted precision and exact execution time were not established, and this test does not establish Mac 16GB memory usage, local 4-bit quality, API quota availability or general edit reliability.

The separate local MLX trial was paused before inference at the user's request to evaluate hosted/free options first. Partial model downloads and the isolated test environment are resumable; the production worker remained enabled throughout. No paid API, subscription or top-up was used. Private images, download URLs and account data are not part of this repository.

## 2026-10-28: complete-face hosted follow-up

Two additional portraits with complete main-subject faces were tested in the same official free Space: a couple selfie and an outdoor park portrait. Both used Distilled four steps, guidance 1, seed 42, 1024×576, prompt upsampling off, and explicit instructions to preserve the complete original while adding only side backgrounds. Outputs were inspected directly without original-pixel restoration or softening.

The couple output did not show an additional prominent stranger, but reframed and reduced the subjects and regenerated clothing/body details. The park output continued the scenery plausibly, but also regenerated the main subject and rearranged or removed existing background people. Complete faces therefore did not solve original-content preservation in these two examples. This is a small qualitative pilot, not an identity metric or a general model success rate. Missing portions of an edge-cropped face cannot be established truthfully from that image alone; generation supplies a guess.

Both original/output comparisons are available only on the private LAN preview. None entered production batches, and no paid inference was used. Existing production settings and the paused local MLX experiment were unchanged.

## 2026-10-29: local Klein 4B source-locked outpaint pilot

Two complete-main-face portraits (park portrait and couple selfie) ran offline on the M4/16 GiB machine in an isolated MLX-Gen 0.38.0 environment. Weights: `mlx-community/flux2-klein-4b-4bit`, revision `860e87183ceb29e39627c0612ebd66d8ea66e68c`, pinned and SHA-256 verified. Pillow was pinned to 12.2.0 after 13.0.0 failed to import its typing helper on the test Python 3.12 runtime. Production packages were unchanged.

Each source was proportionally resized to 432×576, then padded by 296 pixels on each horizontal side to 1024×576. The existing `Flux2KleinOutpaint` route used neutral canvas fill, four steps, guidance 1, seed 42 and low-RAM mode, without LoRA or output softening. The family CLI was used for the verified local weight directory because the unified CLI's explicit-base route did not advertise outpaint for that directory. Wall times including process startup/model loading/export were 62.3 s and 57.8 s; denoising took about 48 s and 46 s. MLX reported peak allocation 9.338 GiB in each run; process peak RSS was 1.70/2.01 GiB and must not be substituted for GPU allocation or total system memory. These runs did not establish long-run thermal behavior or library acceptance rate.

Scenery extended plausibly and visual inspection did not find additional prominent strangers in these two outputs. Nevertheless, the direct pipeline output still changed source pixels (mean absolute channel difference 4.190/5.203 on the 0–255 scale). A separate PNG export explicitly restored the complete resized original crop at its known position; its source-region difference was exactly zero. This avoids relying on prompt or latent locking alone. Hard seams, vegetation continuity and inconsistent clothing continuation remain visible, especially on the edge-cropped bodies in the selfie. No claim of production-quality acceptance is made. The private preview shows original, direct pipeline output and explicitly restored version separately; preview JPEG compression is not part of the pixel-preservation measurement.

The current producer was temporarily suspended while its already-submitted GPU task finished, ComfyUI model memory was released, and both isolated tests ran sequentially. The original configuration was restored and the producer resumed afterward. Neither pilot was uploaded as a production result or added to any TV batch. Photos and machine-specific runners remain private. No paid or remote inference was used.
