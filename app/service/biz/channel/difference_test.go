package channel

import (
	"context"
	"fmt"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestClampChannelDifferenceLimit(t *testing.T) {
	if got := clampChannelDifferenceLimit(0, false); got != 100 {
		t.Fatalf("user default %d", got)
	}
	if got := clampChannelDifferenceLimit(1, false); got != 1 {
		t.Fatalf("user 1 %d", got)
	}
	if got := clampChannelDifferenceLimit(100, false); got != 100 {
		t.Fatalf("user 100 %d", got)
	}
	if got := clampChannelDifferenceLimit(101, false); got != 100 {
		t.Fatalf("user cap %d", got)
	}
	if got := clampChannelDifferenceLimit(100000, false); got != 100 {
		t.Fatalf("user hard cap %d", got)
	}
	if got := clampChannelDifferenceLimit(0, true); got != 100 {
		t.Fatalf("bot default %d", got)
	}
	if got := clampChannelDifferenceLimit(250, true); got != 250 {
		t.Fatalf("bot 250 %d", got)
	}
	if got := clampChannelDifferenceLimit(100000, true); got != 100000 {
		t.Fatalf("bot max %d", got)
	}
	if got := clampChannelDifferenceLimit(100001, true); got != 100000 {
		t.Fatalf("bot cap %d", got)
	}
}

func TestChannelDifferenceEmptyWhenCaughtUp(t *testing.T) {
	svc, ctx := newTest(t)
	in, id := newDiffGroup(t, svc, ctx)
	before := ChannelDifferenceTooLongTotal()
	pts := mustChannel(t, svc, ctx, id).Pts
	diff, err := svc.Difference(ctx, 1, in, pts, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	if diff.GetPredicateName() != mtproto.Predicate_updates_channelDifferenceEmpty || !diff.Final || diff.Pts != pts {
		t.Fatalf("caught up = %s final=%v pts=%d", diff.GetPredicateName(), diff.Final, diff.Pts)
	}
	if len(diff.NewMessages) != 0 || len(diff.OtherUpdates) != 0 {
		t.Fatalf("empty difference carried messages=%d updates=%d", len(diff.NewMessages), len(diff.OtherUpdates))
	}
	ahead, err := svc.Difference(ctx, 1, in, pts+10, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	if ahead.GetPredicateName() != mtproto.Predicate_updates_channelDifferenceEmpty || ahead.Pts != pts {
		t.Fatalf("client ahead = %s pts=%d", ahead.GetPredicateName(), ahead.Pts)
	}
	if mustChannel(t, svc, ctx, id).Pts != pts {
		t.Fatal("server pts changed")
	}
	if ChannelDifferenceTooLongTotal() != before {
		t.Fatal("caught up counted as tooLong")
	}
}

func TestChannelDifferenceFifty(t *testing.T) {
	svc, ctx := newTest(t)
	in, id := newDiffGroup(t, svc, ctx)
	const extra = 50
	sendN(t, svc, ctx, 1, id, extra)
	ch := mustChannel(t, svc, ctx, id)
	before := ChannelDifferenceTooLongTotal()
	diff, err := svc.Difference(ctx, 1, in, 0, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	if diff.GetPredicateName() != mtproto.Predicate_updates_channelDifference || !diff.Final || diff.Pts != ch.Pts {
		t.Fatalf("difference = %s final=%v pts=%d want pts %d", diff.GetPredicateName(), diff.Final, diff.Pts, ch.Pts)
	}
	if len(diff.NewMessages) != extra+1 {
		t.Fatalf("new messages %d", len(diff.NewMessages))
	}
	if ChannelDifferenceTooLongTotal() != before {
		t.Fatal("50 updates returned tooLong")
	}
}

func TestChannelDifferenceSixHundredPages(t *testing.T) {
	svc, ctx := newTest(t)
	in, id := newDiffGroup(t, svc, ctx)
	const extra = 600
	sendN(t, svc, ctx, 1, id, extra)
	ch := mustChannel(t, svc, ctx, id)
	want := liveIDs(t, svc, ctx, id)
	if len(want) != extra+1 {
		t.Fatalf("stored messages %d", len(want))
	}
	before := ChannelDifferenceTooLongTotal()
	got, end, pages := drainChannelDifference(t, svc, ctx, 1, in, 0, 100, false)
	if pages < 7 {
		t.Fatalf("pages %d, want several pages for %d events", pages, len(want))
	}
	if end != ch.Pts {
		t.Fatalf("end pts %d channel %d", end, ch.Pts)
	}
	if len(got) != len(want) {
		t.Fatalf("delivered %d stored %d", len(got), len(want))
	}
	seen := map[int32]struct{}{}
	for _, id := range got {
		if _, ok := seen[id]; ok {
			t.Fatalf("duplicate %d", id)
		}
		seen[id] = struct{}{}
		if _, ok := want[id]; !ok {
			t.Fatalf("unexpected message %d", id)
		}
	}
	for id := range want {
		if _, ok := seen[id]; !ok {
			t.Fatalf("missing message %d", id)
		}
	}
	empty, err := svc.Difference(ctx, 1, in, end, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	if empty.GetPredicateName() != mtproto.Predicate_updates_channelDifferenceEmpty || !empty.Final || empty.Pts != end {
		t.Fatalf("after drain = %s final=%v pts=%d", empty.GetPredicateName(), empty.Final, empty.Pts)
	}
	if ChannelDifferenceTooLongTotal() != before {
		t.Fatal("paged backlog counted as tooLong")
	}
}

func TestChannelDifferencePtsGap(t *testing.T) {
	svc, ctx := newTest(t)
	in, id := newDiffGroup(t, svc, ctx)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	sendN(t, svc, ctx, 1, id, 20)
	ch := mustChannel(t, svc, ctx, id)
	if err := svc.ReadHistory(ctx, 2, in, ch.TopMessage); err != nil {
		t.Fatal(err)
	}
	st, err := svc.store.GetRead(ctx, id, 2)
	if err != nil {
		t.Fatal(err)
	}
	if st.ReadInboxMaxID != ch.TopMessage {
		t.Fatalf("read inbox %d top %d", st.ReadInboxMaxID, ch.TopMessage)
	}

	// Oldest rows are gone. The client cursor cannot be continued.
	dropChannelPts(t, svc, id, func(e *PtsEvent) bool { return e.Pts <= 5 })
	before := ChannelDifferenceTooLongTotal()
	diff, err := svc.Difference(ctx, 2, in, 0, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	if diff.GetPredicateName() != mtproto.Predicate_updates_channelDifferenceTooLong || !diff.Final {
		t.Fatalf("gap = %s final=%v", diff.GetPredicateName(), diff.Final)
	}
	dlg := diff.GetDialog()
	if dlg == nil || dlg.GetPts().GetValue() != ch.Pts || dlg.GetTopMessage() != ch.TopMessage {
		t.Fatalf("dialog %#v", dlg)
	}
	if dlg.GetReadInboxMaxId() != st.ReadInboxMaxID || dlg.GetPeer().GetChannelId() != id {
		t.Fatalf("dialog read_inbox_max_id=%d peer=%d", dlg.GetReadInboxMaxId(), dlg.GetPeer().GetChannelId())
	}
	if dlg.GetNotifySettings() == nil {
		t.Fatal("dialog notify_settings missing")
	}
	live := len(liveIDs(t, svc, ctx, id))
	if live <= 10 {
		t.Fatalf("fixture has %d messages", live)
	}
	if len(diff.Messages) != live {
		t.Fatalf("tooLong messages %d, want %d (the request limit, not 10)", len(diff.Messages), live)
	}
	if ChannelDifferenceTooLongTotal() != before+1 {
		t.Fatalf("tooLong count %d", ChannelDifferenceTooLongTotal()-before)
	}

	// A smaller limit trims the snapshot to that many newest messages.
	small, err := svc.Difference(ctx, 2, in, 0, 4, false)
	if err != nil {
		t.Fatal(err)
	}
	if small.GetPredicateName() != mtproto.Predicate_updates_channelDifferenceTooLong || len(small.GetMessages()) != 4 {
		t.Fatalf("limit 4 tooLong messages %d predicate %s", len(small.GetMessages()), small.GetPredicateName())
	}
	prev := int32(1 << 30)
	for _, m := range small.GetMessages() {
		if m.GetId() >= prev {
			t.Fatalf("newest-first broken at %d after %d", m.GetId(), prev)
		}
		prev = m.GetId()
	}
}

func TestChannelDifferenceEmptyLogIsTooLong(t *testing.T) {
	svc, ctx := newTest(t)
	in, id := newDiffGroup(t, svc, ctx)
	sendN(t, svc, ctx, 1, id, 12)
	ch := mustChannel(t, svc, ctx, id)
	dropChannelPts(t, svc, id, func(*PtsEvent) bool { return true })
	before := ChannelDifferenceTooLongTotal()
	diff, err := svc.Difference(ctx, 1, in, 0, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	if diff.GetPredicateName() != mtproto.Predicate_updates_channelDifferenceTooLong || !diff.Final {
		t.Fatalf("cleared log = %s final=%v", diff.GetPredicateName(), diff.Final)
	}
	if diff.GetDialog().GetPts().GetValue() != ch.Pts {
		t.Fatalf("dialog pts %d channel %d", diff.GetDialog().GetPts().GetValue(), ch.Pts)
	}
	live := len(liveIDs(t, svc, ctx, id))
	if live <= 10 || len(diff.GetMessages()) != live {
		t.Fatalf("messages %d live %d", len(diff.GetMessages()), live)
	}
	if ChannelDifferenceTooLongTotal() != before+1 {
		t.Fatal("cleared log did not count")
	}
}

func TestChannelDifferenceMiddleHoleThenTooLong(t *testing.T) {
	svc, ctx := newTest(t)
	in, id := newDiffGroup(t, svc, ctx)
	sendN(t, svc, ctx, 1, id, 40)
	ch := mustChannel(t, svc, ctx, id)
	dropChannelPts(t, svc, id, func(e *PtsEvent) bool { return e.Pts > 15 && e.Pts <= 25 })
	before := ChannelDifferenceTooLongTotal()
	first, err := svc.Difference(ctx, 1, in, 0, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	if first.GetPredicateName() != mtproto.Predicate_updates_channelDifference || first.Final || first.Pts != 15 {
		t.Fatalf("prefix = %s final=%v pts=%d", first.GetPredicateName(), first.Final, first.Pts)
	}
	if ChannelDifferenceTooLongTotal() != before {
		t.Fatal("continuous prefix counted as tooLong")
	}
	second, err := svc.Difference(ctx, 1, in, first.Pts, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	if second.GetPredicateName() != mtproto.Predicate_updates_channelDifferenceTooLong || !second.Final {
		t.Fatalf("hole = %s final=%v", second.GetPredicateName(), second.Final)
	}
	if second.GetDialog().GetPts().GetValue() != ch.Pts {
		t.Fatalf("reset pts %d want %d", second.GetDialog().GetPts().GetValue(), ch.Pts)
	}
	if ChannelDifferenceTooLongTotal() != before+1 {
		t.Fatal("hole did not count")
	}
}

func TestChannelDifferenceDeletePtsCountIsContinuous(t *testing.T) {
	svc, ctx := newTest(t)
	in, id := newDiffGroup(t, svc, ctx)
	sendN(t, svc, ctx, 1, id, 6)
	all, err := svc.store.ListMessages(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int32
	for _, m := range all {
		if m != nil && !m.Deleted && m.ID > 1 {
			ids = append(ids, m.ID)
		}
	}
	if len(ids) < 3 {
		t.Fatal("not enough messages")
	}
	ids = ids[:3]
	if _, _, err := svc.Delete(ctx, 1, id, 0, ids); err != nil {
		t.Fatal(err)
	}
	before := ChannelDifferenceTooLongTotal()
	diff, err := svc.Difference(ctx, 1, in, 0, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	ch := mustChannel(t, svc, ctx, id)
	if diff.GetPredicateName() != mtproto.Predicate_updates_channelDifference || !diff.Final || diff.Pts != ch.Pts {
		t.Fatalf("delete difference = %s final=%v pts=%d want %d", diff.GetPredicateName(), diff.Final, diff.Pts, ch.Pts)
	}
	var saw bool
	for _, u := range diff.OtherUpdates {
		if u.GetPredicateName() == mtproto.Predicate_updateDeleteChannelMessages && u.GetPtsCount() == int32(len(ids)) {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("missing pts_count=%d delete update", len(ids))
	}
	if ChannelDifferenceTooLongTotal() != before {
		t.Fatal("pts_count treated as a hole")
	}
}

func TestChannelDifferenceLimitBoundaries(t *testing.T) {
	svc, ctx := newTest(t)
	in, id := newDiffGroup(t, svc, ctx)
	sendN(t, svc, ctx, 1, id, 130)
	ch := mustChannel(t, svc, ctx, id)
	events := ch.Pts // create + 130 sends, one pts each
	if events != 131 {
		t.Fatalf("pts %d", events)
	}

	zero, err := svc.Difference(ctx, 1, in, 0, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if zero.GetPredicateName() != mtproto.Predicate_updates_channelDifference || zero.Final || len(zero.NewMessages) != 100 || zero.Pts != 100 {
		t.Fatalf("limit 0 = %s final=%v n=%d pts=%d", zero.GetPredicateName(), zero.Final, len(zero.NewMessages), zero.Pts)
	}

	one, err := svc.Difference(ctx, 1, in, 0, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if one.Final || len(one.NewMessages) != 1 || one.Pts != 1 {
		t.Fatalf("limit 1 final=%v n=%d pts=%d", one.Final, len(one.NewMessages), one.Pts)
	}

	over, err := svc.Difference(ctx, 1, in, 0, 250, false)
	if err != nil {
		t.Fatal(err)
	}
	if over.Final || len(over.NewMessages) != 100 {
		t.Fatalf("user limit 250 final=%v n=%d", over.Final, len(over.NewMessages))
	}

	bot, err := svc.Difference(ctx, 1, in, 0, 250, true)
	if err != nil {
		t.Fatal(err)
	}
	if !bot.Final || bot.Pts != ch.Pts || len(bot.NewMessages) != int(events) {
		t.Fatalf("bot limit 250 final=%v n=%d pts=%d", bot.Final, len(bot.NewMessages), bot.Pts)
	}

	// Exactly one page is final. One past the page is not.
	exact, id2 := newDiffGroup(t, svc, ctx)
	sendN(t, svc, ctx, 1, id2, 99) // 100 events with the create message
	page, err := svc.Difference(ctx, 1, exact, 0, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	if !page.Final || len(page.NewMessages) != 100 || page.Pts != 100 {
		t.Fatalf("exact page final=%v n=%d pts=%d", page.Final, len(page.NewMessages), page.Pts)
	}
	more, id3 := newDiffGroup(t, svc, ctx)
	sendN(t, svc, ctx, 1, id3, 100) // 101 events
	first, err := svc.Difference(ctx, 1, more, 0, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	if first.Final || len(first.NewMessages) != 100 || first.Pts != 100 {
		t.Fatalf("101 first final=%v n=%d pts=%d", first.Final, len(first.NewMessages), first.Pts)
	}
	rest, err := svc.Difference(ctx, 1, more, first.Pts, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	if !rest.Final || len(rest.NewMessages) != 1 || rest.Pts != 101 {
		t.Fatalf("101 rest final=%v n=%d pts=%d", rest.Final, len(rest.NewMessages), rest.Pts)
	}
}

func newDiffGroup(t *testing.T, svc *Service, ctx context.Context) (*mtproto.InputChannel, int64) {
	t.Helper()
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Diff", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	return input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue()), ch.Id
}

func sendN(t *testing.T, svc *Service, ctx context.Context, sender, channelID int64, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		_, err := svc.Send(ctx, sender, channelID, 0, []Outgoing{{
			RandomID: int64(i+1) + channelID,
			Message:  &mtproto.Message{Message: fmt.Sprintf("m-%d", i), Date: int32(1_700_000_000 + i)},
		}})
		if err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
}

func liveIDs(t *testing.T, svc *Service, ctx context.Context, id int64) map[int32]struct{} {
	t.Helper()
	all, err := svc.store.ListMessages(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	out := map[int32]struct{}{}
	for _, m := range all {
		if m != nil && !m.Deleted {
			out[m.ID] = struct{}{}
		}
	}
	return out
}

func dropChannelPts(t *testing.T, svc *Service, channelID int64, drop func(*PtsEvent) bool) {
	t.Helper()
	mem, ok := svc.store.(*memoryStore)
	if !ok {
		t.Fatalf("store %T", svc.store)
	}
	mem.mu.Lock()
	defer mem.mu.Unlock()
	src := mem.events[channelID]
	kept := make([]*PtsEvent, 0, len(src))
	for _, e := range src {
		if e != nil && drop(e) {
			continue
		}
		kept = append(kept, e)
	}
	mem.events[channelID] = kept
}

func drainChannelDifference(t *testing.T, svc *Service, ctx context.Context, user int64, in *mtproto.InputChannel, from, limit int32, bot bool) (ids []int32, end int32, pages int) {
	t.Helper()
	pts := from
	seen := map[int32]struct{}{}
	for {
		pages++
		if pages > 20 {
			t.Fatal("difference did not finish")
		}
		diff, err := svc.Difference(ctx, user, in, pts, limit, bot)
		if err != nil {
			t.Fatal(err)
		}
		switch diff.GetPredicateName() {
		case mtproto.Predicate_updates_channelDifferenceEmpty:
			if !diff.Final || diff.Pts < pts {
				t.Fatalf("empty final=%v pts=%d from %d", diff.Final, diff.Pts, pts)
			}
			return ids, diff.Pts, pages
		case mtproto.Predicate_updates_channelDifferenceTooLong:
			t.Fatalf("tooLong from pts %d on page %d", pts, pages)
		case mtproto.Predicate_updates_channelDifference:
		default:
			t.Fatalf("predicate %s", diff.GetPredicateName())
		}
		cursor := pts
		for _, u := range diff.GetOtherUpdates() {
			count := u.GetPtsCount()
			if count <= 0 {
				continue
			}
			if u.GetPts_INT32()-count > cursor {
				t.Fatalf("hole cursor=%d pts=%d count=%d", cursor, u.GetPts_INT32(), count)
			}
			if u.GetPts_INT32() <= cursor {
				t.Fatalf("pts did not move cursor=%d update=%d", cursor, u.GetPts_INT32())
			}
			cursor = u.GetPts_INT32()
		}
		if cursor != diff.Pts {
			t.Fatalf("page pts %d last update %d", diff.Pts, cursor)
		}
		if diff.Pts <= pts {
			t.Fatalf("response pts %d from %d", diff.Pts, pts)
		}
		for _, m := range diff.GetNewMessages() {
			id := m.GetId()
			if id == 0 {
				continue
			}
			if _, ok := seen[id]; ok {
				t.Fatalf("duplicate message %d", id)
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
		pts = diff.Pts
		if diff.Final {
			return ids, pts, pages
		}
	}
}
