"""Unit tests for the video pipeline's pure parts and its job queue.

No model, GPU or ffmpeg is touched. Run with: python -m unittest app.video_test
"""

from __future__ import annotations

import json
import tempfile
import time
import unittest
from pathlib import Path

from app import config, video


def request(job_id: str = "", shots: int = 2) -> dict:
    return {"id": job_id, "shots": [{"narration": f"line {i}", "visual": f"scene {i}"} for i in range(shots)]}


class PureHelperTests(unittest.TestCase):
    def test_clip_frames_sit_on_the_8n_plus_1_grid_within_bounds(self) -> None:
        for seconds in (0.1, 1.0, 2.3, 4.0, 5.0, 30.0):
            frames = video.clip_frames(seconds)
            self.assertEqual((frames - 1) % 8, 0, seconds)
            self.assertGreaterEqual(frames, 25)
            self.assertLessEqual(frames, config.VIDEO_MAX_CLIP_FRAMES)
        self.assertEqual(video.clip_frames(4.0), 97)

    def test_short_narration_still_gets_a_watchable_shot(self) -> None:
        self.assertEqual(video.shot_seconds(0.4), video.MIN_SHOT)
        self.assertAlmostEqual(video.shot_seconds(6.0), 6.0 + video.AUDIO_LEAD + video.SHOT_TAIL)

    def test_captions_cover_the_narration_exactly_once(self) -> None:
        text = "For two hundred years, this lighthouse has guided sailors home through storms that swallowed ships."
        cues = video.caption_cues(text, 0.15, 6.0)
        self.assertEqual(" ".join(c[2] for c in cues), text)
        self.assertAlmostEqual(cues[0][0], 0.15)
        self.assertAlmostEqual(cues[-1][1], 6.15)
        for (_, end, _), (start, _, _) in zip(cues, cues[1:]):
            self.assertAlmostEqual(end, start)
        self.assertTrue(all(len(c[2].split()) <= 7 for c in cues))
        # Breaks at the comma once the line is long enough.
        self.assertEqual(cues[0][2], "For two hundred years,")

    def test_srt_timestamps(self) -> None:
        out = video.srt([(0.15, 1.5, "Hello"), (3661.0, 3662.25, "there")])
        self.assertIn("1\n00:00:00,150 --> 00:00:01,500\nHello\n", out)
        self.assertIn("2\n01:01:01,000 --> 01:01:02,250\nthere\n", out)

    def test_ids_and_voices_are_validated(self) -> None:
        self.assertTrue(video.valid_id("8a1b2c3d-0000-4000-8000-000000000000"))
        self.assertFalse(video.valid_id("../../etc"))
        self.assertFalse(video.valid_id("short"))
        self.assertTrue(video.valid_voice("bf_emma"))
        self.assertFalse(video.valid_voice("--voice"))


class QueueTests(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)

    def tearDown(self) -> None:
        self.tmp.cleanup()

    def test_a_repeated_id_returns_the_existing_job(self) -> None:
        r = video.VideoRenderer(self.root)
        first, created = r.submit(request("job-00000001"))
        again, created_again = r.submit(request("job-00000001"))
        self.assertTrue(created)
        self.assertFalse(created_again)
        self.assertIs(first, again)
        self.assertEqual(json.loads((self.root / "job-00000001" / "job.json").read_text())["status"], video.QUEUED)

    def test_the_queue_refuses_work_beyond_its_limit(self) -> None:
        r = video.VideoRenderer(self.root)
        for i in range(config.VIDEO_MAX_QUEUE + 1):
            r.submit(request(f"job-{i:08d}"))
        with self.assertRaises(video.QueueFull):
            r.submit(request("job-overflow"))

    def test_a_restart_requeues_interrupted_jobs_and_gives_up_on_repeat_offenders(self) -> None:
        r = video.VideoRenderer(self.root)
        retry, _ = r.submit(request("job-retry001"))
        crash, _ = r.submit(request("job-crash001"))
        r._update(retry, status=video.RUNNING, attempts=1)
        r._update(crash, status=video.RUNNING, attempts=config.VIDEO_MAX_ATTEMPTS)

        after = video.VideoRenderer(self.root)
        after._load()
        self.assertEqual(after.get("job-retry001")["status"], video.QUEUED)
        self.assertEqual(after._queue.get_nowait(), "job-retry001")
        self.assertEqual(after.get("job-crash001")["status"], video.FAILED)

    def test_old_finished_jobs_are_pruned_with_their_files(self) -> None:
        r = video.VideoRenderer(self.root)
        old, _ = r.submit(request("job-old00001"))
        r._update(old, status=video.DONE, finished_at=time.time() - (config.VIDEO_KEEP_DAYS + 1) * 86400)
        r.prune()
        self.assertIsNone(r.get("job-old00001"))
        self.assertFalse((self.root / "job-old00001").exists())

    def test_one_collector_at_a_time_until_its_lease_runs_out(self) -> None:
        r = video.VideoRenderer(self.root)
        job, _ = r.submit(request("job-claim001"))
        self.assertIsNone(r.claim("job-claim001", "a", 60), "nothing to collect before it is done")
        r._update(job, status=video.DONE)
        self.assertTrue(r.claim("job-claim001", "a", 60))
        self.assertFalse(r.claim("job-claim001", "b", 60), "a second replica must stand down")
        self.assertTrue(r.claim("job-claim001", "a", 60), "the holder may renew")
        r._update(job, claim_until=time.time() - 1)
        self.assertTrue(r.claim("job-claim001", "b", 60), "an expired lease is up for grabs")

    def test_cancelling_a_queued_job_means_it_never_renders(self) -> None:
        r = video.VideoRenderer(self.root)
        r.submit(request("job-cancel01"))
        self.assertEqual(r.cancel("job-cancel01")["status"], video.CANCELLED)
        self.assertIsNone(r.cancel("job-unknown1"))
        # The worker skips anything no longer queued; the queue limit no longer counts it.
        waiting = [j for j in r.list() if j["status"] in (video.QUEUED, video.RUNNING)]
        self.assertEqual(waiting, [])

    def test_a_running_job_is_flagged_and_stops_at_its_next_step(self) -> None:
        r = video.VideoRenderer(self.root)
        job, _ = r.submit(request("job-cancel02"))
        r._update(job, status=video.RUNNING)
        r.cancel("job-cancel02")
        self.assertTrue(job["cancel_requested"])
        render = video._Render(job, self.root / "job-cancel02", lambda **kw: None)
        with self.assertRaises(video.Cancelled):
            render._progress(0, 1, 0, "narration", "x")

    def test_a_cancel_survives_a_restart(self) -> None:
        r = video.VideoRenderer(self.root)
        job, _ = r.submit(request("job-cancel03"))
        r._update(job, status=video.RUNNING, attempts=1)
        r.cancel("job-cancel03")
        after = video.VideoRenderer(self.root)
        after._load()
        self.assertEqual(after.get("job-cancel03")["status"], video.CANCELLED)
        self.assertTrue(after._queue.empty(), "a cancelled job must not be requeued")

    def test_the_api_view_hides_the_request(self) -> None:
        job, _ = video.VideoRenderer(self.root).submit(request("job-public01"))
        self.assertNotIn("request", video.public(job))
        self.assertEqual(video.public(job)["shots"], 2)


if __name__ == "__main__":
    unittest.main()
