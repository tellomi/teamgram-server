#!/usr/bin/env python3
"""Regenerate the cn-politics coverage files. Does not write username rules.

--fetch downloads the pinned blobs, checks them, and rewrites generated/.
--check recomputes pinyin from the vendored files and the overlap keys.
A blob mismatch or a pypinyin version other than 0.55.0 is a failure.
"""

from __future__ import annotations

import hashlib
import json
import re
import sys
import unicodedata
import urllib.parse
import urllib.request
from collections import defaultdict
from pathlib import Path

if sys.version_info < (3, 11):
    raise SystemExit(f"update.py needs Python 3.11+; this is {sys.version.split()[0]}")

import tomllib

ROOT = Path(__file__).resolve().parent
SOURCE = ROOT / "SOURCE.toml"
PYPINYIN_VERSION = "0.55.0"
TRAILING_PUNCT = "。．.，,、;；"
USERNAME_SAFE = re.compile(r"[_a-z][a-z0-9_]{2,31}\Z")
NON_ALNUM = re.compile(r"[^a-z0-9]")


class UpdateError(Exception):
    pass


def git_blob_sha1(data: bytes) -> str:
    header = f"blob {len(data)}\0".encode()
    return hashlib.sha1(header + data).hexdigest()


def parse_lines(text: str, *, strip_trailing_comma: bool) -> list[str]:
    out: list[str] = []
    for line in text.splitlines():
        word = line.strip()
        if strip_trailing_comma:
            word = word.rstrip(",").strip()
        word = word.rstrip(TRAILING_PUNCT).strip()
        if not word or word.startswith("#"):
            continue
        if "\t" in word:
            raise UpdateError(f"tab inside word: {word!r}")
        out.append(word)
    return out


def dedup_key(word: str) -> str:
    return unicodedata.normalize("NFKC", word).casefold()


def choose_display(forms: set[str]) -> str:
    return sorted(forms, key=lambda s: (s != s.casefold(), s))[0]


def load_source() -> dict:
    with SOURCE.open("rb") as fh:
        return tomllib.load(fh)


def read_vendored(rel: str) -> bytes:
    path = ROOT / rel
    if not path.is_file():
        raise UpdateError(f"missing {path}")
    return path.read_bytes()


def download(url: str) -> bytes:
    req = urllib.request.Request(url, headers={"User-Agent": "teamgram-usernamepolicy"})
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            return resp.read()
    except Exception as err:
        raise UpdateError(f"download failed {url}: {err}") from err


def raw_url(name: str, commit: str, path: str) -> str:
    quoted = urllib.parse.quote(path, safe="/")
    return f"https://raw.githubusercontent.com/{name}/{commit}/{quoted}"


def load_drops(rel: str) -> dict[str, str]:
    drops: dict[str, str] = {}
    for lineno, line in enumerate(read_vendored(rel).decode("utf-8").splitlines(), 1):
        raw = line.strip()
        if not raw or raw.startswith("#"):
            continue
        parts = raw.split("\t")
        if len(parts) != 2 or not parts[0] or not parts[1]:
            raise UpdateError(f"{rel}:{lineno}: expected word<TAB>reason")
        word, reason = parts
        if word == "word" and reason == "reason":
            continue
        if word in drops:
            raise UpdateError(f"duplicate drop {word}")
        drops[word] = reason
    return drops


def load_block(rel: str) -> set[str]:
    blocked = set()
    for line in read_vendored(rel).decode("utf-8").splitlines():
        raw = line.split("#", 1)[0].strip()
        if raw:
            blocked.add(raw.split()[0].casefold())
    return blocked


def category_rows(meta: dict) -> tuple[dict[str, set[str]], dict[str, set[str]]]:
    forms: dict[str, set[str]] = defaultdict(set)
    sources: dict[str, set[str]] = defaultdict(set)
    for side in ("konsheng", "fwwdn"):
        block = meta[side]
        for item in block["file"]:
            data = read_vendored(item["vendored"])
            got = git_blob_sha1(data)
            if got != item["blob"]:
                raise UpdateError(f"{item['vendored']}: blob {got} != {item['blob']}")
            for word in parse_lines(data.decode("utf-8-sig"), strip_trailing_comma=bool(item.get("strip_trailing_comma"))):
                key = dedup_key(word)
                forms[key].add(word)
                sources[key].add(side)
    return forms, sources


