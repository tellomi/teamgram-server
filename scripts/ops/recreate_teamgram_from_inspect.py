#!/usr/bin/env python3
"""Rebuild tellomi-main-teamgram from its own docker inspect.

The origin container is not a compose service. This reads one inspect
document, changes only the datacenter env keys, and keeps every other env
entry, bind, network endpoint, and port binding (including 10445).

It does not print env values.
"""

import json
import socket
import sys

NAME = "tellomi-main-teamgram"
INSPECT_PATH = "/var/backups/tellomi-main-teamgram.inspect.json"
CREATE_PATH = "/var/backups/tellomi-main-teamgram.create.json"
DC_ENV_KEYS = (
    "TELLOMI_PUBLIC_DC_HOST",
    "TELLOMI_DC_OPTIONS",
    "TELLOMI_DC_TXT_DOMAIN",
)

# HostConfig fields inspect adds that ContainerCreate rejects or ignores.
_DROP_HOST = {
    "ContainerIDFile",
}


def update_env(env, updates):
    """Replace or append updates. Every other KEY=VALUE stays, in order."""
    if env is None:
        env = []
    out = []
    seen = set()
    for item in env:
        key, sep, _ = item.partition("=")
        if sep and key in updates:
            out.append(key + "=" + updates[key])
            seen.add(key)
        else:
            out.append(item)
    for key in DC_ENV_KEYS:
        if key in updates and key not in seen:
            out.append(key + "=" + updates[key])
    return out


def _endpoint(net):
    ipam = {}
    if net.get("IPAddress"):
        ipam["IPv4Address"] = net["IPAddress"]
    if net.get("GlobalIPv6Address"):
        ipam["IPv6Address"] = net["GlobalIPv6Address"]
    body = {
        "Links": net.get("Links"),
        "Aliases": net.get("Aliases"),
        "MacAddress": net.get("MacAddress") or "",
        "DriverOpts": net.get("DriverOpts"),
    }
    if ipam:
        body["IPAMConfig"] = ipam
    return body


def build_body(inspect_doc, updates):
    item = inspect_doc[0] if isinstance(inspect_doc, list) else inspect_doc
    cfg = item["Config"]
    host = {k: v for k, v in item["HostConfig"].items() if k not in _DROP_HOST}
    ports = host.get("PortBindings") or {}
    if "10445/tcp" not in ports:
        raise SystemExit("inspect has no 10445/tcp port binding; not recreating")
    networks = (item.get("NetworkSettings") or {}).get("Networks") or {}
    if not networks and host.get("NetworkMode") in (None, "", "default"):
        raise SystemExit("inspect has no network; not recreating")
    env = update_env(cfg.get("Env") or [], updates)
    old_keys = []
    for item_env in cfg.get("Env") or []:
        key, sep, _ = item_env.partition("=")
        if sep:
            old_keys.append(key)
    new_keys = []
    for item_env in env:
        key, sep, _ = item_env.partition("=")
        if sep:
            new_keys.append(key)
    for key in old_keys:
        if key not in new_keys:
            raise SystemExit("refusing to drop env key " + key)
    body = {
        "Hostname": cfg.get("Hostname") or "",
        "Domainname": cfg.get("Domainname") or "",
        "User": cfg.get("User") or "",
        "AttachStdin": bool(cfg.get("AttachStdin")),
        "AttachStdout": bool(cfg.get("AttachStdout")),
        "AttachStderr": bool(cfg.get("AttachStderr")),
        "Tty": bool(cfg.get("Tty")),
        "OpenStdin": bool(cfg.get("OpenStdin")),
        "StdinOnce": bool(cfg.get("StdinOnce")),
        "Env": env,
        "Cmd": cfg.get("Cmd"),
        "Image": cfg["Image"],
        "Volumes": cfg.get("Volumes") or {},
        "WorkingDir": cfg.get("WorkingDir") or "",
        "Entrypoint": cfg.get("Entrypoint"),
        "Labels": cfg.get("Labels") or {},
        "ExposedPorts": cfg.get("ExposedPorts") or {},
        "HostConfig": host,
        "NetworkingConfig": {
            "EndpointsConfig": {name: _endpoint(net) for name, net in networks.items()}
        },
    }
    if cfg.get("StopSignal"):
        body["StopSignal"] = cfg["StopSignal"]
    if cfg.get("StopTimeout") is not None:
        body["StopTimeout"] = cfg["StopTimeout"]
    if cfg.get("Healthcheck"):
        body["Healthcheck"] = cfg["Healthcheck"]
    return body


def docker_api(method, path, body=None):
    payload = b"" if body is None else json.dumps(body).encode()
    head = (
        "%s %s HTTP/1.0\r\nHost: docker\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n"
        % (method, path, len(payload))
    ).encode()
    sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    try:
        sock.connect("/var/run/docker.sock")
        sock.sendall(head + payload)
        data = b""
        while True:
            chunk = sock.recv(65536)
            if not chunk:
                break
            data += chunk
    finally:
        sock.close()
    raw_head, _, raw_body = data.partition(b"\r\n\r\n")
    parts = raw_head.split()
    if len(parts) < 2:
        raise SystemExit("docker api returned no status")
    return int(parts[1]), raw_body


def create_container(body_path, name):
    with open(body_path, "r", encoding="utf-8") as fh:
        body = json.load(fh)
    status, raw = docker_api("POST", "/v1.41/containers/create?name=" + name, body)
    if status not in (200, 201):
        # The engine error names the rejected field. Do not print the request.
        sys.stderr.write("docker create failed: http %d\n" % status)
        raise SystemExit(1)
    sys.stdout.write("container created\n")


def main(argv):
    if len(argv) == 3 and argv[1] == "create":
        create_container(argv[2], NAME)
        return
    if len(argv) != 6:
        raise SystemExit(
            "usage: recreate_teamgram_from_inspect.py INSPECT OUT HOST OPTIONS DOMAIN\n"
            "       recreate_teamgram_from_inspect.py create CREATE_JSON"
        )
    _, inspect_path, out_path, host, options, domain = argv
    with open(inspect_path, "r", encoding="utf-8") as fh:
        doc = json.load(fh)
    body = build_body(doc, {
        "TELLOMI_PUBLIC_DC_HOST": host,
        "TELLOMI_DC_OPTIONS": options,
        "TELLOMI_DC_TXT_DOMAIN": domain,
    })
    with open(out_path, "w", encoding="utf-8") as fh:
        json.dump(body, fh)
    ports = body["HostConfig"].get("PortBindings") or {}
    sys.stdout.write("create json written; 10445/tcp bindings %d\n" % len(ports.get("10445/tcp") or []))


if __name__ == "__main__":
    main(sys.argv)
