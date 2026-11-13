package core

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
)

type scriptedInbox struct {
	mu       sync.Mutex
	current  int
	max      int
	seen     int
	failUser int64
	delay    time.Duration
}

func (s *scriptedInbox) InboxSendUserMessageToInboxV2(_ context.Context, in *inbox.TLInboxSendUserMessageToInboxV2) (*mtproto.Void, error) {
	s.mu.Lock()
	s.current++
	s.seen++
	if s.current > s.max {
		s.max = s.current
	}
	s.mu.Unlock()
	time.Sleep(s.delay)
	s.mu.Lock()
	s.current--
	user := in.GetUserId()
	s.mu.Unlock()
	if user == s.failUser {
		return nil, errors.New("publish failed")
	}
	return mtproto.EmptyVoid, nil
}

func TestInboxFanoutOverlapsMembers(t *testing.T) {
	cli := &scriptedInbox{delay: 30 * time.Millisecond}
	reqs := make([]*inbox.TLInboxSendUserMessageToInboxV2, 4)
	for i := range reqs {
		reqs[i] = &inbox.TLInboxSendUserMessageToInboxV2{UserId: int64(i + 1)}
	}
	start := time.Now()
	if err := publishInboxFanout(context.Background(), cli, reqs); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 90*time.Millisecond {
		t.Fatalf("fanout took %s, want the publishes to overlap", time.Since(start))
	}
	if cli.max < 2 || cli.seen != 4 {
		t.Fatalf("max in flight %d seen %d", cli.max, cli.seen)
	}
}

func TestInboxFanoutReturnsFirstError(t *testing.T) {
	cli := &scriptedInbox{failUser: 2}
	reqs := []*inbox.TLInboxSendUserMessageToInboxV2{
		{UserId: 1},
		{UserId: 2},
	}
	if err := publishInboxFanout(context.Background(), cli, reqs); err == nil {
		t.Fatal("expected publish error")
	}
	if cli.seen != 2 {
		t.Fatalf("seen %d", cli.seen)
	}
}

func TestInboxFanoutNilClient(t *testing.T) {
	err := publishInboxFanout(context.Background(), nil, []*inbox.TLInboxSendUserMessageToInboxV2{{UserId: 1}})
	if err == nil {
		t.Fatal("nil client")
	}
}