def classify(meta: dict, tencent_keys: set[str]) -> list[dict]:
    forms, sources = category_rows(meta)
    drops = load_drops(meta["editorial"]["drops"])
    if not tencent_keys:
        raise UpdateError("Tencent overlap set is empty")
    rows = []
    seen_drop = set()
    for key, words in forms.items():
        word = choose_display(words)
        srcs = sources[key]
        if srcs == {"konsheng", "fwwdn"}:
            source = "both"
        elif srcs == {"konsheng"}:
            source = "konsheng"
        elif srcs == {"fwwdn"}:
            source = "fwwdn"
        else:
            raise UpdateError(f"unexpected sources {srcs} for {word}")
        if key in tencent_keys:
            decision = "defer-tencent"
        elif word in drops:
            decision = "drop"
            seen_drop.add(word)
        elif source == "both":
            decision = "defer-both"
        else:
            decision = "admit"
        rows.append({"word": word, "key": key, "source": source, "decision": decision})
    known = {r["word"] for r in rows}
    known_keys = {r["key"] for r in rows}
    for word in drops:
        if word not in known and dedup_key(word) not in known_keys:
            raise UpdateError(f"drop list word is not a politics candidate: {word}")
    rows.sort(key=lambda r: r["word"])
    return rows


def overlap_from_file(path: Path) -> set[str]:
    keys = set()
    for line in path.read_text(encoding="utf-8").splitlines():
        raw = line.strip()
        if not raw or raw.startswith("#"):
            continue
        keys.add(raw)
    return keys


def to_pinyin(word: str) -> str:
    from importlib.metadata import PackageNotFoundError, version

    try:
        got = version("pypinyin")
    except PackageNotFoundError:
        raise UpdateError("pypinyin==0.55.0 is required") from None
    if got != PYPINYIN_VERSION:
        raise UpdateError(f"pypinyin {got} != {PYPINYIN_VERSION}")
    from pypinyin import Style, lazy_pinyin

    parts = lazy_pinyin(word, style=Style.NORMAL, errors="default", strict=False)
    return NON_ALNUM.sub("", "".join(parts).casefold())


def pinyin_rules(rows: list[dict], blocked: set[str]) -> list[tuple[str, str]]:
    pairs = []
    seen = {r["word"] for r in rows if r["decision"] == "admit"}
    emitted: list[tuple[str, str]] = []
    for row in rows:
        if row["decision"] != "admit":
            continue
        py = to_pinyin(row["word"])
        pairs.append((row["word"], py))
        if not py or py == row["word"].casefold() or py in blocked or py in seen:
            continue
        if not USERNAME_SAFE.fullmatch(py):
            continue
        seen.add(py)
        emitted.append((row["word"], py))
    return pairs


