"""Host rewrites apply to string values, never to langpack key names."""

from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from langpack_import import parse_android
from langpack_official import (
    brand,
    brand_entries,
    merge_lang,
    parse_android as parse_official_android,
    rewrite_link,
    tellomi_owned_key,
)

IDENTIFIERS = (
    "Chat.Message",
    "Attachment.Media",
    "BoostGift.Members",
    "RichText.Menu",
    "GroupBoost.MemberBoosted",
    "VoiceChat.Message",
    "Gift.Message",
    "Chat.PaidMessage.Confirm.Text.Messages",
    "Stars.Transaction.Giveaway.Boost.Members",
)


class RewriteValuesOnlyTest(unittest.TestCase):
    def test_identifiers_are_not_hosts(self):
        for key in IDENTIFIERS:
            self.assertEqual(rewrite_link(key), key, key)
            self.assertEqual(brand(key), key, key)

    def test_real_hosts_are_rewritten(self):
        self.assertEqual(rewrite_link("https://t.me/tellomi"), "https://tell.cc/tellomi")
        self.assertEqual(rewrite_link("see t.me/name"), "see tell.cc/name")
        self.assertEqual(rewrite_link("https://telegram.org/faq"), "https://tellomi.cn/faq")
        self.assertEqual(rewrite_link("詳しくはtelegram.orgからどうぞ。"), "詳しくはtellomi.cnからどうぞ。")
        self.assertEqual(rewrite_link("desktop.telegram.orgから"), "desktop.tellomi.cnから")
        self.assertEqual(brand("詳しくはtelegram.orgからどうぞ。"), "詳しくはtellomi.cnからどうぞ。")

    def test_brand_entries_copies_keys(self):
        out = brand_entries([
            {"key": "Chat.Message", "value": "Open https://t.me/tellomi"},
            {"key": "Attachment.Media", "plural": {"one": "1", "other": "t.me/files"}},
        ])
        self.assertEqual(out[0]["key"], "Chat.Message")
        self.assertEqual(out[0]["value"], "Open https://tell.cc/tellomi")
        self.assertEqual(out[1]["key"], "Attachment.Media")
        self.assertEqual(out[1]["plural"]["other"], "tell.cc/files")

    def test_brand_entries_rejects_a_rewritten_key(self):
        with self.assertRaises(ValueError):
            brand_entries([{"key": "Chatell.ccssage", "value": "Message"}])


class ForkOnlyKeyTest(unittest.TestCase):
    def test_tellomi_keys_are_owned_and_search_urls_are_not(self):
        self.assertTrue(tellomi_owned_key("Tellomi.Settings.Help"))
        self.assertTrue(tellomi_owned_key("TellomiLoginWithPhone"))
        self.assertTrue(tellomi_owned_key("AppName"))
        self.assertFalse(tellomi_owned_key("SearchEngine1AutocompleteURL"))
        self.assertFalse(tellomi_owned_key("AreTyping"))

    def test_merge_keeps_tellomi_keys_and_drops_apk_configuration(self):
        official = [{"key": "Hello", "value": "Hi"}]
        overrides = {
            "en": {
                "Tellomi.Settings.Help": {"key": "Tellomi.Settings.Help", "value": "Help Center", "version": 2},
                "SearchEngine1AutocompleteURL": {
                    "key": "SearchEngine1AutocompleteURL",
                    "value": "https://suggestqueries.google.com/complete/search?client=chrome&q=",
                    "version": 1,
                },
                "AreTyping": {"key": "AreTyping", "value": "are typing...", "version": 1},
            }
        }
        out = merge_lang("en", official, overrides, set(), 10, {"Hello": {"key": "Hello", "value": "Hi"}})
        keys = [item["key"] for item in out]
        self.assertIn("Hello", keys)
        self.assertIn("Tellomi.Settings.Help", keys)
        self.assertNotIn("SearchEngine1AutocompleteURL", keys)
        self.assertNotIn("AreTyping", keys)

    def test_curated_override_keeps_strings_the_export_does_not_ship(self):
        official = [{"key": "Hello", "value": "Hi"}]
        overrides = {
            "zh-hans": {
                "AreTyping": {"key": "AreTyping", "value": "正在输入…", "version": 3},
                "SearchEngine1AutocompleteURL": {
                    "key": "SearchEngine1AutocompleteURL",
                    "value": "https://suggestqueries.google.com/complete/search?client=chrome&q=",
                    "version": 1,
                },
            }
        }
        out = merge_lang("zh-hans", official, overrides, set(), 10, {"Hello": {"key": "Hello", "value": "Hi"}})
        keys = {item["key"]: item for item in out}
        self.assertEqual(keys["AreTyping"]["value"], "正在输入…")
        self.assertNotIn("SearchEngine1AutocompleteURL", keys)

    def test_android_import_skips_translatable_false(self):
        body = """<?xml version="1.0" encoding="utf-8"?>
<resources>
    <string name="Hello">Hi</string>
    <string name="SearchEngine1AutocompleteURL" translatable="false">https://x.test/?q=&amp;q=</string>
    <string translatable="false" name="SearchEngine4AutocompleteURL">https://y.test/&amp;nresults=10</string>
</resources>
"""
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "strings.xml"
            path.write_text(body, encoding="utf-8")
            imported = dict(parse_android(path))
            official = dict(parse_official_android(path))
        self.assertEqual(imported, {"Hello": "Hi"})
        self.assertEqual(official, {"Hello": "Hi"})


if __name__ == "__main__":
    unittest.main()
