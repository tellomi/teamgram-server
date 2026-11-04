-- Unified report rows. stories.report, messages.report, messages.reportReaction,
-- account.reportPeer, account.reportProfilePhoto, messages.reportSpam, and
-- channels.reportSpam all insert here. Safe to run more than once.
-- The report service copies existing story_reports rows on startup
-- (legacy_story_id keeps that copy idempotent).
-- Does not read or write Redis.

CREATE TABLE IF NOT EXISTS reports (
  id BIGINT NOT NULL AUTO_INCREMENT,
  source VARCHAR(64) NOT NULL,
  reporter_user_id BIGINT NOT NULL,
  lang_code VARCHAR(32) NOT NULL DEFAULT '',
  peer_type INT NOT NULL DEFAULT 0,
  peer_id BIGINT NOT NULL DEFAULT 0,
  subject_user_id BIGINT NOT NULL DEFAULT 0,
  target_key VARCHAR(191) NOT NULL,
  reason VARCHAR(64) NOT NULL DEFAULT '',
  message TEXT,
  object_ids VARCHAR(255) NOT NULL DEFAULT '',
  photo_id BIGINT NOT NULL DEFAULT 0,
  reaction_peer_id BIGINT NOT NULL DEFAULT 0,
  evidence_key VARCHAR(255) NOT NULL DEFAULT '',
  status VARCHAR(16) NOT NULL DEFAULT 'needs_human',
  legacy_story_id BIGINT NULL,
  dedupe_key VARCHAR(191) NULL,
  created_at INT NOT NULL,
  decided_at INT NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY reports_legacy_story (legacy_story_id),
  UNIQUE KEY reports_dedupe (dedupe_key),
  KEY reports_reporter_time (reporter_user_id, created_at),
  KEY reports_target (target_key, status),
  KEY reports_status (status, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS report_notices (
  target_key VARCHAR(191) NOT NULL,
  user_id BIGINT NOT NULL,
  kind VARCHAR(32) NOT NULL,
  decision VARCHAR(16) NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (target_key, user_id, kind, decision)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
