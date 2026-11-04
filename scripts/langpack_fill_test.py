"""Placeholder shielding for langpack_fill."""

from __future__ import annotations

import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from langpack_fill import KEEP_RE, brand_safe, pieces_of, placeholders, same_tokens


class PlaceholderTest(unittest.TestCase):
    def test_placeholders_cover_client_tokens(self):
        text = "Hello %1$@, %d files, {count} left, {} selected, %2 clicks, 50%"
        self.assertEqual(
            placeholders(text),
            ["%1$@", "%d", "{count}", "{}", "%2"],
        )

    def test_keep_split_does_not_feed_placeholders_to_the_translator(self):
        text = "Hello %1$@\nYou have {count} items. [Tellomi](telegram) Tellomi"
        parts = KEEP_RE.split(text)
        kept = [part for i, part in enumerate(parts) if i % 2 == 1]
        prose = [part for i, part in enumerate(parts) if i % 2 == 0]
        self.assertIn("%1$@", kept)
        self.assertIn("{count}", kept)
        self.assertIn("](telegram)", kept)
        self.assertIn("Tellomi", kept)
        joined = "".join(prose)
        self.assertNotIn("%1$@", joined)
        self.assertNotIn("{count}", joined)
        self.assertNotIn("telegram", joined)
        self.assertNotIn("Tellomi", joined)
        self.assertEqual(pieces_of(text)[0][:5], "Hello")

    def test_percent_after_specifier_stays_attached(self):
        text = "Battery %@%"
        kept = [part for i, part in enumerate(KEEP_RE.split(text)) if i % 2 == 1]
        self.assertEqual(kept, ["%@%"])
        self.assertEqual(placeholders(text), ["%@"])
        self.assertTrue(same_tokens(text, "电池 %@%"))

    def test_brand_keeps_link_targets(self):
        got = brand_safe("Open [Telegram](telegram) at https://t.me/tellomi")
        self.assertIn("](telegram)", got)
        self.assertNotIn("](Tellomi)", got)
        self.assertNotIn("t.me", got)
        self.assertNotIn("Telegram", got.replace("](telegram)", ""))


if __name__ == "__main__":
    unittest.main()
