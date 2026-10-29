"""Insert contacts into the booted iOS Simulator address book.

The simulator has no `simctl` contact command. This writes AddressBook.sqlitedb
when that database is the one the runtime created. A command-line Contacts
insert is attempted first by ios-acceptance.sh.
"""

from __future__ import annotations

import json
import os
import sqlite3
import subprocess
import sys
import time
from pathlib import Path


def insert_contact(db: Path, name: str, phone: str) -> None:
    digits = "".join(ch for ch in phone if ch.isdigit())
    if not name or not digits:
        raise ValueError("name and phone are required")
    con = sqlite3.connect(db)
    try:
        person_cols = {row[1] for row in con.execute("PRAGMA table_info(ABPerson)")}
        value_cols = {row[1] for row in con.execute("PRAGMA table_info(ABMultiValue)")}
        if "First" not in person_cols or "record_id" not in value_cols or "value" not in value_cols:
            raise RuntimeError(
                "unexpected Address Book schema: "
                f"ABPerson={sorted(person_cols)} ABMultiValue={sorted(value_cols)}"
            )
        person = {"First": name}
        now = int(time.time())
        if "Last" in person_cols:
            person["Last"] = ""
        if "CreationDate" in person_cols:
            person["CreationDate"] = now
        if "ModificationDate" in person_cols:
            person["ModificationDate"] = now
        columns = ", ".join(person)
        placeholders = ", ".join("?" for _ in person)
        cur = con.execute(
            f"INSERT INTO ABPerson ({columns}) VALUES ({placeholders})",
            tuple(person.values()),
        )
        record_id = cur.lastrowid
        value = {
            "record_id": record_id,
            "property": 3,
            "identifier": 0,
            "label": 1,
            "value": f"+{digits}",
        }
        value = {key: item for key, item in value.items() if key in value_cols}
        columns = ", ".join(value)
        placeholders = ", ".join("?" for _ in value)
        con.execute(
            f"INSERT INTO ABMultiValue ({columns}) VALUES ({placeholders})",
            tuple(value.values()),
        )
        con.commit()
    finally:
        con.close()


def booted_udid() -> str:
    raw = subprocess.check_output(["xcrun", "simctl", "list", "devices", "booted", "-j"], text=True)
    devices = json.loads(raw).get("devices", {})
    for runtime_devices in devices.values():
        for device in runtime_devices:
            if device.get("state") == "Booted" and device.get("udid"):
                return device["udid"]
    raise RuntimeError("no booted iOS Simulator")


def find_database(udid: str) -> Path:
    root = Path.home() / "Library/Developer/CoreSimulator/Devices" / udid / "data"
    matches = sorted(root.rglob("AddressBook.sqlitedb"))
    if not matches:
        raise RuntimeError(f"AddressBook.sqlitedb not found under {root}")
    return matches[0]


def main() -> None:
    udid = booted_udid()
    subprocess.run(
        ["xcrun", "simctl", "launch", "booted", "com.apple.MobileAddressBook"],
        check=False,
        text=True,
    )
    time.sleep(2)
    db = find_database(udid)
    insert_contact(db, os.environ["NAME_A"], os.environ["PHONE_A"])
    insert_contact(db, os.environ["NAME_B"], os.environ["PHONE_B"])
    print(f"inserted contacts into {db}")


if __name__ == "__main__":
    try:
        main()
    except (KeyError, RuntimeError, sqlite3.Error, subprocess.CalledProcessError, ValueError) as exc:
        print(f"ios contacts failed: {exc}", file=sys.stderr)
        raise SystemExit(1) from exc
