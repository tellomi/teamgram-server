#!/usr/bin/env python3
"""Download official Telegram language packs into data/langpack.

Source (no login): https://translations.telegram.org/{code}/{platform}/export
The Content-Disposition filename carries the upstream pack version, for
example android_ja_v61521017.xml. That integer becomes the pack version so
clients that cached an empty version-1 pack receive the strings from
langpack.getDifference.

Tellomi's existing en / zh-hans / zh-hant catalogs are kept in
data/langpack/overrides and applied on top:

- AppName / Application.Name stay Tellomi.
- Where the Tellomi English string differs from the branded official English
  string, the Tellomi wording wins in en, zh-hans, and zh-hant.
- Every other key uses the official translation. Hosts and the Telegram name
  are rewritten (tell.cc, tellomi.cn, Tellomi).

Re-running the script reuses data/langpack/overrides and a download cache.
Pass --cache to choose the cache directory.
"""

from __future__ import annotations

import argparse
import gzip
import json
import re
import sys
import urllib.request
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from langpack_import import (  # noqa: E402
    ANDROID_FORMS,
    IOS_FORMS,
    group,
    parse_ios,
)

PLATFORMS = ("ios", "android", "tdesktop", "macos")
TELLOMI_LANGS = ("en", "zh-hans", "zh-hant")

# code, english name, native name, plural code, rtl, base lang ("" = none for en, else en)
# classic-zh-cn and taiwan are not imported. They duplicated zh-hans (简体中文)
# and zh-hant (繁體中文). pkg/langpack maps those old codes onto the official packs.
REMOVED_DUPLICATES = ("classic-zh-cn", "taiwan")
LANGS = [
    ("en", "English", "English", "en", False, ""),
    ("ar", "Arabic", "العربية", "ar", True, "en"),
    ("be", "Belarusian", "Беларуская", "be", False, "en"),
    ("ca", "Catalan", "Català", "ca", False, "en"),
    ("cs", "Czech", "Čeština", "cs", False, "en"),
    ("de", "German", "Deutsch", "de", False, "en"),
    ("el", "Greek", "Ελληνικά", "el", False, "en"),
    ("es", "Spanish", "Español", "es", False, "en"),
    ("fa", "Persian", "فارسی", "fa", True, "en"),
    ("fi", "Finnish", "Suomi", "fi", False, "en"),
    ("fr", "French", "Français", "fr", False, "en"),
    ("he", "Hebrew", "עברית", "he", True, "en"),
    ("hi", "Hindi", "हिन्दी", "hi", False, "en"),
    ("id", "Indonesian", "Bahasa Indonesia", "id", False, "en"),
    ("it", "Italian", "Italiano", "it", False, "en"),
    ("ja", "Japanese", "日本語", "ja", False, "en"),
    ("ko", "Korean", "한국어", "ko", False, "en"),
    ("ms", "Malay", "Bahasa Melayu", "ms", False, "en"),
    ("nb", "Norwegian", "Norsk", "nb", False, "en"),
    ("nl", "Dutch", "Nederlands", "nl", False, "en"),
    ("pl", "Polish", "Polski", "pl", False, "en"),
    ("pt-br", "Portuguese (Brazil)", "Português (Brasil)", "pt", False, "en"),
    ("ro", "Romanian", "Română", "ro", False, "en"),
    ("ru", "Russian", "Русский", "ru", False, "en"),
    ("sv", "Swedish", "Svenska", "sv", False, "en"),
    ("th", "Thai", "ไทย", "th", False, "en"),
    ("tr", "Turkish", "Türkçe", "tr", False, "en"),
    ("uk", "Ukrainian", "Українська", "uk", False, "en"),
    ("uz", "Uzbek", "Oʻzbek", "uz", False, "en"),
    ("vi", "Vietnamese", "Tiếng Việt", "vi", False, "en"),
    ("zh-hans", "Chinese (Simplified)", "简体中文", "zh", False, "en"),
    ("zh-hant", "Chinese (Traditional)", "繁體中文", "zh", False, "en"),
]

HASH_FORMS = (
    ("#zero", "zero"),
    ("#one", "one"),
    ("#two", "two"),
    ("#few", "few"),
    ("#many", "many"),
    ("#other", "other"),
)
BRAND_KEYS = {"AppName", "AppNameBeta", "Application.Name", "Watch.AppName"}
FORMS = ("zero", "one", "two", "few", "many", "other")


