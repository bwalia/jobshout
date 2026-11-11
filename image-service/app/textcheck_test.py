"""The lettering check, against what Vision read in int's pictures on 2026-11-11."""

from __future__ import annotations

import unittest

from app.textcheck import problems


class ProblemsTests(unittest.TestCase):
    def test_clean_flow_figure(self) -> None:
        seen = "Client\nAPI\nGateway\nIngress\nController\nService\nPod\n"
        self.assertEqual(problems(["Client", "API Gateway", "Ingress Controller", "Service Pod"], seen), [])

    def test_cover_that_repeats_its_labels(self) -> None:
        seen = ("API SECURITY\n→\nHTTP\n+\nAUTH\n→\nAUTH\nRATE\nLIMIT\nRATE\nFORWARD\nREQUEST\n→\n"
                "ROUTING FORWARD\nPOLICY\nREQUEST\nPROXY\nPROXY\nTO POD\n")
        got = problems(["API SECURITY", "HTTP", "auth", "rate limit", "routing policy",
                        "forward request", "proxy to pod"], seen)
        self.assertIn("repeated AUTH", got)
        self.assertIn("repeated PROXY", got)

    def test_misspelt_label(self) -> None:
        seen = "auth\nservice\nbilling\nservice\nAPI\nGateway\nordng\nservice\nRedis\ncache\n"
        got = problems(["API Gateway", "auth service", "orders service", "billing service", "Redis cache"], seen)
        self.assertEqual(got, ["missing ORDERS", "extra ORDNG"])

    def test_icon_noise_is_not_a_word(self) -> None:
        self.assertEqual(problems(["Deploy", "Build"], "Deploy\nII\nBuild\n=\nOo\n"), [])


if __name__ == "__main__":
    unittest.main()
