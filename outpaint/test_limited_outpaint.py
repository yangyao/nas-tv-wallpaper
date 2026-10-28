import unittest
from PIL import Image
from limited_outpaint import compose


class LimitedOutpaintTests(unittest.TestCase):
    def test_original_rectangle_survives_adversarial_generated_content(self):
        original = Image.new("RGB", (432, 576), (12, 91, 143))
        # Even a completely different generated centre must never replace people
        # or other source pixels. Test the lossless composition before JPEG export.
        generated = Image.new("RGB", (560, 576), (245, 10, 10))
        for margin in (32, 64):
            result = compose(original, generated, 432, margin, feather=0)
            self.assertEqual(result.size, (1920, 1080))
            expected = original.resize((810, 1080), Image.Resampling.LANCZOS)
            self.assertEqual(result.crop((555, 0, 1365, 1080)).tobytes(), expected.tobytes())

    def test_default_feather_preserves_source_core(self):
        source = Image.new("RGB", (432, 576), (12, 91, 143))
        generated = Image.new("RGB", (560, 576), (245, 10, 10))
        result = compose(source, generated, 432, 64)
        expected = source.resize((810, 1080), Image.Resampling.LANCZOS)
        self.assertEqual(result.crop((619, 0, 1301, 1080)).tobytes(),
                         expected.crop((64, 0, 746, 1080)).tobytes())

    def test_rejects_margin_outside_generated_region(self):
        with self.assertRaises(ValueError):
            compose(Image.new("RGB", (432, 576)), Image.new("RGB", (560, 576)), 432, 65)


if __name__ == "__main__":
    unittest.main()
