"""Conservative local detection gate for people extending into generated margins."""

import json
import math
import pathlib
import subprocess
import tempfile
from PIL import Image, ImageOps

VERSION = "margin-large-face-v2"


class NewPeopleDetected(Exception):
    pass


class NoSourcePerson(Exception):
    pass


def outside_subject(item, left, right):
    kind = item["kind"]
    confidence = float(item["confidence"])
    x, y, w, h = map(float, item["box"])
    if kind not in ("face", "person") or not all(
        math.isfinite(v) for v in (confidence, x, y, w, h)
    ) or w <= 0 or h <= 0:
        raise ValueError("invalid detector response")
    if confidence < (0.25 if kind == "face" else 0.35):
        return False
    overlap = max(0, min(x + w, right) - max(x, left))
    outside = 1 - min(1, overlap / w)
    # Full-frame normalised size, including detections from side crops. Bodies,
    # backs and small distant faces are allowed. At 1080p the face-height cutoff
    # is about 86 pixels; this is a visual-size heuristic, not identity recognition.
    return (kind == "face" and outside >= 0.20
            and h >= 0.08 and w >= 0.025)



def inspect_image(path, helper, source_left, source_right):
    detections = []
    with Image.open(path) as im, tempfile.TemporaryDirectory(prefix="people-check-") as tmp:
        im = im.convert("RGB")
        width, height = im.size
        # Include some source context so partial people on a margin boundary can
        # still be detected. Normalise crop detections back to full-frame space.
        windows = [(0, width), (0, min(width, round(source_left * width) + 64)),
                   (max(0, round(source_right * width) - 64), width)]
        for i, (x1, x2) in enumerate(windows):
            crop = pathlib.Path(tmp) / (str(i) + ".png")
            im.crop((x1, 0, x2, height)).save(crop)
            reply = subprocess.run([str(pathlib.Path(helper).expanduser()), str(crop)],
                                   capture_output=True, check=True, timeout=45)
            if len(reply.stdout) > 256 * 1024:
                raise ValueError("detector response too large")
            items = json.loads(reply.stdout)
            if not isinstance(items, list) or len(items) > 100:
                raise ValueError("invalid detector response")
            for item in items:
                x, y, w, h = item["box"]
                mapped = dict(item, box=[(x1 + x * (x2-x1)) / width, y,
                                         w * (x2-x1) / width, h])
                mapped["outside"] = outside_subject(mapped, source_left, source_right)
                detections.append(mapped)
    return {"version": VERSION, "passed": not any(d["outside"] for d in detections),
            "detections": detections}


def check_result(folder, helper):
    folder = pathlib.Path(folder)
    with Image.open(folder / "input.png") as prepared:
        width = prepared.width
    if width <= 0 or width >= 1024 or width % 8:
        raise ValueError("invalid source geometry")
    left = ((1024 - width) // 16) * 8
    reports = {}
    for name in ("generated.png", "result.jpg"):
        with Image.open(folder / name) as image:
            expected = (1024, 576) if name.endswith("png") else (1920, 1080)
            if image.size != expected:
                raise ValueError("invalid guarded output size")
        reports[name] = inspect_image(folder / name, helper, left/1024, (left+width)/1024)
    return {"version": VERSION, "passed": all(r["passed"] for r in reports.values()),
            "images": reports}


def check_source(path, helper):
    """Check original pixels only, before spending GPU time on outpainting."""
    with Image.open(path) as im, tempfile.TemporaryDirectory(prefix="source-check-") as tmp:
        prepared = pathlib.Path(tmp) / "source.png"
        ImageOps.exif_transpose(im).convert("RGB").save(prepared)
        reply = subprocess.run([str(pathlib.Path(helper).expanduser()), str(prepared)],
                               capture_output=True, check=True, timeout=45)
        if len(reply.stdout) > 256 * 1024:
            raise ValueError("detector response too large")
        items = json.loads(reply.stdout)
        if not isinstance(items, list) or len(items) > 100:
            raise ValueError("invalid detector response")
        for item in items:
            outside_subject(item, 0, 1)  # Validate every result, including low confidence.
        present = any(float(item["confidence"]) >=
                      (0.25 if item["kind"] == "face" else 0.35) for item in items)
        return {"version": "source-person-v1", "passed": present, "detections": items}
