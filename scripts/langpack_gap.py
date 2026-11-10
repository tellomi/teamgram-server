#!/usr/bin/env python3
"""Report langpack gaps, including Tellomi keys the clients ship.

The English key-count table is unchanged: exit 1 when any language is missing
more than 1% of the English keys. Extra keys do not fail that check.

Tellomi product keys are a separate check. They are read from the client
string catalogs (Android ``res/values*/strings*.xml``, iOS
``Localizable.strings``) and must exist in every server pack for that
platform. ``data/langpack/tellomi-client-keys.json`` is the list ``go test``
locks. Re-run against client checkouts, including open PR branches:

    python3 scripts/langpack_gap.py \\
        --android /path/to/Telegram \\
        --ios /path/to/Telegram-iOS \\
        --upstream-android /tmp/upstream/android-en.xml \\
        --upstream-ios /tmp/upstream/ios-en.strings \\
        --android-ref pr:134 \\
        --android-ref pr:135 \\
        --ios-ref pr:161 \\
        --ios-ref pr:160 \\
        --ios-ref pr:162 \\
        --apply

The working trees should be Android ``master`` and iOS ``release/ios``.
``--android-ref`` / ``--ios-ref`` add another git ref in that checkout.
``pr:N`` fetches the pull request head (``gh pr view``) and also reads
translation tables in the PR body, which is where zh-Hans / zh-Hant live
when the branch only commits English. A plain ref (branch or SHA) reads
string files only. ``--apply`` writes the packs, the reviewed pins, and the
key list. Without ``--apply`` the command prints the gap and exits 1 when a
key is missing.

Upstream English catalogs (DrKLO/Telegram ``strings.xml`` and
Telegram-iOS ``Localizable.strings``) separate Tellomi keys from strings
Telegram itself still has. Keys that fell out of upstream but are not
Tellomi product copy are listed in ``NOT_PRODUCT`` and are not required.
"""

from __future__ import annotations

import argparse
import gzip
import json
import re
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from langpack_import import (  # noqa: E402
    _NAME_ATTR,
    _NOT_TRANSLATABLE,
    _STRING_TAG,
    unescape_android,
    unescape_ios,
)

PLATFORMS = ("ios", "android", "tdesktop", "macos")
CLIENT_PLATFORMS = ("android", "ios")
MANIFEST_NAME = "tellomi-client-keys.json"

# In the Tellomi tree and absent from current upstream, but not Tellomi copy.
# Icon names and a few strings upstream removed; the server already ships the
# ones that are still official.
NOT_PRODUCT = {
    "AppIconClassic",
    "AppIconClassicBlack",
    "AppIconFilledBlack",
    "NearbyVenue",
    "NoPlacesFoundInfo",
    "PlacesInThisArea",
    "QuickReplyDefault1",
    "QuickReplyDefault2",
    "QuickReplyDefault3",
    "QuickReplyDefault4",
    "TapToSendLocation",
    "RichText.ButtonActionCopy",
}

ANDROID_QUALIFIER = {
    "values": "en",
    "values-ar": "ar",
    "values-de": "de",
    "values-es": "es",
    "values-it": "it",
    "values-ko": "ko",
    "values-nl": "nl",
    "values-pt-rBR": "pt-br",
    "values-ru": "ru",
    "values-uk": "uk",
    "values-zh-rCN": "zh-hans",
    "values-zh-rTW": "zh-hant",
}
IOS_LPROJ = {
    "en": "en",
    "ar": "ar",
    "be": "be",
    "ca": "ca",
    "de": "de",
    "es": "es",
    "fa": "fa",
    "fr": "fr",
    "id": "id",
    "it": "it",
    "ko": "ko",
    "ms": "ms",
    "nl": "nl",
    "pl": "pl",
    "pt": "pt-br",
    "ru": "ru",
    "tr": "tr",
    "uk": "uk",
    "uz": "uz",
    "zh-Hans": "zh-hans",
    "zh-Hant": "zh-hant",
}
ANDROID_RES = "TMessagesProj/src/main/res"
IOS_LOCA = "Telegram/Telegram-iOS"
REPOS = {"android": "tellomi/Telegram", "ios": "tellomi/Telegram-iOS"}

_IOS_ROW = re.compile(r'^"((?:\\.|[^"\\])*)"\s*=\s*"((?:\\.|[^"\\])*)"\s*;', re.M)
_KEY_CELL = re.compile(r"^[A-Za-z][A-Za-z0-9_.]*$")
_PRINTF = re.compile(
    r"%%|%(\d+)\$((?:0?\d+)?)([sdifouxXeEfFgGaAcbBhH@])|%((?:0?\d+)?)([sdifouxXeEfFgGaAcbBhH@])"
)
_LANG_COL = {
    "en": "en",
    "english": "en",
    "zh-hans": "zh-hans",
    "zh-cn": "zh-hans",
    "zh_cn": "zh-hans",
    "zh-hant": "zh-hant",
    "zh-tw": "zh-hant",
    "zh_tw": "zh-hant",
}


