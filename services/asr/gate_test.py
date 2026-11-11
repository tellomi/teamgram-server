#!/usr/bin/env python3
"""Request-slot tests for the ASR sidecar. Does not load SenseVoice."""

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

    def transcribe(self, audio):
        self.started.set()
        if not self.release.wait(3):
            raise RuntimeError("release timed out")
        return "hi", "zh"


def post(base, body=b"abc"):
    req = urllib.request.Request(base + "/v1/transcribe", data=body, method="POST")
    try:
        with urllib.request.urlopen(req, timeout=3) as resp:
            return resp.status, resp.read()
    except urllib.error.HTTPError as err:
        return err.code, err.read()


def test_limits_and_env():
    assert server.positive_int("ASR_MAX_INFLIGHT_UNSET", 1) == 1
    assert server.DEFAULT_INFLIGHT == 2
    assert server.DEFAULT_QUEUE == 8
    assert server.Admission(1, 0).try_acquire() is not None


def test_sheds_when_full_and_health_stays_up():
    engine = Hold()
    admission = server.Admission(1, 0)
    httpd = ThreadingHTTPServer(("127.0.0.1", 0), server.make_handler(engine, admission))
    threading.Thread(target=httpd.serve_forever, daemon=True).start()
    base = "http://127.0.0.1:%d" % httpd.server_address[1]
    try:
        with urllib.request.urlopen(base + "/healthz", timeout=2) as resp:
            assert resp.status == 200, resp.status
            assert b'"ok":true' in resp.read()
        result = []

        def first():
            result.append(post(base))

        threading.Thread(target=first, daemon=True).start()
        assert engine.started.wait(2)
        code, body = post(base)
        assert code == 503, (code, body)
        assert b"overloaded" in body
        with urllib.request.urlopen(base + "/healthz", timeout=2) as resp:
            assert resp.status == 200
        engine.release.set()
        for _ in range(50):
            if result:
                break
            threading.Event().wait(0.05)
        assert result and result[0][0] == 200, result
        again, payload = post(base)
        # The holder already finished, so a new clip is accepted.
        assert again == 200, (again, payload)
        assert json.loads(payload)["text"] == "hi"
    finally:
        engine.release.set()
        httpd.shutdown()


def test_queue_waits_then_recovers():
    admission = server.Admission(1, 1)
    running = admission.try_acquire()
    assert running is not None
    started = threading.Event()
    release = threading.Event()

    def occupy():
        with running:
            started.set()
            release.wait(3)

    threading.Thread(target=occupy, daemon=True).start()
    assert started.wait(2)
    waiting = admission.try_acquire()
    assert waiting is not None
    assert admission.try_acquire() is None
    release.set()
    with waiting:
        pass
    assert admission.try_acquire() is not None


if __name__ == "__main__":
    test_limits_and_env()
    test_sheds_when_full_and_health_stays_up()
    test_queue_waits_then_recovers()
    print("ok")
