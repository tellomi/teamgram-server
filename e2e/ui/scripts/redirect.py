"""Point the shipped Android client at staging without rebuilding it.

tellomi/Telegram release APKs dial an IPv4 literal (gradle TELLOMI_PROFILE_DEV,
currently 8.137.39.82:10443). A hosts entry cannot change that. On a rooted
emulator, DNAT sends every TCP connection to the MTProto port to
staging-api.tellomi.cn. The client repositories are not modified.
"""

from __future__ import annotations

import argparse
import os
import socket
import subprocess
import sys


def resolve_host(host: str) -> str:
    infos = socket.getaddrinfo(host, None, socket.AF_INET, socket.SOCK_STREAM)
    if not infos:
        raise OSError(f"no IPv4 address for {host}")
    return infos[0][4][0]


def _is_ipv4(value: str) -> bool:
    parts = value.split(".")
    if len(parts) != 4:
        return False
    for part in parts:
        if not part.isdigit() or (len(part) > 1 and part.startswith("0")):
            return False
        number = int(part)
        if number > 255:
            return False
    return True


def dnat_arguments(staging_ip: str, port: str) -> list[str]:
    if not _is_ipv4(staging_ip):
        raise ValueError(f"expected an IPv4 address, got {staging_ip!r}")
    if not port.isdigit():
        raise ValueError(port)
    return [
        "iptables",
        "-t",
        "nat",
        "-A",
        "OUTPUT",
        "-p",
        "tcp",
        "--dport",
        port,
        "-j",
        "DNAT",
        "--to-destination",
        f"{staging_ip}:{port}",
    ]


def _adb(args: list[str], check: bool = True) -> subprocess.CompletedProcess[str]:
    return subprocess.run(["adb", *args], check=check, text=True, capture_output=True)


def apply_dnat(staging_ip: str, port: str) -> None:
    rule = dnat_arguments(staging_ip, port)
    probe = _adb(["shell", "iptables", "-t", "nat", "-S", "OUTPUT"], check=False)
    if probe.returncode != 0:
        probe = _adb(["shell", "su", "0", "iptables", "-t", "nat", "-S", "OUTPUT"], check=False)
        if probe.returncode != 0:
            detail = (probe.stderr or probe.stdout or "").strip()
            raise SystemExit(
                "adb root iptables is not available, so the release APK would "
                f"keep dialing its baked-in IP instead of {staging_ip}. {detail}"
            )
        prefix = ["shell", "su", "0"]
    else:
        prefix = ["shell"]
    needle = f"--dport {port} -j DNAT --to-destination {staging_ip}:{port}"
    if needle in (probe.stdout or ""):
        print(f"DNAT already sends port {port} to {staging_ip}")
        return
    completed = _adb([*prefix, *rule], check=False)
    if completed.returncode != 0:
        detail = (completed.stderr or completed.stdout or "").strip()
        raise SystemExit(f"iptables DNAT failed: {detail}")
    print(f"DNAT: tcp/{port} -> {staging_ip}:{port}")


def preflight(host: str, port: int, timeout: float = 20.0) -> str:
    ip = resolve_host(host)
    try:
        with socket.create_connection((ip, port), timeout=timeout):
            pass
    except OSError as exc:
        raise SystemExit(
            f"cannot open TCP {host} ({ip}):{port} from this runner: {exc}. "
            "Staging must accept MTProto from the GitHub runner, or set "
            "E2E_ANDROID_RUNNER / E2E_IOS_RUNNER to a self-hosted runner that can."
        ) from exc
    print(f"staging reachable: {host} ({ip}):{port}")
    return ip


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description="Staging network helpers")
    parser.add_argument("--preflight", action="store_true")
    parser.add_argument("--apply", action="store_true")
    args = parser.parse_args(argv)
    host = os.environ.get("E2E_API_HOST", "staging-api.tellomi.cn").strip()
    port = os.environ.get("E2E_API_PORT", "10443").strip()
    if args.preflight:
        preflight(host, int(port))
    if args.apply:
        ip = resolve_host(host)
        apply_dnat(ip, port)
    if not args.preflight and not args.apply:
        parser.error("pass --preflight and/or --apply")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
