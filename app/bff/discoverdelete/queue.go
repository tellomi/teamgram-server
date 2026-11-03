package discoverdelete

import (
	"context"
	"sync"
	"time"

	"github.com/teamgram/teamgram-server/pkg/discoverauth"
	"github.com/zeromicro/go-zero/core/logx"
)

type notice struct {
	sub      []byte
	attempts int
	next     time.Time
	sweep    bool
}

type queue struct {
	mu    sync.Mutex
	items []notice
	cfg   settings
	do    func(sub []byte) Result
}

var (
	qOnce sync.Once
	qMu   sync.Mutex
	q     *queue
	// startBackground is false in tests that drain the queue themselves.
	startBackground = true
	loggedConfig    sync.Once
)

// Enqueue asks the engine to delete this user's content. It returns
// immediately. The flag, a bad configuration, or a full failure inside
// the queue does not affect account deletion.
func Enqueue(userID int64) {
	defer func() {
		if recover() != nil {
			logx.Error("discover delete: enqueue failed")
		}
	}()
	if userID <= 0 || !enabled() {
		return
	}
	cfg, err := load()
	if err != nil {
		loggedConfig.Do(func() {
			logx.Error("discover delete: configuration is missing or invalid")
		})
		return
	}
	sub := discoverauth.Pseudonym(cfg.pseudonym, userID)
	qMu.Lock()
	if q == nil {
		q = &queue{cfg: cfg, do: func(sub []byte) Result {
			ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
			defer cancel()
			return Call(ctx, nil, cfg, sub)
		}}
	}
	qq := q
	qMu.Unlock()
	qq.add(notice{sub: append([]byte(nil), sub...), next: time.Now()})
	if startBackground {
		qOnce.Do(func() {
			go qq.loop()
		})
	}
}

func (qq *queue) add(n notice) {
	qq.mu.Lock()
	qq.items = append(qq.items, n)
	qq.mu.Unlock()
}

func (qq *queue) loop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		qq.drain(time.Now())
	}
}

func (qq *queue) drain(now time.Time) {
	qq.mu.Lock()
	due := make([]notice, 0, len(qq.items))
	rest := qq.items[:0]
	for _, item := range qq.items {
		if !item.next.After(now) {
			due = append(due, item)
			continue
		}
		rest = append(rest, item)
	}
	qq.items = append([]notice(nil), rest...)
	qq.mu.Unlock()
	var again []notice
	for _, item := range due {
		result := qq.do(item.sub)
		item.attempts++
		next, keep := qq.decide(&item, result, now)
		if keep {
			item.next = next
			again = append(again, item)
		}
	}
	if len(again) > 0 {
		qq.mu.Lock()
		qq.items = append(qq.items, again...)
		qq.mu.Unlock()
	}
}

// decide reports when to try again. keep false drops the notice.
// A successful first delete is queued once more after secondDeleteAfter,
// with sweep set so it is not scheduled a third time.
func (qq *queue) decide(item *notice, result Result, now time.Time) (time.Time, bool) {
	kind := "delete"
	if item.sweep {
		kind = "sweep"
	}
	switch result.Outcome {
	case OutcomeDeleted:
		logx.Infof("discover delete: %s deleted status %d attempts %d", kind, result.Status, item.attempts)
		if !item.sweep && secondDeleteAfter > 0 {
			item.sweep = true
			item.attempts = 0
			return now.Add(secondDeleteAfter), true
		}
		return time.Time{}, false
	case OutcomeAuth, OutcomePermanent:
		logx.Errorf("discover delete: %s stopped status %d", kind, result.Status)
		return time.Time{}, false
	default:
		if item.attempts >= maxAttempts {
			logx.Errorf("discover delete: %s gave up status %d attempts %d", kind, result.Status, item.attempts)
			return time.Time{}, false
		}
		return now.Add(backoff(item.attempts)), true
	}
}

func backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := time.Second << (attempt - 1)
	if d > 15*time.Minute || d <= 0 {
		return 15 * time.Minute
	}
	return d
}