def rewrite_link(text: str) -> str:
    # t.me must be a host. Chat.Message contains the letters t.me and is not a link.
    text = re.sub(r"(?i)(?<![A-Za-z0-9])https?://(?:www\.)?(?:t\.me|telegram\.me|telegram\.dog)", "https://tell.cc", text)
    text = re.sub(r"(?i)https?://(?:www\.)?((?:[a-z0-9-]+\.)*)telegram\.org", r"https://\1tellomi.cn", text)
    text = re.sub(r"(?i)(?<![A-Za-z0-9])(?:t\.me|telegram\.me|telegram\.dog)\b", "tell.cc", text)
    text = re.sub(r"(?i)(?<![A-Za-z0-9])telegram\.org\b", "tellomi.cn", text)
    return text


def brand(text: str) -> str:
    if not text:
        return text
    if "telegram" not in text.lower() and "t.me" not in text.lower():
        return text
    text = rewrite_link(text)
    # Walk the lowercased copy so TElegram and Telegramı both match.
    # The boundary is ASCII, the same rule as the Go test: Turkish ı is not
    # a word byte, so the product name is still replaced.
    low = text.lower()
    out = []
    i = 0
    while True:
        j = low.find("telegram", i)
        if j < 0:
            out.append(text[i:])
            break
        end = j + len("telegram")
        prev_ok = j == 0 or not _ascii_word(low[j - 1])
        next_word = end < len(low) and _ascii_word(low[end])
        if prev_ok and not next_word and not low.startswith(".org", end):
            word = text[j:end]
            if word.isupper():
                repl = "TELLOMI"
            elif word[:1].isupper():
                repl = "Tellomi"
            else:
                repl = "tellomi"
            out.append(text[i:j])
            out.append(repl)
        else:
            out.append(text[i:end])
        i = end
    return "".join(out)


def _ascii_word(ch: str) -> bool:
    return ("a" <= ch <= "z") or ("0" <= ch <= "9") or ch == "_"


def has_telegram_word(text: str) -> bool:
    low = text.lower()
    start = 0
    while True:
        j = low.find("telegram", start)
        if j < 0:
            return False
        end = j + len("telegram")
        prev_ok = j == 0 or not _ascii_word(low[j - 1])
        next_word = end < len(low) and _ascii_word(low[end])
        next_org = low.startswith(".org", end)
        if prev_ok and not next_word and not next_org:
            return True
        start = end


def unescape_android(s: str) -> str:
    s = re.sub(r"<!\[CDATA\[(.*?)\]\]>", r"\1", s, flags=re.S)
    s = s.replace("\\n", "\n").replace("\\t", "\t").replace("\\'", "'").replace('\\"', '"')
    s = s.replace("\\\\", "\\")
    s = (
        s.replace("&lt;", "<")
        .replace("&gt;", ">")
        .replace("&amp;", "&")
        .replace("&quot;", '"')
        .replace("&apos;", "'")
        .replace("&#39;", "'")
    )
    return s


def parse_android(path: Path) -> list[tuple[str, str]]:
    text = path.read_text(encoding="utf-8", errors="replace")
    rows = []
    for name, body in re.findall(r'<string\s+name="([^"]+)"[^>]*>(.*?)</string>', text, re.S):
        rows.append((name, unescape_android(body)))
    return rows


def forms_for(platform: str, rows: list[tuple[str, str]]) -> tuple:
    hash_hits = sum(1 for key, _ in rows if "#" in key)
    if platform == "ios":
        return IOS_FORMS
    if platform == "macos":
        return IOS_FORMS + ANDROID_FORMS
    if platform == "tdesktop" or hash_hits > 20:
        return HASH_FORMS + ANDROID_FORMS
    return ANDROID_FORMS


def brand_entries(entries: list[dict]) -> list[dict]:
    out = []
    for entry in entries:
        item = {"key": entry["key"]}
        if "plural" in entry:
            item["plural"] = {k: brand(v) for k, v in entry["plural"].items()}
        else:
            item["value"] = brand(entry.get("value", ""))
        if entry["key"] in BRAND_KEYS and "plural" not in item:
            item["value"] = "Tellomi"
        out.append(item)
    return out


def signature(entry: dict) -> tuple:
    if "plural" in entry:
        return ("p", tuple((k, entry["plural"].get(k, "")) for k in FORMS))
    return ("v", entry.get("value", ""))


