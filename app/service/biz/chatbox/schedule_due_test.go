package chatbox

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestScheduleWhenOnlineAndRepeat(t *testing.T) {
	b := newBook(nil)
	b.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	ctx := context.Background()
	user := mtproto.MakePeerUtil(mtproto.PEER_USER, 9)
	chat := mtproto.MakePeerUtil(mtproto.PEER_CHAT, 4)
	if _, err := b.Schedule(ctx, 3, chat, ScheduleWhenOnline, []*mtproto.Message{{Message: "no"}}); err != mtproto.ErrScheduleStatusPrivate {
		t.Fatalf("group when-online: %v", err)
	}
	if _, err := b.Schedule(ctx, 3, user, ScheduleWhenOnline, []*mtproto.Message{{Message: "hi"}}); err != nil {
		t.Fatal(err)
	}
	page, err := b.ScheduledHistory(ctx, 3, user, 0)
	if err != nil || len(page.Messages) != 1 || page.Messages[0].GetDate() != ScheduleWhenOnline {
		t.Fatalf("online history %+v %v", page, err)
	}
	if page.Messages[0].GetId() < SchedIDBase {
		t.Fatalf("scheduled id %d shares the live box", page.Messages[0].GetId())
	}
	far := int32(b.now().Add(400 * 24 * time.Hour).Unix())
	if _, err := b.Schedule(ctx, 3, user, far, []*mtproto.Message{{Message: "late"}}); err != mtproto.ErrScheduleDateTooLate {
		t.Fatalf("too late: %v", err)
	}
	when := int32(b.now().Add(time.Hour).Unix())
	bad := &mtproto.Message{Message: "x", ScheduleRepeatPeriod: wrapperspb.Int32(10)}
	if _, err := b.Schedule(ctx, 3, user, when, []*mtproto.Message{bad}); err != mtproto.ErrScheduleDateInvalid {
		t.Fatalf("bad repeat: %v", err)
	}
	if _, err := b.Schedule(ctx, 3, user, ScheduleWhenOnline, []*mtproto.Message{{
		Message: "x", ScheduleRepeatPeriod: wrapperspb.Int32(86400),
	}}); err != mtproto.ErrScheduleDateInvalid {
		t.Fatalf("repeat while online: %v", err)
	}
	ups, err := b.Schedule(ctx, 3, user, when, []*mtproto.Message{{
		Message: "daily", ScheduleRepeatPeriod: wrapperspb.Int32(86400),
	}})
	if err != nil {
		t.Fatal(err)
	}
	got := ups.Updates[0].GetMessage_MESSAGE()
	if got.GetScheduleRepeatPeriod().GetValue() != 86400 {
		t.Fatalf("repeat dropped: %+v", got.GetScheduleRepeatPeriod())
	}
}

