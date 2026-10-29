-- Per-user acceptance of a terms-of-service id.
-- help.acceptTermsOfService inserts a row. help.getTermsOfServiceUpdate
-- returns help.termsOfServiceUpdateEmpty unless Tos.UpdateId is an id this
-- user has not accepted. Safe to run more than once.
-- The bff process also creates this table on startup.

CREATE TABLE IF NOT EXISTS user_tos_acceptances (
  user_id BIGINT NOT NULL,
  tos_id VARCHAR(128) NOT NULL,
  accepted_at INT NOT NULL,
  PRIMARY KEY (user_id, tos_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
