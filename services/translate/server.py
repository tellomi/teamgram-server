#!/usr/bin/env python3
"""Self-hosted English to simplified Chinese translation.

The process loads a local CTranslate2 conversion of Helsinki-NLP/opus-mt-en-zh.
GET /health is 200 only after that model has translated a warmup sentence.
POST /v1/translate {"texts":[...],"from":"en","to":"zh"} -> {"texts":[...]}.

This process does not call Bailian, Google, Apple, or Telegram. It does not
download weights at runtime. The image build converts the model; see Dockerfile
and NOTICE.
"""

import json
import os
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

MODEL_DIR = os.environ.get("TELLOMI_TRANSLATE_MODEL", "/opt/translate/model").strip()
THREADS = int(os.environ.get("TELLOMI_TRANSLATE_THREADS", "2"))
MAX_TEXTS = 32
MAX_CHARS = 2000
ZH = {"zh", "zh-cn", "zh-hans"}

# Filled by load(). None means the process is up but unhealthy.
ENGINE = None
LOAD_ERROR = "model_missing"


class Engine:
    def __init__(self, model_dir: str, threads: int):
        import ctranslate2
        import sentencepiece as spm

        source = os.path.join(model_dir, "source.spm")
        target = os.path.join(model_dir, "target.spm")
        if not os.path.isfile(os.path.join(model_dir, "model.bin")):
            raise FileNotFoundError("model.bin")
        if not os.path.isfile(source) or not os.path.isfile(target):
            raise FileNotFoundError("sentencepiece")
        self.translator = ctranslate2.Translator(
            model_dir,
            device="cpu",
            inter_threads=1,
            intra_threads=max(1, threads),
        )
        self.src = spm.SentencePieceProcessor(model_file=source)
        self.tgt = spm.SentencePieceProcessor(model_file=target)
        self.lock = threading.Lock()
        # A failed warmup leaves the process unhealthy. "Thank you." is a
        # sentence the upstream model finishes; the bare word "hello" does not.
        warmed = self.translate(["Thank you."])
        if not warmed or not warmed[0].strip():
            raise RuntimeError("warmup empty")

    def translate(self, texts):
        batch = []
        for text in texts:
            pieces = self.src.encode(text, out_type=str)
            pieces.append("</s>")
            batch.append(pieces)
        with self.lock:
            # beam 4 is the Marian config this checkpoint was published with.
            results = self.translator.translate_batch(
                batch,
                beam_size=4,
                max_decoding_length=128,
            )
        out = []
        for result in results:
            hyp = result.hypotheses[0] if result.hypotheses else []
            out.append(self.tgt.decode(stabilize(hyp)).strip())
        return out


def stabilize(tokens):
    """Keep one copy when the whole hypothesis is a single repeated token.

    Helsinki-NLP opus-mt-en-zh does this on the bare word "hello" in both
    PyTorch and CTranslate2. A normal sentence is left unchanged.
    """
    if len(tokens) >= 4 and len(set(tokens)) == 1:
        return tokens[:1]
    return tokens


def load(model_dir: str = MODEL_DIR, threads: int = THREADS):
    global ENGINE, LOAD_ERROR
    if not model_dir or not os.path.isdir(model_dir):
        ENGINE = None
        LOAD_ERROR = "model_missing"
        return False
    try:
        ENGINE = Engine(model_dir, threads)
        LOAD_ERROR = ""
        return True
    except Exception as err:
        ENGINE = None
        LOAD_ERROR = "runtime_not_loaded"
        print("translate load failed:", err, file=sys.stderr)
        return False


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def do_GET(self):
        if self.path.split("?", 1)[0] != "/health":
            self._send(404, {"error": "not_found"})
            return
        if ENGINE is None:
            self._send(503, {"ok": False, "reason": LOAD_ERROR or "model_missing"})
            return
        self._send(200, {
            "ok": True,
            "model": "Helsinki-NLP/opus-mt-en-zh",
            "quantization": "float32",
            "license": "CC-BY-4.0",
        })

    def do_POST(self):
        if self.path.split("?", 1)[0] != "/v1/translate":
            self._send(404, {"error": "not_found"})
            return
        if ENGINE is None:
            self._send(503, {"error": LOAD_ERROR or "model_missing"})
            return
        try:
            length = int(self.headers.get("Content-Length", "0"))
        except ValueError:
            self._send(400, {"error": "bad_length"})
            return
        if length <= 0 or length > 1 << 20:
            self._send(400, {"error": "bad_length"})
            return
        raw = self.rfile.read(length)
        try:
            body = json.loads(raw.decode("utf-8"))
        except (UnicodeError, json.JSONDecodeError):
            self._send(400, {"error": "bad_json"})
            return
        texts = body.get("texts")
        src = str(body.get("from", "")).strip().lower()
        dst = str(body.get("to", "")).strip().lower()
        if src != "en" or dst not in ZH:
            self._send(400, {"error": "lang"})
            return
        if not isinstance(texts, list) or not texts or len(texts) > MAX_TEXTS:
            self._send(400, {"error": "texts"})
            return
        clean = []
        for item in texts:
            if not isinstance(item, str) or len(item) > MAX_CHARS:
                self._send(400, {"error": "texts"})
                return
            clean.append(item)
        try:
            started = time.perf_counter()
            out = ENGINE.translate(clean)
            elapsed_ms = int((time.perf_counter() - started) * 1000)
        except Exception as err:
            print("translate failed:", err, file=sys.stderr)
            self._send(503, {"error": "runtime_not_loaded"})
            return
        if len(out) != len(clean):
            self._send(503, {"error": "runtime_not_loaded"})
            return
        self._send(200, {"texts": out, "ms": elapsed_ms})

    def _send(self, code, body):
        raw = json.dumps(body, ensure_ascii=False).encode("utf-8")
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def log_message(self, fmt, *args):
        return


def main():
    ok = load()
    if not ok:
        print("translate unhealthy:", LOAD_ERROR, file=sys.stderr)
    port = int(os.environ.get("PORT", "8091"))
    ThreadingHTTPServer(("0.0.0.0", port), Handler).serve_forever()


if __name__ == "__main__":
    main()