def from_file_string(raw: dict) -> dict:
    entry = {"key": raw["key"], "version": int(raw.get("version") or 1)}
    forms = {k: raw[k] for k in FORMS if raw.get(k)}
    if forms:
        entry["plural"] = forms
    else:
        entry["value"] = raw.get("value", "")
    return entry


def load_doc(path: Path) -> dict | None:
    if not path.exists():
        return None
    if str(path).endswith(".gz"):
        with gzip.open(path, "rt", encoding="utf-8") as fh:
            return json.load(fh)
    return json.loads(path.read_text(encoding="utf-8"))


def ensure_overrides(root: Path) -> None:
    """Keep the pre-import Tellomi catalogs so a later fetch can reapply them."""
    for platform in ("ios", "android"):
        for lang in TELLOMI_LANGS:
            dest = root / "overrides" / platform / f"{lang}.json.gz"
            if dest.exists():
                continue
            src = root / platform / f"{lang}.json"
            doc = load_doc(src) if src.exists() else None
            if not doc or len(doc.get("strings") or []) < 100:
                continue
            dest.parent.mkdir(parents=True, exist_ok=True)
            payload = json.dumps(doc, ensure_ascii=False, separators=(",", ":")).encode()
            with gzip.open(dest, "wb", compresslevel=9) as fh:
                fh.write(payload)
            print(f"saved override {platform}/{lang} ({len(doc['strings'])} strings)")


def load_overrides(root: Path, platform: str) -> dict[str, dict]:
    found = {}
    for lang in TELLOMI_LANGS:
        path = root / "overrides" / platform / f"{lang}.json.gz"
        doc = load_doc(path)
        if not doc:
            continue
        found[lang] = {s["key"]: from_file_string(s) for s in doc.get("strings") or []}
    return found


def diverged_keys(official_en: dict[str, dict], tellomi_en: dict[str, dict]) -> set[str]:
    keys = set()
    for key, tel in tellomi_en.items():
        off = official_en.get(key)
        if off is None:
            continue
        if signature(tel) != signature(off):
            keys.add(key)
    return keys


def use_tellomi(lang: str, key: str, diverged: set[str]) -> bool:
    if key in BRAND_KEYS:
        return True
    return lang in TELLOMI_LANGS and key in diverged


def to_json_string(entry: dict, version: int) -> dict:
    item = {"key": entry["key"], "version": version}
    if "plural" in entry:
        for form in FORMS:
            val = entry["plural"].get(form, "")
            if val != "":
                item[form] = val
        return item
    item["value"] = entry.get("value", "")
    return item


def download(url: str) -> tuple[bytes, str]:
    req = urllib.request.Request(url, headers={"User-Agent": "TellomiLangpack/1.0"})
    with urllib.request.urlopen(req, timeout=90) as resp:
        filename = ""
        disp = resp.headers.get("Content-Disposition", "")
        m = re.search(r'filename="([^"]+)"', disp)
        if m:
            filename = m.group(1)
        return resp.read(), filename


def fetch_cached(cache: Path, platform: str, code: str) -> tuple[Path, str, int]:
    body = cache / f"{platform}_{code}.body"
    meta = cache / f"{platform}_{code}.meta"
    if body.exists() and meta.exists() and body.stat().st_size > 100:
        filename, version = meta.read_text(encoding="utf-8").split("\n", 1)
        return body, filename.strip(), int(version.strip() or "1")
    url = f"https://translations.telegram.org/{code}/{platform}/export"
    last_err = None
    for _ in range(3):
        try:
            data, filename = download(url)
            m = re.search(r"_v(\d+)\.", filename)
            version = int(m.group(1)) if m else 1
            cache.mkdir(parents=True, exist_ok=True)
            body.write_bytes(data)
            meta.write_text(f"{filename}\n{version}\n", encoding="utf-8")
            return body, filename, version
        except Exception as exc:  # noqa: BLE001 - retry network failures
            last_err = exc
    raise RuntimeError(f"{url}: {last_err}")


def parse_body(path: Path, platform: str) -> list[dict]:
    head = path.read_bytes()[:80]
    if head.lstrip().startswith(b"<?xml") or b"<resources" in head:
        rows = parse_android(path)
    else:
        rows = parse_ios(path)
    entries = group(rows, forms_for(platform, rows))
    return brand_entries(entries)


