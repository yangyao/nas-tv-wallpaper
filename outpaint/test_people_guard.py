import unittest
from people_guard import outside_subject, check_source
from unittest.mock import patch
import tempfile
from pathlib import Path
from types import SimpleNamespace
import json
from PIL import Image


class PeopleGuardTests(unittest.TestCase):
    def test_existing_central_subject_is_preserved(self):
        self.assertFalse(outside_subject(dict(kind="face", confidence=0.8,
                                             box=[0.4, 0.3, 0.1, 0.2]), 0.29, 0.71))

    def test_new_and_substantially_completed_edge_faces_are_rejected(self):
        for x in (0.0, 0.22, 0.76):
            self.assertTrue(outside_subject(dict(kind="face", confidence=0.8,
                                                box=[x, 0.3, 0.1, 0.2]), 0.29, 0.71))

    def test_margin_body_without_visible_face_is_allowed(self):
        self.assertFalse(outside_subject(dict(kind="person", confidence=0.7,
                                            box=[0.8, 0.2, 0.15, 0.7]), 0.29, 0.71))

    def test_distant_face_is_allowed_but_large_margin_face_is_blocked(self):
        self.assertFalse(outside_subject(dict(kind="face", confidence=0.9,
                                             box=[0.1, 0.3, 0.02, 0.04]), 0.29, 0.71))
        self.assertTrue(outside_subject(dict(kind="face", confidence=0.9,
                                            box=[0.1, 0.3, 0.08, 0.14]), 0.29, 0.71))

    def test_malformed_detection_fails_closed(self):
        with self.assertRaises(ValueError):
            outside_subject(dict(kind="face", confidence=0.9,
                                 box=[float("nan"), 0, 0.1, 0.1]), 0.29, 0.71)


class SourcePresenceTests(unittest.TestCase):
    def test_original_back_view_is_allowed_and_empty_landscape_is_not(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "source.jpg"
            Image.new("RGB", (300, 400)).save(path)
            for detections, expected in [([], False),
                ([dict(kind="person", confidence=0.7, box=[0.2, 0.2, 0.5, 0.7])], True)]:
                with patch("people_guard.subprocess.run", return_value=SimpleNamespace(
                    stdout=json.dumps(detections).encode())):
                    self.assertEqual(check_source(path, "helper")["passed"], expected)