def load_doc(path: Path) -> dict:
    with gzip.open(path, "rt", encoding="utf-8") as fh:
        return json.load(fh)


def load_keys(path: Path) -> set[str]:
    return {item["key"] for item in load_doc(path).get("strings") or []}


def write_gz(path: Path, doc: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    raw = json.dumps(doc, ensure_ascii=False, separators=(",", ":")).encode()
    with gzip.open(path, "wb", compresslevel=9) as fh:
        fh.write(raw)


def product_shaped(key: str) -> bool:
    """Tellomi strings, not the whole Login.* catalog.

    Login.Code and its siblings are official strings that also start with
    Login. Only the keys Tellomi added belong here. PR body tables can name
    other keys (SettingsTools, MainTabs*) without this prefix.
    """
    if key.startswith("Tellomi") or key.startswith("Moments"):
        return True
    return key in {
        "Login.CodeFloodWait",
        "Story.Privacy.CategoryMutualFollowers",
        "Story.Privacy.WhoCanViewMoment",
    }


def android_strings(text: str) -> dict[str, str]:
    out: dict[str, str] = {}
    for attrs, body in _STRING_TAG.findall(text):
        if _NOT_TRANSLATABLE.search(attrs):
            continue
        name = _NAME_ATTR.search(attrs)
        if not name:
            continue
        out[name.group(1)] = unescape_android(body)
    return out


def ios_strings(text: str) -> dict[str, str]:
    out: dict[str, str] = {}
    for key, val in _IOS_ROW.findall(text):
        out[unescape_ios(key)] = unescape_ios(val)
    return out


def _cell(text: str) -> str:
    return text.strip().strip("`").strip()


def parse_body_tables(body: str) -> dict[str, dict[str, str]]:
    """Key to language to value, from markdown tables in a pull request body.

    A table counts when it has a Key column and an English column. zh-CN /
    zh-Hans map to zh-hans, zh-TW / zh-Hant map to zh-hant. Handler tables
    have no Key column and are ignored.
    """
    found: dict[str, dict[str, str]] = {}
    lines = (body or "").splitlines()
    i = 0
    while i < len(lines):
        header = _split_row(lines[i])
        if header is None or i + 1 >= len(lines):
            i += 1
            continue
        cols = {_cell(cell).lower(): idx for idx, cell in enumerate(header)}
        if "key" not in cols or not any(_LANG_COL.get(name) == "en" for name in cols):
            i += 1
            continue
        if _split_row(lines[i + 1]) is None or not set(_cell(lines[i + 1]).replace("|", "").replace("-", "").replace(":", "").strip()):
            # separator row is optional; require the --- row so prose pipes do not match
            sep = _split_row(lines[i + 1])
            if sep is None or not all(set(_cell(cell)) <= set("-: ") for cell in sep):
                i += 1
                continue
        lang_at = []
        for name, idx in cols.items():
            code = _LANG_COL.get(name)
            if code:
                lang_at.append((idx, code))
        i += 2
        while i < len(lines):
            row = _split_row(lines[i])
            if row is None:
                break
            key = _cell(row[cols["key"]]) if cols["key"] < len(row) else ""
            if _KEY_CELL.match(key):
                bucket = found.setdefault(key, {})
                for idx, code in lang_at:
                    if idx < len(row):
                        value = _cell(row[idx])
                        if value and value.lower() not in {"en", "zh-hans", "zh-hant"}:
                            bucket[code] = value
            i += 1
    return found


def _split_row(line: str) -> list[str] | None:
    stripped = line.strip()
    if not stripped.startswith("|"):
        return None
    parts = stripped.strip("|").split("|")
    return parts


def printf_kind(conv: str) -> str:
    if conv in "dDioxXu":
        return "int"
    if conv in "fFeEgGaA":
        return "float"
    if conv in "sSbBhH":
        return "str"
    if conv == "@":
        return "at"
    if conv in "cC":
        return "char"
    return conv


def printf_args(text: str) -> list[tuple[int, str]]:
    out = []
    ordinary = 0
    for match in _PRINTF.finditer(text or ""):
        if match.group(0) == "%%":
            continue
        if match.group(1):
            idx = int(match.group(1))
            conv = match.group(3)
        else:
            ordinary += 1
            idx = ordinary
            conv = match.group(5)
        out.append((idx, printf_kind(conv)))
    return out


def printf_accepts(conv_kind: str, arg_kind: str) -> bool:
    if conv_kind == "str":
        return arg_kind in {"str", "int", "float", "char"}
    return conv_kind == arg_kind


# Android keys whose English is shared with another row, so a one-to-one
# match is ambiguous. The iOS string is the traditional text to copy.
ANDROID_IOS_HANT = {
    "Moments_Publish": "Moments.Publish",
    "Moments_Composer_Title": "Moments.Composer.Title",
    "Moments_Segment_Following": "Moments.Segment.Following",
    "Moments_FollowingState": "Moments.FollowingState",
    "Moments_Title": "Moments.Title",
    "Moments_Search": "Moments.Search",
    "Moments_WhoCanSee": "Moments.WhoCanSee",
}
# Written for review. The clients ship these two keys in English only.
STORY_ZH = {
    "Story.Privacy.CategoryMutualFollowers": {"zh-hans": "互关好友", "zh-hant": "互關好友"},
    "Story.Privacy.WhoCanViewMoment": {"zh-hans": "谁可以看", "zh-hant": "誰可以看"},
}


def _norm_en(text: str) -> str:
    return re.sub(r"\s+", " ", (text or "").strip().lower()).rstrip(".")


def _adapt_ios_hant(android_en: str, ios_hant: str) -> str | None:
    """Use the iOS traditional string if Android's placeholders still fit.

    iOS uses %@ where Android uses %s. Other mismatches stay on OpenCC.
    """
    text = (ios_hant or "").replace("%@", "%s")
    if not text or not placeholders_ok(android_en, text):
        return None
    return text


def placeholders_ok(english: str, translated: str) -> bool:
    args: dict[int, str] = {}
    for idx, kind in printf_args(english):
        # An int argument wins, matching pkg/langpack android_format_test.
        if idx not in args or kind == "int":
            args[idx] = kind
    for idx, kind in printf_args(translated):
        have = args.get(idx)
        if have is None or not printf_accepts(kind, have):
            return False
    return True


def english_gap(root: Path) -> bool:
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
    return failed


def manifest_path(root: Path) -> Path:
    return root / MANIFEST_NAME


def load_manifest(root: Path) -> dict | None:
    path = manifest_path(root)
    if not path.exists():
        return None
    return json.loads(path.read_text(encoding="utf-8"))


def manifest_keys(doc: dict, platform: str) -> list[str]:
    block = doc.get(platform) or {}
    keys = set(block.get("base") or [])
    for names in (block.get("pull_requests") or {}).values():
        keys.update(names)
    return sorted(keys)


ALLOWLIST_NAME = "tellomi-english-allowlist.json"
# Whole values that are names. A sentence is not a brand, so this cannot
# hide an untranslated phrase. Documented in the allowlist file.
BRAND_VALUES = {"PDF", "Tellomi", "TELLOMI"}


def load_allowlist(root: Path) -> dict | None:
    path = root / ALLOWLIST_NAME
    if not path.exists():
        return None
    return json.loads(path.read_text(encoding="utf-8"))


def tellomi_not_english(root: Path) -> bool:
    """Fail when a Tellomi value equals its English string and is not allowlisted.

    data/langpack/tellomi-english-allowlist.json is the list. brands match the
    whole value (PDF, Tellomi). entries name one platform, language, and key,
    and the reason says why that language keeps the English word. A stale
    entry, whose pack value is no longer that English string, also fails.
    """
    doc = load_manifest(root)
    allow = load_allowlist(root)
    if doc is None or allow is None:
        print("tellomi english: manifest or allowlist missing")
        return True
    brands = set(allow.get("brands") or [])
    if not brands <= BRAND_VALUES or not brands:
        print(f"tellomi english: brand list {sorted(brands)} is not the documented set")
        return True
    allowed: dict[tuple[str, str, str], dict] = {}
    for row in allow.get("entries") or []:
        reason = (row.get("reason") or "").strip()
        if not reason or not row.get("platform") or not row.get("code") or not row.get("key"):
            print(f"tellomi english: entry missing a field {row}")
            return True
        allowed[(row["platform"], row["code"], row["key"])] = row
    failed = False
    seen_entries: set[tuple[str, str, str]] = set()
    print("| platform | code | tellomi | equal English | allowlisted |")
    print("|---|---|---:|---:|---:|")
    for platform in CLIENT_PLATFORMS:
        need = manifest_keys(doc, platform)
        if not need:
            continue
        en_doc = load_doc(root / platform / "en.json.gz")
        en_text = {item["key"]: item.get("value", "") for item in en_doc.get("strings") or []}
        for path in sorted((root / platform).glob("*.json.gz")):
            code = path.name[: -len(".json.gz")]
            pack = load_doc(path)
            text = {item["key"]: item.get("value", "") for item in pack.get("strings") or []}
            equal = 0
            allow_n = 0
            for key in need:
                english = en_text.get(key, "")
                value = text.get(key, "")
                if code == "en":
                    continue
                if value != english:
                    continue
                equal += 1
                row = allowed.get((platform, code, key))
                if value in brands:
                    allow_n += 1
                    if row is not None:
                        seen_entries.add((platform, code, key))
                        if row.get("value") != value:
                            failed = True
                            print(f"allowlist value drifted {platform}/{code} {key}")
                    continue
                if row is None or row.get("value") != value:
                    failed = True
                    print(f"tellomi still English {platform}/{code} {key}: {value!r}")
                    continue
                seen_entries.add((platform, code, key))
                allow_n += 1
            if code != "en":
                print(f"| {platform} | {code} | {len(need)} | {equal} | {allow_n} |")
    for key, row in allowed.items():
        if key in seen_entries:
            continue
        failed = True
        print(f"allowlist entry is not an English copy anymore: {row}")
    if not failed:
        print("tellomi english: every non-English pack differs from English, except the allowlist")
    return failed


def manifest_missing(root: Path) -> bool:
    doc = load_manifest(root)
    if doc is None:
        print("tellomi client keys: no manifest, skip")
        return False
    failed = False
    for platform in CLIENT_PLATFORMS:
        need = manifest_keys(doc, platform)
        if not need:
            continue
        folder = root / platform
        for path in sorted(folder.glob("*.json.gz")):
            code = path.name[: -len(".json.gz")]
            have = load_keys(path)
            missing = [key for key in need if key not in have]
            if missing:
                failed = True
                print(f"tellomi keys missing {platform}/{code}: {len(missing)} e.g. {missing[:8]}")
    if not failed:
        print("tellomi client keys: every manifest key is in every pack")
    return failed


def _read_text(checkout: Path, rev: str | None, rel: str) -> str | None:
    if rev is None:
        path = checkout / rel
        if not path.is_file():
            return None
        return path.read_text(encoding="utf-8", errors="replace")
    try:
        return subprocess.check_output(
            ["git", "show", f"{rev}:{rel}"],
            cwd=checkout,
            stderr=subprocess.DEVNULL,
        ).decode("utf-8", errors="replace")
    except subprocess.CalledProcessError:
        return None


def _list_rels(checkout: Path, rev: str | None, platform: str) -> list[tuple[str, str]]:
    """(server code, path relative to the checkout) for string catalogs."""
    rows = []
    if platform == "android":
        prefix = ANDROID_RES
        if rev is None:
            root = checkout / prefix
            if not root.is_dir():
                return []
            for folder, code in ANDROID_QUALIFIER.items():
                path = root / folder / "strings.xml"
                if path.is_file():
                    rows.append((code, f"{prefix}/{folder}/strings.xml"))
            return rows
        listing = subprocess.check_output(
            ["git", "ls-tree", "-r", "--name-only", rev, prefix],
            cwd=checkout,
            text=True,
        )
        for rel in listing.splitlines():
            if not rel.endswith("/strings.xml"):
                continue
            folder = rel.split("/")[-2]
            code = ANDROID_QUALIFIER.get(folder)
            if code:
                rows.append((code, rel))
        return rows
    prefix = IOS_LOCA
    if rev is None:
        root = checkout / prefix
        if not root.is_dir():
            return []
        for folder, code in IOS_LPROJ.items():
            path = root / f"{folder}.lproj" / "Localizable.strings"
            if path.is_file():
                rows.append((code, f"{prefix}/{folder}.lproj/Localizable.strings"))
        return rows
    listing = subprocess.check_output(
        ["git", "ls-tree", "-r", "--name-only", rev, prefix],
        cwd=checkout,
        text=True,
    )
    for rel in listing.splitlines():
        if not rel.endswith(".lproj/Localizable.strings"):
            continue
        folder = rel.split("/")[-2][: -len(".lproj")]
        code = IOS_LPROJ.get(folder)
        if code:
            rows.append((code, rel))
    return rows


def load_client_maps(checkout: Path, rev: str | None, platform: str) -> dict[str, dict[str, str]]:
    parser = android_strings if platform == "android" else ios_strings
    maps: dict[str, dict[str, str]] = {}
    for code, rel in _list_rels(checkout, rev, platform):
        text = _read_text(checkout, rev, rel)
        if text is None:
            continue
        bucket = maps.setdefault(code, {})
        bucket.update(parser(text))
    return maps


def resolve_ref(platform: str, ref: str, checkout: Path) -> tuple[str, str, str]:
    """Return (label, git sha, pr body). ``pr:N`` uses gh and fetches the head."""
    if ref.startswith("pr:"):
        number = ref.split(":", 1)[1]
        if not number.isdigit():
            raise SystemExit(f"bad pull request ref {ref}")
        raw = subprocess.check_output(
            [
                "gh",
                "pr",
                "view",
                number,
                "--repo",
                REPOS[platform],
                "--json",
                "headRefName,body,state",
            ],
            text=True,
        )
        meta = json.loads(raw)
        branch = meta["headRefName"]
        subprocess.check_call(
            ["git", "fetch", "--depth", "1", "origin", branch],
            cwd=checkout,
        )
        sha = subprocess.check_output(
            ["git", "rev-parse", "FETCH_HEAD"],
            cwd=checkout,
            text=True,
        ).strip()
        return f"pr:{number}", sha, meta.get("body") or ""
    subprocess.check_call(["git", "rev-parse", "--verify", ref], cwd=checkout)
    sha = subprocess.check_output(["git", "rev-parse", ref], cwd=checkout, text=True).strip()
    return ref, sha, ""


def upstream_keys(path: Path | None, platform: str) -> set[str] | None:
    if path is None:
        return None
    text = path.read_text(encoding="utf-8", errors="replace")
    data = android_strings(text) if platform == "android" else ios_strings(text)
    return set(data)


def select_base_keys(english: dict[str, str], upstream: set[str] | None) -> list[str]:
    if upstream is None:
        keys = [key for key in english if product_shaped(key)]
    else:
        keys = [key for key in english if key not in upstream and key not in NOT_PRODUCT]
    return sorted(keys)


def android_safe(key: str, value: str) -> bool:
    from langpack_official import android_entry_safe

    return android_entry_safe({"key": key, "value": value})


class Choice:
    def __init__(self, value: str, source: str) -> None:
        self.value = value
        self.source = source  # client | pr-body | english | placeholder


def choose_value(
    platform: str,
    key: str,
    english: str,
    client: str | None,
    body: str | None,
) -> Choice:
    """Prefer a client string, then a PR-body translation, then English."""
    # A PR body table is the explicit translation for that key. A client
    # string fills languages the table does not list.
    for value, source in ((body, "pr-body"), (client, "client")):
        if not value:
            continue
        if not placeholders_ok(english, value):
            continue
        if platform == "android" and not android_safe(key, value):
            continue
        return Choice(value, source)
    if client or body:
        # A translation existed but would break String.format or Android XML.
        return Choice(english, "placeholder")
    return Choice(english, "english")


def pack_codes(root: Path, platform: str) -> list[str]:
    folder = root / platform
    return sorted(path.name[: -len(".json.gz")] for path in folder.glob("*.json.gz"))


def apply_to_pack(path: Path, chosen: dict[str, Choice]) -> tuple[int, int, int]:
    """Insert missing keys and refresh values that changed. Returns added, updated, version."""
    doc = load_doc(path)
    by_key = {item["key"]: item for item in doc["strings"]}
    added = 0
    updated = 0
    pending: list[tuple[str, str]] = []
    for key, choice in chosen.items():
        item = by_key.get(key)
        if item is None:
            pending.append((key, choice.value))
            added += 1
            continue
        current = item.get("value", "")
        if current != choice.value:
            pending.append((key, choice.value))
            updated += 1
    if not pending:
        return 0, 0, int(doc.get("version") or 1)
    new_version = int(doc.get("version") or 1) + 1
    fresh = {key: value for key, value in pending}
    for item in doc["strings"]:
        if item["key"] in fresh and item["key"] in by_key and by_key[item["key"]] is item:
            # updated in place; new keys are not in the list yet
            if "value" in item or "zero" not in item:
                item["value"] = fresh[item["key"]]
                item["version"] = new_version
                fresh.pop(item["key"])
    for key, value in sorted(fresh.items()):
        doc["strings"].append({"key": key, "version": new_version, "value": value})
    doc["version"] = new_version
    write_gz(path, doc)
    return added, updated, new_version


def apply_reviewed(root: Path, platform: str, code: str, chosen: dict[str, Choice], version: int) -> None:
    path = root / "reviewed" / platform / f"{code}.json.gz"
    if path.exists():
        doc = load_doc(path)
    else:
        doc = {
            "lang_code": code,
            "note": "Pinned Tellomi client strings. Reapplied by scripts/langpack_official.py.",
            "strings": [],
        }
    by_key = {item["key"]: item for item in doc.get("strings") or []}
    for key, choice in chosen.items():
        item = by_key.get(key)
        if item is None:
            doc["strings"].append({"key": key, "pin": True, "version": version, "value": choice.value})
            continue
        item["value"] = choice.value
        item["version"] = version
        item["pin"] = True
    write_gz(path, doc)


def update_source(root: Path) -> None:
    path = root / "SOURCE.json"
    if not path.exists():
        return
    manifest = json.loads(path.read_text(encoding="utf-8"))
    packs = manifest.setdefault("packs", {})
    for platform in CLIENT_PLATFORMS:
        for code in pack_codes(root, platform):
            pack_path = root / platform / f"{code}.json.gz"
            doc = load_doc(pack_path)
            entry = packs.get(f"{platform}/{code}") or {}
            entry["strings"] = len(doc["strings"])
            entry["bytes_gz"] = pack_path.stat().st_size
            entry["version"] = doc.get("version")
            packs[f"{platform}/{code}"] = entry
    path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def _group_fallbacks(rows: list[tuple[str, list[str]]]) -> list[tuple[list[str], list[str]]]:
    grouped: dict[tuple[str, ...], list[str]] = {}
    for code, keys in rows:
        grouped.setdefault(tuple(keys), []).append(code)
    return [(codes, list(keys)) for keys, codes in grouped.items()]


def client_main(args: argparse.Namespace, root: Path) -> int:
    if bool(args.android) != bool(args.ios):
        print("pass both --android and --ios checkouts", file=sys.stderr)
        return 1
    if args.apply and (not args.android or not args.ios):
        print("--apply needs --android and --ios", file=sys.stderr)
        return 1
    checkouts = {"android": args.android, "ios": args.ios}
    upstreams = {
        "android": upstream_keys(args.upstream_android, "android"),
        "ios": upstream_keys(args.upstream_ios, "ios"),
    }
    refs = {"android": args.android_ref or [], "ios": args.ios_ref or []}
    summary: dict[str, dict] = {}
    manifest = {
        "note": "Tellomi product keys that must exist in every server langpack. "
        "base is master / release/ios. pull_requests lists the keys named by that ref "
        "(a PR body table when it has one, otherwise keys that are not on the base client).",
        "rerun": "python3 scripts/langpack_gap.py --android <Telegram> --ios <Telegram-iOS> "
        "--upstream-android <DrKLO strings.xml> --upstream-ios <Telegram-iOS Localizable.strings> "
        "--android-ref pr:134 --android-ref pr:135 "
        "--ios-ref pr:161 --ios-ref pr:160 --ios-ref pr:162",
    }
    any_missing = False
    for platform in CLIENT_PLATFORMS:
        checkout: Path = checkouts[platform]
        if not checkout or not checkout.is_dir():
            print(f"missing {platform} checkout {checkout}", file=sys.stderr)
            return 1
        base_maps = load_client_maps(checkout, None, platform)
        english = base_maps.get("en") or {}
        if len(english) < 1000:
            print(f"{platform} English catalog has {len(english)} strings", file=sys.stderr)
            return 1
        base_keys = select_base_keys(english, upstreams[platform])
        base_set = set(base_keys)
        pr_keys: dict[str, list[str]] = {}
        pr_delta: dict[str, list[str]] = {}
        # code -> key -> (value, source label)
        overlay: dict[str, dict[str, tuple[str, str]]] = {}
        body_by_key: dict[str, dict[str, str]] = {}
        for ref in refs[platform]:
            label, sha, body = resolve_ref(platform, ref, checkout)
            extra_maps = load_client_maps(checkout, sha, platform)
            tables = parse_body_tables(body)
            for key, langs in tables.items():
                body_by_key.setdefault(key, {}).update(langs)
            extra_en = extra_maps.get("en") or {}
            fresh = [
                key
                for key in extra_en
                if key not in english and key not in NOT_PRODUCT and (product_shaped(key) or key in tables)
            ]
            # Keys that exist only in the PR body (the branch may not have
            # committed the translation, or the name is not Moments/Tellomi).
            for key in tables:
                if key not in english and key not in NOT_PRODUCT and key not in fresh:
                    fresh.append(key)
            fresh = sorted(set(fresh))
            # The PR body is the key list the test locks, including keys that
            # already exist on the base client (PR 162's 105 Moments strings).
            listed = sorted(set(tables) - NOT_PRODUCT) if tables else fresh
            pr_keys[label] = listed
            pr_delta[label] = fresh
            for key in fresh:
                base_set.add(key)
            wanted = set(fresh) | (set(tables) & set(base_keys))
            for code, strings in extra_maps.items():
                bucket = overlay.setdefault(code, {})
                for key in wanted:
                    if key in strings:
                        bucket[key] = (strings[key], label)
        # English catalog after overlays, so PR-only keys have an English source.
        full_english = dict(english)
        for key, (value, _src) in overlay.get("en", {}).items():
            full_english[key] = value
        for key, langs in body_by_key.items():
            if key not in full_english and langs.get("en"):
                full_english[key] = langs["en"]
        all_keys = sorted(base_set)
        missing_base = []
        packs = {code: load_keys(root / platform / f"{code}.json.gz") for code in pack_codes(root, platform)}
        en_pack = packs.get("en", set())
        for key in base_keys:
            if key not in en_pack:
                missing_base.append(key)
        pr_missing = {label: [key for key in keys if key not in en_pack] for label, keys in pr_keys.items()}
        if missing_base or any(pr_missing.values()):
            any_missing = True
        print(f"\n{platform}: base product keys {len(base_keys)}, missing from server en {len(missing_base)}")
        for label, keys in pr_keys.items():
            delta = pr_delta.get(label, [])
            print(
                f"  {label}: {len(keys)} keys listed, {len(delta)} not on the base client, "
                f"missing from server en {len(pr_missing[label])}"
            )
        per_lang = []
        fallback_rows = []
        for code in pack_codes(root, platform):
            chosen: dict[str, Choice] = {}
            for key in all_keys:
                eng = full_english.get(key)
                if not eng:
                    print(f"{platform} {key} has no English source", file=sys.stderr)
                    return 1
                client_value = (base_maps.get(code) or {}).get(key)
                over = overlay.get(code, {}).get(key)
                if over:
                    client_value = over[0]
                body_value = (body_by_key.get(key) or {}).get(code)
                # The base client wins when the extra ref did not mention the key.
                if client_value is None:
                    client_value = (base_maps.get(code) or {}).get(key)
                chosen[key] = choose_value(platform, key, eng, client_value, body_value)
            fallbacks = sorted(key for key, choice in chosen.items() if choice.source in {"english", "placeholder"})
            client_n = len(all_keys) - len(fallbacks)
            base_miss = sum(1 for key in base_keys if key not in packs[code])
            pr_miss = sum(1 for keys in pr_keys.values() for key in keys if key not in packs[code])
            per_lang.append((code, base_miss, pr_miss, client_n, len(fallbacks)))
            fallback_rows.append((code, fallbacks, {key: chosen[key].source for key in fallbacks}))
            if args.apply:
                added, updated, version = apply_to_pack(root / platform / f"{code}.json.gz", chosen)
                apply_reviewed(root, platform, code, chosen, version)
                print(f"  wrote {platform}/{code}: +{added} updated {updated} v{version} fallback {len(fallbacks)}")
        print(f"| {platform} code | base missing | pr missing | from client | english fallback |")
        print("|---|---:|---:|---:|---:|")
        for code, base_miss, pr_miss, client_n, fall_n in per_lang:
            print(f"| {platform} {code} | {base_miss} | {pr_miss} | {client_n} | {fall_n} |")
        print(f"\n{platform} English fallbacks (no client translation, or a translation whose placeholders were unsafe):")
        grouped: dict[tuple[str, ...], list[str]] = {}
        sources: dict[tuple[str, ...], dict[str, str]] = {}
        for code, keys, src in fallback_rows:
            stamp = tuple(keys)
            grouped.setdefault(stamp, []).append(code)
            sources[stamp] = src
        for stamp, codes in grouped.items():
            placeholder = [key for key in stamp if sources[stamp].get(key) == "placeholder"]
            print(f"  {', '.join(codes)}: {len(stamp)} keys")
            if placeholder:
                print(f"    placeholder fallback: {', '.join(placeholder)}")
            if len(codes) == len(per_lang) and len(stamp) == len(all_keys):
                print("    entire catalog (no built-in locale)")
            elif len(stamp) <= 40:
                print("    " + ", ".join(stamp))
            else:
                print("    " + ", ".join(stamp[:40]) + f", … (+{len(stamp) - 40})")
        manifest[platform] = {"base": base_keys, "pull_requests": {label: keys for label, keys in pr_keys.items()}}
        summary[platform] = {
            "base": len(base_keys),
            "base_missing_en": len(missing_base),
            "pull_requests": {
                label: {
                    "listed": len(keys),
                    "not_on_base": len(pr_delta.get(label, [])),
                    "missing_en": len(pr_missing[label]),
                }
                for label, keys in pr_keys.items()
            },
        }
    print("\nsummary " + json.dumps(summary, ensure_ascii=False))
    if args.apply:
        manifest_path(root).write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        update_source(root)
        print(f"wrote {manifest_path(root)}")
    return 1 if any_missing and not args.apply else 0


def _opencc_s2twp():
    try:
        import opencc
    except ImportError:
        print("opencc is required (pip install opencc-python-reimplemented)", file=sys.stderr)
        return None
    return opencc.OpenCC("s2twp")


def traditional_android(android_en: dict[str, str], android_hans: dict[str, str], ios_en: dict[str, str], ios_hant: dict[str, str], keys: list[str]) -> dict[str, tuple[str, str]]:
    """Traditional text for Android keys that are still the English string.

    Prefer one iOS #162 traditional string when the English matches exactly
    one Android key and one iOS key, or an explicit pair. Otherwise OpenCC
    s2twp of the simplified string.
    """
    converter = _opencc_s2twp()
    if converter is None:
        raise SystemExit(1)
    ios_moments = {key: value for key, value in ios_en.items() if key.startswith("Moments")}
    by_en: dict[str, list[str]] = {}
    for key, value in ios_moments.items():
        by_en.setdefault(_norm_en(value), []).append(key)
    android_by_en: dict[str, list[str]] = {}
    for key in keys:
        android_by_en.setdefault(_norm_en(android_en.get(key, "")), []).append(key)
    out: dict[str, tuple[str, str]] = {}
    for key in keys:
        hans = android_hans.get(key, "")
        eng = android_en.get(key, "")
        if not hans:
            continue
        ios_key = ANDROID_IOS_HANT.get(key)
        source = ""
        text = None
        if ios_key:
            text = _adapt_ios_hant(eng, ios_hant.get(ios_key, ""))
            source = f"ios:{ios_key}" if text else ""
        elif key not in ANDROID_IOS_HANT:
            norm = _norm_en(eng)
            ios_hits = by_en.get(norm) or []
            android_hits = android_by_en.get(norm) or []
            if len(ios_hits) == 1 and len(android_hits) == 1:
                text = _adapt_ios_hant(eng, ios_hant.get(ios_hits[0], ""))
                source = f"ios:{ios_hits[0]}" if text else ""
        if not text:
            text = converter.convert(hans)
            source = "opencc:s2twp"
        if not placeholders_ok(eng, text):
            text = hans
            source = "opencc-kept-hans"
        out[key] = (text, source)
    return out


def _replace_values(path: Path, updates: dict[str, str]) -> int:
    """Replace string values in place. Pack version and string versions stay."""
    doc = load_doc(path)
    changed = 0
    for item in doc.get("strings") or []:
        new = updates.get(item.get("key"))
        if new is None or item.get("value") == new:
            continue
        item["value"] = new
        changed += 1
    if changed:
        write_gz(path, doc)
    return changed


def fill_traditional(root: Path) -> int:
    manifest = load_manifest(root)
    if manifest is None:
        print("missing tellomi client key manifest", file=sys.stderr)
        return 1
    android_keys = manifest_keys(manifest, "android")
    ios_keys = manifest_keys(manifest, "ios")

    def pack_text(platform: str, code: str) -> dict[str, str]:
        doc = load_doc(root / platform / f"{code}.json.gz")
        return {item["key"]: item.get("value", "") for item in doc.get("strings") or []}

    a_en, a_hans, a_hant = pack_text("android", "en"), pack_text("android", "zh-hans"), pack_text("android", "zh-hant")
    i_en, i_hans, i_hant = pack_text("ios", "en"), pack_text("ios", "zh-hans"), pack_text("ios", "zh-hant")
    fallback = [key for key in android_keys if key.startswith("Moments") and a_hant.get(key) == a_en.get(key)]
    chosen = traditional_android(a_en, a_hans, i_en, i_hant, fallback)
    ios_n = sum(1 for _text, source in chosen.values() if source.startswith("ios:"))
    opencc_n = sum(1 for _text, source in chosen.values() if source.startswith("opencc"))
    print(f"android zh-hant: {len(chosen)} Moments keys, {ios_n} from iOS traditional, {opencc_n} OpenCC s2twp")
    updates = {key: text for key, (text, _source) in chosen.items() if text != a_hant.get(key)}
    pack_n = _replace_values(root / "android" / "zh-hant.json.gz", updates)
    reviewed_n = _replace_values(root / "reviewed" / "android" / "zh-hant.json.gz", updates)
    print(f"  wrote android zh-hant pack {pack_n}, reviewed {reviewed_n}")

    story_pack = 0
    for code in ("zh-hans", "zh-hant"):
        story_updates = {}
        current = i_hans if code == "zh-hans" else i_hant
        for key, texts in STORY_ZH.items():
            if current.get(key) == i_en.get(key):
                story_updates[key] = texts[code]
        story_pack += _replace_values(root / "ios" / f"{code}.json.gz", story_updates)
        _replace_values(root / "reviewed" / "ios" / f"{code}.json.gz", story_updates)
        print(f"  ios {code} story keys written: {list(story_updates)}")
    if story_pack == 0 and not updates:
        print("traditional fill: nothing to change")
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Langpack gap versus English, and Tellomi client keys")
    parser.add_argument("--android", type=Path, help="tellomi/Telegram checkout on master")
    parser.add_argument("--ios", type=Path, help="tellomi/Telegram-iOS checkout on release/ios")
    parser.add_argument("--upstream-android", type=Path, help="DrKLO/Telegram values/strings.xml")
    parser.add_argument("--upstream-ios", type=Path, help="Telegram-iOS en.lproj/Localizable.strings")
    parser.add_argument("--android-ref", action="append", default=[], help="extra Android ref, or pr:N")
    parser.add_argument("--ios-ref", action="append", default=[], help="extra iOS ref, or pr:N")
    parser.add_argument("--apply", action="store_true", help="write packs, reviewed pins, and the key manifest")
    parser.add_argument(
        "--fill-traditional",
        action="store_true",
        help="fill Android zh-hant Moments keys and the two iOS story keys without bumping versions",
    )
    parser.add_argument("--root", type=Path, default=None, help="langpack directory (default data/langpack)")
    args = parser.parse_args(argv)
    root = args.root or Path(__file__).resolve().parent.parent / "data" / "langpack"
    if args.fill_traditional:
        return fill_traditional(root)
    failed = False
    if args.android or args.ios or args.apply:
        if client_main(args, root) != 0:
            failed = True
    if english_gap(root):
        failed = True
    if manifest_missing(root):
        failed = True
    if tellomi_not_english(root):
        failed = True
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
