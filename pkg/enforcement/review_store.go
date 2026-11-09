package enforcement

import (
	"database/sql"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	reviewAccount = "account"
	reviewContent = "content"
	reviewLift    = "lift"

	reviewPending    = "pending"
	reviewConfirming = "confirming"
	reviewBanned     = "banned"
	reviewClosed     = "closed"
	reviewRemoved    = "removed"
	reviewKept       = "kept"
	reviewClear      = "clear"
	reviewLifted     = "lifted"
)

// ReviewCase is one AI decision waiting on an admin, or already decided.
// It lives in the enforcement database, not in the chat schema.
type ReviewCase struct {
	ID            string
	Kind          string
	Status        string
	ReportID      int64
	SubjectUserID int64
	SubjectLabel  string
	ContentRef    string
	Summary       string
	AIReason      string
	AIAction      string
	Confidence    float64
	SanctionID    string
	AdminUser     string
	AdminAnswer   string
	Deliveries    string
	CreatedAt     time.Time
	DecidedAt     time.Time
}

type reviewBook struct {
	mu   sync.Mutex
	db   *sql.DB
	rows map[string]*ReviewCase
}

func newReviewBook(db *sql.DB) *reviewBook {
	return &reviewBook{db: db, rows: map[string]*ReviewCase{}}
}

func (s *Service) book() *reviewBook {
	if s == nil {
		return nil
	}
	if s.reviews == nil {
		s.reviews = newReviewBook(s.db)
	}
	return s.reviews
}

