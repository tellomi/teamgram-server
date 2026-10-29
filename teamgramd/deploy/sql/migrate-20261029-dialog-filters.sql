-- Per-user chat folders and the folder-tag flag.
-- messages.updateDialogFilter writes filter_blob (layer 232 DialogFilter).
-- messages.updateDialogFiltersOrder rewrites position.
-- messages.toggleDialogFilterTags writes dialog_filter_prefs.
-- messages.getDialogFilters reads both. Safe to run more than once.
-- The bff process also creates these tables on startup.

CREATE TABLE IF NOT EXISTS dialog_filters (
  user_id BIGINT NOT NULL,
  filter_id INT NOT NULL,
  position INT NOT NULL,
  filter_blob MEDIUMBLOB NOT NULL,
  PRIMARY KEY (user_id, filter_id),
  KEY dialog_filters_order (user_id, position, filter_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS dialog_filter_prefs (
  user_id BIGINT NOT NULL,
  tags_enabled TINYINT NOT NULL,
  PRIMARY KEY (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
