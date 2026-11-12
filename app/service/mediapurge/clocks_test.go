package mediapurge

import (
	"testing"
	"time"

	"github.com/teamgram/teamgram-server/app/service/media/retention"
)

// Orphan GC, media age, and premium grace are three clocks. A file inside
// the age window is young even if nobody references it. An old file a
// subscriber still holds is not expired. The purge queue waits its own
// grace and does not read the age window.
func TestPurgeGraceIsSeparateFromMediaAge(t *testing.T) {
	if Grace() != 24*time.Hour {
		t.Fatalf("orphan grace %s", Grace())
	}
	cfg := retention.Defaults()
	if cfg.Retention() != 90*24*time.Hour || cfg.Grace() != 30*24*time.Hour {
		t.Fatalf("age policy %+v", cfg)
	}
	now := time.Unix(1_700_000_000, 0)
	young := retention.Media{Kind: retention.KindDocument, ID: 1, UploadedAt: now.Add(-48 * time.Hour).Unix()}
	if got := retention.Decide(young, nil, now, cfg); got != retention.ReasonYoung {
		t.Fatalf("two-day file %s", got)
	}
	old := young
	old.UploadedAt = now.Add(-91 * 24 * time.Hour).Unix()
	if got := retention.Decide(old, nil, now, cfg); got != retention.ReasonUnreferenced {
		t.Fatalf("old unreferenced %s", got)
	}
	held := []retention.Subscription{{UserID: 7, Premium: true}}
	if got := retention.Decide(old, held, now, cfg); got != retention.ReasonSubscriber {
		t.Fatalf("subscriber %s", got)
	}
	enqueued := now
	if !enqueued.Add(Grace()).After(now.Add(23*time.Hour + 59*time.Minute)) {
		t.Fatal("purge became due inside the grace hour")
	}
	if enqueued.Add(Grace()).After(now.Add(24 * time.Hour)) {
		t.Fatal("purge still waiting at the grace boundary")
	}
	if objs := retention.Objects(old); len(objs) != 1 || objs[0][1] != "1.dat" {
		t.Fatalf("age cleanup object set %v", objs)
	}
}
