"""Point the shipped Android client at staging without rebuilding it.

tellomi/Telegram release APKs dial an IPv4 literal (gradle TELLOMI_PROFILE_DEV,
currently 8.137.39.82:10443). A hosts entry cannot change that. On a rooted
emulator, DNAT sends every TCP connection to that production port to
staging-api.tellomi.cn on the staging MTProto port (default 11443).
The client repositories are not modified.
"""

from __future__ import annotations

import argparse
import os
import socket
import subprocess
import sys

# Port baked into release clients (TELLOMI_PROFILE_DEV / iOS seed_port).
CLIENT_DIAL_PORT = "10443"
# TCP port staging-api.tellomi.cn listens on.
DEFAULT_STAGING_PORT = "11443"


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


def dnat_arguments(
    staging_ip: str,
    staging_port: str,
    client_port: str = CLIENT_DIAL_PORT,
) -> list[str]:
    if not _is_ipv4(staging_ip):
        raise ValueError(f"expected an IPv4 address, got {staging_ip!r}")
    if not str(staging_port).isdigit() or not str(client_port).isdigit():
        raise ValueError(staging_port if not str(staging_port).isdigit() else client_port)
    return [
        "iptables",
        "-t",
        "nat",
        "-A",
        "OUTPUT",
        "-p",
        "tcp",
        "--dport",
        str(client_port),
        "-j",
        "DNAT",
        "--to-destination",
        f"{staging_ip}:{staging_port}",
    ]


def _adb(args: list[str], check: bool = True) -> subprocess.CompletedProcess[str]:
    return subprocess.run(["adb", *args], check=check, text=True, capture_output=True)


def apply_dnat(
    staging_ip: str,
    staging_port: str,
    client_port: str = CLIENT_DIAL_PORT,
) -> None:
    rule = dnat_arguments(staging_ip, staging_port, client_port)
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
    needle = (
        f"--dport {client_port} -j DNAT --to-destination {staging_ip}:{staging_port}"
    )
    if needle in (probe.stdout or ""):
        print(f"DNAT already sends port {client_port} to {staging_ip}:{staging_port}")
        return
    completed = _adb([*prefix, *rule], check=False)
    if completed.returncode != 0:
        detail = (completed.stderr or completed.stdout or "").strip()
        raise SystemExit(f"iptables DNAT failed: {detail}")
    print(f"DNAT: tcp/{client_port} -> {staging_ip}:{staging_port}")


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
    port = os.environ.get("E2E_API_PORT", DEFAULT_STAGING_PORT).strip()
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
