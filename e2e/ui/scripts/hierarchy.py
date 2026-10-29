"""Read a Maestro or UIAutomator hierarchy and check chat message order."""

from __future__ import annotations

import argparse
import json
import re
import sys
import xml.etree.ElementTree as ET
from pathlib import Path

from accounts import MESSAGE_1, MESSAGE_2, MESSAGE_3, RECEIPT

BOUNDS_PAIR = re.compile(r"\[(\d+),(\d+)\]\[(\d+),(\d+)\]")
ORDER = (MESSAGE_1, MESSAGE_2, MESSAGE_3)
UNREAD_MARKERS = ("Not seen", "未查看", "对方未读")


def load_tree(path: Path) -> object:
    raw = path.read_text(encoding="utf-8", errors="replace")
    stripped = raw.lstrip()
    json_at = raw.find("{")
    list_at = raw.find("[")
    xml_at = raw.find("<")
    if stripped.startswith("<") or (xml_at != -1 and (json_at == -1 or xml_at < json_at) and (list_at == -1 or xml_at < list_at)):
        return ET.fromstring(raw[xml_at:])
    start = json_at if json_at != -1 else list_at
    if start == -1:
        raise ValueError(f"{path} has no JSON or XML hierarchy")
    return json.loads(raw[start:])


def _bounds_top(value: object) -> int | None:
    if isinstance(value, str):
        match = BOUNDS_PAIR.search(value)
        if match:
            return int(match.group(2))
        parts = value.split(",")
        if len(parts) == 4 and all(part.strip().lstrip("-").isdigit() for part in parts):
            return int(parts[1])
        return None
    if isinstance(value, dict):
        for key in ("y", "top"):
            if key in value and isinstance(value[key], (int, float)):
                return int(value[key])
    return None


def _node_text(node: object) -> str:
    if isinstance(node, ET.Element):
        parts = [node.attrib.get("text", ""), node.attrib.get("content-desc", "")]
        return " ".join(part for part in parts if part)
    if not isinstance(node, dict):
        return ""
    attrs = node.get("attributes") if isinstance(node.get("attributes"), dict) else node
    parts: list[str] = []
    for key in ("text", "accessibilityText", "title", "label", "value", "hintText", "content-desc"):
        value = attrs.get(key)
        if isinstance(value, str) and value:
            parts.append(value)
    return " ".join(parts)


def _children(node: object) -> list[object]:
    if isinstance(node, ET.Element):
        return list(node)
    if isinstance(node, dict):
        children = node.get("children")
        if isinstance(children, list):
            return children
    if isinstance(node, list):
        return node
    return []


def iter_nodes(node: object):
    yield node
    for child in _children(node):
        yield from iter_nodes(child)


def text_positions(tree: object) -> list[tuple[str, int]]:
    found: list[tuple[str, int]] = []
    for node in iter_nodes(tree):
        text = _node_text(node)
        if not text:
            continue
        top = None
        if isinstance(node, ET.Element):
            top = _bounds_top(node.attrib.get("bounds", ""))
        elif isinstance(node, dict):
            attrs = node.get("attributes") if isinstance(node.get("attributes"), dict) else node
            top = _bounds_top(attrs.get("bounds"))
            if top is None:
                top = _bounds_top(attrs.get("frame") or attrs.get("rect"))
        if top is None:
            top = 0
        found.append((text, top))
    return found


def top_of(tree: object, needle: str) -> int | None:
    hits = [top for text, top in text_positions(tree) if needle in text]
    if not hits:
        return None
    return min(hits)


def assert_vertical_order(tree: object, messages: tuple[str, ...] = ORDER) -> None:
    positions: list[tuple[str, int]] = []
    missing: list[str] = []
    for message in messages:
        top = top_of(tree, message)
        if top is None:
            missing.append(message)
        else:
            positions.append((message, top))
    if missing:
        raise AssertionError("missing messages: " + ", ".join(missing))
    tops = [top for _, top in positions]
    if tops != sorted(tops) or len(set(tops)) != len(tops):
        detail = ", ".join(f"{name}@{top}" for name, top in positions)
        raise AssertionError("messages are not top-to-bottom send order: " + detail)


def assert_receipt_unread(tree: object, receipt: str = RECEIPT) -> None:
    blob = "\n".join(text for text, _ in text_positions(tree))
    if receipt not in blob:
        raise AssertionError(f"{receipt} is not on screen")
    if any(marker in blob for marker in UNREAD_MARKERS):
        return
    if re.search(r"(?<!Not )Seen", blob) or "已读" in blob or "已查看" in blob:
        raise AssertionError(f"{receipt} already shows a read receipt")


def assert_chat(path: Path) -> None:
    tree = load_tree(path)
    assert_vertical_order(tree)
    assert_receipt_unread(tree)


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description="Assert message order in a UI hierarchy")
    parser.add_argument("command", choices=("assert",))
    parser.add_argument("path", type=Path)
    args = parser.parse_args(argv)
    try:
        assert_chat(args.path)
    except (AssertionError, ValueError, ET.ParseError, json.JSONDecodeError) as exc:
        print(f"hierarchy check failed: {exc}", file=sys.stderr)
        return 1
    print(f"hierarchy ok: {args.path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
