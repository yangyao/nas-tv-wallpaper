#!/usr/bin/env python3
"""Local experiment: narrow AI margins fading into non-generative colour fields."""

import argparse
from PIL import Image, ImageFilter, ImageOps


def compose(source, generated, prepared_width, margin=32, feather=64):
    if generated.height != 576 or prepared_width <= 0 or prepared_width % 8:
        raise ValueError("invalid prepared dimensions")
    padding = (generated.width - prepared_width) // 2
    if generated.width != prepared_width + 2 * padding or not 8 <= margin <= padding:
        raise ValueError("margin must fit symmetric generated padding")
    scale = 1080 / 576
    core_width = round(prepared_width * scale)
    side = round(margin * scale)
    left = (1920 - core_width) // 2
    if left <= side or core_width + 2 * side > 1920:
        raise ValueError("source is too wide for this portrait experiment")
    crop = generated.convert("RGB").crop(
        (padding - margin, 0, padding + prepared_width + margin, 576)
    ).resize((core_width + 2 * side, 1080), Image.Resampling.LANCZOS)
    softened = crop.filter(ImageFilter.GaussianBlur(10))
    result = Image.new("RGB", (1920, 1080))
    # Collapse each outer strip into a very coarse vertical colour field. No
    # mirroring or horizontal stretching of faces into the remaining canvas.
    for box, dest in [
        ((0, 0, min(24, side), 1080), (0, 0, left, 1080)),
        ((crop.width - min(24, side), 0, crop.width, 1080),
         (left + core_width, 0, 1920, 1080)),
    ]:
        field = softened.crop(box).resize((1, 12), Image.Resampling.BOX)
        field = field.resize((dest[2] - dest[0], 1080), Image.Resampling.BICUBIC)
        result.paste(field.filter(ImageFilter.GaussianBlur(48)), dest[:2])
    weights = []
    for x in range(crop.width):
        t = max(0, min(1, min(x, crop.width - 1 - x) / side))
        weights.append(round(255 * t * t * (3 - 2 * t)))
    mask = Image.new("L", (crop.width, 1))
    mask.putdata(weights)
    result.paste(softened, (left - side, 0), mask.resize(crop.size))
    # Restore the original centre; blend only its side-edge transition zones.
    original = ImageOps.exif_transpose(source).convert("RGB").resize(
        (core_width, 1080), Image.Resampling.LANCZOS
    )
    feather = min(max(0, feather), core_width // 4)
    if feather:
        edge = Image.new("L", (core_width, 1))
        weights = []
        for x in range(core_width):
            t = max(0, min(1, min(x, core_width - 1 - x) / feather))
            weights.append(round(255 * t * t * (3 - 2 * t)))
        edge.putdata(weights)
        result.paste(original, (left, 0), edge.resize(original.size))
    else:
        result.paste(original, (left, 0))
    return result


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--source", required=True)
    p.add_argument("--generated", required=True)
    p.add_argument("--prepared-width", type=int, required=True)
    p.add_argument("--margin", type=int, default=32)
    p.add_argument("--feather", type=int, default=64)
    p.add_argument("--output", required=True)
    a = p.parse_args()
    with Image.open(a.source) as source, Image.open(a.generated) as generated:
        compose(source, generated, a.prepared_width, a.margin, a.feather).save(a.output, quality=95)


if __name__ == "__main__":
    main()