def merge_lang(
    lang: str,
    official: list[dict],
    overrides: dict[str, dict],
    diverged: set[str],
    upstream: int,
    official_en: dict[str, dict],
) -> list[dict]:
    tel = overrides.get(lang, {})
    by_key: dict[str, dict] = {}
    order: list[str] = []
    for entry in official:
        by_key[entry["key"]] = entry
        order.append(entry["key"])
    # Fork-only keys and brand/diverged overrides.
    for key, entry in tel.items():
        if key in by_key:
            continue
        order.append(key)
        by_key[key] = {"key": key, "version": entry.get("version") or 1, **_payload(entry)}
    strings = []
    for key in order:
        official_entry = by_key[key]
        chosen = official_entry
        baseline = tel.get(key)
        if baseline is not None and use_tellomi(lang, key, diverged):
            chosen = {"key": key, **_payload(baseline)}
        if key in BRAND_KEYS and "plural" not in chosen:
            chosen = {"key": key, "value": "Tellomi"}
        # Android exports untranslated languages as a full English copy.
        # Drop those copies so base_lang_code=en supplies Tellomi English.
        if lang != "en":
            en_entry = official_en.get(key)
            if en_entry is not None and signature(chosen) == signature(en_entry) and not use_tellomi(lang, key, diverged):
                continue
        if baseline is not None and signature(chosen) == signature(baseline):
            version = int(baseline.get("version") or 1)
        else:
            version = upstream
        if version < 1:
            version = 1
        strings.append(to_json_string(chosen, version))
    return strings


def _payload(entry: dict) -> dict:
    if "plural" in entry:
        return {"plural": dict(entry["plural"])}
    return {"value": entry.get("value", "")}


def texts_of(item: dict) -> list[str]:
    if "value" in item:
        return [item["value"]]
    return [item[k] for k in FORMS if k in item]


