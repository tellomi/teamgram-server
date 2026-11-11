import unittest

import run_probes as r


class TargetGateTests(unittest.TestCase):
    def test_required_targets_are_new_ip_and_api_domain(self):
        required = {(host, port) for _name, host, port, _dc, req in r.TARGETS if req}
        self.assertIn(
            ("api-10443", "api.tellomi.cn", 10443, 1, True),
            r.TARGETS,
        )
        self.assertEqual(
            required,
            {
                ("8.156.40.105", 443),
                ("8.156.40.105", 10443),
                ("api.tellomi.cn", 10443),
                ("api.tellomi.cn", 443),
            },
        )

    def test_old_ip_is_informational_only(self):
        for name, host, port, _dc, required in r.TARGETS:
            if host == "8.137.39.82":
                self.assertFalse(required, f"{name} {host}:{port} must not be required")
            if port == 5222:
                self.assertFalse(required, f"{name} port 5222 must not be required")
                self.assertEqual(host, "8.137.39.82")

    def test_old_ip_still_listed_for_context(self):
        old = {(host, port) for _name, host, port, _dc, _req in r.TARGETS if host == "8.137.39.82"}
        self.assertEqual(old, {("8.137.39.82", 10443), ("8.137.39.82", 5222), ("8.137.39.82", 443)})


if __name__ == "__main__":
    unittest.main()
