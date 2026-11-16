-- Telegram Business profile: hours, location, greeting, away message,
-- intro, and chat links. account.updateBusiness* writes this table.
-- Safe to run more than once. Does not delete or rewrite existing rows.

CREATE TABLE IF NOT EXISTS `business_profile` (
  `doc_key` varchar(191) NOT NULL,
  `data` mediumblob NOT NULL,
  PRIMARY KEY (`doc_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
