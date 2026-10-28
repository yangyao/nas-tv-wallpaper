#!/usr/bin/env python3
"""Deterministic local softening of generated margins; no inference or network."""

import argparse
import json
import pathlib
from PIL import Image, ImageFilter, ImageOps


def smooth(value):
    value = max(0.0, min(1.0, value))
    return value * value * (3 - 2 * value)


def soften(source, generated, prepared_size, radius=10, feather=96, ramp=260):
    if generated.size != (1024, 576):
        raise ValueError("Expected an unchanged 1024x576 benchmark output")
    width, height = prepared_size
    if height != 576 or width >= 1024 or width % 8:
        raise ValueError("Invalid prepared portrait dimensions")
    output_size = (1920, 1080)
    scale = output_size[0] / 1024
    left = ((1024 - width) // 16) * 8
    x = round(left * scale)
    size = (round(width * scale), 1080)
    feather = min(feather, max(1, size[0] // 4))
    original = (
        ImageOps.exif_transpose(source)
        .convert("RGB")
        .resize(size, Image.Resampling.LANCZOS)
    )
    base = generated.convert("RGB").resize(output_size, Image.Resampling.LANCZOS)
    # Blend a small Gaussian pyramid to vary blur continuously across the canvas.
    strengths = []
    for column in range(1920):
        distance = max(x - column, column - (x + size[0] - 1))
        strengths.append(smooth((distance + feather) / (ramp + feather)))
    levels = 5
    result = base
    for level in range(1, levels + 1):
        mask = Image.new("L", (1920, 1))
        mask.putdata(
            [
                round(255 * max(0, min(1, strength * levels - (level - 1))))
                for strength in strengths
            ]
        )
        mask = mask.resize(output_size)
        blurred = base.filter(ImageFilter.GaussianBlur(radius * level / levels))
        result = Image.composite(blurred, result, mask)
    # Restore source pixels in the centre, feathering only the source's side edges.
    mask = Image.new("L", (size[0], 1))
    mask.putdata(
        [round(255 * smooth(min(c, size[0] - 1 - c) / feather)) for c in range(size[0])]
    )
    mask = mask.resize(size)
    result.paste(original, (x, 0), mask)
    return result


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--source", type=pathlib.Path, required=True)
    p.add_argument("--generated", type=pathlib.Path, required=True)
    p.add_argument("--prepared", type=pathlib.Path, required=True)
    p.add_argument("--output", type=pathlib.Path, required=True)
    p.add_argument("--radius", type=float, default=10)
    p.add_argument("--feather", type=int, default=96)
    p.add_argument("--ramp", type=int, default=260)
    a = p.parse_args()
    if not (0 <= a.radius <= 100 and 1 <= a.feather <= 160 and 1 <= a.ramp <= 600):
        p.error("Invalid blur settings")
    with (
        Image.open(a.source) as source,
        Image.open(a.generated) as generated,
        Image.open(a.prepared) as prepared,
    ):
        result = soften(source, generated, prepared.size, a.radius, a.feather, a.ramp)
    a.output.parent.mkdir(parents=True, exist_ok=True)
    result.save(a.output)
    a.output.chmod(0o600)
    a.output.with_suffix(".json").write_text(
        json.dumps(
            {
                "method": "progressive-margin-gaussian-blur",
                "output_size": [1920, 1080],
                "radius": a.radius,
                "source_edge_feather": a.feather,
                "outer_ramp": a.ramp,
                "source_core_restored": True,
            },
            indent=2,
        )
    )
    print(a.output)


if __name__ == "__main__":
    main()
