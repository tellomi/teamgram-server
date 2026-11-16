-- Passkeys (layer 232 account.*Passkey / auth.*PasskeyLogin).
-- New tables only. Safe to run more than once. Does not change existing rows.
-- migrate.sh records this filename in schema_migrations.

CREATE TABLE IF NOT EXISTS `user_passkeys` (
  `raw_id` varbinary(1023) NOT NULL,
  `credential_id` text COLLATE utf8mb4_unicode_ci NOT NULL,
  `user_id` bigint NOT NULL,
  `public_key` blob NOT NULL,
  `sign_count` int unsigned NOT NULL DEFAULT 0,
  `aaguid` binary(16) NOT NULL,
  `attestation_type` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `user_handle` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `user_present` tinyint NOT NULL DEFAULT 0,
  `user_verified` tinyint NOT NULL DEFAULT 0,
  `backup_eligible` tinyint NOT NULL DEFAULT 0,
  `backup_state` tinyint NOT NULL DEFAULT 0,
  `name` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'Passkey',
  `created_at` int NOT NULL,
  `last_used_at` int NOT NULL DEFAULT 0,
  PRIMARY KEY (`raw_id`),
  KEY `user_passkeys_user` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `user_passkey_challenges` (
  `challenge` varchar(256) COLLATE utf8mb4_unicode_ci NOT NULL,
  `user_id` bigint NOT NULL DEFAULT 0,
  `auth_key_id` bigint NOT NULL,
  `kind` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `session_json` text COLLATE utf8mb4_unicode_ci NOT NULL,
  `expires_at` int NOT NULL,
  PRIMARY KEY (`challenge`),
  KEY `user_passkey_challenges_user` (`user_id`, `kind`),
  KEY `user_passkey_challenges_auth` (`auth_key_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
