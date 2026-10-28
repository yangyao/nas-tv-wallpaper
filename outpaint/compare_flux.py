#!/usr/bin/env python3
"""Create a private offline comparison of source, old pipeline and raw Fill output."""

import argparse
import base64
import html
import io
import pathlib
from PIL import Image, ImageOps


def uri(path, contain=False):
    with Image.open(path) as source:
        image = ImageOps.exif_transpose(source).convert("RGB")
        if contain:
            image = ImageOps.contain(image, (1024, 576))
            canvas = Image.new("RGB", (1024, 576), "#202124")
            canvas.paste(image, ((1024 - image.width) // 2, (576 - image.height) // 2))
            image = canvas
        image.thumbnail((1536, 864))
        stream = io.BytesIO()
        image.save(stream, "JPEG", quality=94)
    return "data:image/jpeg;base64," + base64.b64encode(stream.getvalue()).decode()


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--source", type=pathlib.Path, required=True)
    p.add_argument("--previous", type=pathlib.Path, required=True)
    p.add_argument("--current", type=pathlib.Path, required=True)
    p.add_argument("--output", type=pathlib.Path, required=True)
    p.add_argument("--title", default="FLUX Fill 同圖比較")
    a = p.parse_args()
    cards = []
    for label, path, contain in [
        ("原照片（灰色區域為待擴充範圍）", a.source, True),
        ("之前：SD 1.5＋原圖邊緣混合", a.previous, False),
        ("現在：FLUX Fill 原始輸出", a.current, False),
    ]:
        cards.append(
            f'<figure><img src="{uri(path, contain)}"><figcaption>{label}</figcaption></figure>'
        )
    document = (
        '<!doctype html><html lang="zh-Hant"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>'
        + html.escape(a.title)
        + "</title><style>body{max-width:1200px;margin:auto;padding:24px;background:#101114;color:#eee;font:16px system-ui}h1{font-size:24px}p,figcaption{color:#b9bbc2}figure{margin:32px 0}img{display:block;width:100%;border-radius:8px}figcaption{padding:8px 0}</style><h1>"
        + html.escape(a.title)
        + "</h1><p>同一張照片。新版本直接展示模型輸出，沒有重新貼回原圖或混合修邊。請一起檢查接縫、人物、透視，以及有沒有多出不合理的物件。所有圖片內嵌於本頁，不會向外部服務載入資料。</p>"
        + "".join(cards)
        + "</html>"
    )
    a.output.write_text(document)
    a.output.chmod(0o600)
    print(a.output)


if __name__ == "__main__":
    main()
