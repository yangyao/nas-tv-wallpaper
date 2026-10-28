"""Conservative local detection gate for people extending into generated margins."""

import json
import math
import pathlib
import subprocess
import tempfile
from PIL import Image

VERSION = "margin-people-v1"


class NewPeopleDetected(Exception):
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
    # Reject even an existing edge-cropped face being substantially invented.
    # A person box can span several original subjects, so require its centre
    # outside too; face detections use the stricter margin-overlap rule.
    return outside >= 0.20 if kind == "face" else (
        outside >= 0.35 and not left <= x + w / 2 <= right
    )


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
