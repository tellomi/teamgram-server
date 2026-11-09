-- Keyword lists for imported sticker packs are JSON when a sticker has more
-- than one language. VARCHAR(512) truncates that. Safe to run more than once.

CALL teamgram_change_column('tellomi_sticker_items', 'keywords', 'ALTER TABLE `tellomi_sticker_items` MODIFY `keywords` TEXT NOT NULL');
