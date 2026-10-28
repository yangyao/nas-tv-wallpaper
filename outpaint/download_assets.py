#!/usr/bin/env python3
"""Resumable, SHA-256 verified model downloads. No inference data is sent anywhere."""

import argparse, concurrent.futures, hashlib, json, os, pathlib, threading, time, urllib.request, urllib.error


def checksum(path):
    h = hashlib.sha256()
    with path.open("rb") as f:
        for data in iter(lambda: f.read(8 * 1024 * 1024), b""):
            h.update(data)
    return h.hexdigest()


def download(asset, root, mirror, workers, chunk):
    relative = pathlib.PurePosixPath(asset["path"])
    if relative.is_absolute() or ".." in relative.parts:
        raise ValueError("unsafe asset path")
    target = root / relative
    target.parent.mkdir(parents=True, exist_ok=True)
    size = asset["size"]
    if (
        target.exists()
        and target.stat().st_size == size
        and checksum(target) == asset["sha256"]
    ):
        print("Verified existing", relative, flush=True)
        return
    partial = target.with_name(target.name + ".part")
    state = target.with_name(target.name + ".progress.json")
    saved = json.loads(state.read_text()) if state.exists() else {}
    if saved and (
        saved.get("sha256") != asset["sha256"] or saved.get("chunk") != chunk
    ):
        raise ValueError("incompatible resume state")
    done = saved.get("done", {}) if partial.exists() else {}
    fd = os.open(partial, os.O_CREAT | os.O_RDWR, 0o600)
    os.ftruncate(fd, size)
    # Recheck completed ranges after an interrupted process/power loss.
    for key, digest in list(done.items()):
        start = int(key) * chunk
        data = os.pread(fd, min(chunk, size - start), start)
        if hashlib.sha256(data).hexdigest() != digest:
            del done[key]
    url = asset.get("url") or (
        mirror.rstrip("/")
        + "/"
        + asset["repo"]
        + "/resolve/main/"
        + asset["file"]
        + "?download=true"
    )
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    with opener.open(urllib.request.Request(url, method="HEAD"), timeout=60) as reply:
        direct = reply.url
    lock = threading.Lock()
    began = time.monotonic()
    total = (size + chunk - 1) // chunk
    initial = len(done)

    def save():
        tmp = state.with_name(state.name + ".tmp")
        tmp.write_text(
            json.dumps({"sha256": asset["sha256"], "chunk": chunk, "done": done})
        )
        tmp.chmod(0o600)
        tmp.replace(state)

    def fetch(index):
        key = str(index)
        if key in done:
            return
        start = index * chunk
        end = min(size, start + chunk) - 1
        for attempt in range(5):
            try:
                req = urllib.request.Request(
                    direct, headers={"Range": f"bytes={start}-{end}"}
                )
                with opener.open(req, timeout=90) as reply:
                    if (
                        reply.status != 206
                        or reply.headers.get("Content-Range", "").split("/")[0]
                        != f"bytes {start}-{end}"
                    ):
                        raise ValueError("server returned wrong range")
                    data = reply.read(end - start + 1)
                    if len(data) != end - start + 1:
                        raise ValueError("incomplete range")
                os.pwrite(fd, data, start)
                with lock:
                    done[key] = hashlib.sha256(data).hexdigest()
                    if len(done) % 8 == 0 or len(done) == total:
                        save()
                    if len(done) % 32 == 0 or len(done) == total:
                        elapsed = time.monotonic() - began
                        speed = (
                            (len(done) - initial)
                            * chunk
                            / max(1, elapsed)
                            / 1024
                            / 1024
                        )
                        print(
                            f"{relative.name}: {len(done)}/{total} ranges ({len(done) / total:.1%}), {speed:.1f} MiB/s",
                            flush=True,
                        )
                return
            except Exception:
                if attempt == 4:
                    raise
                time.sleep(1 + attempt)

    try:
        with concurrent.futures.ThreadPoolExecutor(max_workers=workers) as pool:
            list(pool.map(fetch, range(total)))
        os.fsync(fd)
    finally:
        with lock:
            save()
        os.close(fd)
    if checksum(partial) != asset["sha256"]:
        raise ValueError("whole-file checksum mismatch")
    partial.replace(target)
    state.unlink(missing_ok=True)
    print("SHA-256 verified", relative, flush=True)


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--root", type=pathlib.Path, required=True)
    p.add_argument(
        "--manifest",
        type=pathlib.Path,
        default=pathlib.Path(__file__).with_name("flux-assets.json"),
    )
    p.add_argument("--mirror", default="https://huggingface.co")
    p.add_argument("--workers", type=int, default=48)
    p.add_argument("--chunk-mib", type=int, default=2)
    a = p.parse_args()
    if not a.mirror.startswith("https://"):
        p.error("HTTPS mirror required")
    for asset in json.loads(a.manifest.read_text()):
        download(
            asset,
            a.root.expanduser(),
            a.mirror,
            max(1, min(96, a.workers)),
            max(1, min(8, a.chunk_mib)) * 1024 * 1024,
        )


if __name__ == "__main__":
    main()
