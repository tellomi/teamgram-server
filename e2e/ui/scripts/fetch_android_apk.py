"""Download the latest tellomi/Telegram release APK, or TELLOMI_ANDROID_APK_URL."""

from __future__ import annotations

import json
import os
import sys
import urllib.request
from pathlib import Path

API = "https://api.github.com/repos/tellomi/Telegram/releases/latest"
USER_AGENT = "tellomi-e2e"


def _headers() -> dict[str, str]:
    headers = {"Accept": "application/vnd.github+json", "User-Agent": USER_AGENT}
    token = os.environ.get("TELLOMI_GH_TOKEN", "").strip()
    if token:
        headers["Authorization"] = f"Bearer {token}"
    return headers


def latest_apk_url() -> tuple[str, str]:
    request = urllib.request.Request(API, headers=_headers())
    with urllib.request.urlopen(request, timeout=60) as response:
        payload = json.load(response)
    assets = [item for item in payload.get("assets", []) if str(item.get("name", "")).endswith(".apk")]
    if not assets:
        tag = payload.get("tag_name", "")
        raise SystemExit(f"tellomi/Telegram release {tag} has no .apk asset")
    assets.sort(key=lambda item: int(item.get("size") or 0), reverse=True)
    chosen = assets[0]
    return str(chosen["name"]), str(chosen["browser_download_url"])


def download(dest: Path) -> None:
    override = os.environ.get("TELLOMI_ANDROID_APK_URL", "").strip()
    if override:
        name = override.rstrip("/").rsplit("/", 1)[-1] or "tellomi.apk"
        url = override
    else:
        name, url = latest_apk_url()
    dest.parent.mkdir(parents=True, exist_ok=True)
    print(f"downloading {name}")
    request = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
    with urllib.request.urlopen(request, timeout=180) as response, dest.open("wb") as handle:
        while True:
            block = response.read(1024 * 1024)
            if not block:
                break
            handle.write(block)
    size = dest.stat().st_size
    if size < 1_000_000:
        raise SystemExit(f"downloaded APK is only {size} bytes")
    print(f"apk {dest} ({size} bytes)")


def main() -> None:
    if len(sys.argv) != 2:
        raise SystemExit("usage: fetch_android_apk.py DEST")
    download(Path(sys.argv[1]))


if __name__ == "__main__":
    main()
