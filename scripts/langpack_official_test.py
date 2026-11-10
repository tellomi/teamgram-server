"""Host rewrites apply to string values, never to langpack key names."""

from __future__ import annotations

import gzip
import json
import tempfile
import unittest
from pathlib import Path

from langpack_import import parse_android
from langpack_official import (
    apply_reviewed,
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
        self.assertEqual(brand("subscribing to __Telegram Premium.__"), "subscribing to __Tellomi Premium.__")
        self.assertEqual(brand("Kuka tahansa Telegramin käyttäjä"), "Kuka tahansa Tellomiin käyttäjä")
        self.assertEqual(brand("https://tell.cc/TelegramTipsDE"), "https://tellomi.cn/premium")
        self.assertEqual(brand("https://t.me/TelegramTips"), "https://tellomi.cn/premium")
        self.assertEqual(brand("See [the app](telegram) today."), "See [the app](telegram) today.")
        self.assertEqual(brand("TELEGRAMIIN"), "TELLOMIIIN")

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


class ReviewedPinTest(unittest.TestCase):
    def test_pin_replaces_a_translation_that_is_not_english(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            path = root / "reviewed" / "android" / "de.json.gz"
            path.parent.mkdir(parents=True)
            doc = {
                "lang_code": "de",
                "strings": [
                    {
                        "key": "CaptionPremiumSubtitle",
                        "pin": True,
                        "version": 5,
                        "other": "Tellomi Premium",
                    },
                    {
                        "key": "Login.EmailCodeBody",
                        "version": 4,
                        "value": "Kurzer Code",
                    },
                ],
            }
            with gzip.open(path, "wt", encoding="utf-8") as fh:
                json.dump(doc, fh)
            english = {
                "CaptionPremiumSubtitle": {"key": "CaptionPremiumSubtitle", "plural": {"other": "Tellomi Premium"}},
                "Login.EmailCodeBody": {"key": "Login.EmailCodeBody", "value": "Short code"},
                "Hello": {"key": "Hello", "value": "Hi"},
            }
            strings = [
                {"key": "CaptionPremiumSubtitle", "version": 9, "other": "__Telegram Premium__"},
                {"key": "Login.EmailCodeBody", "version": 9, "value": "Echter Satz"},
                {"key": "Hello", "version": 9, "value": "Hi"},
            ]
            out = {item["key"]: item for item in apply_reviewed(root, "android", "de", strings, english)}
        self.assertEqual(out["CaptionPremiumSubtitle"]["other"], "Tellomi Premium")
        self.assertNotIn("pin", out["CaptionPremiumSubtitle"])
        self.assertEqual(out["CaptionPremiumSubtitle"]["version"], 9)
        self.assertEqual(out["Login.EmailCodeBody"]["value"], "Echter Satz")
        self.assertEqual(out["Hello"]["value"], "Hi")


if __name__ == "__main__":
    unittest.main()
