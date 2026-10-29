-- Active group-call binding read by the biz chat service (chat.call).
CREATE TABLE IF NOT EXISTS phone_peer_call (
  peer_type INT NOT NULL,
  peer_id BIGINT NOT NULL,
  call_id BIGINT NOT NULL,
  access_hash BIGINT NOT NULL,
  active TINYINT NOT NULL,
  not_empty TINYINT NOT NULL,
  PRIMARY KEY (peer_type, peer_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
