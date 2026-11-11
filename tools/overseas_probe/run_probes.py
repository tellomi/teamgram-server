#!/usr/bin/env python3
"""GitHub-hosted runner only: no authentication, production writes, or secrets.

Required gate: new ECS 8.156.40.105 on 443 and 10443, plus api.tellomi.cn:443.
The retired SWAS IP 8.137.39.82 is probed only as informational context.
"""
import datetime
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import urllib.request

HERE = Path(__file__).resolve().parent
CST = datetime.timezone(datetime.timedelta(hours=8))

# (name, host, port, dc, required_for_gate)
TARGETS = (
    ("ecs-443", "8.156.40.105", 443, 1, True),
    ("ecs-10443", "8.156.40.105", 10443, 1, True),
    ("api-443", "api.tellomi.cn", 443, 1, True),
    ("old-10443", "8.137.39.82", 10443, 1, False),
    ("old-5222", "8.137.39.82", 5222, 1, False),
    ("old-443", "8.137.39.82", 443, 1, False),
)


def emit(kind, **values):
    print(json.dumps({"kind": kind, **values}, ensure_ascii=False), flush=True)


def public_json(url):
    req = urllib.request.Request(url, headers={"User-Agent": "tellomi-readonly-probe/1"})
    with urllib.request.urlopen(req, timeout=12) as r:
        data = r.read(1024 * 1024)
        return r.status, data, json.loads(data)


def probe_target(name, host, port, dc, attempts=3):
    successes = 0
    latencies = []
    for attempt in range(1, attempts + 1):
        cmd = [
            sys.executable,
            str(HERE / "mtproto_probe.py"),
            host,
            str(port),
            "--dc",
            str(dc),
            "--timeout",
            "10",
        ]
        try:
            p = subprocess.run(cmd, capture_output=True, text=True, timeout=18)
            result = json.loads(p.stdout)
        except Exception as e:  # noqa: BLE001 - one attempt must not stop the rest
            result = {
                "ok": False,
                "error": str(e)[:200],
                "received_bytes": 0,
                "raw_first_bytes_hex": "",
                "raw_class": "probe_process_error",
            }
        emit("attempt", target=name, attempt=attempt, dc=dc, **result)
        if result.get("ok") is True:
            successes += 1
            if "elapsed_ms" in result:
                latencies.append(result["elapsed_ms"])
    item = {
        "target": name,
        "host": host,
        "port": port,
        "successes": successes,
        "attempts": attempts,
        "required_for_vantage": name in {t[0] for t in TARGETS if t[4]},
    }
    if latencies:
        item["latency_ms"] = {
            "min": min(latencies),
            "max": max(latencies),
            "avg": round(sum(latencies) / len(latencies), 2),
            "samples": latencies,
        }
    return item


def main():
    emit(
        "runner",
        time_cst=datetime.datetime.now(CST).isoformat(),
        runner_name=os.getenv("RUNNER_NAME"),
        runner_os=os.getenv("RUNNER_OS"),
        runner_arch=os.getenv("RUNNER_ARCH"),
        sha=os.getenv("GITHUB_SHA"),
        run_id=os.getenv("GITHUB_RUN_ID"),
    )
    try:
        _, _, ip = public_json("https://api.ipify.org?format=json")
        emit("public_ip", ip=ip["ip"])
        _, _, geo = public_json("https://ipapi.co/" + ip["ip"] + "/json/")
        emit(
            "geo",
            source="ipapi.co (approximate)",
            **{k: geo.get(k) for k in ("ip", "city", "region", "country_name", "country_code", "asn", "org")},
        )
    except Exception as e:  # noqa: BLE001
        emit("geo_error", error=str(e)[:200])

    results = []
    for name, host, port, dc, _required in TARGETS:
        item = probe_target(name, host, port, dc)
        results.append(item)
        emit("target_summary", **item)

    https_ok = False
    url = "https://www.tellomi.cn/dl/android/latest.json"
    try:
        status, data, doc = public_json(url)
        https_ok = status == 200 and isinstance(doc, dict)
        emit(
            "https",
            ok=https_ok,
            url=url,
            status=status,
            bytes=len(data),
            sha256=hashlib.sha256(data).hexdigest(),
            public_manifest=doc,
        )
    except Exception as e:  # noqa: BLE001
        emit("https", ok=False, url=url, error=str(e)[:200])

    required = [t for t in results if t["required_for_vantage"]]
    gate_ok = all(t["successes"] == t["attempts"] for t in required)
    emit(
        "overall",
        vantage_valid=gate_ok,
        https_ok=https_ok,
        gate_note=(
            "required: 8.156.40.105:443, 8.156.40.105:10443, api.tellomi.cn:443; "
            "8.137.39.82 is informational only"
        ),
    )
    summary = [
        "# Overseas MTProto probe",
        "",
        "| Target | resPQ / attempts | Latency ms (min/avg/max) | Gate |",
        "|---|---:|---|---|",
    ]
    for t in results:
        if t.get("latency_ms"):
            lat = t["latency_ms"]
            lat_s = f"{lat['min']}/{lat['avg']}/{lat['max']}"
        else:
            lat_s = "n/a"
        gate = "required" if t["required_for_vantage"] else "informational (old IP)"
        summary.append(
            f"| {t['host']}:{t['port']} ({t['target']}) | {t['successes']}/{t['attempts']} | {lat_s} | {gate} |"
        )
    summary += [
        "",
        f"Gate valid: **{gate_ok}**",
        f"HTTPS JSON GET (informational): **{https_ok}**",
        "",
        "Raw first response bytes on failures are in the attempt JSON log; no client init or keys are logged.",
    ]
    if os.getenv("GITHUB_STEP_SUMMARY"):
        with open(os.environ["GITHUB_STEP_SUMMARY"], "a") as f:
            f.write("\n".join(summary) + "\n")
    return 0 if gate_ok else 1


if __name__ == "__main__":
    sys.exit(main())
