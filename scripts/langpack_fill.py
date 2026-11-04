#!/usr/bin/env python3
"""Fill langpack keys that English has and another language does not.

Official strings stay as they are. Missing keys are machine-translated from
the English pack (Google Translate) and marked {"mt": true, "mt_via": ...}
so a reviewer can find them later. Placeholders (%@, %1$s, {count}, {}) and
markdown link targets are not sent to the translator.

zh-hans / zh-hant gaps are filled from the sibling pack with OpenCC
(s2twp / tw2sp) before any translator call. A key missing from both is
translated once into simplified Chinese and then converted.

Writes:
  data/langpack/<platform>/<code>.json.gz
  data/langpack/machine/<platform>/<code>.json.gz   (only the generated strings)

scripts/langpack_official.py reapplies the machine files on a later export.
"""

from __future__ import annotations

import argparse
import gzip
import json
import re
import sys
import threading
import time
import urllib.parse
import urllib.request
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from langpack_official import LANGS, PLATFORMS, brand as official_brand, rewritten_key  # noqa: E402

FORMS = ("zero", "one", "two", "few", "many", "other")

# Placeholders a client substitutes. A trailing % after a specifier is a
# literal percent (battery %@%). The validation regex does not swallow it;
# KEEP does, so the translator never sees the specifier.
PH_RE = re.compile(
    r"%%"
    r"|%(?:\d+\$)?[#0\- +']*(?:\d+|\*)?(?:\.\d+)?(?:hh|h|ll|l|z|j|t|q|L)?[diuoxXfFeEgGaAcspn@]"
    r"|\{[A-Za-z_][A-Za-z0-9_]*\}"
    r"|\{\}"
    r"|\{[0-9]+\}"
    r"|%\d+\b"
)
KEEP_RE = re.compile(
    r"("
    r"https?://[^\s<>()\]]+"
    r"|(?<![A-Za-z0-9])[Tt]ellomi\.cn\b"
    r"|(?<![A-Za-z0-9])[Tt]ell\.cc\b"
    r"|\]\([^)\n]*\)"
    r"|%%"
    r"|%(?:\d+\$)?[#0\- +']*(?:\d+|\*)?(?:\.\d+)?(?:hh|h|ll|l|z|j|t|q|L)?[diuoxXfFeEgGaAcspn@]%?"
    r"|\{[A-Za-z_][A-Za-z0-9_]*\}"
    r"|\{\}"
    r"|\{[0-9]+\}"
    r"|%\d+\b"
    r"|Tellomi"
    r"|TELLOMI"
    r"|\n"
    r")"
)
MD_LINK = re.compile(r"(\[[^\[\]]*\])\(([^)\n]*)\)")
HAS_LETTER = re.compile(r"[A-Za-z]")
TOKEN_MARK = re.compile(r"⟦\s*(\d+)\s*⟧")

GOOGLE = {
    "ar": "ar",
    "be": "be",
    "ca": "ca",
    "cs": "cs",
    "de": "de",
    "el": "el",
    "es": "es",
    "fa": "fa",
    "fi": "fi",
    "fr": "fr",
    "he": "iw",
    "hi": "hi",
    "id": "id",
    "it": "it",
    "ja": "ja",
    "ko": "ko",
    "ms": "ms",
    "nb": "no",
    "nl": "nl",
    "pl": "pl",
    "pt-br": "pt",
    "ro": "ro",
    "ru": "ru",
    "sv": "sv",
    "th": "th",
    "tr": "tr",
    "uk": "uk",
    "uz": "uz",
    "vi": "vi",
    "zh-hans": "zh-CN",
    "zh-hant": "zh-TW",
}


def corrupt_key(key: str) -> bool:
    # brand() used to rewrite the letters t.me inside Chat.Message.
    # Those keys are not real strings. Do not copy them into other languages.
    # The same tell.cc check refuses the key in langpack_official.py.
    return rewritten_key(key)


def placeholders(text: str) -> list[str]:
    return PH_RE.findall(text or "")


def markdown_targets(text: str) -> list[str]:
    return [match.group(2) for match in MD_LINK.finditer(text or "")]


def same_tokens(src: str, dst: str) -> bool:
    return placeholders(src) == placeholders(dst) and markdown_targets(src) == markdown_targets(dst)


