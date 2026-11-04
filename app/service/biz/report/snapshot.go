package report

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zeromicro/go-zero/core/logx"
)

// ObjectStore writes one evidence object. The key always starts with reports/.
type ObjectStore interface {
	Put(ctx context.Context, key string, body []byte, contentType string) error
}

type snapDoc struct {
	ReportID int64     `json:"report_id"`
	Source   string    `json:"source"`
	Ticket   string    `json:"ticket"`
	Reason   string    `json:"reason,omitempty"`
	Comment  string    `json:"comment,omitempty"`
	Items    []Content `json:"items,omitempty"`
	Post     *Content  `json:"post,omitempty"`
}

func snapshotKey(id int64) string {
	return fmt.Sprintf("reports/%d.json", id)
}

func snapshotJSON(row *Row, ev Evidence) ([]byte, error) {
	doc := snapDoc{
		ReportID: row.ID,
		Source:   row.Source,
		Ticket:   row.Ticket(),
		Reason:   row.Reason,
		Comment:  row.Message,
		Items:    ev.Items,
		Post:     ev.Post,
	}
	return json.Marshal(doc)
}

func (s *Service) writeSnapshot(ctx context.Context, row *Row, ev Evidence) {
	if s == nil || s.objects == nil || row == nil || row.ID == 0 {
		return
	}
	body, err := snapshotJSON(row, ev)
	if err != nil {
		return
	}
	key := snapshotKey(row.ID)
	if !strings.HasPrefix(key, "reports/") {
		return
	}
	if err = s.objects.Put(ctx, key, body, "application/json"); err != nil {
		logx.WithContext(ctx).Errorf("report: snapshot failed id=%d", row.ID)
		return
	}
	if err = s.store.UpdateEvidence(ctx, row.ID, key); err != nil {
		logx.WithContext(ctx).Errorf("report: snapshot key failed id=%d", row.ID)
		return
	}
	row.EvidenceKey = key
}
