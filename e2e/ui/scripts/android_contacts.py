"""Insert the two peer contacts on the Android emulator.

Telegram will not open a chat from a typed number unless that number is a
contact. Shell content inserts run as the emulator's privileged shell user.
"""

from __future__ import annotations

import os
import subprocess
import sys


def insert_commands(raw_id: str, name: str, phone: str) -> list[list[str]]:
    if not raw_id.isdigit():
        raise ValueError(f"bad raw contact id {raw_id!r}")
    if not name or any(ch.isspace() for ch in name):
        raise ValueError(f"contact name must be a single token, got {name!r}")
    digits = "".join(ch for ch in phone if ch.isdigit())
    if not digits:
        raise ValueError(phone)
    return [
        [
            "content",
            "insert",
            "--uri",
            "content://com.android.contacts/data",
            "--bind",
            f"raw_contact_id:i:{raw_id}",
            "--bind",
            "mimetype:s:vnd.android.cursor.item/name",
            "--bind",
            f"data1:s:{name}",
        ],
        [
            "content",
            "insert",
            "--uri",
            "content://com.android.contacts/data",
            "--bind",
            f"raw_contact_id:i:{raw_id}",
            "--bind",
            "mimetype:s:vnd.android.cursor.item/phone_v2",
            "--bind",
            f"data1:s:+{digits}",
            "--bind",
            "data2:i:2",
        ],
    ]


def _adb(args: list[str]) -> str:
    completed = subprocess.run(["adb", *args], check=True, text=True, capture_output=True)
    return (completed.stdout or "").strip()


def insert_contact(name: str, phone: str) -> None:
    created = _adb(
        [
            "shell",
            "content",
            "insert",
            "--uri",
            "content://com.android.contacts/raw_contacts",
            "--bind",
            "account_type:s:",
            "--bind",
            "account_name:s:",
        ]
    )
    raw_id = created.rstrip("/").rsplit("/", 1)[-1]
    for command in insert_commands(raw_id, name, phone):
        _adb(["shell", *command])
    print(f"contact {name} +{''.join(ch for ch in phone if ch.isdigit())}")


def main() -> None:
    subprocess.run(
        ["adb", "shell", "content", "delete", "--uri", "content://com.android.contacts/raw_contacts"],
        check=False,
        text=True,
    )
    insert_contact(os.environ["NAME_A"], os.environ["PHONE_A"])
    insert_contact(os.environ["NAME_B"], os.environ["PHONE_B"])


if __name__ == "__main__":
    try:
        main()
    except (subprocess.CalledProcessError, KeyError, ValueError) as exc:
        print(f"android contacts failed: {exc}", file=sys.stderr)
        raise SystemExit(1) from exc
