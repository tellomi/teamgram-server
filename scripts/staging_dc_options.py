#!/usr/bin/env python3
"""Allow only the desktop SSH-tunnel DC list.

help.getConfig replaces dc_options with every entry this parser accepts, and
each entry has this_port_only set. Android dials every one of those
addresses. A value that also contains the production address is therefore a
production dial, even when 1:127.0.0.1:10444 is in the same string.

The only accepted staging value is the canonical tunnel spec. Whitespace,
empty items, duplicates, IPv6, and any other address are refused. This script
does not start Docker.
"""

from __future__ import annotations

import os
import sys

TUNNEL = (1, "127.0.0.1", 10444)


def _canonical_int(raw: str, low: int, high: int) -> int:
    if not raw.isdigit() or raw != str(int(raw)):
        raise ValueError(f"{raw!r} is not a canonical integer")
    number = int(raw)
    if number < low or number > high:
        raise ValueError(f"{number} is outside {low}-{high}")
    return number


def _canonical_ipv4(raw: str) -> str:
    parts = raw.split(".")
    if len(parts) != 4:
        raise ValueError(f"{raw!r} is not an IPv4 literal")
    numbers = []
    for part in parts:
        if not part.isdigit() or part != str(int(part)):
            raise ValueError(f"{raw!r} is not a canonical IPv4 literal")
        number = int(part)
        if number > 255:
            raise ValueError(f"{raw!r} is not a canonical IPv4 literal")
        numbers.append(str(number))
    canonical = ".".join(numbers)
    if canonical != raw:
        raise ValueError(f"{raw!r} is not a canonical IPv4 literal")
    return canonical


def parse_entries(spec: str) -> list[tuple[int, str, int]]:
    """Parse dcId:ipv4:port without trimming. Whitespace is an error."""
    if spec == "":
        raise ValueError("empty")
    if any(character.isspace() for character in spec):
        raise ValueError(f"whitespace alias {spec!r}")
    parts = spec.split(",")
    entries: list[tuple[int, str, int]] = []
    for part in parts:
        if part == "":
            raise ValueError("empty entry")
        fields = part.split(":")
        if len(fields) != 3:
            raise ValueError(f"{part!r}: want dc:ipv4:port (IPv6 is refused)")
        dc_id = _canonical_int(fields[0], 1, 65535)
        ip_address = _canonical_ipv4(fields[1])
        port = _canonical_int(fields[2], 1, 65535)
        entries.append((dc_id, ip_address, port))
    return entries


def canonical(entries: list[tuple[int, str, int]]) -> str:
    return ",".join(f"{dc_id}:{ip_address}:{port}" for dc_id, ip_address, port in entries)


def require_desktop_tunnel(spec: str) -> str:
    entries = parse_entries(spec)
    rendered = canonical(entries)
    if spec != rendered or entries != [TUNNEL]:
        raise ValueError(
            "TELLOMI_DC_OPTIONS must be exactly 1:127.0.0.1:10444. "
            f"Parsed {entries}. Mixed production addresses, duplicates, "
            "empty items, IPv6, whitespace, and any other DC are refused. "
            "Android this_port_only dials every published dc_option."
        )
    return rendered


def main() -> int:
    spec = sys.argv[1] if len(sys.argv) > 1 else os.environ.get("TELLOMI_DC_OPTIONS", "")
    try:
        require_desktop_tunnel(spec)
    except ValueError as exc:
        print(f"refusing before any docker command: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
