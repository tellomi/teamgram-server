#!/usr/bin/env python3
"""Local stand-in for a Bailian chat-completions response.

This process is not DashScope and does not call a model. The client
regression points TELLOMI_AI_BASE_URL here. Appeal text is untrusted: a
marker selects the JSON action, and the rest of the text cannot name
another case or change the action.
"""

import json
import os
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

KEY = os.environ.get("FAKE_BAILIAN_KEY", "e2e-fake-not-production")
MODEL = "e2e-fake-bailian"


def action_for(text):
    if "e2e-timeout" in text:
        time.sleep(5)
        return "insufficient"
    if "e2e-uphold" in text:
        return "uphold"
    if "e2e-insufficient" in text:
        return "insufficient"
    if "e2e-lift" in text:
        return "lift"
    return "insufficient"


def review_json(text):
    """Account and content review use their own JSON. Appeal text does not."""
    if "enforcement-account-review" in text:
        if "e2e-account-clear" in text:
            body = {"substantiated": False, "confidence": 0.2, "reason": "不属实", "summary": "证据不足"}
        else:
            body = {"substantiated": True, "confidence": 0.95, "reason": "隔离假模型判定属实", "summary": "举报与证据一致"}
        return json.dumps(body, ensure_ascii=False)
    if "enforcement-content-review" in text:
        if "e2e-content-uncertain" in text or "e2e-content-low" in text:
            body = {"action": "uncertain", "confidence": 0.2, "reason": "不确定", "summary": "证据不足"}
        elif "e2e-content-keep" in text:
            body = {"action": "keep", "confidence": 0.95, "reason": "正常", "summary": "普通内容"}
        else:
            body = {"action": "remove", "confidence": 0.95, "reason": "违规", "summary": "正文违规"}
        return json.dumps(body, ensure_ascii=False)
    return ""


def verdict(action):
    return json.dumps(
        {
            "action": action,
            "user_reason": "隔离假模型只处理当前这一条",
            "rule_ref": "隔离条款",
            "evidence_refs": ["ev-e2e"],
            "model": MODEL,
            "policy_version": "e2e-1",
        },
        ensure_ascii=False,
    )


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        if self.path != "/chat/completions":
            self.send_error(404)
            return
        if self.headers.get("Authorization") != "Bearer " + KEY:
            body = b'{"error":"unauthorized"}'
            self.send_response(401)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        length = int(self.headers.get("Content-Length", "0"))
        raw = self.rfile.read(length)
        try:
            payload = json.loads(raw.decode())
        except json.JSONDecodeError:
            payload = {}
        text = json.dumps(payload, ensure_ascii=False)
        content = review_json(text) or verdict(action_for(text))
        body = json.dumps(
            {"choices": [{"message": {"content": content}}], "model": MODEL}
        ).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, fmt, *args):
        return


if __name__ == "__main__":
    port = int(os.environ.get("PORT", "18099"))
    ThreadingHTTPServer(("0.0.0.0", port), Handler).serve_forever()
