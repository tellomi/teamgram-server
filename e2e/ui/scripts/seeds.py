"""Point a local Telegram-iOS checkout at staging. The client repo is not pushed."""

from __future__ import annotations

import json
import sys
from pathlib import Path


def point_ios_seeds(path: Path, host: str) -> None:
    if not host or any(ch.isspace() for ch in host):
        raise ValueError(f"bad seed host {host!r}")
    cfg = json.loads(path.read_text(encoding="utf-8"))
    cfg["seeds"] = {
        "production": {"1": [host]},
        "testing": {"1": [host]},
    }
    path.write_text(json.dumps(cfg, indent=2) + "\n", encoding="utf-8")


def main() -> None:
    if len(sys.argv) != 3:
        raise SystemExit("usage: seeds.py TELLOMI_SERVER_JSON HOST")
    point_ios_seeds(Path(sys.argv[1]), sys.argv[2])


if __name__ == "__main__":
    main()
