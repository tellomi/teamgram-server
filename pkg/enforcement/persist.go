package enforcement

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/teamgram/teamgram-server/pkg/mysqlpool"
	"github.com/teamgram/teamgram-server/pkg/mysqltls"

	mysqldriver "github.com/go-sql-driver/mysql"
)

// ErrSchema means Migrate has not been run. OpenMySQL does not create tables.
var ErrSchema = errors.New("enforcement: schema is not initialized")

// OpenMySQL connects to a database that is not the chat schema. It does not
// create or alter tables. Call Migrate first, as an explicit step.
func OpenMySQL(dsn string, cfg Config) (*Service, error) {
	if _, err := databaseName(dsn); err != nil {
		return nil, err
	}
	db, err := mysqltls.Open(dsn)
	if err != nil {
		return nil, err
	}
	if err = db.Ping(); err != nil {
		_ = mysqlpool.Discard(db)
		return nil, err
	}
	var n int
	if err = db.QueryRow("SELECT COUNT(*) FROM sanction").Scan(&n); err != nil {
		_ = mysqlpool.Discard(db)
		return nil, ErrSchema
	}
	s := New(cfg)
	s.db = db
	if s.reviews != nil {
		s.reviews.db = db
	}
	return s, nil
}

// Migrate creates the enforcement tables if they are missing. It is not called
// from process startup. A second call is a no-op. It does not drop data.
func Migrate(dsn string) error {
	if _, err := databaseName(dsn); err != nil {
		return err
	}
	db, err := mysqltls.Open(dsn)
	if err != nil {
		return err
	}
	defer mysqlpool.Discard(db)
	if err = db.Ping(); err != nil {
		return err
	}
	for _, stmt := range schemaDDL {
		if _, err = db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func databaseName(dsn string) (string, error) {
	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(cfg.DBName)
	switch strings.ToLower(name) {
	case "", "teamgram", "mysql", "information_schema", "performance_schema", "sys":
		return "", fmt.Errorf("enforcement: database %q is not a separate enforcement database", name)
	}
	return name, nil
}

var schemaDDL = []string{
	`CREATE TABLE IF NOT EXISTS sanction (
			id VARCHAR(32) PRIMARY KEY,
			user_id BIGINT NOT NULL,
			kind VARCHAR(32) NOT NULL,
			scopes VARCHAR(255) NOT NULL DEFAULT '',
			reason_category VARCHAR(128) NOT NULL DEFAULT '',
			policy_clause VARCHAR(128) NOT NULL DEFAULT '',
			rule_version VARCHAR(64) NOT NULL DEFAULT '',
			evidence_ref VARCHAR(255) NOT NULL DEFAULT '',
			initiator VARCHAR(64) NOT NULL,
			approver VARCHAR(64) NOT NULL DEFAULT '',
			start_at DATETIME(6) NOT NULL,
			end_at DATETIME(6) NULL,
			permanent TINYINT NOT NULL DEFAULT 0,
			status VARCHAR(32) NOT NULL,
			idempotency_key VARCHAR(128) NULL,
			UNIQUE KEY sanction_idem (idempotency_key),
			KEY sanction_user (user_id, status)
		)`,
	`CREATE TABLE IF NOT EXISTS sanction_event (
			id VARCHAR(32) PRIMARY KEY,
			sanction_id VARCHAR(32) NOT NULL,
			event_type VARCHAR(32) NOT NULL,
			actor VARCHAR(64) NOT NULL,
			at DATETIME(6) NOT NULL,
			reason VARCHAR(512) NOT NULL DEFAULT '',
			KEY sanction_event_case (sanction_id)
		)`,
	`CREATE TABLE IF NOT EXISTS notice_outbox (
			idempotency_key VARCHAR(160) PRIMARY KEY,
			user_id BIGINT NOT NULL,
			from_user BIGINT NOT NULL,
			template VARCHAR(16) NOT NULL,
			body TEXT NOT NULL,
			status VARCHAR(16) NOT NULL,
			attempts INT NOT NULL DEFAULT 0
		)`,
	`CREATE TABLE IF NOT EXISTS appeal (
			id VARCHAR(32) PRIMARY KEY,
			sanction_id VARCHAR(32) NOT NULL,
			user_id BIGINT NOT NULL,
			body TEXT NOT NULL,
			status VARCHAR(32) NOT NULL,
			action VARCHAR(32) NOT NULL DEFAULT '',
			user_reason TEXT NOT NULL,
			rule_ref VARCHAR(128) NOT NULL DEFAULT '',
			evidence_refs TEXT NOT NULL,
			model VARCHAR(64) NOT NULL DEFAULT '',
			policy_version VARCHAR(64) NOT NULL DEFAULT '',
			attempts INT NOT NULL DEFAULT 0,
			UNIQUE KEY appeal_case (sanction_id)
		)`,
	`CREATE TABLE IF NOT EXISTS audit_log (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			at DATETIME(6) NOT NULL,
			actor VARCHAR(64) NOT NULL,
			action VARCHAR(64) NOT NULL,
			case_id VARCHAR(32) NOT NULL DEFAULT '',
			detail VARCHAR(512) NOT NULL DEFAULT ''
		)`,
	`CREATE TABLE IF NOT EXISTS identifier_bind (
			kind VARCHAR(16) NOT NULL,
			value VARCHAR(255) NOT NULL,
			user_id BIGINT NOT NULL,
			PRIMARY KEY (kind, value)
		)`,
	`CREATE TABLE IF NOT EXISTS identifier_block (
			kind VARCHAR(16) NOT NULL,
			value VARCHAR(255) NOT NULL,
			PRIMARY KEY (kind, value)
		)`,
	`CREATE TABLE IF NOT EXISTS security_lock (
			user_id BIGINT PRIMARY KEY
		)`,
	`CREATE TABLE IF NOT EXISTS disposition_grant (
			token VARCHAR(128) PRIMARY KEY,
			user_id BIGINT NOT NULL,
			expires_at DATETIME(6) NOT NULL
		)`,
	`CREATE TABLE IF NOT EXISTS disposition_claim (
			proof VARCHAR(128) PRIMARY KEY,
			token VARCHAR(128) NOT NULL,
			user_id BIGINT NOT NULL,
			expires_at DATETIME(6) NOT NULL,
			used TINYINT NOT NULL DEFAULT 0
		)`,
	`CREATE TABLE IF NOT EXISTS login_proof (
			auth_key BIGINT PRIMARY KEY,
			proof VARCHAR(128) NOT NULL
		)`,
	`CREATE TABLE IF NOT EXISTS credential_subject (
			sub VARCHAR(32) PRIMARY KEY,
			user_id BIGINT NOT NULL,
			KEY credential_subject_user (user_id)
		)`,
	`CREATE TABLE IF NOT EXISTS review_case (
			id VARCHAR(32) PRIMARY KEY,
			kind VARCHAR(16) NOT NULL,
			status VARCHAR(32) NOT NULL,
			report_id BIGINT NOT NULL DEFAULT 0,
			subject_user_id BIGINT NOT NULL DEFAULT 0,
			subject_label VARCHAR(160) NOT NULL DEFAULT '',
			content_ref VARCHAR(255) NOT NULL DEFAULT '',
			summary VARCHAR(512) NOT NULL DEFAULT '',
			ai_reason VARCHAR(512) NOT NULL DEFAULT '',
			ai_action VARCHAR(32) NOT NULL DEFAULT '',
			confidence DOUBLE NOT NULL DEFAULT 0,
			sanction_id VARCHAR(32) NOT NULL DEFAULT '',
			admin_user VARCHAR(64) NOT NULL DEFAULT '',
			admin_answer VARCHAR(16) NOT NULL DEFAULT '',
			deliveries VARCHAR(512) NOT NULL DEFAULT '',
			created_at DATETIME(6) NOT NULL,
			decided_at DATETIME(6) NULL,
			KEY review_status (status),
			KEY review_report (report_id, kind)
		)`,
}

func (s *Service) begin() error {
	if s == nil || s.db == nil {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if err = loadAll(tx, s); err != nil {
		_ = tx.Rollback()
		return err
	}
	s.tx = tx
	return nil
}

func (s *Service) finish(err error) error {
	if s == nil || s.tx == nil {
		return err
	}
	tx := s.tx
	s.tx = nil
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if err = saveAll(tx, s); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *Service) markNotice(n *Notice) error {
	if s == nil || s.db == nil || n == nil {
		return nil
	}
	_, err := s.db.Exec(`UPDATE notice_outbox SET status = ?, attempts = ? WHERE idempotency_key = ?`, n.Status, n.Attempts, n.Key)
	return err
}

func loadAll(tx *sql.Tx, s *Service) error {
	rows := map[string]*Sanction{}
	var order []string
	byUser := map[int64][]string{}
	idem := map[string]string{}
	q, err := tx.Query(`SELECT id, user_id, kind, scopes, reason_category, policy_clause, rule_version, evidence_ref, initiator, approver, start_at, end_at, permanent, status, IFNULL(idempotency_key, '') FROM sanction`)
	if err != nil {
		return err
	}
	for q.Next() {
		var row Sanction
		var scopes, idemKey string
		var end sql.NullTime
		var permanent int
		if err = q.Scan(&row.ID, &row.UserID, &row.Kind, &scopes, &row.ReasonCategory, &row.PolicyClause, &row.RuleVersion, &row.EvidenceRef, &row.Initiator, &row.Approver, &row.Start, &end, &permanent, &row.Status, &idemKey); err != nil {
			q.Close()
			return err
		}
		if scopes != "" {
			row.Scopes = strings.Split(scopes, ",")
		}
		if end.Valid {
			t := end.Time.UTC()
			row.End = &t
		}
		row.Permanent = permanent == 1
		row.IdempotencyKey = idemKey
		row.Start = row.Start.UTC()
		rows[row.ID] = &row
		order = append(order, row.ID)
		byUser[row.UserID] = append(byUser[row.UserID], row.ID)
		if idemKey != "" {
			idem[idemKey] = row.ID
		}
	}
	if err = q.Err(); err != nil {
		q.Close()
		return err
	}
	q.Close()

	var events []Event
	q, err = tx.Query(`SELECT id, sanction_id, event_type, actor, at, reason FROM sanction_event`)
	if err != nil {
		return err
	}
	for q.Next() {
		var ev Event
		if err = q.Scan(&ev.ID, &ev.SanctionID, &ev.Type, &ev.Actor, &ev.At, &ev.Reason); err != nil {
			q.Close()
			return err
		}
		events = append(events, ev)
	}
	q.Close()

	notices := map[string]*Notice{}
	var noticeOrder []string
	q, err = tx.Query(`SELECT idempotency_key, user_id, from_user, template, body, status, attempts FROM notice_outbox`)
	if err != nil {
		return err
	}
	for q.Next() {
		n := &Notice{}
		if err = q.Scan(&n.Key, &n.UserID, &n.From, &n.Template, &n.Body, &n.Status, &n.Attempts); err != nil {
			q.Close()
			return err
		}
		notices[n.Key] = n
		noticeOrder = append(noticeOrder, n.Key)
	}
	q.Close()

	appeals := map[string]*Appeal{}
	byCase := map[string]string{}
	q, err = tx.Query(`SELECT id, sanction_id, user_id, body, status, action, user_reason, rule_ref, evidence_refs, model, policy_version, attempts FROM appeal`)
	if err != nil {
		return err
	}
	for q.Next() {
		a := &Appeal{}
		var refs string
		if err = q.Scan(&a.ID, &a.SanctionID, &a.UserID, &a.Text, &a.Status, &a.Action, &a.UserReason, &a.RuleRef, &refs, &a.Model, &a.PolicyVersion, &a.Attempts); err != nil {
			q.Close()
			return err
		}
		if refs != "" {
			a.EvidenceRefs = strings.Split(refs, "\n")
		}
		appeals[a.ID] = a
		byCase[a.SanctionID] = a.ID
	}
	q.Close()

	bind := map[string]int64{}
	q, err = tx.Query(`SELECT kind, value, user_id FROM identifier_bind`)
	if err != nil {
		return err
	}
	for q.Next() {
		var kind, value string
		var userID int64
		if err = q.Scan(&kind, &value, &userID); err != nil {
			q.Close()
			return err
		}
		bind[bindKey(kind, value)] = userID
	}
	q.Close()

	block := map[string]bool{}
	q, err = tx.Query(`SELECT kind, value FROM identifier_block`)
	if err != nil {
		return err
	}
	for q.Next() {
		var kind, value string
		if err = q.Scan(&kind, &value); err != nil {
			q.Close()
			return err
		}
		block[bindKey(kind, value)] = true
	}
	q.Close()

	locks := map[int64]bool{}
	q, err = tx.Query(`SELECT user_id FROM security_lock`)
	if err != nil {
		return err
	}
	for q.Next() {
		var id int64
		if err = q.Scan(&id); err != nil {
			q.Close()
			return err
		}
		locks[id] = true
	}
	q.Close()

	grants := map[string]*Grant{}
	q, err = tx.Query(`SELECT token, user_id, expires_at FROM disposition_grant`)
	if err != nil {
		return err
	}
	for q.Next() {
		g := &Grant{}
		if err = q.Scan(&g.Token, &g.UserID, &g.Expires); err != nil {
			q.Close()
			return err
		}
		grants[g.Token] = g
	}
	q.Close()

	var audits []Audit
	q, err = tx.Query(`SELECT at, actor, action, case_id, detail FROM audit_log ORDER BY id`)
	if err != nil {
		return err
	}
	for q.Next() {
		var a Audit
		if err = q.Scan(&a.At, &a.Actor, &a.Action, &a.CaseID, &a.Detail); err != nil {
			q.Close()
			return err
		}
		audits = append(audits, a)
	}
	q.Close()

	s.rows, s.order, s.byUser, s.idem = rows, order, byUser, idem
	s.events = events
	s.notices, s.noticeOrder = notices, noticeOrder
	s.appeals, s.byCase = appeals, byCase
	s.bind, s.idBlock, s.locks, s.grants = bind, block, locks, grants
	s.audits = audits
	s.auditFrom = len(audits)
	s.eventFrom = len(events)
	return nil
}

func saveAll(tx *sql.Tx, s *Service) error {
	for _, id := range s.order {
		row := s.rows[id]
		if row == nil {
			continue
		}
		var end any
		if row.End != nil {
			end = row.End.UTC()
		}
		perm := 0
		if row.Permanent {
			perm = 1
		}
		var idem any
		if row.IdempotencyKey != "" {
			idem = row.IdempotencyKey
		}
		_, err := tx.Exec(`INSERT INTO sanction (id, user_id, kind, scopes, reason_category, policy_clause, rule_version, evidence_ref, initiator, approver, start_at, end_at, permanent, status, idempotency_key)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON DUPLICATE KEY UPDATE status=VALUES(status), approver=VALUES(approver), end_at=VALUES(end_at)`,
			row.ID, row.UserID, row.Kind, strings.Join(row.Scopes, ","), row.ReasonCategory, row.PolicyClause, row.RuleVersion, row.EvidenceRef, row.Initiator, row.Approver, row.Start.UTC(), end, perm, row.Status, idem)
		if err != nil {
			return err
		}
	}
	for _, ev := range s.events[s.eventFrom:] {
		if _, err := tx.Exec(`INSERT IGNORE INTO sanction_event (id, sanction_id, event_type, actor, at, reason) VALUES (?,?,?,?,?,?)`, ev.ID, ev.SanctionID, ev.Type, ev.Actor, ev.At.UTC(), ev.Reason); err != nil {
			return err
		}
	}
	s.eventFrom = len(s.events)
	for _, key := range s.noticeOrder {
		n := s.notices[key]
		if n == nil {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO notice_outbox (idempotency_key, user_id, from_user, template, body, status, attempts) VALUES (?,?,?,?,?,?,?)
			ON DUPLICATE KEY UPDATE status=VALUES(status), attempts=VALUES(attempts)`, n.Key, n.UserID, n.From, n.Template, n.Body, n.Status, n.Attempts); err != nil {
			return err
		}
	}
	for id, a := range s.appeals {
		if _, err := tx.Exec(`INSERT INTO appeal (id, sanction_id, user_id, body, status, action, user_reason, rule_ref, evidence_refs, model, policy_version, attempts) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
			ON DUPLICATE KEY UPDATE status=VALUES(status), action=VALUES(action), user_reason=VALUES(user_reason), rule_ref=VALUES(rule_ref), evidence_refs=VALUES(evidence_refs), model=VALUES(model), policy_version=VALUES(policy_version), attempts=VALUES(attempts)`,
			id, a.SanctionID, a.UserID, a.Text, a.Status, a.Action, a.UserReason, a.RuleRef, strings.Join(a.EvidenceRefs, "\n"), a.Model, a.PolicyVersion, a.Attempts); err != nil {
			return err
		}
	}
	for key, userID := range s.bind {
		kind, value, _ := strings.Cut(key, "\x00")
		if _, err := tx.Exec(`INSERT INTO identifier_bind (kind, value, user_id) VALUES (?,?,?) ON DUPLICATE KEY UPDATE user_id=VALUES(user_id)`, kind, value, userID); err != nil {
			return err
		}
	}
	for key := range s.idBlock {
		kind, value, _ := strings.Cut(key, "\x00")
		if _, err := tx.Exec(`INSERT IGNORE INTO identifier_block (kind, value) VALUES (?,?)`, kind, value); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM security_lock`); err != nil {
		return err
	}
	for userID := range s.locks {
		if _, err := tx.Exec(`INSERT INTO security_lock (user_id) VALUES (?)`, userID); err != nil {
			return err
		}
	}
	for token, g := range s.grants {
		if _, err := tx.Exec(`INSERT INTO disposition_grant (token, user_id, expires_at) VALUES (?,?,?) ON DUPLICATE KEY UPDATE expires_at=VALUES(expires_at)`, token, g.UserID, g.Expires.UTC()); err != nil {
			return err
		}
	}
	for _, a := range s.audits[s.auditFrom:] {
		if _, err := tx.Exec(`INSERT INTO audit_log (at, actor, action, case_id, detail) VALUES (?,?,?,?,?)`, a.At.UTC(), a.Actor, a.Action, a.CaseID, a.Detail); err != nil {
			return err
		}
	}
	s.auditFrom = len(s.audits)
	return nil
}
