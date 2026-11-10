-- Crash-report bot @TellomiBot and the crash_reports table.
--
-- Idempotent. A second run inserts nothing and does not rewrite another
-- user's row. If 777006 or @TellomiBot already belongs to a different
-- account, the script stops and leaves that account alone.
--
-- User 777006 is user_type 3 (bot) with no phone and secret_key_id 0, so it
-- has no login key. The bots row is what user.getMutableUsers needs in order
-- to mark the account a bot. The token is not a login secret: bot login is
-- not enabled on this server.
--
-- The extra bots columns are the ones BotsDAO.Select already reads. Adding
-- them does not change existing bot rows.

CALL teamgram_add_column('bots', 'attach_menu_enabled', 'ALTER TABLE `bots` ADD `attach_menu_enabled` TINYINT NOT NULL DEFAULT 0');
CALL teamgram_add_column('bots', 'bot_attach_menu', 'ALTER TABLE `bots` ADD `bot_attach_menu` TINYINT NOT NULL DEFAULT 0');
CALL teamgram_add_column('bots', 'bot_business', 'ALTER TABLE `bots` ADD `bot_business` TINYINT NOT NULL DEFAULT 0');
CALL teamgram_add_column('bots', 'bot_has_main_app', 'ALTER TABLE `bots` ADD `bot_has_main_app` TINYINT NOT NULL DEFAULT 0');
CALL teamgram_add_column('bots', 'bot_active_users', 'ALTER TABLE `bots` ADD `bot_active_users` INT NOT NULL DEFAULT 0');
CALL teamgram_add_column('bots', 'has_menu_button', 'ALTER TABLE `bots` ADD `has_menu_button` TINYINT NOT NULL DEFAULT 0');
CALL teamgram_add_column('bots', 'menu_button_text', 'ALTER TABLE `bots` ADD `menu_button_text` VARCHAR(255) NOT NULL DEFAULT ''''');
CALL teamgram_add_column('bots', 'menu_button_url', 'ALTER TABLE `bots` ADD `menu_button_url` VARCHAR(512) NOT NULL DEFAULT ''''');
CALL teamgram_add_column('bots', 'bot_can_edit', 'ALTER TABLE `bots` ADD `bot_can_edit` TINYINT NOT NULL DEFAULT 0');
CALL teamgram_add_column('bots', 'has_preview_medias', 'ALTER TABLE `bots` ADD `has_preview_medias` TINYINT NOT NULL DEFAULT 0');
CALL teamgram_add_column('bots', 'description_photo_id', 'ALTER TABLE `bots` ADD `description_photo_id` BIGINT NOT NULL DEFAULT 0');
CALL teamgram_add_column('bots', 'description_document_id', 'ALTER TABLE `bots` ADD `description_document_id` BIGINT NOT NULL DEFAULT 0');
CALL teamgram_add_column('bots', 'main_app_url', 'ALTER TABLE `bots` ADD `main_app_url` VARCHAR(512) NOT NULL DEFAULT ''''');
CALL teamgram_add_column('bots', 'has_app_settings', 'ALTER TABLE `bots` ADD `has_app_settings` TINYINT NOT NULL DEFAULT 0');
CALL teamgram_add_column('bots', 'placeholder_path', 'ALTER TABLE `bots` ADD `placeholder_path` VARCHAR(255) NOT NULL DEFAULT ''''');
CALL teamgram_add_column('bots', 'background_color', 'ALTER TABLE `bots` ADD `background_color` INT NOT NULL DEFAULT 0');
CALL teamgram_add_column('bots', 'background_dark_color', 'ALTER TABLE `bots` ADD `background_dark_color` INT NOT NULL DEFAULT 0');
CALL teamgram_add_column('bots', 'header_color', 'ALTER TABLE `bots` ADD `header_color` INT NOT NULL DEFAULT 0');
CALL teamgram_add_column('bots', 'header_dark_color', 'ALTER TABLE `bots` ADD `header_dark_color` INT NOT NULL DEFAULT 0');
CALL teamgram_add_column('bots', 'privacy_policy_url', 'ALTER TABLE `bots` ADD `privacy_policy_url` VARCHAR(512) NOT NULL DEFAULT ''''');

CREATE TABLE IF NOT EXISTS crash_reports (
  id BIGINT NOT NULL AUTO_INCREMENT,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  sender_user_id BIGINT NOT NULL,
  msg_id INT NOT NULL,
  kind VARCHAR(16) NOT NULL DEFAULT 'text',
  filename VARCHAR(255) NOT NULL DEFAULT '',
  caption VARCHAR(4096) NOT NULL DEFAULT '',
  message_text TEXT,
  device_info VARCHAR(512) NOT NULL DEFAULT '',
  app_info VARCHAR(512) NOT NULL DEFAULT '',
  file_size BIGINT NOT NULL DEFAULT 0,
  content_type VARCHAR(128) NOT NULL DEFAULT '',
  storage_class VARCHAR(64) NOT NULL DEFAULT 'crash-reports',
  storage_key VARCHAR(512) NOT NULL DEFAULT '',
  PRIMARY KEY (id),
  UNIQUE KEY uniq_crash_msg (sender_user_id, msg_id),
  KEY idx_crash_created (created_at),
  KEY idx_crash_sender (sender_user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

SET @bad = (SELECT username FROM username WHERE username = 'TellomiBot' AND NOT (peer_type = 2 AND peer_id = 777006) LIMIT 1);
CALL teamgram_fail_if_set(@bad, 'crash bot username taken: ');

SET @bad_name = (SELECT CAST(id AS CHAR) FROM users WHERE username = 'TellomiBot' AND id <> 777006 LIMIT 1);
CALL teamgram_fail_if_set(@bad_name, 'crash bot name on user: ');

SET @bad_user = (SELECT CAST(id AS CHAR) FROM users WHERE id = 777006 AND NOT (user_type = 3 AND is_bot = 1 AND username IN ('TellomiBot', '')) LIMIT 1);
CALL teamgram_fail_if_set(@bad_user, 'crash bot user id taken: ');

SET @bad_token = (SELECT CAST(bot_id AS CHAR) FROM bots WHERE token = 'tellomi-crash-collector' AND bot_id <> 777006 LIMIT 1);
CALL teamgram_fail_if_set(@bad_token, 'crash bot token taken: ');

INSERT IGNORE INTO users (id, user_type, access_hash, secret_key_id, first_name, last_name, username, phone, country_code, verified, support, about, is_bot) VALUES (777006, 3, 8100000000000777006, 0, 'Tellomi 反馈', '', 'TellomiBot', NULL, '', 1, 0, '把崩溃日志发到这里。', 1);

UPDATE users
SET username = 'TellomiBot', first_name = 'Tellomi 反馈'
WHERE id = 777006 AND user_type = 3 AND is_bot = 1 AND username = '';

INSERT IGNORE INTO bots (bot_id, bot_type, creator_user_id, token, description, bot_chat_history, bot_nochats, bot_info_version) VALUES (777006, 0, 0, 'tellomi-crash-collector', 'Tellomi crash reports', 0, 1, 1);

INSERT IGNORE INTO username (username, peer_type, peer_id, editable, active, order2, deleted) VALUES ('TellomiBot', 2, 777006, 1, 1, 2, 0);
