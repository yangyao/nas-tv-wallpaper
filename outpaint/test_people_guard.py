import unittest
from people_guard import outside_subject


class PeopleGuardTests(unittest.TestCase):
    def test_existing_central_subject_is_preserved(self):
        self.assertFalse(outside_subject(dict(kind="face", confidence=0.8,
                                             box=[0.4, 0.3, 0.1, 0.2]), 0.29, 0.71))

    def test_new_and_substantially_completed_edge_faces_are_rejected(self):
        for x in (0.0, 0.22, 0.76):
            self.assertTrue(outside_subject(dict(kind="face", confidence=0.8,
                                                box=[x, 0.3, 0.1, 0.2]), 0.29, 0.71))

    def test_margin_body_without_visible_face_is_rejected(self):
        self.assertTrue(outside_subject(dict(kind="person", confidence=0.7,
                                            box=[0.8, 0.2, 0.15, 0.7]), 0.29, 0.71))

    def test_malformed_detection_fails_closed(self):
        with self.assertRaises(ValueError):
            outside_subject(dict(kind="face", confidence=0.9,
                                 box=[float("nan"), 0, 0.1, 0.1]), 0.29, 0.71)
