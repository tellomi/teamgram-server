#!/usr/bin/env python3
"""Chinese-only policy for unsent recordings. Does not load SenseVoice."""

import server


def test_voice_text_stays_cleaned():
    raw = ["<|en|><|NEUTRAL|><|Speech|><|withitn|>hello"]
    assert server.join_parts(raw) == "hello"
    assert server.detect_language(raw) == "en"
    assert server.reject_required_language("", "hello", "en") is False


def test_draft_requires_zh():
    zh = ["<|zh|><|NEUTRAL|><|Speech|><|withitn|>你好"]
    assert server.detect_language(zh) == "zh"
    assert server.join_parts(zh) == "你好"
    assert server.reject_required_language("zh", "你好", "zh") is False
    assert server.reject_required_language("zh", "hello", "en") is True
    assert server.reject_required_language("zh", "粤语", "yue") is True
    assert server.reject_required_language("zh", "hello", "") is True
    # No words is not a language rejection. The caller returns no-speech.
    assert server.reject_required_language("zh", "", "nospeech") is False
    assert server.reject_required_language("zh", "", "") is False


if __name__ == "__main__":
    test_voice_text_stays_cleaned()
    test_draft_requires_zh()
    print("ok")
