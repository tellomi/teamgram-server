"""langpack_zh must not rewrite markdown link targets or placeholders."""

from __future__ import annotations

import unittest

from langpack_zh import brand, translate


class LinkTargetTest(unittest.TestCase):
    def test_brand_keeps_client_identifiers(self):
        self.assertEqual(brand("[@cocoon](telegram)"), "[@cocoon](telegram)")
        self.assertEqual(brand("[cocoon.org](web)"), "[cocoon.org](web)")
        self.assertEqual(brand("Open [Telegram](telegram)"), "Open [Tellomi](telegram)")
        for key in (
            "Chat.Message",
            "Attachment.Media",
            "BoostGift.Members",
            "RichText.Menu",
            "GroupBoost.MemberBoosted",
            "VoiceChat.Message",
            "Gift.Message",
            "Chat.PaidMessage.Confirm.Text.Messages",
        ):
            self.assertEqual(brand(key), key, key)
        self.assertEqual(brand("https://t.me/tellomi"), "https://tell.cc/tellomi")
        self.assertEqual(brand("see t.me/name"), "see tell.cc/name")
        self.assertIn("https://tell.cc/", brand("https://t.me/tellomi"))
        self.assertIn("%1$@", brand("Hello %1$@ from Telegram"))
        self.assertNotIn("Telegram", brand("Hello %1$@ from Telegram"))

    def test_translate_keeps_targets_and_placeholders(self):
        text = "[@cocoon](telegram) [cocoon.org](web) %1$@ {name}"
        hans, _ = translate(text, "zh-hans")
        hant, _ = translate(text, "zh-hant")
        for out in (hans, hant):
            self.assertIn("](telegram)", out)
            self.assertIn("](web)", out)
            self.assertNotIn("](网页)", out)
            self.assertNotIn("](網頁)", out)
            self.assertNotIn("](Tellomi)", out)
            self.assertIn("%1$@", out)
            self.assertIn("{name}", out)


if __name__ == "__main__":
    unittest.main()
