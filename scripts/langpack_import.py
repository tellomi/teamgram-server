#!/usr/bin/env python3
"""Import Tellomi client string catalogs into data/langpack.

Reads iOS Localizable.strings (tellomi/Telegram-iOS) and Android strings.xml
(tellomi/Telegram), groups plural keys, replaces the Telegram app name with
Tellomi, and writes en / zh-hans / zh-hant packs.

Chinese strings are original Tellomi translations (see langpack_zh.py), not a
copy of upstream language packs. Traditional text is OpenCC s2twp.
Requires: pip install opencc-python-reimplemented

Keys that still contain an English word the glossary does not cover are
printed and, with --report, written out. Placeholders and the Tellomi name
are preserved.

Examples:
  python3 scripts/langpack_import.py \
    --ios /tmp/ios-en.strings \
    --android /tmp/android-en.xml \
    --android-extra /tmp/android-standalone.xml \
    --out data/langpack
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from collections import defaultdict
from pathlib import Path

from langpack_zh import brand, translate, unknown_summary

IOS_FORMS = (
    ("3_10", "few"),
    ("many", "many"),
    ("any", "other"),
    ("0", "zero"),
    ("1", "one"),
    ("2", "two"),
)
ANDROID_FORMS = (
    ("zero", "zero"),
    ("one", "one"),
    ("two", "two"),
    ("few", "few"),
    ("many", "many"),
    ("other", "other"),
)

META = {
    "en": {
        "name": "English",
        "native_name": "English",
        "plural_code": "en",
        "base_lang_code": "",
    },
    "zh-hans": {
        "name": "Chinese (Simplified)",
        "native_name": "简体中文",
        "plural_code": "zh",
        "base_lang_code": "en",
    },
    "zh-hant": {
        "name": "Chinese (Traditional)",
        "native_name": "繁體中文",
        "plural_code": "zh",
        "base_lang_code": "en",
    },
}

SPECIAL = {
    "zh-hans": {
        "LanguageName": "简体中文",
        "LanguageNameInEnglish": "Chinese (Simplified)",
        "LanguageCode": "zh-hans",
        "ContinueOnThisLanguage": "使用简体中文继续",
        "English": "英语",
        "ChooseYourLanguage": "选择语言",
        "ChooseYourLanguageOther": "其他",
        "ChangeLanguageLater": "你可以稍后在设置中更改语言。",
    },
    "zh-hant": {
        "LanguageName": "繁體中文",
        "LanguageNameInEnglish": "Chinese (Traditional)",
        "LanguageCode": "zh-hant",
        "ContinueOnThisLanguage": "使用繁體中文繼續",
        "English": "英語",
        "ChooseYourLanguage": "選擇語言",
        "ChooseYourLanguageOther": "其他",
        "ChangeLanguageLater": "你可以稍後在設定中更改語言。",
    },
    "en": {
        "LanguageName": "English",
        "LanguageCode": "en",
    },
}


def unescape_ios(s: str) -> str:
    out = []
    i = 0
    while i < len(s):
        if s[i] == "\\" and i + 1 < len(s):
            n = s[i + 1]
            mapped = {"n": "\n", "r": "\r", "t": "\t", '"': '"', "\\": "\\", "'": "'"}
            if n in mapped:
                out.append(mapped[n])
                i += 2
                continue
            if n in ("U", "u") and i + 6 <= len(s) and re.fullmatch(r"[0-9a-fA-F]{4}", s[i + 2 : i + 6]):
                out.append(chr(int(s[i + 2 : i + 6], 16)))
                i += 6
                continue
        out.append(s[i])
        i += 1
    return "".join(out)


def parse_ios(path: Path) -> list[tuple[str, str]]:
    text = path.read_text(encoding="utf-8", errors="replace")
    rows = []
    for key, val in re.findall(r'^"((?:\\.|[^"\\])*)"\s*=\s*"((?:\\.|[^"\\])*)"\s*;', text, re.M):
        rows.append((unescape_ios(key), unescape_ios(val)))
    return rows


def unescape_android(s: str) -> str:
    s = s.replace("\\n", "\n").replace("\\t", "\t").replace("\\'", "'").replace('\\"', '"')
    s = s.replace("\\\\", "\\")
    s = (
        s.replace("&lt;", "<")
        .replace("&gt;", ">")
        .replace("&amp;", "&")
        .replace("&quot;", '"')
        .replace("&apos;", "'")
    )
    if "CDATA" in s:
        m = re.search(r"<!\[CDATA\[(.*?)\]\]>", s, re.S)
        if m:
            s = m.group(1)
    s = re.sub(r"</?a\b[^>]*>", "", s)
    return s


def parse_android(path: Path) -> list[tuple[str, str]]:
    text = path.read_text(encoding="utf-8", errors="replace")
    rows = []
    for name, body in re.findall(r'<string\s+name="([^"]+)"[^>]*>(.*?)</string>', text, re.S):
        rows.append((name, unescape_android(body)))
    return rows


def split_form(key: str, forms: tuple[tuple[str, str], ...]) -> tuple[str, str] | tuple[None, None]:
    for suf, name in sorted(forms, key=lambda item: -len(item[0])):
        tail = "_" + suf
        if key.endswith(tail) and len(key) > len(tail):
            return key[: -len(tail)], name
    return None, None


def group(rows: list[tuple[str, str]], forms) -> list[dict]:
    """Preserve first-seen order. Plural forms collapse onto the base key."""
    order: list[str] = []
    plain: dict[str, str] = {}
    plural: dict[str, dict[str, str]] = defaultdict(dict)
    for key, val in rows:
        base, form = split_form(key, forms)
        if base is not None:
            if base not in plural and base not in plain:
                order.append(base)
            elif base not in plural and base in plain:
                # A real string already used this key. Keep the suffixed key as its own string.
                base = None
        if base is None:
            if key not in plain:
                order.append(key)
            plain[key] = val
            continue
        plural[base][form] = val
    out = []
    seen = set()
    for key in order:
        if key in seen:
            continue
        seen.add(key)
        if key in plural and len(plural[key]) >= 1 and key not in plain:
            forms_present = plural[key]
            if "other" not in forms_present:
                for fallback in ("many", "few", "two", "one", "zero"):
                    if fallback in forms_present:
                        forms_present["other"] = forms_present[fallback]
                        break
            out.append({"key": key, "plural": forms_present})
        elif key in plain:
            out.append({"key": key, "value": plain[key]})
    return out


def apply_brand_entry(entry: dict) -> dict:
    if "value" in entry:
        entry["value"] = brand(entry["value"])
    if "plural" in entry:
        entry["plural"] = {k: brand(v) for k, v in entry["plural"].items()}
    return entry


def translate_entry(entry: dict, lang: str) -> tuple[dict, list[str]]:
    unknown: list[str] = []
    specials = SPECIAL.get(lang, {})
    if entry["key"] in specials or entry["key"].endswith("AppName"):
        text = specials.get(entry["key"], "Tellomi" if entry["key"].endswith("AppName") else None)
        if text is not None and "plural" not in entry:
            return {"key": entry["key"], "value": text}, []
    if lang == "en":
        return entry, []

    def one(text: str) -> str:
        translated, missing = translate(text, lang)
        unknown.extend(missing)
        return translated

    if "plural" in entry:
        return {"key": entry["key"], "plural": {k: one(v) for k, v in entry["plural"].items()}}, unknown
    return {"key": entry["key"], "value": one(entry["value"])}, unknown


# Chinese packs are version 2 so clients that stored version 1 receive the rewrite.
ZH_PACK_VERSION = 2


def pack_document(platform: str, lang: str, entries: list[dict]) -> dict:
    meta = META[lang]
    strings = []
    for entry in entries:
        item = {"key": entry["key"]}
        if "plural" in entry:
            for form in ("zero", "one", "two", "few", "many", "other"):
                if form in entry["plural"] and entry["plural"][form] != "":
                    item[form] = entry["plural"][form]
        else:
            item["value"] = entry["value"]
        if lang != "en":
            item["version"] = ZH_PACK_VERSION
        strings.append(item)
    doc = {
        "lang_code": lang,
        "name": meta["name"],
        "native_name": meta["native_name"],
        "plural_code": meta["plural_code"],
        "official": True,
        "rtl": False,
        "beta": False,
        "translations_url": f"https://tellomi.app/translations/{platform}/{lang}",
        "version": 1 if lang == "en" else ZH_PACK_VERSION,
        "strings": strings,
    }
    if meta["base_lang_code"]:
        doc["base_lang_code"] = meta["base_lang_code"]
    return doc


def build_platform(rows: list[tuple[str, str]], forms, platform: str) -> tuple[dict[str, dict], dict[str, list[str]]]:
    entries = [apply_brand_entry(e) for e in group(rows, forms)]
    packs = {}
    missing: dict[str, list[str]] = {}
    for lang in ("en", "zh-hans", "zh-hant"):
        translated = []
        unknown_keys = []
        for entry in entries:
            done, unknown = translate_entry(dict(entry) if "plural" not in entry else {"key": entry["key"], "plural": dict(entry["plural"])}, lang)
            translated.append(done)
            if unknown:
                unknown_keys.append(entry["key"])
        packs[lang] = pack_document(platform, lang, translated)
        missing[lang] = unknown_keys
    return packs, missing


def write_pack(path: Path, doc: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(doc, ensure_ascii=False, separators=(",", ":")) + "\n", encoding="utf-8")


def main() -> int:
    here = Path(__file__).resolve().parent
    parser = argparse.ArgumentParser(description="Import Tellomi langpacks from client string catalogs")
    parser.add_argument("--ios", type=Path, help="iOS en Localizable.strings")
    parser.add_argument("--ios-extra", type=Path, action="append", default=[], help="extra iOS .strings files")
    parser.add_argument("--android", type=Path, help="Android res/values/strings.xml")
    parser.add_argument("--android-extra", type=Path, action="append", default=[], help="extra Android strings.xml files")
    parser.add_argument("--out", type=Path, default=here.parent / "data" / "langpack")
    parser.add_argument("--report", type=Path, default=None, help="write untranslated keys here")
    args = parser.parse_args()
    if not args.ios and not args.android:
        parser.error("pass --ios and/or --android")

    report_lines = []
    if args.ios:
        rows = []
        for path in [args.ios, *args.ios_extra]:
            rows.extend(parse_ios(path))
        packs, missing = build_platform(rows, IOS_FORMS, "ios")
        for lang, doc in packs.items():
            write_pack(args.out / "ios" / f"{lang}.json", doc)
            n = len(doc["strings"])
            bad = len(missing[lang])
            line = f"ios {lang}: {n} strings, {n - bad} translated, {bad} untranslated"
            print(line)
            report_lines.append(line)
            report_lines.extend(f"ios {lang}\t{key}" for key in missing[lang])
    if args.android:
        rows = []
        for path in [args.android, *args.android_extra]:
            rows.extend(parse_android(path))
        packs, missing = build_platform(rows, ANDROID_FORMS, "android")
        for lang, doc in packs.items():
            write_pack(args.out / "android" / f"{lang}.json", doc)
            n = len(doc["strings"])
            bad = len(missing[lang])
            line = f"android {lang}: {n} strings, {n - bad} translated, {bad} untranslated"
            print(line)
            report_lines.append(line)
            report_lines.extend(f"android {lang}\t{key}" for key in missing[lang])
    summary = unknown_summary(40)
    if summary:
        print("most common untranslated words:")
        for word, count in summary:
            print(f"  {count:5d}  {word}")
            report_lines.append(f"word\t{count}\t{word}")
    if args.report:
        args.report.write_text("\n".join(report_lines) + "\n", encoding="utf-8")
    return 0


if __name__ == "__main__":
    sys.path.insert(0, str(Path(__file__).resolve().parent))
    raise SystemExit(main())
