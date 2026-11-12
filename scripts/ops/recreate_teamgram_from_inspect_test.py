#!/usr/bin/env python3
"""Fixture check: recreate keeps ports, networks, and non-DC env."""

import gzip
import json
import os
import struct
import sys
import tempfile
import unittest
import zlib

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


class FrameTest(unittest.TestCase):
    def setUp(self):
        import importlib.util
        path = os.path.join(os.path.dirname(__file__), "help-getconfig-dc.py")
        spec = importlib.util.spec_from_file_location("help_getconfig_dc", path)
        self.hg = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.hg)

    def _option(self, dc_id, ip, port):
        hg = self.hg
        return (
            hg.DC_OPTION.to_bytes(4, "little")
            + (1 << 5).to_bytes(4, "little")
            + dc_id.to_bytes(4, "little", signed=True)
            + hg.tl_bytes(ip.encode())
            + port.to_bytes(4, "little", signed=True)
        )

    def _config(self):
        body = (0xCC1A241E).to_bytes(4, "little") + b"\x11" * 8
        body += self._option(1, "8.137.198.240", 10443)
        body += self._option(2, "8.137.198.240", 443)
        return body

    def _outer(self, obj):
        raw = struct.pack("<qqqi", 1, 2, 3, 4) + struct.pack("<i", len(obj)) + obj
        pad = len(raw) % 16
        if pad:
            pad = 16 - pad
        if pad < 12:
            pad += 16
        return raw + b"\x00" * pad

    def _rpc(self, req_id, result):
        return self.hg.RPC_RESULT.to_bytes(4, "little") + struct.pack("<q", req_id) + result

    def _message(self, body):
        return struct.pack("<qii", 9, 1, len(body)) + body

    def test_container_then_result_and_msg_key(self):
        hg = self.hg
        req_id = 42
        new_session = (0x9EC20908).to_bytes(4, "little") + b"\x22" * 24
        container = (
            hg.MSG_CONTAINER.to_bytes(4, "little")
            + struct.pack("<i", 2)
            + self._message(new_session)
            + self._message(self._rpc(req_id, self._config()))
        )
        plain = self._outer(container)
        auth = os.urandom(256)
        auth_id = int.from_bytes(hg.sha1(auth)[12:], "little", signed=True)
        packet = hg.encrypt_server_frame(plain, auth, auth_id)
        opts = hg.dc_options_from_packets([packet], auth, req_id)
        self.assertEqual([(o["id"], o["ip"], o["port"]) for o in opts], [
            (1, "8.137.198.240", 10443),
            (2, "8.137.198.240", 443),
        ])
        bad = bytearray(packet)
        bad[8] ^= 0xFF
        with self.assertRaises(ValueError):
            hg.decrypt_server_frame(bytes(bad), auth)

    def test_gzip_result_after_unrelated_frame(self):
        hg = self.hg
        req_id = 7
        packed = gzip.compress(self._config())
        self.assertTrue(packed.startswith(b"\x1f\x8b"))
        gzip_obj = hg.GZIP_PACKED.to_bytes(4, "little") + hg.tl_bytes(packed)
        first = self._outer((0x9EC20908).to_bytes(4, "little") + b"\x33" * 24)
        second = self._outer(self._rpc(req_id, gzip_obj))
        auth = os.urandom(256)
        auth_id = int.from_bytes(hg.sha1(auth)[12:], "little", signed=True)
        packets = [
            hg.encrypt_server_frame(first, auth, auth_id),
            hg.encrypt_server_frame(second, auth, auth_id),
        ]
        opts = hg.dc_options_from_packets(packets, auth, req_id)
        self.assertEqual([o["port"] for o in opts], [10443, 443])

    def test_zlib_payload_is_a_readable_error(self):
        hg = self.hg
        req_id = 9
        packed = zlib.compress(self._config())
        self.assertFalse(packed.startswith(b"\x1f\x8b"))
        zlib_obj = hg.GZIP_PACKED.to_bytes(4, "little") + hg.tl_bytes(packed)
        with self.assertRaises(ValueError) as caught:
            hg.find_rpc_results(self._rpc(req_id, zlib_obj), req_id)
        message = str(caught.exception)
        self.assertNotIn("Traceback", message)
        self.assertNotIn("incorrect header", message)
        self.assertIn("1f8b", message)

    def test_reads_dc_option_blob(self):
        hg = self.hg
        blob = b"\x00" * 8 + self._option(2, "8.137.198.240", 443)
        found = hg.parse_dc_options(blob)
        self.assertEqual(found, [{"id": 2, "ip": "8.137.198.240", "port": 443, "ipv6": False}])


if __name__ == "__main__":
    unittest.main()
