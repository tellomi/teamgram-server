-- dialog_filters.from_suggested is selected by DialogFiltersDAO.
-- Folder tags themselves live in user_settings (key2 = dialog_filter_tags).

CALL teamgram_add_column('dialog_filters', 'from_suggested', 'ALTER TABLE `dialog_filters` ADD `from_suggested` INT NOT NULL DEFAULT -1 AFTER `order_value`');
