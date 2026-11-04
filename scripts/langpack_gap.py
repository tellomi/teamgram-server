#!/usr/bin/env python3
"""Report langpack key-count gap versus English.

Exit 1 when any language is missing more than 1% of the English keys on a
platform. Extra keys (present in the language, absent in English) are listed
and do not fail the check.
"""

from __future__ import annotations

import gzip
import json
import sys
from pathlib import Path

PLATFORMS = ("ios", "android", "tdesktop", "macos")


def load_keys(path: Path) -> set[str]:
    with gzip.open(path, "rt", encoding="utf-8") as fh:
        doc = json.load(fh)
    return {item["key"] for item in doc.get("strings") or []}


def main() -> int:
    root = Path(__file__).resolve().parent.parent / "data" / "langpack"
    failed = False
    print("| platform | code | keys | en | missing | extra | gap |")
    print("|---|---|---:|---:|---:|---:|---:|")
    for platform in PLATFORMS:
        folder = root / platform
        if not folder.is_dir():
            continue
        en_path = folder / "en.json.gz"
        if not en_path.exists():
            continue
        en_keys = load_keys(en_path)
        en_n = len(en_keys)
        rows = []
        for path in sorted(folder.glob("*.json.gz")):
            code = path.name[: -len(".json.gz")]
            if code == "en":
                continue
            keys = load_keys(path)
            missing = len(en_keys - keys)
            extra = len(keys - en_keys)
            gap = 100.0 * missing / en_n if en_n else 0.0
            if missing * 100 > en_n:
                failed = True
            rows.append((gap, code, len(keys), en_n, missing, extra))
        for gap, code, n, en_count, missing, extra in sorted(rows):
            print(f"| {platform} | {code} | {n} | {en_count} | {missing} | {extra} | {gap:.2f}% |")
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
