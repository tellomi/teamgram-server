-- Invite links for basic groups and for channels / supergroups.
--
-- messages.exportChatInvite and the rest of the invite-link family read
-- these tables. The link stored here is the hash. Clients receive
-- https://tell.cc/+<hash> (brand.LinkBase).
--
-- chat_invite_participants is the basic-group join log. The handler already
-- writes chat_id, requested, and approved_by. A database created from an
-- older 1_teamgram.sql is missing those columns, so import and the usage
-- counter fail.
--
-- channel_invites / channel_invite_importers are the same data for
-- channels and supergroups. The process also creates them from
-- app/service/biz/channel/schema.go when it opens MySQL.
--
-- Apply
--   Containers run teamgramd/deploy/migrate.sh before the processes start.
--   By hand:
--     mysql ... teamgram < teamgramd/deploy/sql/0_migrate_helpers.sql
--     mysql ... teamgram < teamgramd/deploy/sql/migrate-20261104-chat-invites.sql
-- Safe to run more than once.

CALL teamgram_add_column('chat_invites', 'request_needed', 'ALTER TABLE `chat_invites` ADD `request_needed` tinyint(1) NOT NULL DEFAULT 0 AFTER `revoked`');
CALL teamgram_add_column('chat_invites', 'start_date', 'ALTER TABLE `chat_invites` ADD `start_date` bigint(20) NOT NULL DEFAULT 0 AFTER `request_needed`');
CALL teamgram_add_column('chat_invites', 'expire_date', 'ALTER TABLE `chat_invites` ADD `expire_date` bigint(20) NOT NULL DEFAULT 0 AFTER `start_date`');
CALL teamgram_add_column('chat_invites', 'usage_limit', 'ALTER TABLE `chat_invites` ADD `usage_limit` int(11) NOT NULL DEFAULT 0 AFTER `expire_date`');
CALL teamgram_add_column('chat_invites', 'usage2', 'ALTER TABLE `chat_invites` ADD `usage2` int(11) NOT NULL DEFAULT 0 AFTER `usage_limit`');
CALL teamgram_add_column('chat_invites', 'requested', 'ALTER TABLE `chat_invites` ADD `requested` int(11) NOT NULL DEFAULT 0 AFTER `usage2`');
CALL teamgram_add_column('chat_invites', 'title', 'ALTER TABLE `chat_invites` ADD `title` varchar(64) COLLATE utf8mb4_bin NOT NULL DEFAULT '''' AFTER `requested`');

CALL teamgram_add_column('chat_invite_participants', 'chat_id', 'ALTER TABLE `chat_invite_participants` ADD `chat_id` bigint(20) NOT NULL DEFAULT 0 AFTER `id`');
CALL teamgram_add_column('chat_invite_participants', 'requested', 'ALTER TABLE `chat_invite_participants` ADD `requested` tinyint(1) NOT NULL DEFAULT 0 AFTER `user_id`');
CALL teamgram_add_column('chat_invite_participants', 'approved_by', 'ALTER TABLE `chat_invite_participants` ADD `approved_by` bigint(20) NOT NULL DEFAULT 0 AFTER `requested`');
CALL teamgram_change_column('chat_invite_participants', 'link', 'ALTER TABLE `chat_invite_participants` MODIFY `link` varchar(64) COLLATE utf8mb4_bin NOT NULL');
CALL teamgram_add_index('chat_invite_participants', 'chat_id', 'ALTER TABLE `chat_invite_participants` ADD KEY `chat_id` (`chat_id`, `requested`)');

CREATE TABLE IF NOT EXISTS channel_invites (
  id BIGINT NOT NULL AUTO_INCREMENT,
  channel_id BIGINT NOT NULL,
  admin_id BIGINT NOT NULL,
  link VARCHAR(64) NOT NULL,
  permanent TINYINT NOT NULL DEFAULT 0,
  revoked TINYINT NOT NULL DEFAULT 0,
  request_needed TINYINT NOT NULL DEFAULT 0,
  start_date BIGINT NOT NULL DEFAULT 0,
  expire_date BIGINT NOT NULL DEFAULT 0,
  usage_limit INT NOT NULL DEFAULT 0,
  title VARCHAR(64) NOT NULL DEFAULT '',
  date2 BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY channel_invites_link (link),
  KEY channel_invites_channel (channel_id, admin_id, revoked)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS channel_invite_importers (
  id BIGINT NOT NULL AUTO_INCREMENT,
  channel_id BIGINT NOT NULL,
  link VARCHAR(64) NOT NULL,
  user_id BIGINT NOT NULL,
  requested TINYINT NOT NULL DEFAULT 0,
  approved_by BIGINT NOT NULL DEFAULT 0,
  about VARCHAR(128) NOT NULL DEFAULT '',
  date2 BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY channel_invite_importer (link, user_id),
  KEY channel_invite_importers_channel (channel_id, requested)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
