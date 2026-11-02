-- Push notifications follow the recipient's app language.
-- devices.lang_code is the snapshot account.registerDevice takes from the
-- session. auths.lang_code (initConnection) is preferred when a push is sent.
-- Safe to run more than once.

CALL teamgram_add_column('devices', 'lang_code', 'ALTER TABLE `devices` ADD `lang_code` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '''' AFTER `other_uids`');
