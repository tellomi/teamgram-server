package report

import (
	"context"
	"strings"
	"sync"
	"time"
)

// AccountHook receives a stored user report after the receipt is sent.
// Discover posts are not passed here. The hook runs in its own goroutine.
type AccountHook func(ctx context.Context, row *Row, ev Evidence)

var (
	hookMu      sync.Mutex
	accountHook AccountHook
)

// SetAccountHook installs the enforcement review. Nil clears it. Tests restore it.
func SetAccountHook(h AccountHook) {
	hookMu.Lock()
	accountHook = h
	hookMu.Unlock()
}

func currentAccountHook() AccountHook {
	hookMu.Lock()
	defer hookMu.Unlock()
	return accountHook
}

func dispatchAccount(row *Row, ev Evidence) {
	h := currentAccountHook()
	if h == nil || row == nil || row.SubjectUserID == 0 || strings.HasPrefix(row.Source, "discover.") {
		return
	}
	cp := copyRow(row)
	ev2 := ev
	ev2.Items = append([]Content(nil), ev.Items...)
	if ev.Post != nil {
		post := *ev.Post
		ev2.Post = &post
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		h(ctx, cp, ev2)
	}()
}
