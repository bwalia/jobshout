"""Unit tests for the image-service generator wiring.

These do not load mflux or touch the GPU — they only assert the thread-affinity
contract that prevents Stream(gpu, N) failures.
"""

from __future__ import annotations

import threading
import unittest

from app.generator import Generator


class GeneratorThreadAffinityTests(unittest.TestCase):
    def test_mlx_executor_reuses_one_thread(self) -> None:
        gen = Generator()
        seen: list[int] = []

        def mark() -> int:
            ident = threading.get_ident()
            seen.append(ident)
            return ident

        first = gen._mlx_executor.submit(mark).result(timeout=5)
        second = gen._mlx_executor.submit(mark).result(timeout=5)
        self.assertEqual(first, second)
        self.assertEqual(seen, [first, second])
        gen._mlx_executor.shutdown(wait=False)


if __name__ == "__main__":
    unittest.main()
