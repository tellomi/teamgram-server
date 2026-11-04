package report

import (
	"context"
	"errors"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/logx"
)

// Service files reports, sends the @admin receipt, and applies a decision.
type Service struct {
	store    Store
	notifier Notifier
	objects  ObjectStore
	reviewer *Reviewer
	langs    LangLookup
	clock    func() int32
	limit    int
	window   int32
	etaHours int
}

// New returns a service. A nil store uses memory.
// Review and object storage stay off until ApplyEnv or a test sets them.
func New(store Store) *Service {
	if store == nil {
		store = NewMemory()
	}
	return &Service{store: store, limit: 20, window: 3600, etaHours: 24}
}

func (s *Service) now() int32 {
	if s != nil && s.clock != nil {
		return s.clock()
	}
	return int32(time.Now().Unix())
}

// SetNotifier installs the @admin sender. Nil skips notices.
func (s *Service) SetNotifier(n Notifier) {
	if s != nil {
		s.notifier = n
	}
}

// SetObjects installs the evidence store. Nil skips snapshots.
func (s *Service) SetObjects(o ObjectStore) {
	if s != nil {
		s.objects = o
	}
}

// SetReviewer installs the classifier. Nil leaves every row for a person.
func (s *Service) SetReviewer(r *Reviewer) {
	if s != nil {
		s.reviewer = r
	}
}

// SetLangs installs the recipient lang_code lookup.
func (s *Service) SetLangs(l LangLookup) {
	if s != nil {
		s.langs = l
	}
}

// List returns every row. Tests and the pending queue use it.
func (s *Service) List(ctx context.Context) ([]*Row, error) {
	if s == nil || s.store == nil {
		return nil, errors.New("report: service is not configured")
	}
	return s.store.ListAll(ctx)
}

// Pending is the human queue: rows still waiting for a person.
func (s *Service) Pending(ctx context.Context, limit int) ([]*Row, error) {
	if s == nil || s.store == nil {
		return nil, errors.New("report: service is not configured")
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.store.ListPending(ctx, limit)
}

// Get returns one row. A missing id returns a nil row.
func (s *Service) Get(ctx context.Context, id int64) (*Row, error) {
	if s == nil || s.store == nil {
		return nil, errors.New("report: service is not configured")
	}
	return s.store.Get(ctx, id)
}

// Submit stores one report, writes the snapshot when configured, sends the
// receipt, and classifies when the reviewer is on.
func (s *Service) Submit(ctx context.Context, in Submission) (*Row, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	if s == nil || s.store == nil {
		return nil, errors.New("report: service is not configured")
	}
	now := s.now()
	if err := s.rateLimit(ctx, in.Reporter, now); err != nil {
		return nil, err
	}
	if evidenceEmpty(in.Evidence) {
		if loader, ok := s.store.(interface {
			Load(context.Context, Submission) (Evidence, error)
		}); ok {
			if ev, err := loader.Load(ctx, in); err == nil && !evidenceEmpty(ev) {
				in.Evidence = ev
			}
		}
	}
	lang := in.Lang
	if lang == "" {
		lang = LangFrom(ctx)
	}
	row := &Row{
		Source:         in.Source,
		Reporter:       in.Reporter,
		Lang:           lang,
		PeerType:       in.PeerType,
		PeerID:         in.PeerID,
		SubjectUserID:  in.SubjectUserID,
		TargetKey:      TargetKey(in.Source, in.PeerType, in.PeerID, in.ObjectIDs, in.PhotoID, in.ReactionPeerID),
		Reason:         clipRunes(in.Reason, 64),
		Message:        in.Message,
		ObjectIDs:      normalizeIDs(in.ObjectIDs),
		PhotoID:        in.PhotoID,
		ReactionPeerID: in.ReactionPeerID,
		Status:         StatusPending,
		LegacyStoryID:  in.LegacyStoryID,
		CreatedAt:      now,
	}
	if err := s.store.Insert(ctx, row); err != nil {
		return nil, err
	}
	s.writeSnapshot(ctx, row, in.Evidence)
	s.sendReceipt(ctx, row)
	s.review(ctx, row, in.Evidence)
	return row, nil
}

func (s *Service) rateLimit(ctx context.Context, reporter int64, now int32) error {
	limit := s.limit
	if limit <= 0 {
		limit = 20
	}
	window := s.window
	if window <= 0 {
		window = 3600
	}
	since := now - window
	if since < 0 {
		since = 0
	}
	count, oldest, err := s.store.Recent(ctx, reporter, since)
	if err != nil {
		return err
	}
	if count < limit {
		return nil
	}
	retry := window
	if oldest > 0 {
		retry = window - (now - oldest)
	}
	if retry < 1 {
		retry = 1
	}
	return mtproto.NewErrFloodWaitX(retry)
}

func (s *Service) review(ctx context.Context, row *Row, ev Evidence) {
	verdict := StatusNeedsHuman
	if s.reviewer != nil {
		verdict = s.reviewer.Review(ctx, row, ev)
	}
	switch verdict {
	case StatusViolation, StatusClear:
		if err := s.applyDecision(ctx, row, verdict); err != nil {
			logx.WithContext(ctx).Errorf("report: decision failed id=%d", row.ID)
			_ = s.store.UpdateStatus(ctx, []int64{row.ID}, StatusNeedsHuman, 0)
			row.Status = StatusNeedsHuman
			row.DecidedAt = 0
		}
	default:
		_ = s.store.UpdateStatus(ctx, []int64{row.ID}, StatusNeedsHuman, 0)
		row.Status = StatusNeedsHuman
	}
}

// Decide records a human result for one report and every still-open report
// of the same target. violation or clear are the only accepted values.
func (s *Service) Decide(ctx context.Context, id int64, verdict string) error {
	if verdict != StatusViolation && verdict != StatusClear {
		return ErrBadDecision
	}
	if s == nil || s.store == nil {
		return errors.New("report: service is not configured")
	}
	row, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if row == nil {
		return ErrNotFound
	}
	if row.Status == StatusViolation || row.Status == StatusClear {
		if row.Status != verdict {
			return ErrConflict
		}
		return nil
	}
	return s.applyDecision(ctx, row, verdict)
}

func (s *Service) applyDecision(ctx context.Context, row *Row, verdict string) error {
	group, err := s.store.ListByTarget(ctx, row.TargetKey)
	if err != nil {
		return err
	}
	var open []*Row
	for _, g := range group {
		if g.Status == StatusViolation || g.Status == StatusClear {
			continue
		}
		open = append(open, g)
	}
	if len(open) == 0 {
		return nil
	}
	ids := make([]int64, len(open))
	for i, g := range open {
		ids[i] = g.ID
	}
	now := s.now()
	if err = s.store.UpdateStatus(ctx, ids, verdict, now); err != nil {
		return err
	}
	for _, g := range open {
		g.Status = verdict
		g.DecidedAt = now
		if g.ID == row.ID {
			row.Status = verdict
			row.DecidedAt = now
		}
	}
	s.notifyDecision(ctx, row, open, verdict)
	return nil
}

// ErrNotFound means the id is not in the table.
var ErrNotFound = errors.New("report: not found")

// ErrBadDecision means the result is not violation or clear.
var ErrBadDecision = errors.New("report: bad decision")

// ErrConflict means the row already has the other result.
var ErrConflict = errors.New("report: already decided")
