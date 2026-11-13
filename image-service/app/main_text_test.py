"""The generate endpoint redraws a picture whose lettering reads back wrong.

Needs the service's environment (FastAPI, mflux importable); the generator and
the OCR are stubbed, so nothing touches the GPU.
"""

from __future__ import annotations

import base64
import unittest
from unittest import mock

from fastapi.testclient import TestClient

from app import main
from app.generator import GenerationResult


def _picture(seed: int) -> GenerationResult:
    return GenerationResult(image_base64=base64.b64encode(b"png").decode(), model="ernie-image-turbo",
                            seed=seed, width=1024, height=688, steps=8, duration_ms=1)


class GenerateLetteringTests(unittest.TestCase):
    def setUp(self) -> None:
        self.client = TestClient(main.app)
        self.seeds: list[int] = []

        async def draw(req, model_name, width, height, steps, seed):
            self.seeds.append(seed)
            return _picture(seed)

        patcher = mock.patch.object(main, "_draw", draw)
        patcher.start()
        self.addCleanup(patcher.stop)
        auth = mock.patch.dict(main.app.dependency_overrides, {main.require_auth: lambda: "test"})
        auth.start()
        self.addCleanup(auth.stop)

    def post(self, readings: list[str], deadline: float = 540, **body):
        with mock.patch.object(main.textcheck, "read_text", side_effect=readings), \
                mock.patch.object(main.config, "TEXT_ATTEMPTS", 2), \
                mock.patch.object(main.config, "TEXT_DEADLINE", deadline):
            r = self.client.post("/api/generate", json={"prompt": "a figure", "seed": 7, **body})
        self.assertEqual(r.status_code, 200, r.text)
        return r.json()

    def test_wrong_lettering_is_redrawn_with_a_new_seed(self) -> None:
        got = self.post(["auth\nordng service\n", "auth\norders service\n"],
                        expect_text=["auth", "orders service"])
        self.assertEqual((got["text_ok"], got["attempts"], got["text_problems"]), (True, 2, []))
        self.assertEqual(len(self.seeds), 2)
        self.assertNotEqual(self.seeds[0], self.seeds[1])

    def test_still_wrong_after_every_try_says_so(self) -> None:
        got = self.post(["AUTH\nAUTH\n", "AUTH\nAUTH\n"], expect_text=["auth"])
        self.assertEqual((got["text_ok"], got["attempts"]), (False, 2))
        self.assertEqual(got["text_problems"], ["repeated AUTH"])

    def test_no_redraw_that_would_end_past_the_deadline(self) -> None:
        got = self.post(["AUTH\nAUTH\n"], deadline=-1, expect_text=["auth"])
        self.assertEqual((got["text_ok"], got["attempts"], len(self.seeds)), (False, 1, 1))

    def test_words_in_a_wordless_picture_are_redrawn(self) -> None:
        got = self.post(["KUBERNETES\n", "\n"], no_text=True)
        self.assertEqual((got["text_ok"], got["attempts"], got["text_problems"]), (True, 2, []))
        got = self.post(["INGRESS\n", "PODS\n"], no_text=True)
        self.assertEqual((got["text_ok"], got["text_problems"]), (False, ["extra PODS"]))

    def test_nothing_expected_is_drawn_once_and_not_read(self) -> None:
        got = self.post([])
        self.assertEqual((got["text_ok"], got["attempts"], self.seeds), (None, 1, [7]))


if __name__ == "__main__":
    unittest.main()