func TestEditScheduledReschedule(t *testing.T) {
	b := newBook(nil)
	now := int32(1_700_000_000)
	b.now = func() time.Time { return time.Unix(int64(now), 0) }
	ctx := context.Background()
	peer := mtproto.MakePeerUtil(mtproto.PEER_USER, 9)
	when := now + 3600
	ups, err := b.Schedule(ctx, 3, peer, when, []*mtproto.Message{{Message: "later"}})
	if err != nil {
		t.Fatal(err)
	}
	id := ups.Updates[0].GetMessage_MESSAGE().GetId()
	text := "edited"
	next := when + 7200
	edited, err := b.EditScheduled(ctx, 3, peer, id, SchedEdit{Text: &text, When: &next, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	msg := edited.Updates[0].GetMessage_MESSAGE()
	if msg.GetMessage() != "edited" || msg.GetDate() != next || msg.GetId() != id {
		t.Fatalf("edit %+v", msg)
	}
	if edited.Updates[0].GetPredicateName() != mtproto.Predicate_updateNewScheduledMessage {
		t.Fatalf("update %s", edited.Updates[0].GetPredicateName())
	}
	has, err := b.HasScheduled(ctx, 3, peer, id)
	if err != nil || !has {
		t.Fatalf("missing after edit %v", err)
	}
	if !ShouldEditScheduled(true, false, id, true, 9, 3) {
		t.Fatal("foreign live id should edit the scheduled copy")
	}
	if ShouldEditScheduled(true, false, 12, true, 3, 3) {
		t.Fatal("own live message should stay a live edit")
	}
	if !ShouldEditScheduled(true, true, 12, true, 3, 3) {
		t.Fatal("explicit schedule_date should edit the scheduled copy")
	}
}

type memDocs struct {
	mu sync.Mutex
	m  map[string][]byte
}

func newMemDocs() *memDocs { return &memDocs{m: map[string][]byte{}} }

func (m *memDocs) Load(_ context.Context, key string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	raw, ok := m.m[key]
	return append([]byte(nil), raw...), ok, nil
}

func (m *memDocs) Save(_ context.Context, key string, raw []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[key] = append([]byte(nil), raw...)
	return nil
}

func (m *memDocs) ListPrefix(_ context.Context, prefix string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var keys []string
	for key := range m.m {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

type recordSink struct {
	sent []dueSent
	push []duePush
	fail bool
	err  error
}

type dueSent struct {
	owner int64
	peer  int64
	text  string
	flag  bool
}

type duePush struct {
	user int64
	ups  *mtproto.Updates
}

func (s *recordSink) Send(_ context.Context, owner int64, peer *mtproto.PeerUtil, msg *mtproto.Message, _ int64) (int32, error) {
	if s.fail {
		if s.err != nil {
			return 0, s.err
		}
		return 0, mtproto.ErrInternalServerError
	}
	s.sent = append(s.sent, dueSent{owner: owner, peer: peer.PeerId, text: msg.GetMessage(), flag: msg.GetFromScheduled()})
	return 50 + int32(len(s.sent)), nil
}

func (s *recordSink) Push(_ context.Context, userID int64, ups *mtproto.Updates) error {
	s.push = append(s.push, duePush{user: userID, ups: ups})
	return nil
}

func TestDispatchDueSurvivesRestartAndRepeats(t *testing.T) {
	db := newMemDocs()
	now := int32(1_700_000_000)
	b := newBook(db)
	b.now = func() time.Time { return time.Unix(int64(now), 0) }
	ctx := context.Background()
	peer := mtproto.MakePeerUtil(mtproto.PEER_USER, 9)
	when := now + 30
	if _, err := b.Schedule(ctx, 3, peer, when, []*mtproto.Message{{
		Message: "ping", ScheduleRepeatPeriod: wrapperspb.Int32(86400),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Schedule(ctx, 3, peer, ScheduleWhenOnline, []*mtproto.Message{{Message: "online"}}); err != nil {
		t.Fatal(err)
	}

	// A new process sees the same rows.
	b2 := newBook(db)
	b2.now = func() time.Time { return time.Unix(int64(now+60), 0) }
	sink := &recordSink{}
	n, err := DispatchDue(ctx, b2, now+60, nil, sink)
	if err != nil || n != 1 || len(sink.sent) != 1 {
		t.Fatalf("dispatch %d %+v %v", n, sink.sent, err)
	}
	if !sink.sent[0].flag || sink.sent[0].text != "ping" || sink.sent[0].owner != 3 {
		t.Fatalf("sent %+v", sink.sent[0])
	}
	if len(sink.push) != 1 || sink.push[0].user != 3 {
		t.Fatalf("pushes %+v, want only the author", sink.push)
	}
	var sawDelete, sawNext bool
	var nextDate int32
	for _, p := range sink.push {
		for _, u := range p.ups.Updates {
			switch u.GetPredicateName() {
			case mtproto.Predicate_updateDeleteScheduledMessages:
				if u.GetPeer_PEER().GetUserId() != 9 || len(u.GetMessages()) != 1 || len(u.GetSentMessages()) != 1 {
					t.Fatalf("delete %+v", u)
				}
				sawDelete = true
			case mtproto.Predicate_updateNewScheduledMessage:
				msg := u.GetMessage_MESSAGE()
				if msg.GetScheduleRepeatPeriod().GetValue() != 86400 {
					t.Fatalf("next %+v", msg)
				}
				nextDate = msg.GetDate()
				sawNext = true
			}
		}
	}
	if !sawDelete || !sawNext {
		t.Fatalf("delete %v next %v", sawDelete, sawNext)
	}
	// The next copy is send-time plus the period, not the original slot.
	if nextDate != now+60+86400 {
		t.Fatalf("next date %d", nextDate)
	}
	left, err := b2.ScheduledHistory(ctx, 3, peer, 0)
	if err != nil || len(left.Messages) != 2 {
		t.Fatalf("after dispatch %+v %v", left, err)
	}

	// The online message waits until the peer is online, including after restart.
	b3 := newBook(db)
	sink2 := &recordSink{}
	n, err = DispatchDue(ctx, b3, now+120, map[int64]bool{9: true}, sink2)
	if err != nil || n != 1 || sink2.sent[0].text != "online" {
		t.Fatalf("online dispatch %d %+v %v", n, sink2.sent, err)
	}
}

func TestDispatchDueRestoresOnSendFailure(t *testing.T) {
	b := newBook(nil)
	now := int32(1_700_000_000)
	b.now = func() time.Time { return time.Unix(int64(now), 0) }
	ctx := context.Background()
	peer := mtproto.MakePeerUtil(mtproto.PEER_USER, 9)
	if _, err := b.Schedule(ctx, 3, peer, now+10, []*mtproto.Message{{Message: "keep"}}); err != nil {
		t.Fatal(err)
	}
	b.now = func() time.Time { return time.Unix(int64(now+20), 0) }
	n, err := DispatchDue(ctx, b, now+20, nil, &recordSink{fail: true})
	if err != nil || n != 0 {
		t.Fatalf("fail dispatch %d %v", n, err)
	}
	page, err := b.ScheduledHistory(ctx, 3, peer, 0)
	if err != nil || len(page.Messages) != 1 || page.Messages[0].GetMessage() != "keep" {
		t.Fatalf("not restored %+v %v", page, err)
	}
}

func TestRepeatPeriodsAndShortSchedule(t *testing.T) {
	for _, period := range []int32{86400, 604800, 1209600, 2592000, 7862400, 15724800, 31536000} {
		if !ValidRepeatPeriod(period) {
			t.Fatalf("period %d", period)
		}
	}
	if ValidRepeatPeriod(60) || ValidRepeatPeriod(300) || ValidRepeatPeriod(10) {
		t.Fatal("test-dc or unknown period accepted")
	}
	if err := RepeatSingleOnly(2, 86400); err != mtproto.ErrScheduleDateInvalid {
		t.Fatalf("multi repeat: %v", err)
	}
	if err := RepeatSingleOnly(1, 86400); err != nil {
		t.Fatal(err)
	}
	now := int32(1_700_000_000)
	if !SendInsteadOfSchedule(now+9, now, false) {
		t.Fatal("9s ahead should send now")
	}
	if SendInsteadOfSchedule(now+10, now, false) || SendInsteadOfSchedule(now+9, now, true) || SendInsteadOfSchedule(ScheduleWhenOnline, now, false) || SendInsteadOfSchedule(now-1, now, false) {
		t.Fatal("short window misclassified")
	}
}

func TestPermanentScheduleFailureDropsAndTemporaryWaits(t *testing.T) {
	if !PermanentScheduleErr(mtproto.ErrUserIsBlocked) || !PermanentScheduleErr(mtproto.ErrChatWriteForbidden) {
		t.Fatal("permanent class")
	}
	if PermanentScheduleErr(mtproto.ErrInternalServerError) || PermanentScheduleErr(nil) {
		t.Fatal("temporary class")
	}
	if !PermanentScheduleErr(mtproto.ErrUserBlocked) {
		t.Fatal("USER_BLOCKED should stop the retries")
	}
	now := int32(1_700_000_000)
	ctx := context.Background()
	peer := mtproto.MakePeerUtil(mtproto.PEER_USER, 9)
	b := newBook(nil)
	b.now = func() time.Time { return time.Unix(int64(now), 0) }
	if _, err := b.Schedule(ctx, 3, peer, now+10, []*mtproto.Message{{Message: "nope"}}); err != nil {
		t.Fatal(err)
	}
	perm := &recordSink{fail: true, err: mtproto.ErrUserIsBlocked}
	if _, err := DispatchDue(ctx, b, now+20, nil, perm); err != nil {
		t.Fatal(err)
	}
	page, err := b.ScheduledHistory(ctx, 3, peer, 0)
	if err != nil || len(page.Messages) != 0 {
		t.Fatalf("permanent left %+v", page)
	}
	if len(perm.push) != 1 || perm.push[0].user != 3 {
		t.Fatalf("author notice %+v", perm.push)
	}

	b2 := newBook(nil)
	b2.now = func() time.Time { return time.Unix(int64(now), 0) }
	if _, err := b2.Schedule(ctx, 3, peer, now+10, []*mtproto.Message{{Message: "later"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := DispatchDue(ctx, b2, now+20, nil, &recordSink{fail: true}); err != nil {
		t.Fatal(err)
	}
	if items, err := b2.ClaimDue(ctx, now+21, nil, 10); err != nil || len(items) != 0 {
		t.Fatalf("retried too soon %+v %v", items, err)
	}
	items, err := b2.ClaimDue(ctx, now+20+scheduleBackoff(1), nil, 10)
	if err != nil || len(items) != 1 || items[0].Message.GetMessage() != "later" || items[0].Tries != 1 {
		t.Fatalf("backoff %+v %v", items, err)
	}
}

type countingDocs struct {
	*memDocs
	lists int
}

func (c *countingDocs) ListPrefix(ctx context.Context, prefix string) ([]string, error) {
	c.lists++
	return c.memDocs.ListPrefix(ctx, prefix)
}

func TestScheduledSendIDsDoNotUseTheCollidingFormula(t *testing.T) {
	// owner*131 + peer*17 + schedID collided for these two messages.
	old := func(owner, peer int64, id int32) int64 {
		return owner*131 + peer*17 + int64(id)
	}
	if old(7, 1001, 1000000001) != old(7, 1000, 1000000018) {
		t.Fatal("the old formula no longer collides on the reported pair")
	}
	b := newBook(nil)
	now := int32(1_700_000_000)
	b.now = func() time.Time { return time.Unix(int64(now), 0) }
	ctx := context.Background()
	peer := mtproto.MakePeerUtil(mtproto.PEER_USER, 9)
	if _, err := b.Schedule(ctx, 7, peer, now+10, []*mtproto.Message{{Message: "a"}, {Message: "b"}}); err != nil {
		t.Fatal(err)
	}
	items, err := b.ClaimDue(ctx, now+20, nil, 10)
	if err != nil || len(items) != 2 {
		t.Fatalf("claim %+v %v", items, err)
	}
	seen := map[int64]bool{}
	for _, item := range items {
		if item.Random <= 0 {
			t.Fatalf("random %d", item.Random)
		}
		if seen[item.Random] {
			t.Fatalf("collision %d", item.Random)
		}
		seen[item.Random] = true
		if item.Random == old(item.Owner, item.Peer.PeerId, item.SchedID) {
			t.Fatal("still derived from owner, peer, and id")
		}
	}
	for i := 0; i < 64; i++ {
		n := newScheduleRandom()
		if n <= 0 || seen[n] {
			t.Fatalf("generated %d", n)
		}
		seen[n] = true
	}
}

func TestDueIndexRebuildsOnce(t *testing.T) {
	db := &countingDocs{memDocs: newMemDocs()}
	b := newBook(db)
	ctx := context.Background()
	if _, err := b.ClaimDue(ctx, 10, nil, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := b.ClaimDue(ctx, 12, nil, 10); err != nil {
		t.Fatal(err)
	}
	if db.lists != 1 {
		t.Fatalf("prefix scans %d", db.lists)
	}
}

func TestClaimDueRebuildsIndex(t *testing.T) {
	db := newMemDocs()
	now := int32(1_700_000_000)
	b := newBook(db)
	b.now = func() time.Time { return time.Unix(int64(now), 0) }
	ctx := context.Background()
	peer := mtproto.MakePeerUtil(mtproto.PEER_USER, 9)
	if _, err := b.Schedule(ctx, 3, peer, now+5, []*mtproto.Message{{Message: "later"}}); err != nil {
		t.Fatal(err)
	}
	db.mu.Lock()
	delete(db.m, dueIndexKey())
	db.mu.Unlock()
	b2 := newBook(db)
	items, err := b2.ClaimDue(ctx, now+10, nil, 10)
	if err != nil || len(items) != 1 || items[0].Message.GetMessage() != "later" {
		t.Fatalf("rebuild %+v %v", items, err)
	}
}
