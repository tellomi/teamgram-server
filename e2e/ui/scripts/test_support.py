"""Local checks for the staging UI harness. No emulator."""

from __future__ import annotations

import os
import plistlib
import sqlite3
import subprocess
import tempfile
import unittest
import xml.etree.ElementTree as ET
from pathlib import Path

import accounts
import android_contacts
import hierarchy
import ios_contacts
import redirect
import seeds
from select_simulator_app import select_app

ROOT = Path(__file__).resolve().parents[3]
MAESTRO = ROOT / "e2e" / "ui" / "maestro"


class AccountsTest(unittest.TestCase):
    def setUp(self) -> None:
        self._saved = {
            key: os.environ.get(key)
            for key in (
                "E2E_PHONE_SEED",
                "E2E_PHONE_A",
                "E2E_PHONE_B",
                "E2E_PHONE_SIGNUP",
                "E2E_DC_DIGIT",
                "GITHUB_RUN_ID",
                "GITHUB_RUN_ATTEMPT",
            )
        }
        for key in self._saved:
            os.environ.pop(key, None)

    def tearDown(self) -> None:
        for key, value in self._saved.items():
            if value is None:
                os.environ.pop(key, None)
            else:
                os.environ[key] = value

    def test_seed_is_stable_and_codes_match_the_datacenter_digit(self) -> None:
        os.environ["E2E_PHONE_SEED"] = "42"
        first = accounts.allocate()
        second = accounts.allocate()
        self.assertEqual(first["PHONE_A"], second["PHONE_A"])
        phones = {first["PHONE_SIGNUP"], first["PHONE_A"], first["PHONE_B"]}
        self.assertEqual(len(phones), 3)
        for phone in phones:
            self.assertEqual(len(phone), 10)
            self.assertTrue(phone.startswith("999661"))
            self.assertEqual(accounts.code_for(phone), "11111")
            self.assertEqual(accounts.national_number(phone), phone[3:])
        self.assertEqual(first["CODE_A"], "11111")
        self.assertNotIn(" ", first["GROUP_NAME"])

    def test_override_must_be_a_test_number(self) -> None:
        os.environ["E2E_PHONE_A"] = "+86 13800138000"
        with self.assertRaises(ValueError):
            accounts.allocate()


class RedirectTest(unittest.TestCase):
    def test_dnat_sends_the_mtproto_port_to_staging(self) -> None:
        self.assertEqual(
            redirect.dnat_arguments("203.0.113.10", "10443"),
            [
                "iptables",
                "-t",
                "nat",
                "-A",
                "OUTPUT",
                "-p",
                "tcp",
                "--dport",
                "10443",
                "-j",
                "DNAT",
                "--to-destination",
                "203.0.113.10:10443",
            ],
        )

    def test_dnat_rejects_a_hostname(self) -> None:
        with self.assertRaises(ValueError):
            redirect.dnat_arguments("staging-api.tellomi.cn", "10443")


