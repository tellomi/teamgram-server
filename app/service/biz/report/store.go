package report

import "context"

// Store is the reports table. Memory is process-local. MySQL survives a restart.
type Store interface {
	Insert(ctx context.Context, row *Row) error
	Get(ctx context.Context, id int64) (*Row, error)
	ListAll(ctx context.Context) ([]*Row, error)
	ListByTarget(ctx context.Context, key string) ([]*Row, error)
	ListPending(ctx context.Context, limit int) ([]*Row, error)
	Recent(ctx context.Context, reporter int64, since int32) (count int, oldest int32, err error)
	UpdateStatus(ctx context.Context, ids []int64, status string, decidedAt int32) error
	UpdateEvidence(ctx context.Context, id int64, key string) error
	HasLegacy(ctx context.Context, storyID int64) (bool, error)
	ClaimNotice(ctx context.Context, targetKey string, userID int64, kind, decision string) (fresh bool, err error)
	ReleaseNotice(ctx context.Context, targetKey string, userID int64, kind, decision string) error
}

func copyRow(r *Row) *Row {
	if r == nil {
		return nil
	}
	cp := *r
	cp.ObjectIDs = append([]int32(nil), r.ObjectIDs...)
	return &cp
}
