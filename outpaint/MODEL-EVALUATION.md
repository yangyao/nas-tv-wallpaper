# Prototype quality assessment

The current SD 1.5 workflow proves local MPS generation, persisted jobs, recovery and review-only delivery. **Its visual quality is not an accepted production baseline.** Broad expansion produced inconsistent scenery and obvious joins; face-protected border blending helps joins but does not solve scene understanding. Do not interpret successful generation as acceptable wallpaper.

Primary references checked:

- IOPaint's Expander demonstration uses PowerPaint, rather than promising identical outpainting quality from every supported checkpoint: https://www.iopaint.com/models . PowerPaint's own examples and task-specific outpainting mode: https://github.com/open-mmlab/PowerPaint .
- ComfyUI's basic outpaint tutorial explicitly includes feathering for smooth boundaries: https://docs.comfy.org/tutorials/basic/outpaint . Its dedicated FLUX Fill example and downloadable workflow: https://docs.comfy.org/tutorials/flux/flux-1-fill-dev .
- FLUX.1 Fill dev is a 12B model specifically trained for filling/extending images; its model card also acknowledges colour shifts and edge artefacts: https://huggingface.co/black-forest-labs/FLUX.1-Fill-dev . Full-precision model and encoder requirements are substantial; performance/memory on a 16 GiB Apple Silicon Mac must be measured, not assumed. Quantized/offloaded variants need their own validation.
- BFL's newer Outpainting service advertises coherent continuation without seams or detailed prompts: https://bfl.ai/blog/outpainting-extend-any-image-in-any-direction . This is a hosted API, not a promise that the older local Fill dev checkpoint reproduces those demonstrations.

Next quality gate: run one unchanged representative portrait through an official dedicated-fill workflow before extending the scheduler. Check seams, perspective, subject identity and unintended added people, then test at least two different scene types. Keep model/workflow/seed and processing time with every comparison. Selected official examples establish possible quality on those inputs, not a success rate on arbitrary family photographs.

Current operational stance: generated output remains review-only; automatic sampling and TV serving are off in the personal pilot. No family photos were uploaded to a cloud inference service. Cloud demonstrations are references only. Keep the existing single-landscape/paired-portrait wallpaper fallback while evaluating another local model.
