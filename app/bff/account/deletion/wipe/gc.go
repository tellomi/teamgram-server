package wipe

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/teamgram/teamgram-server/app/service/media/retention"
)

const (
	EnvDryRun   = "TELLOMI_ACCOUNT_GC_DRY_RUN"
	EnvKeepDays = "TELLOMI_ACCOUNT_GC_KEEP_DAYS"
	EnvBatch    = "TELLOMI_ACCOUNT_GC_BATCH"

	statusPending = "pending"
	statusTrashed = "trashed"
	statusSkipped = "skipped"
	statusDeleted = "deleted"
)

// GCConfig controls the async object cleaner. DryRun defaults to true.
type GCConfig struct {
	DryRun   bool
	Keep     time.Duration
	Batch    int
	Endpoint string
}

// DefaultGC is the production default: log the plan, do not delete objects,
// keep a trash copy for 30 days once a live run is turned on.
func DefaultGC() GCConfig {
	return GCConfig{DryRun: true, Keep: 30 * 24 * time.Hour, Batch: 100}
}

// GCFromEnv reads TELLOMI_ACCOUNT_GC_*. An unset dry-run flag stays true.
// Set TELLOMI_ACCOUNT_GC_DRY_RUN=false to move and delete objects.
func GCFromEnv() GCConfig {
	cfg := DefaultGC()
	if v := strings.TrimSpace(os.Getenv(EnvDryRun)); v != "" {
		cfg.DryRun = v != "0" && !strings.EqualFold(v, "false")
	}
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(EnvKeepDays))); err == nil && n > 0 {
		cfg.Keep = time.Duration(n) * 24 * time.Hour
	}
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(EnvBatch))); err == nil && n > 0 {
		cfg.Batch = n
	}
	return cfg
}

// GCItem is one queued object.
type GCItem struct {
	ID        int64
	UserID    int64
	Kind      string
	MediaID   int64
	Class     string
	Path      string
	Status    string
	TrashedAt int64
}

// GCStore is the queue and the reference count.
type GCStore interface {
	Due(ctx context.Context, status string, limit int) ([]GCItem, error)
	// Refs is how many live messages, profiles, or sticker rows still use it.
	// A negative result means the count could not be proven and the object
	// must be kept.
	Refs(ctx context.Context, item GCItem) (int, error)
	Exempt(ctx context.Context, item GCItem) (bool, error)
	Mark(ctx context.Context, id int64, status string, trashedAt int64) error
}

// Mover moves objects inside the same bucket the dfs client writes.
type Mover interface {
	MoveToTrash(ctx context.Context, class, path string) error
	DeleteTrash(ctx context.Context, class, path string) error
}

// GCReport is one pass, safe to log. It has no credentials and no file bytes.
type GCReport struct {
	DryRun      bool
	Endpoint    string
	WouldTrash  int
	WouldDelete int
	Trashed     int
	Deleted     int
	Referenced  int
	Exempt      int
	Errors      int
	Samples     []string
}

func (r GCReport) String() string {
	mode := "LIVE"
	if r.DryRun {
		mode = "DRY_RUN"
	}
	return fmt.Sprintf("account gc %s endpoint=%s would_trash=%d would_delete=%d trashed=%d deleted=%d still_referenced=%d exempt=%d errors=%d samples=%q",
		mode, r.Endpoint, r.WouldTrash, r.WouldDelete, r.Trashed, r.Deleted, r.Referenced, r.Exempt, r.Errors, r.Samples)
}

// RunGC processes one batch. Referenced objects stay pending so a later pass
// can trash them after the last group message is gone. Dry-run only logs.
func RunGC(ctx context.Context, store GCStore, mover Mover, cfg GCConfig, now time.Time) (GCReport, error) {
	if cfg.Batch <= 0 {
		cfg.Batch = DefaultGC().Batch
	}
	if cfg.Keep <= 0 {
		cfg.Keep = DefaultGC().Keep
	}
	rep := GCReport{DryRun: cfg.DryRun, Endpoint: cfg.Endpoint}
	if store == nil {
		return rep, fmt.Errorf("account gc store is nil")
	}
	trashed, err := store.Due(ctx, statusTrashed, cfg.Batch)
	if err != nil {
		return rep, err
	}
	for _, item := range trashed {
		if err := ctx.Err(); err != nil {
			return rep, err
		}
		if item.TrashedAt <= 0 || now.Unix() < item.TrashedAt+int64(cfg.Keep/time.Second) {
			continue
		}
		if cfg.DryRun {
			rep.WouldDelete++
			sample(&rep, "would_delete "+item.Class+"/"+item.Path)
			continue
		}
		if mover == nil {
			rep.Errors++
			continue
		}
		if err := mover.DeleteTrash(ctx, item.Class, item.Path); err != nil {
			rep.Errors++
			continue
		}
		if err := store.Mark(ctx, item.ID, statusDeleted, item.TrashedAt); err != nil {
			rep.Errors++
			continue
		}
		rep.Deleted++
	}
	pending, err := store.Due(ctx, statusPending, cfg.Batch)
	if err != nil {
		return rep, err
	}
	for _, item := range pending {
		if err := ctx.Err(); err != nil {
			return rep, err
		}
		exempt, err := store.Exempt(ctx, item)
		if err != nil {
			rep.Errors++
			continue
		}
		if exempt || exemptKind(item) {
			if !cfg.DryRun {
				if err := store.Mark(ctx, item.ID, statusSkipped, 0); err != nil {
					rep.Errors++
					continue
				}
			}
			rep.Exempt++
			sample(&rep, "exempt "+item.Class+"/"+item.Path)
			continue
		}
		refs, err := store.Refs(ctx, item)
		if err != nil || refs < 0 {
			rep.Errors++
			continue
		}
		if refs > 0 {
			rep.Referenced++
			continue
		}
		if cfg.DryRun {
			rep.WouldTrash++
			sample(&rep, "would_trash "+item.Class+"/"+item.Path)
			continue
		}
		if mover == nil {
			rep.Errors++
			continue
		}
		if err := mover.MoveToTrash(ctx, item.Class, item.Path); err != nil {
			rep.Errors++
			continue
		}
		if err := store.Mark(ctx, item.ID, statusTrashed, now.Unix()); err != nil {
			rep.Errors++
			continue
		}
		rep.Trashed++
	}
	return rep, nil
}

func exemptKind(item GCItem) bool {
	switch item.Class {
	case "stickers", "emoji", "wallpapers":
		return true
	}
	if item.Kind == string(retention.KindDocument) && item.Class == "documents" {
		return false
	}
	return false
}

func sample(rep *GCReport, line string) {
	if len(rep.Samples) < 20 {
		rep.Samples = append(rep.Samples, line)
	}
}

// DocumentExempt reports stickers, custom emoji, GIFs and wallpapers.
func DocumentExempt(mime, attributes string) bool {
	return retention.Exempt(retention.Media{
		Kind:       retention.KindDocument,
		MimeType:   mime,
		Attributes: attributes,
	})
}
