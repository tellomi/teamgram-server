#!/usr/bin/env python3
"""HTTP ASR for Telegram voice notes and video notes.

POST /v1/transcribe with the raw audio bytes (OGG/Opus, WAV, MP4, ...).
Response is {"text": "..."} . SenseVoice ITN and punctuation are on.
Emotion and event tags are removed.

The model is loaded from disk. This process never downloads weights.
"""

import json
import os
import re
import subprocess
import sys
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

TAG = re.compile(r"<\|[^>|]{0,64}\|>")
SAMPLE_RATE = 16000


def clean_text(text: str) -> str:
    if not text:
        return ""
    return TAG.sub("", text).strip()


def join_parts(parts):
    out = []
    for part in parts:
        part = clean_text(part)
        if not part:
            continue
        if (
            out
            and out[-1][-1].isascii()
            and out[-1][-1].isalnum()
            and part[0].isascii()
            and part[0].isalnum()
        ):
            out.append(" ")
        out.append(part)
    return "".join(out)


def decode_pcm(audio: bytes, timeout: int) -> bytes:
    proc = subprocess.run(
        [
            "ffmpeg",
            "-hide_banner",
            "-loglevel",
            "error",
            "-i",
            "pipe:0",
            "-f",
            "s16le",
            "-acodec",
            "pcm_s16le",
            "-ac",
            "1",
            "-ar",
            str(SAMPLE_RATE),
            "pipe:1",
        ],
        input=audio,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        timeout=timeout,
        check=False,
    )
    if proc.returncode != 0:
        err = proc.stderr.decode("utf-8", "replace").strip()
        raise DecodeError(err[:400] or "ffmpeg failed")
    return proc.stdout


class DecodeError(Exception):
    pass


class TooLong(Exception):
    pass


class Engine:
    def __init__(self, recognizer, max_seconds: int, chunk_seconds: int):
        self.recognizer = recognizer
        self.max_seconds = max_seconds
        self.chunk_seconds = chunk_seconds
        self.lock = threading.Lock()

    def transcribe(self, audio: bytes) -> str:
        # One clip at a time. The production container is capped at 1.5GB.
        with self.lock:
            pcm = decode_pcm(audio, timeout=60)
            return self._pcm(pcm)

    def _pcm(self, pcm: bytes) -> str:
        import numpy as np

        if len(pcm) < 2:
            return ""
        if len(pcm) % 2:
            pcm = pcm[:-1]
        samples = np.frombuffer(pcm, dtype=np.int16).astype(np.float32) / 32768.0
        limit = self.max_seconds * SAMPLE_RATE
        if samples.shape[0] > limit:
            raise TooLong()
        step = max(1, self.chunk_seconds) * SAMPLE_RATE
        parts = []
        for start in range(0, samples.shape[0], step):
            chunk = samples[start : start + step]
            if chunk.shape[0] < SAMPLE_RATE // 5:
                continue
            stream = self.recognizer.create_stream()
            stream.accept_waveform(SAMPLE_RATE, chunk)
            self.recognizer.decode_stream(stream)
            parts.append(stream.result.text or "")
        return join_parts(parts)


def load_recognizer():
    import sherpa_onnx

    threads = int(os.environ.get("ASR_THREADS", "2"))
    return sherpa_onnx.OfflineRecognizer.from_sense_voice(
        model=os.environ.get("ASR_MODEL", "/opt/asr/model/model.int8.onnx"),
        tokens=os.environ.get("ASR_TOKENS", "/opt/asr/model/tokens.txt"),
        num_threads=threads,
        use_itn=True,
        language="",
        provider="cpu",
        debug=False,
    )


def make_handler(engine: Engine):
    class Handler(BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"

        def log_message(self, fmt, *args):
            return

        def do_GET(self):
            if self.path.split("?", 1)[0] != "/healthz":
                self._send(404, b'{"error":"not_found"}')
                return
            body = b'{"ok":true,"model":"SenseVoiceSmall","itn":true}'
            self._send(200, body, "application/json")

        def do_POST(self):
            if self.path.split("?", 1)[0] != "/v1/transcribe":
                self._send(404, b'{"error":"not_found"}')
                return
            try:
                length = int(self.headers.get("Content-Length", "0"))
            except ValueError:
                self._send(400, b'{"error":"bad_length"}')
                return
            if length <= 0 or length > 48 * 1024 * 1024:
                self._send(400, b'{"error":"bad_length"}')
                return
            audio = self.rfile.read(length)
            if len(audio) != length:
                self._send(400, b'{"error":"short_body"}')
                return
            try:
                text = engine.transcribe(audio)
            except TooLong:
                self._send(400, b'{"error":"too_long"}')
                return
            except DecodeError as err:
                print("asr decode:", err, file=sys.stderr)
                self._send(422, b'{"error":"decode_failed"}')
                return
            except Exception as err:
                print("asr failed:", err, file=sys.stderr)
                self._send(500, b'{"error":"asr_failed"}')
                return
            self._send(200, json.dumps({"text": text}, ensure_ascii=False).encode("utf-8"))

        def _send(self, code: int, body: bytes, content_type: str = "application/json"):
            self.send_response(code)
            self.send_header("Content-Type", content_type)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

    return Handler


def main():
    engine = Engine(
        load_recognizer(),
        max_seconds=int(os.environ.get("ASR_MAX_SECONDS", "300")),
        chunk_seconds=int(os.environ.get("ASR_CHUNK_SECONDS", "30")),
    )
    port = int(os.environ.get("ASR_PORT", "8080"))
    server = ThreadingHTTPServer(("0.0.0.0", port), make_handler(engine))
    server.serve_forever()


if __name__ == "__main__":
    main()