func (b *reviewBook) insert(row ReviewCase) error {
	if b == nil {
		return ErrNotFound
	}
	row.SubjectLabel = clipRunes(row.SubjectLabel, 160)
	row.Summary = clipRunes(row.Summary, 500)
	row.AIReason = clipRunes(row.AIReason, 500)
	row.ContentRef = clipRunes(row.ContentRef, 250)
	row.CreatedAt = row.CreatedAt.UTC()
	if b.db == nil {
		b.mu.Lock()
		defer b.mu.Unlock()
		if row.ReportID > 0 {
			for _, got := range b.rows {
				if got.ReportID == row.ReportID && got.Kind == row.Kind {
					return nil
				}
			}
		}
		cp := row
		b.rows[row.ID] = &cp
		return nil
	}
	if row.ReportID > 0 {
		var n int
		err := b.db.QueryRow(`SELECT COUNT(*) FROM review_case WHERE report_id=? AND kind=?`, row.ReportID, row.Kind).Scan(&n)
		if err != nil {
			return err
		}
		if n > 0 {
			return nil
		}
	}
	var decided any
	if !row.DecidedAt.IsZero() {
		decided = row.DecidedAt.UTC()
	}
	_, err := b.db.Exec(`INSERT INTO review_case (
		id, kind, status, report_id, subject_user_id, subject_label, content_ref,
		summary, ai_reason, ai_action, confidence, sanction_id, admin_user, admin_answer,
		deliveries, created_at, decided_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		row.ID, row.Kind, row.Status, row.ReportID, row.SubjectUserID, row.SubjectLabel, row.ContentRef,
		row.Summary, row.AIReason, row.AIAction, row.Confidence, row.SanctionID, row.AdminUser, row.AdminAnswer,
		row.Deliveries, row.CreatedAt, decided)
	return err
}

func (b *reviewBook) get(id string) (ReviewCase, bool, error) {
	if b == nil || id == "" {
		return ReviewCase{}, false, nil
	}
	if b.db == nil {
		b.mu.Lock()
		defer b.mu.Unlock()
		got := b.rows[id]
		if got == nil {
			return ReviewCase{}, false, nil
		}
		return *got, true, nil
	}
	row := b.db.QueryRow(`SELECT `+reviewCols+` FROM review_case WHERE id=?`, id)
	got, err := scanReview(row.Scan)
	if err == sql.ErrNoRows {
		return ReviewCase{}, false, nil
	}
	if err != nil {
		return ReviewCase{}, false, err
	}
	return got, true, nil
}

func (b *reviewBook) byReport(reportID int64, kind string) (ReviewCase, bool, error) {
	if b == nil || reportID == 0 {
		return ReviewCase{}, false, nil
	}
	if b.db == nil {
		b.mu.Lock()
		defer b.mu.Unlock()
		for _, got := range b.rows {
			if got.ReportID == reportID && got.Kind == kind {
				return *got, true, nil
			}
		}
		return ReviewCase{}, false, nil
	}
	row := b.db.QueryRow(`SELECT `+reviewCols+` FROM review_case WHERE report_id=? AND kind=? ORDER BY created_at LIMIT 1`, reportID, kind)
	got, err := scanReview(row.Scan)
	if err == sql.ErrNoRows {
		return ReviewCase{}, false, nil
	}
	if err != nil {
		return ReviewCase{}, false, err
	}
	return got, true, nil
}

func (b *reviewBook) pending() ([]ReviewCase, error) {
	if b == nil {
		return nil, nil
	}
	if b.db == nil {
		b.mu.Lock()
		defer b.mu.Unlock()
		var out []ReviewCase
		for _, got := range b.rows {
			if got.Status == reviewPending {
				out = append(out, *got)
			}
		}
		return out, nil
	}
	q, err := b.db.Query(`SELECT `+reviewCols+` FROM review_case WHERE status=?`, reviewPending)
	if err != nil {
		return nil, err
	}
	defer q.Close()
	var out []ReviewCase
	for q.Next() {
		got, err := scanReview(q.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, got)
	}
	return out, q.Err()
}

// claim moves a pending case to confirming. The second caller gets false.
func (b *reviewBook) claim(id string) (ReviewCase, bool, error) {
	if b == nil || id == "" {
		return ReviewCase{}, false, nil
	}
	if b.db == nil {
		b.mu.Lock()
		defer b.mu.Unlock()
		got := b.rows[id]
		if got == nil || got.Status != reviewPending {
			return ReviewCase{}, false, nil
		}
		got.Status = reviewConfirming
		return *got, true, nil
	}
	res, err := b.db.Exec(`UPDATE review_case SET status=? WHERE id=? AND status=?`, reviewConfirming, id, reviewPending)
	if err != nil {
		return ReviewCase{}, false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return ReviewCase{}, false, err
	}
	if n == 0 {
		return ReviewCase{}, false, nil
	}
	return b.get(id)
}

func (b *reviewBook) save(row ReviewCase) error {
	if b == nil {
		return ErrNotFound
	}
	row.SubjectLabel = clipRunes(row.SubjectLabel, 160)
	row.Summary = clipRunes(row.Summary, 500)
	row.AIReason = clipRunes(row.AIReason, 500)
	if b.db == nil {
		b.mu.Lock()
		defer b.mu.Unlock()
		cp := row
		b.rows[row.ID] = &cp
		return nil
	}
	var decided any
	if !row.DecidedAt.IsZero() {
		decided = row.DecidedAt.UTC()
	}
	_, err := b.db.Exec(`UPDATE review_case SET status=?, summary=?, ai_reason=?, ai_action=?, confidence=?, sanction_id=?, admin_user=?, admin_answer=?, deliveries=?, decided_at=? WHERE id=?`,
		row.Status, row.Summary, row.AIReason, row.AIAction, row.Confidence, row.SanctionID, row.AdminUser, row.AdminAnswer, row.Deliveries, decided, row.ID)
	return err
}

const reviewCols = `id, kind, status, report_id, subject_user_id, subject_label, content_ref, summary, ai_reason, ai_action, confidence, sanction_id, admin_user, admin_answer, deliveries, created_at, decided_at`

func scanReview(scan func(dest ...any) error) (ReviewCase, error) {
	var row ReviewCase
	var decided sql.NullTime
	err := scan(&row.ID, &row.Kind, &row.Status, &row.ReportID, &row.SubjectUserID, &row.SubjectLabel, &row.ContentRef,
		&row.Summary, &row.AIReason, &row.AIAction, &row.Confidence, &row.SanctionID, &row.AdminUser, &row.AdminAnswer,
		&row.Deliveries, &row.CreatedAt, &decided)
	if err != nil {
		return ReviewCase{}, err
	}
	row.CreatedAt = row.CreatedAt.UTC()
	if decided.Valid {
		row.DecidedAt = decided.Time.UTC()
	}
	return row, nil
}

func clipRunes(s string, n int) string {
	if n <= 0 || utf8.RuneCountInString(s) <= n {
		return s
	}
	i := 0
	for k := range s {
		if i == n {
			return s[:k]
		}
		i++
	}
	return s
}

func (s *Service) RecordAudit(actor, action, caseID, detail string) {
	if s == nil {
		return
	}
	detail = clipRunes(strings.TrimSpace(detail), 480)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.begin(); err != nil {
		return
	}
	s.auditLocked(actor, action, caseID, detail)
	_ = s.finish(nil)
}
