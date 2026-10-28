import unittest
from PIL import Image
from worker import prepare, finish


class CompositionTests(unittest.TestCase):
    def test_original_high_resolution_region_is_not_generated(self):
        source = Image.new("RGB", (300, 400), (23, 101, 79))
        generated = Image.new("RGB", (896, 504), (204, 15, 33))
        result = finish(generated, source)
        self.assertEqual(result.size, (1920, 1080))
        self.assertEqual(
            result.crop((555, 0, 1365, 1080)).tobytes(),
            source.resize((810, 1080), Image.Resampling.LANCZOS).tobytes(),
        )
        self.assertEqual(result.getpixel((0, 500)), (204, 15, 33))

    def test_only_outer_canvas_is_masked(self):
        canvas, original, (x, y) = prepare(Image.new("RGB", (300, 400)))
        self.assertEqual(canvas.size, (896, 504))
        self.assertEqual(canvas.getpixel((0, 0))[3], 0)
        self.assertEqual(canvas.getpixel((448, 252))[3], 255)
        self.assertEqual(canvas.getpixel((x, y))[3], 255)


class ProtectedCompositionTests(unittest.TestCase):
    def test_blending_changes_only_background_border_and_keeps_face_box(self):
        source = Image.new("RGB", (300, 400), (23, 101, 79))
        generated = Image.new("RGB", (896, 504), (204, 15, 33))
        result = finish(generated, source, [(0, 0.25, 0.1, 0.2)])
        self.assertEqual(result.getpixel((555, 400)), (23, 101, 79))
        self.assertEqual(result.getpixel((555, 50)), (204, 15, 33))
        self.assertEqual(result.getpixel((960, 400)), (23, 101, 79))

    def test_face_helper_finalization_saves_and_uploads(self):
        import tempfile, pathlib, types, time, contextlib, io
        from unittest.mock import patch
        from worker import Worker

        with tempfile.TemporaryDirectory() as directory:
            worker = Worker(
                {
                    "nas_url": "http://127.0.0.1:1",
                    "token": "test-token",
                    "work_dir": directory,
                    "face_helper": "/not-actually-run",
                }
            )
            folder = pathlib.Path(directory)
            uploaded = []
            worker.nas = lambda method, path, **kw: uploaded.append(
                (method, path, len(kw["data"].read()))
            )
            with patch(
                "worker.subprocess.run",
                return_value=types.SimpleNamespace(stdout=b"[]"),
            ) as helper:
                with contextlib.redirect_stdout(io.StringIO()):
                    worker.finalize(
                        {"id": "test-job"},
                        folder,
                        Image.new("RGB", (896, 504)),
                        Image.new("RGB", (300, 400)),
                        time.monotonic(),
                    )
                helper.assert_called_once()
            self.assertEqual(uploaded[0][0], "PUT")
            self.assertTrue((folder / "result.jpg").is_file())


class WorkerInputTests(unittest.TestCase):
    def test_job_id_cannot_escape_work_directory(self):
        import tempfile
        from worker import Worker

        with tempfile.TemporaryDirectory() as directory:
            worker = Worker(
                {
                    "nas_url": "http://127.0.0.1:1",
                    "token": "test-token",
                    "work_dir": directory,
                }
            )
            with self.assertRaises(ValueError):
                worker.process({"id": "../../outside"})


if __name__ == "__main__":
    unittest.main()
