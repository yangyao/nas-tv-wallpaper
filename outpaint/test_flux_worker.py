import io
import json
import pathlib
import tempfile
import time
import unittest
from unittest.mock import patch
from PIL import Image
from flux_worker import FluxWorker, DEFAULT_PROMPT
from soften_outpaint import soften
from people_guard import NewPeopleDetected, NoSourcePerson


class Response:
    def __init__(self, value=None, content=None):
        self.value, self.content = value, content

    def json(self):
        return self.value

    def raise_for_status(self):
        pass


class Http:
    def __init__(self, raw, prompt, other=False):
        self.calls = []
        self.raw = raw
        self.prompt = prompt
        self.other = other

    def get(self, url, **kw):
        self.calls.append(("GET", url))
        if "/history/" in url:
            return Response(
                {
                    self.prompt: {
                        "status": {"status_str": "success", "messages": []},
                        "outputs": {
                            "9": {
                                "images": [
                                    {
                                        "filename": "sample.png",
                                        "subfolder": "outpaint",
                                        "type": "output",
                                    }
                                ]
                            }
                        },
                    }
                }
            )
        if url.endswith("/view"):
            return Response(content=self.raw)
        if url.endswith("/queue"):
            return Response(
                {
                    "queue_running": [[0, "other" if self.other else self.prompt]],
                    "queue_pending": [],
                }
            )
        raise AssertionError(url)

    def post(self, url, **kw):
        self.calls.append(("POST", url))
        raise AssertionError("Unexpected submission/interrupt: " + url)


class FluxWorkerTests(unittest.TestCase):
    def test_empty_source_is_skipped_before_generation_or_cached_upload(self):
        for cached in (False, True):
            with tempfile.TemporaryDirectory() as directory:
                w, job, folder, calls = self.setup_worker(
                    directory, require_source_person=True, people_detector="detector"
                )
                if cached:
                    Image.new("RGB", (1920, 1080)).save(folder / "result.jpg")
                with patch("flux_worker.check_source", return_value={"passed": False}):
                    with self.assertRaises(NoSourcePerson):
                        w.process(job)
                self.assertEqual(calls, [])

    def test_cached_result_cannot_bypass_people_guard(self):
        with tempfile.TemporaryDirectory() as directory:
            w, job, folder, calls = self.setup_worker(
                directory, auto_approve=True, people_guard=True, people_detector="detector"
            )
            Image.new("RGB", (1920, 1080)).save(folder / "result.jpg")
            with patch("flux_worker.check_result", return_value={"passed": False}):
                with self.assertRaises(NewPeopleDetected):
                    w.process(job)
            self.assertEqual(calls, [])

    def test_detector_failure_cannot_upload_or_approve(self):
        with tempfile.TemporaryDirectory() as directory:
            w, job, folder, calls = self.setup_worker(
                directory, auto_approve=True, people_guard=True, people_detector="detector"
            )
            Image.new("RGB", (1920, 1080)).save(folder / "result.jpg")
            with patch("flux_worker.check_result", side_effect=ValueError("bad detector")):
                with self.assertRaises(ValueError):
                    w.process(job)
            self.assertEqual(calls, [])

    def setup_worker(self, directory, **overrides):
        root = pathlib.Path(directory)
        emb = root / "comfy/models/embeddings"
        emb.mkdir(parents=True)
        (emb / "text.safetensors").write_bytes(b"cached")
        (emb / "text.json").write_text(json.dumps({"prompt": DEFAULT_PROMPT}))
        config = {
            "nas_url": "http://127.0.0.1:1",
            "token": "test-only",
            "work_dir": str(root / "work"),
            "comfy_root": str(root / "comfy"),
            "flux_conditioning": "text.safetensors",
            **overrides,
        }
        w = FluxWorker(config)
        job = {"id": "a" * 32, "workflow": w.workflow, "prompt": ""}
        folder = w.directory / job["id"]
        folder.mkdir()
        Image.new("RGB", (300, 400), (40, 80, 120)).save(
            folder / "original.jpg", quality=100
        )
        Image.new("RGB", (432, 576)).save(folder / "input.png")
        calls = []

        def nas(method, path, **kw):
            calls.append((method, path))
            if method == "PUT":
                with Image.open(io.BytesIO(kw["data"].read())) as im:
                    self.assertEqual(im.size, (1920, 1080))
            return Response({"jobs": []})

        w.nas = nas
        return w, job, folder, calls

    def test_recovery_reuses_saved_prompt_and_publishes_light_result(self):
        with tempfile.TemporaryDirectory() as directory:
            w, job, folder, calls = self.setup_worker(directory, auto_approve=True)
            prompt = "saved-prompt"
            (folder / "job.json").write_text(
                json.dumps(
                    {
                        "prompt_id": prompt,
                        "recipe": w.recipe,
                        "workflow": {"17": {"inputs": {"image": "original.png"}}},
                    }
                )
            )
            buffer = io.BytesIO()
            Image.new("RGB", (1024, 576), "red").save(buffer, "PNG")
            w.http = Http(buffer.getvalue(), prompt)
            w.process(job)
            self.assertTrue((folder / "generated.png").exists())
            self.assertTrue((folder / "result.jpg").exists())
            self.assertEqual(
                calls,
                [
                    ("PUT", "jobs/" + job["id"] + "/result"),
                    ("POST", "jobs/" + job["id"] + "/decision"),
                ],
            )
            self.assertFalse(any(method == "POST" for method, _ in w.http.calls))
            w.http.calls = []
            calls.clear()
            w.process(job)
            self.assertEqual(w.http.calls, [])
            self.assertEqual(len(calls), 2)

    def test_manual_review_default_and_recipe_change_invalidates_queue_key(self):
        with tempfile.TemporaryDirectory() as directory:
            w, job, folder, calls = self.setup_worker(directory)
            old = w.workflow
            generated = Image.new("RGB", (1024, 576), "red")
            with Image.open(folder / "original.jpg") as source:
                w.finalize(job, folder, generated, source, time.monotonic())
            self.assertEqual(calls, [("PUT", "jobs/" + job["id"] + "/result")])
            config = dict(w.c, soften_radius=16)
            self.assertNotEqual(FluxWorker(config).workflow, old)

    def test_timeout_does_not_interrupt_unrelated_prompt(self):
        with tempfile.TemporaryDirectory() as directory:
            w, job, folder, calls = self.setup_worker(directory, timeout_seconds=0)
            (folder / "job.json").write_text(
                json.dumps({"prompt_id": "saved", "recipe": w.recipe})
            )
            w.http = Http(b"", "saved", other=True)
            with self.assertRaises(TimeoutError):
                w.process(job)
            self.assertFalse(any(method == "POST" for method, _ in w.http.calls))

    def test_original_centre_is_exact_before_jpeg_encoding(self):
        source = Image.new("RGB", (810, 1080), (40, 80, 120))
        output = soften(
            source, Image.new("RGB", (1024, 576), "red"), (432, 576), 10, 96, 260
        )
        self.assertEqual(
            output.crop((651, 0, 1269, 1080)).tobytes(),
            source.crop((96, 0, 714, 1080)).tobytes(),
        )
