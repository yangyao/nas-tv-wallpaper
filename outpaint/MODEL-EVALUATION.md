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
