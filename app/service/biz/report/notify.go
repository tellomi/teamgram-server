package report

import (
	"context"
	"strings"

	"github.com/zeromicro/go-zero/core/logx"
)

// Notifier delivers one private message from @admin.
type Notifier interface {
	Send(ctx context.Context, to int64, text string) error
}

func (s *Service) send(ctx context.Context, to int64, text string) error {
	if s == nil || s.notifier == nil || to == 0 || text == "" {
		return nil
	}
	if err := s.notifier.Send(ctx, to, text); err != nil {
		logx.WithContext(ctx).Errorf("report: notice failed user=%d", to)
		return err
	}
	return nil
}

func (s *Service) sendReceipt(ctx context.Context, row *Row) {
	if row == nil {
		return
	}
	text := Receipt(row.Lang, row.Ticket(), etaText(s.etaHours, row.Lang))
	_ = s.send(ctx, row.Reporter, text)
}

func (s *Service) langFor(ctx context.Context, userID int64, stored string) string {
	if strings.TrimSpace(stored) != "" {
		return stored
	}
	if s != nil && s.langs != nil && userID != 0 {
		if v := s.langs.Lang(ctx, userID); v != "" {
			return v
		}
	}
	return ""
}

// notifyDecision tells each reporter once for this batch. A violation also
// tells the reported person once per target. That message never names the
// reporter. A clear result does not message the reported person.
func (s *Service) notifyDecision(ctx context.Context, target *Row, rows []*Row, verdict string) {
	if s == nil || target == nil || len(rows) == 0 {
		return
	}
	byReporter := map[int64][]*Row{}
	for _, row := range rows {
		if row == nil || row.Reporter == 0 {
			continue
		}
		byReporter[row.Reporter] = append(byReporter[row.Reporter], row)
	}
	for uid, group := range byReporter {
		ids := make([]int64, 0, len(group))
		lang := ""
		for _, row := range group {
			ids = append(ids, row.ID)
			if lang == "" {
				lang = row.Lang
			}
		}
		lang = s.langFor(ctx, uid, lang)
		text := DecisionForReporter(lang, verdict, JoinTickets(ids, lang))
		_ = s.send(ctx, uid, text)
	}
	if verdict != StatusViolation || target.SubjectUserID == 0 {
		return
	}
	fresh, err := s.store.ClaimNotice(ctx, target.TargetKey, target.SubjectUserID, noticeSubject, verdict)
	if err != nil || !fresh {
		return
	}
	lang := s.langFor(ctx, target.SubjectUserID, "")
	text := DecisionForSubject(lang, verdict)
	if text == "" {
		return
	}
	if err = s.send(ctx, target.SubjectUserID, text); err != nil {
		_ = s.store.ReleaseNotice(ctx, target.TargetKey, target.SubjectUserID, noticeSubject, verdict)
	}
}