class SeedsTest(unittest.TestCase):
    def test_only_datacenter_one_is_rewritten(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "tellomi-server.json"
            path.write_text(
                '{"seeds": {"production": {"2": ["old.example"]}, "testing": {"1": ["old.example"]}}, "note": "keep"}',
                encoding="utf-8",
            )
            seeds.point_ios_seeds(path, "staging-api.tellomi.cn")
            import json

            cfg = json.loads(path.read_text(encoding="utf-8"))
            self.assertEqual(cfg["seeds"]["production"]["1"], ["staging-api.tellomi.cn"])
            self.assertEqual(cfg["seeds"]["testing"]["1"], ["staging-api.tellomi.cn"])
            self.assertEqual(cfg["note"], "keep")


class HierarchyTest(unittest.TestCase):
    def test_send_order_is_top_to_bottom_and_receipt_is_unread(self) -> None:
        xml = """<hierarchy>
          <node text="e2e-order-1" bounds="[0,100][10,120]"/>
          <node text="e2e-order-2" bounds="[0,140][10,160]"/>
          <node text="e2e-order-3" bounds="[0,180][10,200]"/>
          <node text="e2e-receipt Not seen" bounds="[0,220][10,240]"/>
        </hierarchy>"""
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "ui.xml"
            path.write_text(xml, encoding="utf-8")
            hierarchy.assert_chat(path)

    def test_reversed_messages_fail(self) -> None:
        tree = ET.fromstring(
            """<hierarchy>
              <node text="e2e-order-1" bounds="[0,300][10,320]"/>
              <node text="e2e-order-2" bounds="[0,140][10,160]"/>
              <node text="e2e-order-3" bounds="[0,180][10,200]"/>
              <node text="e2e-receipt Not seen" bounds="[0,340][10,360]"/>
            </hierarchy>"""
        )
        with self.assertRaises(AssertionError):
            hierarchy.assert_vertical_order(tree)

    def test_read_receipt_fails_when_the_peer_has_already_read(self) -> None:
        tree = {
            "attributes": {"text": "e2e-receipt Seen", "bounds": "[0,10][10,20]"},
            "children": [],
        }
        with self.assertRaises(AssertionError):
            hierarchy.assert_receipt_unread(tree)


class ContactsTest(unittest.TestCase):
    def test_android_insert_uses_the_raw_id_and_e164(self) -> None:
        commands = android_contacts.insert_commands("7", "E2EAda", "9996612345")
        self.assertIn("raw_contact_id:i:7", commands[0])
        self.assertIn("data1:s:+9996612345", commands[1])

    def test_ios_sqlite_insert(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            db = Path(tmp) / "AddressBook.sqlitedb"
            con = sqlite3.connect(db)
            con.execute("CREATE TABLE ABPerson (ROWID INTEGER PRIMARY KEY, First TEXT, Last TEXT, CreationDate INTEGER)")
            con.execute(
                "CREATE TABLE ABMultiValue (UID INTEGER PRIMARY KEY, record_id INTEGER, property INTEGER, identifier INTEGER, label INTEGER, value TEXT)"
            )
            con.commit()
            con.close()
            ios_contacts.insert_contact(db, "E2EAda", "+9996612345")
            con = sqlite3.connect(db)
            row = con.execute("SELECT First FROM ABPerson").fetchone()
            phone = con.execute("SELECT value, property FROM ABMultiValue").fetchone()
            con.close()
            self.assertEqual(row[0], "E2EAda")
            self.assertEqual(phone, ("+9996612345", 3))


class SimulatorAppTest(unittest.TestCase):
    def test_selects_the_tellomi_bundle(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            other = root / "Share.app"
            other.mkdir()
            main = root / "Telegram.app"
            main.mkdir()
            with (other / "Info.plist").open("wb") as handle:
                plistlib.dump({"CFBundleIdentifier": "app.tellomi.chat.Share", "CFBundleExecutable": "Share"}, handle)
            (other / "Share").write_bytes(b"")
            with (main / "Info.plist").open("wb") as handle:
                plistlib.dump({"CFBundleIdentifier": "app.tellomi.chat", "CFBundleExecutable": "Telegram"}, handle)
            (main / "Telegram").write_bytes(b"")
            self.assertEqual(select_app(root, require_simulator=False), main)


class ChecklistTest(unittest.TestCase):
    def test_flows_cover_the_v1_checklist(self) -> None:
        text = "\n".join(path.read_text(encoding="utf-8") for path in MAESTRO.rglob("*.yaml"))
        for phrase in (
            "我已阅读并同意",
            "服务条款",
            "同意 & 继续",
            "正在更新",
            "doubleTapOn",
            "New Group|新建群组",
            "Not seen",
            "${MESSAGE_1}",
            "${MESSAGE_2}",
            "${MESSAGE_3}",
            "${RECEIPT}",
            "group-avatar.png",
        ):
            self.assertIn(phrase, text)
        self.assertEqual(hierarchy.ORDER, ("e2e-order-1", "e2e-order-2", "e2e-order-3"))
        self.assertEqual(hierarchy.RECEIPT, "e2e-receipt")
        self.assertIn("staging-api.tellomi.cn", (ROOT / "e2e/ui/README.md").read_text(encoding="utf-8"))
        workflow = (ROOT / ".github/workflows/ui-acceptance.yml").read_text(encoding="utf-8")
        self.assertIn("deploy-staging", workflow)
        self.assertIn("workflow_dispatch", workflow)
        self.assertIn("reactivecircus/android-emulator-runner", workflow)


class ShellSyntaxTest(unittest.TestCase):
    def test_shell_scripts_parse(self) -> None:
        for path in (ROOT / "e2e/ui/scripts").glob("*.sh"):
            subprocess.check_call(["bash", "-n", str(path)])


if __name__ == "__main__":
    unittest.main()
