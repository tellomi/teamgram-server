#!/usr/bin/env python3
"""Translate iOS/Android strings the official export left in English.

Login email templates, PUSH_* notification text, and App Store preview
strings are real UI copy. When the export copies English, merge_lang drops
the key and the client shows the English base. This writes a reviewed
translation for those keys and puts it back into the pack at a new version
so an existing client fetches it.

Reviewed files live in data/langpack/reviewed and langpack_official.py
reapplies them when a later export is still the English sentence.
"""

from __future__ import annotations

import gzip
import json
import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from langpack_fill import (  # noqa: E402
    FORMS,
    GOOGLE,
    Translator,
    convert_forms,
    entry_forms,
    fallback_forms,
    forms_cdata_ok,
    forms_ok,
    google_forms,
    source_key_fillable,
    to_string,
)
from langpack_official import LANGS, PLATFORMS, brand_machine_item  # noqa: E402

# Login.PRIVACY_URL and other bare links are not sentences.
LEFTOVER_KEY = re.compile(r"^(?:PUSH_|Login\.|Appstore|AppUpgrade)")
NATIVE = {
    "zh-hans": re.compile(r"[\u4e00-\u9fff]"),
    "zh-hant": re.compile(r"[\u4e00-\u9fff]"),
    "ja": re.compile(r"[\u3040-\u30ff\u4e00-\u9fff]"),
    "ko": re.compile(r"[\uac00-\ud7af]"),
    "ru": re.compile(r"[\u0400-\u04ff]"),
    "uk": re.compile(r"[\u0400-\u04ff]"),
    "be": re.compile(r"[\u0400-\u04ff]"),
    "bg": re.compile(r"[\u0400-\u04ff]"),
    "ar": re.compile(r"[\u0600-\u06ff]"),
    "fa": re.compile(r"[\u0600-\u06ff]"),
    "he": re.compile(r"[\u0590-\u05ff]"),
    "el": re.compile(r"[\u0370-\u03ff]"),
    "th": re.compile(r"[\u0e00-\u0e7f]"),
    "hi": re.compile(r"[\u0900-\u097f]"),
}


def load_doc(path: Path) -> dict | None:
    if not path.exists():
        return None
    with gzip.open(path, "rt", encoding="utf-8") as fh:
        return json.load(fh)


