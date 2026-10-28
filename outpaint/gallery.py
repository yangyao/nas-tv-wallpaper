#!/usr/bin/env python3
"""Generate a private, self-contained before/after page. Never commit its output."""

import argparse, base64, html, io, json, pathlib
from PIL import Image, ImageEnhance


def image_uri(image):
    image = image.copy()
    image.thumbnail((1024, 576))
    b = io.BytesIO()
    image.save(b, format="JPEG", quality=87)
    return "data:image/jpeg;base64," + base64.b64encode(b.getvalue()).decode()


def original_background(source):
    source = source.convert("RGB")
    base = source.resize((32, 18)).resize((1920, 1080), Image.Resampling.BILINEAR)
    base = ImageEnhance.Brightness(base).enhance(0.5)
    factor = min(1920 / source.width, 1080 / source.height)
    size = (round(source.width * factor), round(source.height * factor))
    photo = source.resize(size, Image.Resampling.LANCZOS)
    base.paste(photo, ((1920 - size[0]) // 2, (1080 - size[1]) // 2))
    return base


def main():
    p = argparse.ArgumentParser()
    p.add_argument("results", type=pathlib.Path)
    p.add_argument("--output", type=pathlib.Path, required=True)
    p.add_argument("--limit", type=int, default=6)
    p.add_argument("--all-versions", action="store_true")
    a = p.parse_args()
    candidates = sorted(
        [
            f.parent
            for f in a.results.expanduser().glob("*/result.jpg")
            if (f.parent / "original.jpg").is_file()
        ],
        key=lambda folder: folder.stat().st_mtime,
        reverse=True,
    )
    folders = []
    seen = set()
    for folder in candidates:
        metadata = (
            json.loads((folder / "job.json").read_text())
            if (folder / "job.json").exists()
            else {}
        )
        photo_id = metadata.get("job", {}).get("photo_id", folder.name)
        if photo_id in seen and not a.all_versions:
            continue
        seen.add(photo_id)
        folders.append(folder)
        if len(folders) >= max(1, min(20, a.limit)):
            break
    cards = []
    for i, folder in enumerate(folders, 1):
        with Image.open(folder / "original.jpg") as im:
            before = image_uri(original_background(im))
        with Image.open(folder / "result.jpg") as im:
            after = image_uri(im)
        timing = (
            json.loads((folder / "timing.json").read_text())
            if (folder / "timing.json").exists()
            else {}
        )
        elapsed = timing.get("seconds")
        if timing.get("recovered_without_generation"):
            elapsed = round(timing.get("generation_seconds", 0) + (elapsed or 0), 2)
        seconds = html.escape(str(elapsed if elapsed is not None else "—"))
        cards.append(
            f'<article><h2>樣本 {i} <small>處理 {seconds} 秒</small></h2><div class="pair"><figure><img src="{before}"><figcaption>原圖＋柔化背景</figcaption></figure><figure><img src="{after}"><figcaption>AI 擴圖；中間原照片保留</figcaption></figure></div></article>'
        )
    document = (
        """<!doctype html><html lang="zh-Hant"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>照片擴圖實測</title><style>body{margin:0;background:#101114;color:#e9e9eb;font:16px system-ui,sans-serif;padding:32px;max-width:1600px;margin:auto}h1{font-size:28px}p,small,figcaption{color:#b3b5bc}small{font-size:14px;font-weight:400;margin-left:16px}.pair{display:grid;grid-template-columns:1fr 1fr;gap:16px}figure{margin:0}img{width:100%;border-radius:8px}figcaption{margin-top:8px}article{margin:36px 0 56px}h2{font-size:20px}@media(max-width:800px){.pair{grid-template-columns:1fr}body{padding:16px}}</style><h1>照片擴圖實測</h1><p>左右比較原有柔化背景與 AI 擴圖。AI 只補外圍背景；原照片於生成後重新貼回，仍需檢查新增的人物、文字與建築是否自然。本頁完全本機顯示，不載入外部資源。</p>"""
        + "".join(cards)
        + "</html>"
    )
    a.output.expanduser().write_text(document)
    print("Private gallery:", a.output, "samples:", len(cards))


if __name__ == "__main__":
    main()
