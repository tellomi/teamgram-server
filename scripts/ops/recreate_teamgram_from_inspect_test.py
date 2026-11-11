#!/usr/bin/env python3
"""Fixture check: recreate keeps ports, networks, and non-DC env."""

import json
import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(__file__))
import recreate_teamgram_from_inspect as rec


def sample_inspect():
    turn = [
        "TELLOMI_CALL_IP=203.0.113.10",
        "TELLOMI_TURN_HOST=203.0.113.10",
        "TELLOMI_TURN_PORT=3478",
        "TELLOMI_STUN_HOST=203.0.113.10",
        "TELLOMI_STUN_PORT=3478",
        "TELLOMI_TURN_SECRET=fixture-not-a-real-secret",
        "TELLOMI_TURN_SERVERS=203.0.113.10:3478",
    ]
    return [{
        "Config": {
            "Hostname": "teamgram",
            "Domainname": "",
            "User": "",
            "AttachStdin": False,
            "AttachStdout": True,
            "AttachStderr": True,
            "Tty": False,
            "OpenStdin": False,
            "StdinOnce": False,
            "Env": turn + [
                "TELLOMI_DC_OPTIONS=1:8.137.198.240:10443,1:8.137.198.240:443,2:8.137.198.240:10443,2:8.137.198.240:443",
                "TELLOMI_PUBLIC_DC_HOST=",
            ],
            "Cmd": ["/app/teamgram"],
            "Image": "example.invalid/teamgram:fixture",
            "Volumes": None,
            "WorkingDir": "/app",
            "Entrypoint": None,
            "Labels": {"kept": "yes"},
            "ExposedPorts": {"10443/tcp": {}, "10445/tcp": {}},
        },
        "HostConfig": {
            "Binds": ["/data/turn-relay.env:/etc/tellomi/turn-relay.env:ro"],
            "ContainerIDFile": "",
            "PortBindings": {
                "10443/tcp": [{"HostIp": "0.0.0.0", "HostPort": "10443"}],
                "10445/tcp": [{"HostIp": "10.20.0.218", "HostPort": "10445"}],
            },
            "NetworkMode": "tellomi",
            "RestartPolicy": {"Name": "always", "MaximumRetryCount": 0},
        },
        "NetworkSettings": {
            "Networks": {
                "tellomi": {
                    "IPAddress": "172.19.0.8",
                    "GlobalIPv6Address": "",
                    "MacAddress": "02:42:ac:13:00:08",
                    "Links": None,
                    "Aliases": ["teamgram"],
                    "DriverOpts": None,
                }
            }
        },
    }]


class RecreateTest(unittest.TestCase):
    def test_keeps_turn_env_binds_and_private_port(self):
        body = rec.build_body(sample_inspect(), {
            "TELLOMI_PUBLIC_DC_HOST": "8.137.198.240",
            "TELLOMI_DC_OPTIONS": "1:8.137.198.240:10443,1:8.137.198.240:443,2:8.137.198.240:10443,2:8.137.198.240:443",
            "TELLOMI_DC_TXT_DOMAIN": "-",
        })
        env = body["Env"]
        self.assertEqual(sum(1 for item in env if item.startswith("TELLOMI_TURN") or item.startswith("TELLOMI_CALL") or item.startswith("TELLOMI_STUN")), 7)
        self.assertIn("TELLOMI_TURN_SECRET=fixture-not-a-real-secret", env)
        self.assertIn("TELLOMI_PUBLIC_DC_HOST=8.137.198.240", env)
        self.assertNotIn("com.docker.compose.project", body["Labels"])
        self.assertEqual(body["Labels"], {"kept": "yes"})
        self.assertEqual(body["HostConfig"]["Binds"], ["/data/turn-relay.env:/etc/tellomi/turn-relay.env:ro"])
        binding = body["HostConfig"]["PortBindings"]["10445/tcp"]
        self.assertEqual(binding, [{"HostIp": "10.20.0.218", "HostPort": "10445"}])
        self.assertEqual(body["NetworkingConfig"]["EndpointsConfig"]["tellomi"]["IPAMConfig"]["IPv4Address"], "172.19.0.8")
        self.assertNotIn("ContainerIDFile", body["HostConfig"])

    def test_refuses_missing_private_port(self):
        doc = sample_inspect()
        del doc[0]["HostConfig"]["PortBindings"]["10445/tcp"]
        with self.assertRaises(SystemExit):
            rec.build_body(doc, {"TELLOMI_PUBLIC_DC_HOST": "8.137.198.240"})

    def test_cli_writes_fixed_shape(self):
        doc = sample_inspect()
        with tempfile.TemporaryDirectory() as tmp:
            src = os.path.join(tmp, "inspect.json")
            dst = os.path.join(tmp, "create.json")
            with open(src, "w", encoding="utf-8") as fh:
                json.dump(doc, fh)
            rec.main([
                "recreate_teamgram_from_inspect.py",
                src,
                dst,
                "8.137.198.240",
                "1:8.137.198.240:10443,1:8.137.198.240:443,2:8.137.198.240:10443,2:8.137.198.240:443",
                "-",
            ])
            with open(dst, encoding="utf-8") as fh:
                body = json.load(fh)
        self.assertIn("10445/tcp", body["HostConfig"]["PortBindings"])


class ParseTest(unittest.TestCase):
    def test_reads_dc_option_blob(self):
        import importlib.util
        path = os.path.join(os.path.dirname(__file__), "help-getconfig-dc.py")
        spec = importlib.util.spec_from_file_location("help_getconfig_dc", path)
        hg = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(hg)

        def tl_string(buf):
            return hg.tl_bytes(buf)

        blob = hg.DC_OPTION.to_bytes(4, "little")
        blob += (1 << 5).to_bytes(4, "little")
        blob += (2).to_bytes(4, "little", signed=True)
        blob += tl_string(b"8.137.198.240")
        blob += (443).to_bytes(4, "little", signed=True)
        found = hg.parse_dc_options(b"\x00" * 8 + blob)
        self.assertEqual(found, [{"id": 2, "ip": "8.137.198.240", "port": 443, "ipv6": False}])


if __name__ == "__main__":
    unittest.main()