def write_generated(meta: dict, rows: list[dict], pairs: list[tuple[str, str]], tencent_meta: dict) -> None:
    gen = ROOT / "generated"
    gen.mkdir(exist_ok=True)
    blocked = load_block(meta["editorial"]["pinyin_block"])
    admit = [r["word"] for r in rows if r["decision"] == "admit"]
    seen = set(admit)
    rule_n = 0
    for word, py in pairs:
        if word not in seen:
            continue
        if not py or py == word.casefold() or py in blocked or py in seen:
            continue
        if USERNAME_SAFE.fullmatch(py):
            seen.add(py)
            rule_n += 1
    counts = {
        "admit": sum(r["decision"] == "admit" for r in rows),
        "pinyin_rules": rule_n,
        "defer_tencent": sum(r["decision"] == "defer-tencent" for r in rows),
        "defer_both": sum(r["decision"] == "defer-both" for r in rows),
        "drop": sum(r["decision"] == "drop" for r in rows),
        "drop_list_rows": len(load_drops(meta["editorial"]["drops"])),
        "overlap_keys": tencent_meta["overlap_keys"],
        "tencent_parsed_lines": tencent_meta["parsed_lines"],
        "tencent_unique_keys": tencent_meta["unique_keys"],
        "policy_rules": 0,
    }
    counts["policy_rules"] = counts["admit"] + counts["pinyin_rules"]
    expect = meta["admission"]
    for key, got in counts.items():
        if int(expect[key]) != got:
            raise UpdateError(f"{key}: got {got}, SOURCE.toml admission says {expect[key]}")

    overlap_path = gen / "tencent-overlap-keys.txt"
    # Caller passes the key set through tencent_meta["keys"].
    keys = tencent_meta["keys"]
    overlap_path.write_text("\n".join(keys) + "\n", encoding="utf-8")
    tsv = gen / "admit-pinyin.tsv"
    lines = [f"{word}\t{py}" for word, py in pairs]
    tsv.write_text("\n".join(lines) + "\n", encoding="utf-8")
    payload = {
        "pypinyin": PYPINYIN_VERSION,
        "konsheng_commit": meta["konsheng"]["commit"],
        "fwwdn_commit": meta["fwwdn"]["commit"],
        "tencent_blob": meta["konsheng"]["excluded"][0]["blob"],
        "tencent_file_vendored": False,
        **counts,
    }
    (gen / "coverage.json").write_text(json.dumps(payload, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(counts, ensure_ascii=False))


def tencent_keys_from_bytes(data: bytes) -> tuple[set[str], int, int]:
    words = parse_lines(data.decode("utf-8-sig"), strip_trailing_comma=False)
    keys = {dedup_key(w) for w in words}
    return keys, len(words), len(keys)


def fetch_tencent(meta: dict) -> tuple[set[str], dict]:
    item = meta["konsheng"]["excluded"][0]
    if item["path"] != "Vocabulary/零时-Tencent.txt":
        raise UpdateError("first konsheng exclusion must be the Tencent file")
    data = download(raw_url(meta["konsheng"]["name"], meta["konsheng"]["commit"], item["path"]))
    got = git_blob_sha1(data)
    if got != item["blob"]:
        raise UpdateError(f"Tencent blob {got} != {item['blob']}")
    if len(data) != int(item["bytes"]):
        raise UpdateError(f"Tencent bytes {len(data)} != {item['bytes']}")
    keys, parsed, unique = tencent_keys_from_bytes(data)
    return keys, parsed_meta(keys, parsed, unique)


def parsed_meta(keys: set[str], parsed: int, unique: int) -> dict:
    return {"keys": sorted(keys), "parsed_lines": parsed, "unique_keys": unique, "overlap_keys": 0}


def overlap_against(forms_keys: set[str], tencent_keys: set[str], parsed: int, unique: int) -> dict:
    overlap = forms_keys & tencent_keys
    meta = parsed_meta(overlap, parsed, unique)
    meta["overlap_keys"] = len(overlap)
    meta["keys"] = sorted(overlap)
    return meta


def forms_keys(meta: dict) -> set[str]:
    forms, _ = category_rows(meta)
    return set(forms)


def verify_upstream_blobs(meta: dict) -> None:
    for side in ("konsheng", "fwwdn"):
        block = meta[side]
        lic = read_vendored(block["license_path"])
        got = git_blob_sha1(lic)
        if got != block["license_blob"]:
            raise UpdateError(f"{side} license blob {got} != {block['license_blob']}")
        for item in block["file"]:
            data = download(raw_url(block["name"], block["commit"], item["path"]))
            got = git_blob_sha1(data)
            if got != item["blob"]:
                raise UpdateError(f"{side} {item['path']}: blob {got} != {item['blob']}")
            vendored = read_vendored(item["vendored"])
            if vendored != data:
                raise UpdateError(f"{item['vendored']} is not the pinned bytes")


def load_runtime_defer(rel: str) -> dict[str, str]:
    out: dict[str, str] = {}
    for line in read_vendored(rel).decode("utf-8").splitlines():
        raw = line.strip()
        if not raw or raw.startswith("#"):
            continue
        word, reason = raw.split("\t", 1)
        if word == "word" and reason == "reason":
            continue
        key = dedup_key(word)
        if key in out:
            raise UpdateError(f"duplicate runtime-defer {word}")
        out[key] = reason
    if not out:
        raise UpdateError("runtime-defer is empty")
    return out


def promoted_terms(meta: dict) -> list[str]:
    forms, _ = category_rows(meta)
    drops = load_drops(meta["editorial"]["drops"])
    extra = load_runtime_defer("editorial/runtime-defer.tsv")
    overlap = overlap_from_file(ROOT / meta["generated"]["overlap_keys"])
    drop_keys = {dedup_key(word) for word in drops}
    chosen: list[str] = []
    seen_extra: set[str] = set()
    for key, words in forms.items():
        if key not in overlap:
            continue
        word = choose_display(words)
        if word in drops or key in drop_keys:
            continue
        if key in extra:
            seen_extra.add(key)
            continue
        chosen.append(word)
    missing = set(extra) - seen_extra
    if missing:
        raise UpdateError("runtime-defer is not a Tencent-overlap category line: " + ",".join(sorted(missing)))
    chosen.sort()
    return chosen


def runtime_pinyin_rows(terms: list[str], blocked: set[str]) -> list[tuple[str, str]]:
    # Readings shorter than 8 are exact-only in the old build and were
    # reviewed one by one. New unreviewed short readings stay out.
    # A reading that is already one of the promoted source lines uses that
    # line's own rule.
    term_set = set(terms)
    rows: list[tuple[str, str]] = []
    seen: set[str] = set()
    allow = must_allow_usernames()
    english = english_words()
    for word in terms:
        py = to_pinyin(word)
        if not py or py == word.casefold() or py in term_set or py in blocked or py in seen:
            continue
        if not USERNAME_SAFE.fullmatch(py) or len(py) < 8:
            continue
        if py in allow or py in english:
            continue
        seen.add(py)
        rows.append((word, py))
    return rows


def must_allow_usernames() -> set[str]:
    # Only cn rows. A global must-allow line stays allowed because runtime
    # rules are cn-only. username global xijinping is that case.
    path = ROOT.parent / "corpus" / "must-allow.txt"
    out: set[str] = set()
    for line in path.read_text(encoding="utf-8").splitlines():
        raw = line.strip()
        if not raw or raw.startswith("#"):
            continue
        field, regions, text = raw.split(" ", 2)
        if "cn" not in regions.split(","):
            continue
        if field in {"username", "display_name"}:
            out.add(text.casefold())
    return out


def english_words() -> set[str]:
    path = Path("/usr/share/dict/words")
    if not path.is_file():
        return set()
    return {line.strip().casefold() for line in path.read_text(encoding="utf-8", errors="ignore").splitlines() if line.strip()}


def check_runtime(meta: dict) -> None:
    terms = promoted_terms(meta)
    blocked = load_block(meta["editorial"]["pinyin_block"])
    rows = runtime_pinyin_rows(terms, blocked)
    path = ROOT / "generated" / "runtime-pinyin.tsv"
    want = "".join(f"{word}\t{py}\n" for word, py in rows)
    current = path.read_text(encoding="utf-8") if path.is_file() else ""
    if current != want:
        raise UpdateError("runtime-pinyin.tsv does not match pypinyin 0.55.0; rerun --emit-runtime")
    print(f"runtime terms {len(terms)} pinyin {len(rows)}")


def emit_runtime(meta: dict) -> None:
    terms = promoted_terms(meta)
    blocked = load_block(meta["editorial"]["pinyin_block"])
    rows = runtime_pinyin_rows(terms, blocked)
    path = ROOT / "generated" / "runtime-pinyin.tsv"
    path.write_text("".join(f"{word}\t{py}\n" for word, py in rows), encoding="utf-8")
    print(f"wrote {path} terms {len(terms)} pinyin {len(rows)}")


def main(argv: list[str]) -> int:
    meta = load_source()
    if PYPINYIN_VERSION != meta["pinyin"]["version"]:
        raise UpdateError("script pin and SOURCE.toml pinyin version differ")
    fetch = "--fetch" in argv
    check = "--check" in argv
    if "--emit-runtime" in argv:
        emit_runtime(meta)
        return 0
    if "--check-runtime" in argv:
        check_runtime(meta)
        return 0
    if fetch:
        verify_upstream_blobs(meta)
        tencent_keys, fetched = fetch_tencent(meta)
        forms = forms_keys(meta)
        info = overlap_against(forms, tencent_keys, fetched["parsed_lines"], fetched["unique_keys"])
    else:
        overlap = overlap_from_file(ROOT / meta["generated"]["overlap_keys"])
        info = {"keys": sorted(overlap), "parsed_lines": int(meta["admission"]["tencent_parsed_lines"]), "unique_keys": int(meta["admission"]["tencent_unique_keys"]), "overlap_keys": len(overlap)}
        if not check and "--emit" not in argv:
            raise UpdateError("pass --fetch, --check, or --emit")
    rows = classify(meta, set(info["keys"]))
    blocked = load_block(meta["editorial"]["pinyin_block"])
    pairs = pinyin_rules(rows, blocked)
    if check and not fetch:
        current = (ROOT / meta["generated"]["pinyin_tsv"]).read_text(encoding="utf-8")
        want = "\n".join(f"{w}\t{py}" for w, py in pairs) + "\n"
        if current != want:
            raise UpdateError("admit-pinyin.tsv does not match pypinyin 0.55.0; rerun --fetch or --emit")
        print("check ok")
        return 0
    write_generated(meta, rows, pairs, info)
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main(sys.argv[1:]))
    except UpdateError as err:
        print(f"update failed: {err}", file=sys.stderr)
        raise SystemExit(1)
