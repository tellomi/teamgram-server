#!/usr/bin/env python3
"""Request-slot tests for the translate sidecar. Does not load the model."""

import json
import threading
import urllib.error
import urllib.request
from http.server import ThreadingHTTPServer

import server


class Hold:
    def __init__(self):
        self.started = threading.Event()
        self.release = threading.Event()

    def translate(self, texts):
        self.started.set()
        if not self.release.wait(3):
            raise RuntimeError("release timed out")
        return ["好"] * len(texts)


def post(base, payload=None):
    if payload is None:
        payload = {"texts": ["hello"], "from": "en", "to": "zh"}
    raw = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(base + "/v1/translate", data=raw, method="POST")
    req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=3) as resp:
            return resp.status, json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as err:
        return err.code, json.loads(err.read().decode("utf-8"))


def serve(engine, admission):
    server.ENGINE = engine
    server.LOAD_ERROR = ""
    server.ADMISSION = admission
    httpd = ThreadingHTTPServer(("127.0.0.1", 0), server.Handler)
    threading.Thread(target=httpd.serve_forever, daemon=True).start()
    return httpd, "http://127.0.0.1:%d" % httpd.server_address[1]


def test_sheds_and_health_stays_up():
    engine = Hold()
    httpd, base = serve(engine, server.Admission(1, 0))
    try:
        with urllib.request.urlopen(base + "/health", timeout=2) as resp:
            body = json.loads(resp.read().decode("utf-8"))
            assert resp.status == 200 and body["ok"] is True
        result = []

        def first():
            result.append(post(base))

        threading.Thread(target=first, daemon=True).start()
        assert engine.started.wait(2)
        code, body = post(base)
        assert code == 503 and body["error"] == "overloaded", (code, body)
        with urllib.request.urlopen(base + "/health", timeout=2) as resp:
            assert resp.status == 200
        engine.release.set()
        for _ in range(50):
            if result:
                break
            threading.Event().wait(0.05)
        assert result and result[0][0] == 200, result
        again, payload = post(base)
        assert again == 200 and payload["texts"] == ["好"], (again, payload)
    finally:
        engine.release.set()
        server.ENGINE = None
        server.ADMISSION = None
        httpd.shutdown()


def test_unhealthy_without_model():
    server.ENGINE = None
    server.LOAD_ERROR = "model_missing"
    server.ADMISSION = server.Admission(1, 0)
    httpd = ThreadingHTTPServer(("127.0.0.1", 0), server.Handler)
    threading.Thread(target=httpd.serve_forever, daemon=True).start()
    base = "http://127.0.0.1:%d" % httpd.server_address[1]
    try:
        try:
            urllib.request.urlopen(base + "/health", timeout=2)
            raise AssertionError("unloaded model returned 200")
        except urllib.error.HTTPError as err:
            assert err.code == 503
            body = json.loads(err.read().decode("utf-8"))
            assert body["reason"] == "model_missing"
    finally:
        server.ADMISSION = None
        httpd.shutdown()


def test_bad_env_keeps_default():
    assert server.positive_int("TELLOMI_TRANSLATE_MAX_INFLIGHT", 1) == 1


if __name__ == "__main__":
    test_sheds_and_health_stays_up()
    test_unhealthy_without_model()
    test_bad_env_keeps_default()
    print("ok")
