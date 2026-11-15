package profile

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

func nonContactRecipients() *mtproto.InputBusinessRecipients {
	return mtproto.MakeTLInputBusinessRecipients(&mtproto.InputBusinessRecipients{NonContacts: true}).To_InputBusinessRecipients()
}

func TestGreetingAndAwaySendOnce(t *testing.T) {
	b := New()
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	b.now = func() time.Time { return now }
	if err := b.SetGreeting(9, mtproto.MakeTLInputBusinessGreetingMessage(&mtproto.InputBusinessGreetingMessage{
		ShortcutId: 3, NoActivityDays: 0, Recipients: nonContactRecipients(),
	}).To_InputBusinessGreetingMessage()); err != nil {
		t.Fatal(err)
	}
	if err := b.SetAway(9, mtproto.MakeTLInputBusinessAwayMessage(&mtproto.InputBusinessAwayMessage{
		ShortcutId: 8,
		Schedule:   mtproto.MakeTLBusinessAwayMessageScheduleAlways(nil).To_BusinessAwayMessageSchedule(),
		Recipients: nonContactRecipients(),
	}).To_InputBusinessAwayMessage()); err != nil {
		t.Fatal(err)
	}
	facts := PeerFacts{Now: now, HistoryKnown: true}
	got := b.ConsumeAutoReply(9, 4, facts)
	if len(got) != 2 || got[0] != 3 || got[1] != 8 {
		t.Fatalf("first %v", got)
	}
	again := b.ConsumeAutoReply(9, 4, PeerFacts{
		Now: now.Add(time.Minute), HistoryKnown: true, PriorMessage: int32(now.Unix()),
	})
	if len(again) != 0 {
		t.Fatalf("second %v", again)
	}
	if contact := b.ConsumeAutoReply(9, 5, PeerFacts{Contact: true, Now: now, HistoryKnown: true}); len(contact) != 0 {
		t.Fatalf("contact %v", contact)
	}
}

func TestGreetingWaitsForInactivity(t *testing.T) {
	b := New()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := b.SetGreeting(1, mtproto.MakeTLInputBusinessGreetingMessage(&mtproto.InputBusinessGreetingMessage{
		ShortcutId: 2, NoActivityDays: 7, Recipients: nonContactRecipients(),
	}).To_InputBusinessGreetingMessage()); err != nil {
		t.Fatal(err)
	}
	if got := b.ConsumeAutoReply(1, 2, PeerFacts{Now: start, HistoryKnown: true}); len(got) != 1 || got[0] != 2 {
		t.Fatalf("new chat %v", got)
	}
	talked := int32(start.Unix())
	if got := b.ConsumeAutoReply(1, 2, PeerFacts{
		Now: start.Add(6 * 24 * time.Hour), HistoryKnown: true, PriorMessage: talked,
	}); len(got) != 0 {
		t.Fatalf("inside window %v", got)
	}
	if got := b.ConsumeAutoReply(1, 2, PeerFacts{
		Now: start.Add(14 * 24 * time.Hour), HistoryKnown: true, PriorMessage: int32(start.Add(6 * 24 * time.Hour).Unix()),
	}); len(got) != 1 {
		t.Fatalf("after idle %v", got)
	}
}

func TestGreetingReadsDialogHistory(t *testing.T) {
	b := New()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	if err := b.SetGreeting(1, mtproto.MakeTLInputBusinessGreetingMessage(&mtproto.InputBusinessGreetingMessage{
		ShortcutId: 2, NoActivityDays: 0, Recipients: nonContactRecipients(),
	}).To_InputBusinessGreetingMessage()); err != nil {
		t.Fatal(err)
	}
	recent := int32(now.Add(-time.Hour).Unix())
	if got := b.ConsumeAutoReply(1, 2, PeerFacts{Now: now, HistoryKnown: true, PriorMessage: recent}); len(got) != 0 {
		t.Fatalf("existing chat days=0 %v", got)
	}
	if got := b.ConsumeAutoReply(1, 3, PeerFacts{Now: now}); len(got) != 0 {
		t.Fatalf("unknown history %v", got)
	}
	if got := b.ConsumeAutoReply(1, 4, PeerFacts{Now: now, HistoryKnown: true}); len(got) != 1 || got[0] != 2 {
		t.Fatalf("new chat %v", got)
	}
	if err := b.SetGreeting(1, mtproto.MakeTLInputBusinessGreetingMessage(&mtproto.InputBusinessGreetingMessage{
		ShortcutId: 2, NoActivityDays: 7, Recipients: nonContactRecipients(),
	}).To_InputBusinessGreetingMessage()); err != nil {
		t.Fatal(err)
	}
	if got := b.ConsumeAutoReply(1, 5, PeerFacts{Now: now, HistoryKnown: true, PriorMessage: recent}); len(got) != 0 {
		t.Fatalf("talked an hour ago %v", got)
	}
	old := int32(now.Add(-8 * 24 * time.Hour).Unix())
	if got := b.ConsumeAutoReply(1, 6, PeerFacts{Now: now, HistoryKnown: true, PriorMessage: old}); len(got) != 1 {
		t.Fatalf("idle eight days %v", got)
	}
}

