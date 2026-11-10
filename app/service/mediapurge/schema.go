package mediapurge

// schemaSQL matches teamgramd/deploy/sql/migrate-20261120-media-purge.sql.
// Both are safe to run more than once. New tables do not lock messages.
const schemaSQL = `
CREATE TABLE IF NOT EXISTS message_media_refs (
  id bigint NOT NULL AUTO_INCREMENT,
  box_kind tinyint NOT NULL,
  owner_id bigint NOT NULL,
  peer_type int NOT NULL DEFAULT 0,
  peer_id bigint NOT NULL DEFAULT 0,
  box_id int NOT NULL,
  media_kind varchar(16) NOT NULL,
  media_id bigint NOT NULL,
  created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_box_media (box_kind, owner_id, peer_type, peer_id, box_id, media_kind, media_id),
  KEY idx_media (media_kind, media_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS media_purge_queue (
  id bigint NOT NULL AUTO_INCREMENT,
  media_kind varchar(16) NOT NULL,
  media_id bigint NOT NULL,
  enqueued_at bigint NOT NULL,
  available_at bigint NOT NULL,
  attempts int NOT NULL DEFAULT 0,
  last_error varchar(255) NOT NULL DEFAULT '',
  PRIMARY KEY (id),
  UNIQUE KEY uk_purge_media (media_kind, media_id),
  KEY idx_purge_due (available_at, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS media_external_refs (
  id bigint NOT NULL AUTO_INCREMENT,
  source varchar(32) NOT NULL,
  media_kind varchar(16) NOT NULL,
  media_id bigint NOT NULL,
  created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_ext (source, media_kind, media_id),
  KEY idx_ext_media (media_kind, media_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
`

// Tombstone is the message_data value written when a user box is deleted.
// It is valid JSON and contains no text and no media.
const Tombstone = `{}`

// Generated columns from migrate-20261125-media-json-columns.sql.
// Filtering on the column is what makes MySQL use the secondary index.
const (
	exprDocumentID = "media_document_id"
	exprPhotoID    = "media_photo_id"
)
