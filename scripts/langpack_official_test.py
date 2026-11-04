"""Host rewrites apply to string values, never to langpack key names."""

from __future__ import annotations

import unittest

from langpack_official import brand, brand_entries, rewrite_link

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


if __name__ == "__main__":
    unittest.main()
