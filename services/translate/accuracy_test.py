#!/usr/bin/env python3
"""Synthetic English-to-Chinese checks for the local Opus-MT sidecar.

Run after the model directory exists:

    TELLOMI_TRANSLATE_MODEL=/opt/translate/model python3 accuracy_test.py

The test refuses to pass when the model is missing. It does not call a
network translator.
"""

import os
import sys
import time

import server


# Expected strings were checked against Helsinki-NLP/opus-mt-en-zh itself
# (Marian beam 4), not against a third-party translator.
CASES = (
    ("Thank you.", ("谢谢",)),
    ("Good morning.", ("早上",)),
    ("I like this book.", ("我喜欢这本书",)),
    ("hello", ("你好",)),
)


def main():
    model = os.environ.get("TELLOMI_TRANSLATE_MODEL", "").strip()
    if not model:
        print("TELLOMI_TRANSLATE_MODEL is unset", file=sys.stderr)
        return 2
    if not server.load(model):
        print("model did not load:", server.LOAD_ERROR, file=sys.stderr)
        return 2
    # Warmup is inside load(). Time a second pass.
    samples = []
    for text, expected in CASES:
        started = time.perf_counter()
        got = server.ENGINE.translate([text])[0]
        elapsed = time.perf_counter() - started
        samples.append(elapsed)
        if not any(piece in got for piece in expected):
            print(f"FAIL {text!r} -> {got!r} want one of {expected}", file=sys.stderr)
            return 1
        print(f"ok {text!r} -> {got!r} {elapsed * 1000:.0f}ms")
    samples.sort()
    print(f"latency_ms min={samples[0]*1000:.0f} median={samples[len(samples)//2]*1000:.0f} max={samples[-1]*1000:.0f}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