def brand_safe(text: str) -> str:
    """Brand Telegram hosts and the product name, leaving link targets alone."""
    slots: list[str] = []

    def hold(match: re.Match[str]) -> str:
        slots.append(match.group(2))
        return f"{match.group(1)}(\u0001{len(slots) - 1}\u0001)"

    shielded = MD_LINK.sub(hold, text)
    branded = official_brand(shielded)
    for i, target in enumerate(slots):
        branded = branded.replace(f"\u0001{i}\u0001", target, 1)
    return branded


def entry_forms(item: dict) -> dict[str, str]:
    if any(item.get(form) for form in FORMS):
        return {form: item[form] for form in FORMS if item.get(form)}
    return {"value": item.get("value") or ""}


def load_doc(path: Path) -> dict:
    with gzip.open(path, "rt", encoding="utf-8") as fh:
        return json.load(fh)


def write_gz(path: Path, doc: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    raw = json.dumps(doc, ensure_ascii=False, separators=(",", ":")).encode()
    with gzip.open(path, "wb", compresslevel=9) as fh:
        fh.write(raw)


class Translator:
    def __init__(self, cache_path: Path, workers: int) -> None:
        self.cache_path = cache_path
        self.workers = workers
        self.cache: dict[tuple[str, str], str] = {}
        self.lock = threading.Lock()
        self.session_bytes = 0
        if cache_path.exists():
            with cache_path.open(encoding="utf-8") as fh:
                for line in fh:
                    line = line.strip()
                    if not line:
                        continue
                    row = json.loads(line)
                    self.cache[(row["tl"], row["src"])] = row["dst"]
        self.cache_path.parent.mkdir(parents=True, exist_ok=True)

    def _remember(self, tl: str, src: str, dst: str) -> None:
        with self.lock:
            if (tl, src) in self.cache:
                return
            self.cache[(tl, src)] = dst
            with self.cache_path.open("a", encoding="utf-8") as fh:
                fh.write(json.dumps({"tl": tl, "src": src, "dst": dst}, ensure_ascii=False) + "\n")

    def get(self, tl: str, src: str) -> str | None:
        return self.cache.get((tl, src))

    def translate_all(self, pairs: list[tuple[str, str]]) -> None:
        missing: dict[str, list[str]] = {}
        seen: set[tuple[str, str]] = set()
        for tl, src in pairs:
            if not src or not HAS_LETTER.search(src):
                continue
            key = (tl, src)
            if key in seen or key in self.cache:
                continue
            seen.add(key)
            missing.setdefault(tl, []).append(src)
        total = sum(len(v) for v in missing.values())
        print(f"translate {total} unique segments across {len(missing)} languages", flush=True)
        if total == 0:
            return
        done = 0
        done_lock = threading.Lock()

        def one_lang(item: tuple[str, list[str]]) -> None:
            nonlocal done
            tl, texts = item
            batches = pack_batches(texts)
            for batch in batches:
                translated = translate_batch(tl, batch)
                for src, dst in zip(batch, translated):
                    self._remember(tl, src, dst)
                with done_lock:
                    done += len(batch)
                    if done % 200 < len(batch) or done == total:
                        print(f"  translated {done}/{total}", flush=True)

        with ThreadPoolExecutor(max_workers=self.workers) as pool:
            futures = [pool.submit(one_lang, item) for item in missing.items()]
            for fut in as_completed(futures):
                fut.result()


def pack_batches(texts: list[str], limit: int = 3500) -> list[list[str]]:
    batches: list[list[str]] = []
    cur: list[str] = []
    size = 0
    for text in texts:
        extra = len(text) + 12
        if cur and (size + extra > limit or len(cur) >= 24):
            batches.append(cur)
            cur = []
            size = 0
        cur.append(text)
        size += extra
    if cur:
        batches.append(cur)
    return batches


def gtx(tl: str, query: str) -> str:
    params = urllib.parse.urlencode(
        {"client": "gtx", "sl": "en", "tl": tl, "dt": "t", "q": query}
    )
    url = "https://translate.googleapis.com/translate_a/single?" + params
    last = None
    for attempt in range(5):
        try:
            req = urllib.request.Request(url, headers={"User-Agent": "Mozilla/5.0"})
            with urllib.request.urlopen(req, timeout=40) as resp:
                payload = json.loads(resp.read().decode("utf-8"))
            parts = []
            for seg in payload[0]:
                if seg and seg[0]:
                    parts.append(seg[0])
            return "".join(parts)
        except Exception as exc:  # noqa: BLE001 - retry the public endpoint
            last = exc
            time.sleep(1.5 * (attempt + 1))
    raise RuntimeError(f"translate {tl}: {last}")


def translate_batch(tl: str, batch: list[str]) -> list[str]:
    """Translate one line per segment. A mismatched line count is retried smaller.

    Markers get pulled into the sentence (Arabic moves them), so the separator
    is a newline and each segment is a single line.
    """
    if len(batch) == 1:
        return [translate_one(tl, batch[0])]
    if any("\n" in text for text in batch):
        return [translate_one(tl, text) for text in batch]
    try:
        raw = gtx(tl, "\n".join(batch))
    except Exception:
        return [translate_one(tl, text) for text in batch]
    lines = raw.split("\n")
    if lines and lines[-1] == "":
        lines = lines[:-1]
    if len(lines) != len(batch):
        if len(batch) > 4:
            mid = len(batch) // 2
            return translate_batch(tl, batch[:mid]) + translate_batch(tl, batch[mid:])
        return [translate_one(tl, text) for text in batch]
    out = []
    for src, line in zip(batch, lines):
        line = line.strip()
        out.append(line if line else src)
    return out


def translate_one(tl: str, text: str) -> str:
    raw = gtx(tl, text)
    cleaned = raw.strip()
    return cleaned if cleaned else text


def translate_piece(translator: Translator, tl: str, piece: str) -> str:
    if piece == "" or not HAS_LETTER.search(piece):
        return piece
    match = re.match(r"^(\s*)(.*?)(\s*)$", piece, re.S)
    assert match is not None
    lead, core, trail = match.group(1), match.group(2), match.group(3)
    if not HAS_LETTER.search(core):
        return piece
    got = translator.get(tl, core)
    if got is None:
        got = translate_one(tl, core)
        translator._remember(tl, core, got)
    return lead + got.strip() + trail


def stitch(text: str, tl: str, translator: Translator) -> str:
    parts = KEEP_RE.split(text)
    out = []
    for i, part in enumerate(parts):
        if i % 2 == 1:
            out.append(part)
        else:
            out.append(translate_piece(translator, tl, part))
    return "".join(out)


def pieces_of(text: str) -> list[str]:
    parts = KEEP_RE.split(text)
    found = []
    for i, part in enumerate(parts):
        if i % 2 == 1 or not HAS_LETTER.search(part):
            continue
        match = re.match(r"^(\s*)(.*?)(\s*)$", part, re.S)
        core = match.group(2) if match else part
        if HAS_LETTER.search(core):
            found.append(core)
    return found


def opencc_convert(text: str, converter) -> str:
    slots: list[str] = []

    def hold(match: re.Match[str]) -> str:
        slots.append(match.group(0))
        return f"⟦{len(slots) - 1}⟧"

    shielded = KEEP_RE.sub(hold, text)
    converted = converter.convert(shielded)
    converted = TOKEN_MARK.sub(lambda m: f"⟦{m.group(1)}⟧", converted)
    for i, slot in enumerate(slots):
        token = f"⟦{i}⟧"
        if token not in converted:
            # Conversion ate a shield. Keep the source text for this string.
            return text
        converted = converted.replace(token, slot, 1)
    return converted


def to_string(key: str, forms: dict[str, str], version: int, via: str) -> dict:
    item: dict = {"key": key, "version": version}
    if "value" in forms and not any(forms.get(form) for form in FORMS):
        item["value"] = forms.get("value", "")
    else:
        for form in FORMS:
            val = forms.get(form, "")
            if val != "":
                item[form] = val
    item["mt"] = True
    item["mt_via"] = via
    return item


def convert_forms(forms: dict[str, str], converter) -> dict[str, str]:
    return {name: opencc_convert(value, converter) for name, value in forms.items()}


def google_forms(forms: dict[str, str], tl: str, translator: Translator) -> dict[str, str]:
    return {name: brand_safe(stitch(value, tl, translator)) for name, value in forms.items()}


def forms_ok(src: dict[str, str], dst: dict[str, str]) -> bool:
    if set(src) != set(dst):
        return False
    return all(same_tokens(src[name], dst[name]) for name in src)


def fallback_forms(src: dict[str, str], dst: dict[str, str]) -> dict[str, str]:
    """Keep a source form when a translation drops or invents a placeholder."""
    fixed = {}
    for name, value in src.items():
        got = dst.get(name, "")
        fixed[name] = got if same_tokens(value, got) else value
    return fixed


def main() -> int:
    here = Path(__file__).resolve().parent
    parser = argparse.ArgumentParser(description="Machine-translate missing langpack keys")
    parser.add_argument("--out", type=Path, default=here.parent / "data" / "langpack")
    parser.add_argument("--cache", type=Path, default=Path("/tmp/tellomi-mt-cache.jsonl"))
    parser.add_argument("--workers", type=int, default=4)
    parser.add_argument("--codes", default="")
    parser.add_argument("--platforms", default="")
    args = parser.parse_args()

    try:
        import opencc
    except ImportError:
        print("opencc is required (pip install opencc-python-reimplemented)", file=sys.stderr)
        return 1
    s2t = opencc.OpenCC("s2twp")
    t2s = opencc.OpenCC("tw2sp")

    codes = [part.strip() for part in args.codes.split(",") if part.strip()] or [meta[0] for meta in LANGS]
    platforms = [part.strip() for part in args.platforms.split(",") if part.strip()] or list(PLATFORMS)
    translator = Translator(args.cache, args.workers)

    # Pass 1: discover English segments that still need a translator.
    needed: list[tuple[str, str]] = []
    for platform in platforms:
        en = {item["key"]: item for item in load_doc(args.out / platform / "en.json.gz")["strings"]}
        present = {}
        for code in codes:
            if code == "en":
                continue
            doc = load_doc(args.out / platform / f"{code}.json.gz")
            present[code] = {
                item["key"]: entry_forms(item)
                for item in doc["strings"]
                if not (item.get("mt") and corrupt_key(item["key"]))
            }
        hans_forms = present.get("zh-hans", {})
        hant_forms = present.get("zh-hant", {})
        for code in codes:
            if code == "en":
                continue
            tl = GOOGLE.get(code)
            if not tl:
                print(f"no translator for {code}", file=sys.stderr)
                return 1
            target = "zh-CN" if code == "zh-hant" else tl
            have = present[code]
            for key, item in en.items():
                if corrupt_key(key):
                    continue
                src_forms = entry_forms(item)
                existing = have.get(key)
                if existing is None:
                    if code == "zh-hant" and key in hans_forms:
                        continue
                    if code == "zh-hans" and key in hant_forms:
                        continue
                    texts = src_forms.values()
                else:
                    texts = []
                    for name, text in src_forms.items():
                        if existing.get(name):
                            continue
                        sibling = hans_forms.get(key) if code == "zh-hant" else None
                        if sibling and sibling.get(name):
                            continue
                        texts.append(text)
                for text in texts:
                    for piece in pieces_of(text):
                        needed.append((target, piece))
    translator.translate_all(needed)

    report: dict[str, int] = {}
    problems: list[str] = []
    for platform in platforms:
        en_doc = load_doc(args.out / platform / "en.json.gz")
        en = {item["key"]: item for item in en_doc["strings"]}
        loaded = {}
        removed = {}
        for code in codes:
            loaded[code] = load_doc(args.out / platform / f"{code}.json.gz")
            removed[code] = 0
            if code == "en":
                continue
            kept = [
                item for item in loaded[code]["strings"] if not (item.get("mt") and corrupt_key(item["key"]))
            ]
            removed[code] = len(loaded[code]["strings"]) - len(kept)
            loaded[code]["strings"] = kept
        by = {code: {item["key"]: item for item in doc["strings"]} for code, doc in loaded.items()}

        generated: dict[str, list[dict]] = {code: [] for code in codes if code != "en"}

        def add(code: str, key: str, forms: dict[str, str], via: str, src_forms: dict[str, str]) -> None:
            if key in by[code]:
                return
            if not forms_ok(src_forms, forms):
                forms = fallback_forms(src_forms, forms)
            if not forms_ok(src_forms, forms):
                problems.append(f"{platform}/{code} {key} placeholder mismatch")
                return
            version = int(loaded[code].get("version") or 1) + 1
            item = to_string(key, forms, version, via)
            by[code][key] = item
            generated[code].append(item)

        # Chinese sibling gaps, including keys that are not in English.
        if "zh-hans" in by and "zh-hant" in by:
            for key, item in list(by["zh-hans"].items()):
                if key in by["zh-hant"]:
                    continue
                src = entry_forms(item)
                add("zh-hant", key, convert_forms(src, s2t), "opencc", src)
            for key, item in list(by["zh-hant"].items()):
                if key in by["zh-hans"]:
                    continue
                src = entry_forms(item)
                add("zh-hans", key, convert_forms(src, t2s), "opencc", src)

        for code in codes:
            if code == "en":
                continue
            tl = "zh-CN" if code == "zh-hant" else GOOGLE[code]
            for key, item in en.items():
                if corrupt_key(key) or key in by[code]:
                    continue
                src = entry_forms(item)
                forms = google_forms(src, tl, translator)
                if code == "zh-hant":
                    forms = convert_forms(forms, s2t)
                    via = "google+opencc"
                else:
                    via = "copy" if forms == src else "google"
                add(code, key, forms, via, src)

        form_updates: dict[str, list[dict]] = {code: [] for code in codes if code != "en"}
        for code in codes:
            if code == "en":
                continue
            tl = "zh-CN" if code == "zh-hant" else GOOGLE[code]
            for key, item in en.items():
                if corrupt_key(key) or key not in by[code]:
                    continue
                src = entry_forms(item)
                if "value" in src:
                    continue
                existing = by[code][key]
                missing = {name: text for name, text in src.items() if not existing.get(name)}
                if not missing:
                    continue
                filled: dict[str, str] = {}
                rest = dict(missing)
                if code == "zh-hant":
                    sibling = by.get("zh-hans", {}).get(key) or {}
                    taken = {name: sibling[name] for name in list(rest) if sibling.get(name)}
                    for name in taken:
                        rest.pop(name, None)
                    if taken:
                        filled.update(convert_forms(taken, s2t))
                if rest:
                    got = google_forms(rest, tl, translator)
                    if code == "zh-hant":
                        got = convert_forms(got, s2t)
                    filled.update(got)
                if not forms_ok(missing, filled):
                    filled = fallback_forms(missing, filled)
                if not forms_ok(missing, filled):
                    problems.append(f"{platform}/{code} {key} plural placeholder mismatch")
                    continue
                noted = list(existing.get("mt_forms") or [])
                for name, text in filled.items():
                    existing[name] = text
                    if name not in noted:
                        noted.append(name)
                existing["mt_forms"] = noted
                form_updates[code].append(existing)

        for code, items in generated.items():
            doc = loaded[code]
            forms_changed = form_updates[code]
            if not items and not forms_changed and not removed[code]:
                report[f"{platform}/{code}"] = 0
                continue
            if items or forms_changed:
                new_version = int(doc.get("version") or 1) + 1
                for item in items:
                    item["version"] = new_version
                for item in forms_changed:
                    item["version"] = new_version
                doc["strings"].extend(items)
                doc["version"] = new_version
            write_gz(args.out / platform / f"{code}.json.gz", doc)
            print(
                f"filled {platform}/{code}: +{len(items)} keys, {len(forms_changed)} plural forms, -{removed[code]} corrupt v{doc.get('version')}",
                flush=True,
            )
            mt_strings = [
                item for item in doc["strings"] if item.get("mt") is True or item.get("mt_forms")
            ]
            if mt_strings:
                write_gz(
                    args.out / "machine" / platform / f"{code}.json.gz",
                    {
                        "lang_code": code,
                        "platform": platform,
                        "note": "Machine translations for review. mt:true. Placeholders were not sent to the translator.",
                        "strings": mt_strings,
                    },
                )
            report[f"{platform}/{code}"] = len(items)

    source_path = args.out / "SOURCE.json"
    if source_path.exists():
        manifest = json.loads(source_path.read_text(encoding="utf-8"))
    else:
        manifest = {"packs": {}}
    packs = manifest.setdefault("packs", {})
    for platform in platforms:
        for code in codes:
            path = args.out / platform / f"{code}.json.gz"
            if not path.exists():
                continue
            doc = load_doc(path)
            entry = packs.get(f"{platform}/{code}") or {}
            entry["strings"] = len(doc["strings"])
            entry["bytes_gz"] = path.stat().st_size
            entry["version"] = doc.get("version")
            packs[f"{platform}/{code}"] = entry
    manifest["machine"] = "Strings with mt:true were machine-translated and need review. See data/langpack/machine/."
    source_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    (args.out / "MACHINE.json").write_text(
        json.dumps(
            {
                "note": "Counts of strings added with mt:true. Review these before treating them as final copy.",
                "added": report,
            },
            ensure_ascii=False,
            indent=2,
        )
        + "\n",
        encoding="utf-8",
    )
    if problems:
        print(f"{len(problems)} placeholder failures", file=sys.stderr)
        for line in problems[:30]:
            print(line, file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
