-- Maps a content-engine sub (16-byte pseudonym) back to a teamgram user id.
-- Written when discover.getEngineCredential succeeds and
-- TELLOMI_DISCOVER_RELATIONS=1. The engine never sees the user id.
-- Safe to run more than once.

CREATE TABLE IF NOT EXISTS discover_subject (
  sub BINARY(16) NOT NULL,
  user_id BIGINT NOT NULL,
  PRIMARY KEY (sub),
  UNIQUE KEY discover_subject_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
