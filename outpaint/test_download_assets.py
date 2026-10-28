import hashlib
import io
import pathlib
import tempfile
import unittest
from unittest.mock import patch
from download_assets import download


class Reply(io.BytesIO):
    url = "https://example.invalid/weights"
    status = 206

    def __init__(self, data=b"", headers=None):
        super().__init__(data)
        self.headers = headers or {}


class Opener:
    def __init__(self, data, broken=None):
        self.data, self.broken, self.ranges = data, broken, []

    def open(self, request, timeout):
        if request.get_method() == "HEAD":
            return Reply()
        start, end = map(
            int, request.get_header("Range").removeprefix("bytes=").split("-")
        )
        self.ranges.append(start)
        if start == self.broken:
            raise OSError("interrupted")
        return Reply(
            self.data[start : end + 1],
            {"Content-Range": f"bytes {start}-{end}/{len(self.data)}"},
        )


class DownloadIntegrityTests(unittest.TestCase):
    def asset(self, data):
        return {
            "path": "models/test.gguf",
            "repo": "test/repo",
            "file": "test.gguf",
            "size": len(data),
            "sha256": hashlib.sha256(data).hexdigest(),
        }

    def run_download(self, asset, root, opener):
        with (
            patch("download_assets.urllib.request.build_opener", return_value=opener),
            patch("download_assets.time.sleep"),
        ):
            download(asset, root, "https://example.invalid", 2, 4)

    def test_interrupt_resumes_verified_chunks_and_installs_only_complete_file(self):
        data = b"0123456789abcdef"
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            asset = self.asset(data)
            with self.assertRaises(OSError):
                self.run_download(asset, root, Opener(data, broken=4))
            self.assertFalse((root / asset["path"]).exists())
            opener = Opener(data)
            self.run_download(asset, root, opener)
            self.assertIn(4, opener.ranges)
            self.assertNotIn(0, opener.ranges)
            self.assertEqual((root / asset["path"]).read_bytes(), data)

    def test_complete_but_wrong_bytes_are_never_installed(self):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            asset = self.asset(b"correct!")
            with self.assertRaisesRegex(ValueError, "checksum mismatch"):
                self.run_download(asset, root, Opener(b"WRONG!!!"))
            self.assertFalse((root / asset["path"]).exists())

    def test_path_cannot_escape_destination(self):
        asset = self.asset(b"x")
        asset["path"] = "../escape.gguf"
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaisesRegex(ValueError, "unsafe asset path"):
                self.run_download(asset, pathlib.Path(directory), Opener(b"x"))
