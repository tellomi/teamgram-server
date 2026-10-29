"""Staging login numbers. No SMS is sent.

Staging keeps Telegram's test range: 99966XYYYY, ten digits. X is the
datacenter digit. The login code is that digit five times, so 9996612345
signs in with 11111. See pkg/phonenumber/test_phone.go and docs/sms-email.md.
"""

from __future__ import annotations

import argparse
import os
import random
from pathlib import Path

PREFIX = "99966"
PHONE_LENGTH = 10
COUNTRY_CODE = "999"
MESSAGE_1 = "e2e-order-1"
MESSAGE_2 = "e2e-order-2"
MESSAGE_3 = "e2e-order-3"
RECEIPT = "e2e-receipt"


def normalize_digits(phone: str) -> str:
    return "".join(ch for ch in phone if ch.isdigit())


def code_for(digits: str, dc_digit: str = "1") -> str:
    digits = normalize_digits(digits)
    if len(digits) != PHONE_LENGTH or not digits.startswith(PREFIX):
        raise ValueError(f"not a staging test number: {digits}")
    digit = digits[len(PREFIX)]
    if digit != dc_digit:
        raise ValueError(f"{digits} is datacenter {digit}, expected {dc_digit}")
    return digit * 5


def national_number(digits: str) -> str:
    digits = normalize_digits(digits)
    if not digits.startswith(COUNTRY_CODE):
        raise ValueError(digits)
    return digits[len(COUNTRY_CODE) :]


def _random_phone(rng: random.Random, dc_digit: str) -> str:
    suffix = f"{rng.randrange(0, 10000):04d}"
    return f"{PREFIX}{dc_digit}{suffix}"


def _seed() -> int | None:
    raw = os.environ.get("E2E_PHONE_SEED", "").strip()
    if raw:
        return int(raw)
    run = os.environ.get("GITHUB_RUN_ID", "").strip()
    if run.isdigit():
        attempt = int(os.environ.get("GITHUB_RUN_ATTEMPT", "1") or "1")
        return int(run) * 10 + attempt
    return None


def _one(name: str, rng: random.Random, dc_digit: str, used: set[str]) -> str:
    override = os.environ.get(name, "").strip()
    if override:
        digits = normalize_digits(override)
        code_for(digits, dc_digit)
        if digits in used:
            raise ValueError(f"{name} duplicates another test number")
        used.add(digits)
        return digits
    for _ in range(100):
        digits = _random_phone(rng, dc_digit)
        if digits not in used:
            used.add(digits)
            return digits
    raise RuntimeError("could not allocate a unique test number")


def allocate() -> dict[str, str]:
    dc_digit = os.environ.get("E2E_DC_DIGIT", "1").strip() or "1"
    if len(dc_digit) != 1 or not dc_digit.isdigit():
        raise ValueError("E2E_DC_DIGIT must be a single digit")
    seed = _seed()
    rng = random.Random(seed) if seed is not None else random.Random()
    used: set[str] = set()
    signup = _one("E2E_PHONE_SIGNUP", rng, dc_digit, used)
    phone_a = _one("E2E_PHONE_A", rng, dc_digit, used)
    phone_b = _one("E2E_PHONE_B", rng, dc_digit, used)
    group_suffix = f"{(seed or rng.randrange(0, 1_000_000)) % 1_000_000:06d}"
    return {
        "PHONE_SIGNUP": signup,
        "PHONE_SIGNUP_NATIONAL": national_number(signup),
        "CODE_SIGNUP": code_for(signup, dc_digit),
        "NAME_SIGNUP": "E2ESign",
        "PHONE_A": phone_a,
        "PHONE_A_NATIONAL": national_number(phone_a),
        "CODE_A": code_for(phone_a, dc_digit),
        "NAME_A": "E2EAda",
        "PHONE_B": phone_b,
        "PHONE_B_NATIONAL": national_number(phone_b),
        "CODE_B": code_for(phone_b, dc_digit),
        "NAME_B": "E2EByron",
        "GROUP_NAME": f"E2Eg{group_suffix}",
        "MESSAGE_1": MESSAGE_1,
        "MESSAGE_2": MESSAGE_2,
        "MESSAGE_3": MESSAGE_3,
        "RECEIPT": RECEIPT,
    }


def write_env(path: Path, values: dict[str, str]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    lines = [f"{key}={value}" for key, value in values.items()]
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")


def main() -> None:
    parser = argparse.ArgumentParser(description="Write staging test-account env")
    parser.add_argument("--write", required=True, type=Path)
    args = parser.parse_args()
    values = allocate()
    write_env(args.write, values)
    for key in ("PHONE_SIGNUP", "PHONE_A", "PHONE_B", "GROUP_NAME"):
        print(f"{key}={values[key]}")


if __name__ == "__main__":
    main()
