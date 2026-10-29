"""Pick the Tellomi simulator .app out of a build directory or an unzipped artifact."""

from __future__ import annotations

import plistlib
import subprocess
import sys
from pathlib import Path

BUNDLE_ID = "app.tellomi.chat"


def is_simulator_binary(executable: Path) -> bool:
    listing = subprocess.check_output(["otool", "-l", str(executable)], text=True, errors="replace")
    return "platform 7" in listing


def select_app(root: Path, *, require_simulator: bool = True) -> Path:
    matches: list[Path] = []
    for app in root.rglob("*.app"):
        plist_path = app / "Info.plist"
        if not plist_path.is_file():
            continue
        with plist_path.open("rb") as handle:
            info = plistlib.load(handle)
        if info.get("CFBundleIdentifier") != BUNDLE_ID:
            continue
        executable = app / str(info.get("CFBundleExecutable") or "")
        if not executable.is_file():
            continue
        if require_simulator and not is_simulator_binary(executable):
            continue
        matches.append(app)
    if not matches:
        kind = "simulator " if require_simulator else ""
        raise SystemExit(
            f"no {kind}{BUNDLE_ID} bundle under {root}. "
            "A device IPA cannot run on the iOS Simulator; build debug_sim_arm64 "
            "or set IOS_SIMULATOR_APP_URL to a simulator .app zip."
        )
    return min(matches, key=lambda path: len(path.parts))


def main() -> None:
    if len(sys.argv) != 2:
        raise SystemExit("usage: select_simulator_app.py ROOT")
    print(select_app(Path(sys.argv[1])))


if __name__ == "__main__":
    main()