func TestAwayOutsideHoursAndOffline(t *testing.T) {
	b := New()
	// Monday 10:00 UTC is inside 09:00-18:00. 20:00 is outside.
	open := time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC)
	closed := time.Date(2026, 1, 5, 20, 0, 0, 0, time.UTC)
	hours := mtproto.MakeTLBusinessWorkHours(&mtproto.BusinessWorkHours{
		TimezoneId: "UTC",
		WeeklyOpen: []*mtproto.BusinessWeeklyOpen{
			mtproto.MakeTLBusinessWeeklyOpen(&mtproto.BusinessWeeklyOpen{StartMinute: 9 * 60, EndMinute: 18 * 60}).To_BusinessWeeklyOpen(),
		},
	}).To_BusinessWorkHours()
	if err := b.SetWorkHours(3, hours); err != nil {
		t.Fatal(err)
	}
	if err := b.SetAway(3, mtproto.MakeTLInputBusinessAwayMessage(&mtproto.InputBusinessAwayMessage{
		OfflineOnly: true,
		ShortcutId:  6,
		Schedule:    mtproto.MakeTLBusinessAwayMessageScheduleOutsideWorkHours(nil).To_BusinessAwayMessageSchedule(),
		Recipients:  nonContactRecipients(),
	}).To_InputBusinessAwayMessage()); err != nil {
		t.Fatal(err)
	}
	if got := b.ConsumeAutoReply(3, 4, PeerFacts{Now: open}); len(got) != 0 {
		t.Fatalf("open %v", got)
	}
	if got := b.ConsumeAutoReply(3, 4, PeerFacts{Now: closed, Online: true, LastSeen: int32(closed.Unix())}); len(got) != 0 {
		t.Fatalf("online %v", got)
	}
	if got := b.ConsumeAutoReply(3, 4, PeerFacts{Now: closed}); len(got) != 1 || got[0] != 6 {
		t.Fatalf("closed %v", got)
	}
	if got := b.ConsumeAutoReply(3, 4, PeerFacts{Now: closed.Add(time.Minute)}); len(got) != 0 {
		t.Fatalf("same window %v", got)
	}
}

func TestRecipientsExcludeAndNewChats(t *testing.T) {
	b := New()
	now := time.Unix(1_700_000_000, 0)
	if err := b.SetGreeting(1, mtproto.MakeTLInputBusinessGreetingMessage(&mtproto.InputBusinessGreetingMessage{
		ShortcutId: 1,
		Recipients: mtproto.MakeTLInputBusinessRecipients(&mtproto.InputBusinessRecipients{
			ExcludeSelected: true,
			Users:           []*mtproto.InputUser{mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 9}).To_InputUser()},
		}).To_InputBusinessRecipients(),
	}).To_InputBusinessGreetingMessage()); err != nil {
		t.Fatal(err)
	}
	if got := b.ConsumeAutoReply(1, 9, PeerFacts{Now: now, HistoryKnown: true}); len(got) != 0 {
		t.Fatalf("excluded %v", got)
	}
	if got := b.ConsumeAutoReply(1, 8, PeerFacts{Now: now, HistoryKnown: true}); len(got) != 1 {
		t.Fatalf("everyone else %v", got)
	}
}
