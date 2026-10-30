"""Point a local Telegram-iOS checkout at staging. The client repo is not pushed."""

from __future__ import annotations

import json
import sys
from pathlib import Path

# tellomi/Telegram-iOS check_server_config.py locks these production values.
# A staging checkout rewrites the assertions before that check runs.
_PRODUCTION_PORT_ASSERTION = (
    '    if config["seed_port"] != 10443:\n'
    '        fail("seed port is not Teamgram\'s published 10443")'
)
_PRODUCTION_HOST_ASSERTION = (
    '        if hosts != ["api.tellomi.cn"]:\n'
    '            fail(f"{environment} seeds are not api.tellomi.cn")'
)


def _host(host: str) -> str:
    if not host or any(ch.isspace() or ch in "\\\"'" for ch in host):
        raise ValueError(f"bad seed host {host!r}")
    return host


def _port(port: int | str) -> int:
    if isinstance(port, str) and not port.isdigit():
        raise ValueError(port)
    number = int(port)
    if number < 1 or number > 65535:
        raise ValueError(port)
    return number


def point_ios_seeds(path: Path, host: str, port: int | str = 11443) -> None:
    host = _host(host)
    port_number = _port(port)
    cfg = json.loads(path.read_text(encoding="utf-8"))
    cfg["seeds"] = {
        "production": {"1": [host]},
        "testing": {"1": [host]},
    }
    cfg["seed_port"] = port_number
    path.write_text(json.dumps(cfg, indent=2) + "\n", encoding="utf-8")


def allow_staging_in_server_check(source: str, host: str, port: int | str) -> str:
    """Let the CI checkout's server check accept the staging host and port.

    The committed checker requires production host api.tellomi.cn and TCP 10443.
    This rewrite stays in the CI checkout and is never pushed.
    """
    host = _host(host)
    port_number = _port(port)
    new_port = (
        f'    if config["seed_port"] != {port_number}:\n'
        f'        fail("seed port is not the staging port {port_number}")'
    )
    new_host = (
        '        if hosts != ["' + host + '"]:\n'
        '            fail(f"{environment} seeds are not ' + host + '")'
    )
    if _PRODUCTION_PORT_ASSERTION not in source or _PRODUCTION_HOST_ASSERTION not in source:
        raise ValueError("production host or port assertion was not found")
    return (
        source.replace(_PRODUCTION_PORT_ASSERTION, new_port, 1).replace(
            _PRODUCTION_HOST_ASSERTION, new_host, 1
        )
    )


def main() -> None:
    if len(sys.argv) == 5 and sys.argv[1] == "--allow-staging-check":
        path = Path(sys.argv[2])
        updated = allow_staging_in_server_check(
            path.read_text(encoding="utf-8"), sys.argv[3], sys.argv[4]
        )
        path.write_text(updated, encoding="utf-8")
        return
    if len(sys.argv) not in (3, 4):
        raise SystemExit(
            "usage: seeds.py TELLOMI_SERVER_JSON HOST [PORT]\n"
            "       seeds.py --allow-staging-check CHECK_PY HOST PORT"
        )
    port = sys.argv[3] if len(sys.argv) == 4 else 11443
    point_ios_seeds(Path(sys.argv[1]), sys.argv[2], port)


if __name__ == "__main__":
    main()