def write_gz(path: Path, doc: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    raw = json.dumps(doc, ensure_ascii=False, separators=(",", ":")).encode()
    with gzip.open(path, "wb", compresslevel=9) as fh:
        fh.write(raw)
    plain = path.with_name(path.name[: -len(".gz")]) if path.name.endswith(".json.gz") else None
    # path is lang.json.gz; the old file is lang.json
    old = Path(str(path)[: -len(".gz")])
    if old.exists() and old.suffix == ".json":
        old.unlink()
    if plain is not None and plain != old and plain.exists():
        plain.unlink()


def build_doc(meta: tuple, platform: str, strings: list[dict], upstream: int, filename: str) -> dict:
    code, name, native, plural, rtl, base = meta
    version = upstream
    for item in strings:
        if item.get("version", 1) > version:
            version = item["version"]
    doc = {
        "lang_code": code,
        "name": name,
        "native_name": native,
        "plural_code": plural,
        "official": True,
        "rtl": rtl,
        "beta": False,
        "translations_url": f"https://tellomi.cn/translations/{platform}/{code}",
        "version": version,
        "source": f"https://translations.telegram.org/{code}/{platform}/export",
        "source_file": filename,
        "source_version": upstream,
        "strings": strings,
    }
    if base:
        doc["base_lang_code"] = base
    return doc


def apply_machine(root: Path, platform: str, code: str, strings: list[dict]) -> list[dict]:
    """Reapply reviewed-later machine translations for keys the export still lacks.

    data/langpack/machine holds only the generated strings (mt:true). An official
    translation for the same key wins.
    """
    path = root / "machine" / platform / f"{code}.json.gz"
    doc = load_doc(path)
    if not doc:
        return strings
    have = {item["key"] for item in strings}
    extra = []
    for item in doc.get("strings") or []:
        key = item.get("key")
        if not key or key in have:
            continue
        have.add(key)
        extra.append(item)
    if extra:
        print(f"machine {platform}/{code}: +{len(extra)}")
    return strings + extra


def main() -> int:
    here = Path(__file__).resolve().parent
    parser = argparse.ArgumentParser(description="Import official Telegram language packs")
    parser.add_argument("--out", type=Path, default=here.parent / "data" / "langpack")
    parser.add_argument("--cache", type=Path, default=Path("/tmp/tellomi-langpack-cache"))
    parser.add_argument("--workers", type=int, default=6)
    parser.add_argument("--codes", default="", help="Comma-separated language codes to write. English is still fetched as the baseline.")
    parser.add_argument("--platforms", default="", help="Comma-separated platforms to write (default: all).")
    args = parser.parse_args()
    metas = {meta[0]: meta for meta in LANGS}
    codes = [part.strip() for part in args.codes.split(",") if part.strip()] or [meta[0] for meta in LANGS]
    platforms = [part.strip() for part in args.platforms.split(",") if part.strip()] or list(PLATFORMS)
    unknown = [code for code in codes if code not in metas]
    bad_platforms = [platform for platform in platforms if platform not in PLATFORMS]
    if unknown or bad_platforms:
        print(f"unknown codes {unknown} platforms {bad_platforms}", file=sys.stderr)
        return 1
    code_set = set(codes)
    partial = code_set != {meta[0] for meta in LANGS} or set(platforms) != set(PLATFORMS)
    ensure_overrides(args.out)
    for platform in PLATFORMS:
        for code in REMOVED_DUPLICATES:
            for name in (f"{code}.json.gz", f"{code}.json"):
                stale = args.out / platform / name
                if stale.exists():
                    stale.unlink()
                    print(f"removed duplicate {platform}/{code}")

    fetch_codes = set(codes) | {"en"}
    jobs = [(platform, metas[code]) for platform in platforms for code in sorted(fetch_codes)]
    fetched: dict[tuple[str, str], tuple[Path, str, int]] = {}
    errors = []

    def one(job):
        platform, meta = job
        path, filename, version = fetch_cached(args.cache, platform, meta[0])
        return platform, meta[0], path, filename, version

    with ThreadPoolExecutor(max_workers=args.workers) as pool:
        futures = [pool.submit(one, job) for job in jobs]
        for fut in as_completed(futures):
            try:
                platform, code, path, filename, version = fut.result()
            except Exception as exc:  # noqa: BLE001
                errors.append(str(exc))
                print("FAIL", exc)
                continue
            fetched[(platform, code)] = (path, filename, version)
            print(f"fetched {platform}/{code} v{version}")

    if errors:
        print(f"{len(errors)} downloads failed", file=sys.stderr)
        return 1

    manifest = {
        "source": "https://translations.telegram.org/{code}/{platform}/export",
        "note": "Public export. No login. Version is the integer in the export filename.",
        "overrides": "data/langpack/overrides/{ios,android}/{en,zh-hans,zh-hant}.json.gz",
        "machine": "data/langpack/machine/{platform}/{code}.json.gz strings with mt:true are reapplied when a key is still missing after the official merge.",
        "packs": {},
    }
    problems = []
    for platform in platforms:
        overrides = load_overrides(args.out, platform)
        en_path, _, _ = fetched[(platform, "en")]
        official_en = {e["key"]: e for e in parse_body(en_path, platform)}
        diverged = diverged_keys(official_en, overrides.get("en", {}))
        print(f"{platform}: {len(diverged)} Tellomi English overrides")
        for meta in LANGS:
            code = meta[0]
            if code not in code_set:
                continue
            path, filename, upstream = fetched[(platform, code)]
            if code == "en":
                official = list(official_en.values())
            else:
                official = parse_body(path, platform)
            strings = merge_lang(code, official, overrides, diverged, upstream, official_en)
            strings = apply_machine(args.out, platform, code, strings)
            doc = build_doc(meta, platform, strings, upstream, filename)
            dest = args.out / platform / f"{code}.json.gz"
            write_gz(dest, doc)
            for item in strings:
                for text in texts_of(item):
                    low = text.lower()
                    if has_telegram_word(text) or "telegram.org" in low or "t.me" in low or "telegram.me" in low or "telegram.dog" in low:
                        if len(problems) < 20:
                            problems.append(f"{platform}/{code} {item['key']}: {text[:160]!r}")
                        break
            manifest["packs"][f"{platform}/{code}"] = {
                "source_file": filename,
                "source_version": upstream,
                "strings": len(strings),
                "bytes_gz": dest.stat().st_size,
            }
            print(f"wrote {platform}/{code}: {len(strings)} strings v{doc['version']}")

    source_path = args.out / "SOURCE.json"
    if partial and source_path.exists():
        previous = json.loads(source_path.read_text(encoding="utf-8"))
        packs = previous.get("packs") or {}
        packs.update(manifest["packs"])
        manifest["packs"] = packs
        for key in ("source", "note", "overrides"):
            manifest.setdefault(key, previous.get(key))
    source_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    if problems:
        print("brand leftovers:", file=sys.stderr)
        for line in problems:
            print(line, file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
