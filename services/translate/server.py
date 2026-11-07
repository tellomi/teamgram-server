#!/usr/bin/env python3
"""Self-hosted English to simplified Chinese translation.

GET /health is 200 only when a local model directory is loaded.
POST /v1/translate {"texts":[...],"from":"en","to":"zh"} -> {"texts":[...]}.

This process does not call Bailian, Google, Apple, or Telegram.
Without TELLOMI_TRANSLATE_MODEL the process stays up and reports unhealthy,
so the chat server keeps translations_manual_enabled and translations_auto_enabled
disabled.
"""

import json
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

MODEL = os.environ.get("TELLOMI_TRANSLATE_MODEL", "").strip()


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path != "/health":
            self.send_error(404)
            return
        if not MODEL or not os.path.isdir(MODEL):
            self._json(503, {"ok": False, "reason": "model_missing"})
            return
        self._json(200, {"ok": True})

    def do_POST(self):
        if self.path != "/v1/translate":
            self.send_error(404)
            return
        if not MODEL or not os.path.isdir(MODEL):
            self._json(503, {"error": "model_missing"})
            return
        # Weights are not loaded in this build. A directory of weights is not
        # enough to claim the model runs. Stay unhealthy until a checked
        # runtime is wired to those files.
        self._json(503, {"error": "runtime_not_loaded"})

    def _json(self, code, body):
        raw = json.dumps(body).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def log_message(self, fmt, *args):
        return


if __name__ == "__main__":
    port = int(os.environ.get("PORT", "8091"))
    ThreadingHTTPServer(("0.0.0.0", port), Handler).serve_forever()