def write_gz(path: Path, doc: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    raw = json.dumps(doc, ensure_ascii=False, separators=(",", ":")).encode()
    with gzip.open(path, "wb", compresslevel=9) as fh:
        fh.write(raw)


def english_prose(text: str) -> bool:
    letters = re.findall(r"[A-Za-z]", text or "")
    return len(letters) >= 8


def still_english(code: str, text: str, english: str) -> bool:
    if not text or not english_prose(text):
        return False
    if text != english and code not in NATIVE:
        # A latin-script translation can share words with English. Only an
        # exact copy is an untranslated leftover.
        return False
    native = NATIVE.get(code)
    if native is None:
        return text == english
    return text == english or len(native.findall(text)) * 2 < len(re.findall(r"[A-Za-z]", text))


def leftover_forms(code: str, src: dict[str, str], have: dict[str, str] | None) -> dict[str, str]:
    """Forms that are missing or still the English sentence."""
    needed = {}
    for name, text in src.items():
        if not english_prose(text):
            continue
        current = None if have is None else have.get(name)
        if current is None or still_english(code, current, text):
            needed[name] = text
    return needed


def main() -> int:
    here = Path(__file__).resolve().parent
    root = here.parent / "data" / "langpack"
    try:
        import opencc
    except ImportError:
        print("opencc is required", file=sys.stderr)
        return 1
    s2t = opencc.OpenCC("s2twp")
    translator = Translator(Path("/tmp/tellomi-translate-cache.jsonl"), workers=6)
    codes = [meta[0] for meta in LANGS if meta[0] != "en"]

    pending: list[tuple[str, str, str, dict[str, str]]] = []
    for platform in PLATFORMS:
        en_doc = load_doc(root / platform / "en.json.gz")
        if not en_doc:
            continue
        en = {item["key"]: item for item in en_doc["strings"]}
        packs = {code: load_doc(root / platform / f"{code}.json.gz") for code in codes}
        by = {
            code: {item["key"]: item for item in (doc or {}).get("strings") or []}
            for code, doc in packs.items()
        }
        for key, item in en.items():
            if not LEFTOVER_KEY.match(key):
                continue
            src = entry_forms(item)
            if not source_key_fillable(key, src):
                continue
            for code in codes:
                have = entry_forms(by[code][key]) if key in by[code] else None
                needed = leftover_forms(code, src, have)
                if not needed:
                    continue
                pending.append((platform, code, key, needed))

    pieces: list[tuple[str, str]] = []
    for platform, code, key, needed in pending:
        tl = "zh-CN" if code in ("zh-hans", "zh-hant") else GOOGLE[code]
        for text in needed.values():
            from langpack_fill import pieces_of

            for piece in pieces_of(text):
                pieces.append((tl, piece))
    translator.translate_all(pieces)

    # group by platform/code
    grouped: dict[tuple[str, str], list[tuple[str, dict[str, str]]]] = {}
    for platform, code, key, needed in pending:
        grouped.setdefault((platform, code), []).append((key, needed))

    hans_done: dict[tuple[str, str], dict[str, str]] = {}
    for (platform, code), rows in grouped.items():
        if code != "zh-hans":
            continue
        for key, needed in rows:
            forms = google_forms(needed, "zh-CN", translator)
            forms = fallback_forms(needed, forms)
            hans_done[(platform, key)] = forms

    changed_packs = 0
    for (platform, code), rows in sorted(grouped.items()):
        doc = load_doc(root / platform / f"{code}.json.gz")
        if not doc:
            continue
        by = {item["key"]: item for item in doc["strings"]}
        en_doc = load_doc(root / platform / "en.json.gz")
        en = {item["key"]: item for item in en_doc["strings"]}
        reviewed = []
        version = int(doc.get("version") or 1)
        # Android phones below android_xml_fix_version receive every string.
        # Keep the new text at the pack version so that full replace includes
        # it and a client already at the cutover is not asked twice.
        string_version = version
        if platform != "android":
            string_version = version + 1
            doc["version"] = string_version
        tl = "zh-CN" if code in ("zh-hans", "zh-hant") else GOOGLE[code]
        via = "google"
        for key, needed in rows:
            src = entry_forms(en[key])
            if code == "zh-hant" and (platform, key) in hans_done:
                forms = convert_forms(hans_done[(platform, key)], s2t)
                # hans_done only has the forms zh-hans needed. Fill any extra.
                missing = {name: text for name, text in needed.items() if name not in forms}
                if missing:
                    extra = google_forms(missing, "zh-CN", translator)
                    forms.update(convert_forms(fallback_forms(missing, extra), s2t))
                via = "google+opencc"
            elif code == "zh-hans":
                forms = hans_done.get((platform, key)) or google_forms(needed, "zh-CN", translator)
                forms = fallback_forms(needed, forms)
            else:
                forms = google_forms(needed, tl, translator)
                forms = fallback_forms(needed, forms)
                via = "copy" if forms == needed else "google"
            current = by.get(key)
            if current:
                merged = entry_forms(current)
                merged.update(forms)
                forms = merged
            else:
                # Keep English forms the client plural lookup still needs.
                full = dict(src)
                full.update(forms)
                forms = full
            if not forms_ok(src if set(src) == set(forms) else forms, forms) and set(src) == set(forms):
                if not forms_ok(src, forms):
                    print(f"skip {platform}/{code} {key}: placeholders")
                    continue
            if set(src) == set(forms) and not forms_cdata_ok(src, forms):
                print(f"skip {platform}/{code} {key}: cdata")
                continue
            item = to_string(key, forms, string_version, via)
            item = brand_machine_item(item)
            by[key] = item
            reviewed.append(item)
        if not reviewed:
            continue
        # Preserve order, then append keys that were missing.
        seen = set()
        strings = []
        for item in doc["strings"]:
            strings.append(by[item["key"]])
            seen.add(item["key"])
        for item in reviewed:
            if item["key"] not in seen:
                strings.append(item)
        doc["strings"] = strings
        write_gz(root / platform / f"{code}.json.gz", doc)
        reviewed_doc = {
            "lang_code": code,
            "note": "Translations for strings the official export left in English (login email, PUSH_*, App Store preview).",
            "strings": reviewed,
        }
        write_gz(root / "reviewed" / platform / f"{code}.json.gz", reviewed_doc)
        changed_packs += 1
        print(f"reviewed {platform}/{code}: {len(reviewed)} keys v{doc['version']}")
    print(f"packs {changed_packs}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
