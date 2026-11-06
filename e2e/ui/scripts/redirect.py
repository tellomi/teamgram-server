"""Point the shipped Android client at an already confirmed staging endpoint.

tellomi/Telegram release APKs dial an IPv4 literal (gradle TELLOMI_PROFILE_DEV,
port 10443). A hosts entry cannot change that. On a rooted emulator, DNAT
can send that dial to an explicit staging host. This file does not choose
the host. It does not rewrite a missing DNS name to a production address.
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
# Host ports published by docker-compose.staging.yaml. Not a deploy claim.
STAGING_HOST_TCP = "10444"
STAGING_HOST_WS = "11444"
# Ports the non-staging compose file publishes. Acceptance must not dial them.
REFUSED_PORTS = frozenset({CLIENT_DIAL_PORT, "5222", "11443"})
REFUSED_HOSTS = frozenset({"api.tellomi.cn", "chat.tellomi.cn"})
CONFIRMED_VALUE = "tellomi-staging"


def require_confirmed_endpoint() -> tuple[str, str]:
    """Fail closed unless the operator named an isolated staging endpoint.

    There is no default host and no default port. A name that does not
    resolve stays unresolved; nothing substitutes another address.
    """
    host = os.environ.get("E2E_API_HOST", "").strip().rstrip(".")
    port = os.environ.get("E2E_API_PORT", "").strip()
    confirmed = os.environ.get("E2E_STAGING_CONFIRMED", "").strip()
    if confirmed != CONFIRMED_VALUE:
        raise SystemExit(
            "E2E_STAGING_CONFIRMED must be tellomi-staging, and only after "
            "the endpoint is verified as the isolated tellomi-staging project "
            "(its own data directory, not the production stack). Refusing to dial."
        )
    if not host:
        raise SystemExit(
            "E2E_API_HOST is required. This harness has no default host and "
            "does not rewrite a missing DNS name to another address."
        )
    if host.lower() in REFUSED_HOSTS:
        raise SystemExit(
            f"{host} is a production hostname. Pass the confirmed staging endpoint."
        )
    if port in REFUSED_PORTS:
        raise SystemExit(
            f"E2E_API_PORT {port} is published by the non-staging compose file. "
            "Refusing it. Host TCP 10444 and host WebSocket 11444 are the "
            "staging file's ports. Setting them does not mean they are deployed."
        )
    if port != STAGING_HOST_TCP:
        raise SystemExit(
            "E2E_API_PORT must be 10444. UI acceptance speaks MTProto TCP. "
            "11444 is the staging WebSocket publish and is refused here. "
            "Port 10444 alone does not prove the process is isolated."
        )
    evidence = os.environ.get("E2E_STAGING_EVIDENCE", "").strip()
    if not evidence:
        raise SystemExit(
            "E2E_STAGING_EVIDENCE is required. E2E_STAGING_CONFIRMED and "
            "port 10444 are not proof of isolation."
        )
    try:
        with open(evidence, encoding="utf-8") as handle:
            text = handle.read()
    except OSError as exc:
        raise SystemExit(f"cannot read E2E_STAGING_EVIDENCE: {exc}") from exc
    required = (
        "container=tellomi-staging-teamgram",
        "listen=127.0.0.1:10444",
        "project=tellomi-staging",
        "datadir=data-staging",
    )
    missing = [item for item in required if item not in text]
    if missing:
        raise SystemExit(
            "staging evidence is missing "
            + ", ".join(missing)
            + ". Copy them from the host read-only check. Do not invent them from the port."
        )
    if "mount=/opt/tellomi/teamgram-server/data/" in text:
        raise SystemExit(
            "staging evidence still lists a production data mount. Refusing to dial."
        )
    return host, port


def resolve_host(host: str) -> str:
    try:
        infos = socket.getaddrinfo(host, None, socket.AF_INET, socket.SOCK_STREAM)
    except socket.gaierror as exc:
        raise OSError(
            f"no IPv4 address for {host}; not substituting another address"
        ) from exc
    if not infos:
        raise OSError(f"no IPv4 address for {host}; not substituting another address")
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
    parser.add_argument("--check", action="store_true")
    parser.add_argument("--preflight", action="store_true")
    parser.add_argument("--apply", action="store_true")
    args = parser.parse_args(argv)
    host, port = require_confirmed_endpoint()
    if args.check and not args.preflight and not args.apply:
        print(f"staging endpoint accepted: {host}:{port}")
        return 0
    if args.preflight:
        preflight(host, int(port))
    if args.apply:
        ip = resolve_host(host)
        apply_dnat(ip, port)
    if not args.preflight and not args.apply:
        parser.error("pass --check, --preflight, and/or --apply")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
