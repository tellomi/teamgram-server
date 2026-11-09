package historyttl

import (
	"encoding/json"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestApplyAndExpired(t *testing.T) {
	msg := mtproto.MakeTLMessage(&mtproto.Message{Date: 100}).To_Message()
	Apply(msg, 0)
	if PeriodOf(msg) != 0 || Expired(100, PeriodOf(msg), 1000) {
		t.Fatalf("off timer must not expire, ttl=%d", PeriodOf(msg))
	}
	Apply(msg, 86400)
	if PeriodOf(msg) != 86400 {
		t.Fatalf("stamp = %d", PeriodOf(msg))
	}
	if Expired(100, 86400, 100+86400-1) {
		t.Fatal("one second early")
	}
	if !Expired(100, 86400, 100+86400) {
		t.Fatal("due at date+ttl")
	}
	if Expired(0, 86400, 100) || Expired(100, 0, 100) {
		t.Fatal("missing date or ttl must stay")
	}
}

func TestPeriodAtIgnoresMessagesSentBeforeTheTimer(t *testing.T) {
	actions := []Action{
		{Date: 1000, Period: 86400},
		{Date: 5000, Period: 0},
		{Date: 8000, Period: 3600},
	}
	if got := PeriodAt(actions, 999); got != 0 {
		t.Fatalf("before any action = %d", got)
	}
	if got := PeriodAt(actions, 1000); got != 86400 {
		t.Fatalf("on the service message = %d", got)
	}
	if got := PeriodAt(actions, 4999); got != 86400 {
		t.Fatalf("while first timer is on = %d", got)
	}
	if got := PeriodAt(actions, 5000); got != 0 {
		t.Fatalf("timer disabled = %d", got)
	}
	if got := PeriodAt(actions, 8000); got != 3600 {
		t.Fatalf("second timer = %d", got)
	}
	if got := PeriodAt(nil, 100); got != 0 {
		t.Fatalf("no actions = %d", got)
	}
}

func TestParseAction(t *testing.T) {
	msg := mtproto.MakeTLMessageService(&mtproto.Message{
		Date:   50,
		Action: mtproto.MakeMessageActionSetMessagesTTL(86400),
	}).To_Message()
	raw, err := jsonMarshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := ParseAction(raw)
	if !ok || got.Date != 50 || got.Period != 86400 {
		t.Fatalf("parsed %#v ok=%v", got, ok)
	}
	plain := mtproto.MakeTLMessage(&mtproto.Message{Date: 50, Message: "hi"}).To_Message()
	raw, _ = jsonMarshal(plain)
	if _, ok := ParseAction(raw); ok {
		t.Fatal("plain text parsed as a ttl action")
	}
}

func TestUnreadAfterAndDue(t *testing.T) {
	head := Head{ReadInbox: 10, Unread: 4}
	left := UnreadAfter(head, 7, []Box{
		{BoxID: 11, SenderID: 8},
		{BoxID: 12, SenderID: 8},
		{BoxID: 13, SenderID: 7}, // own outgoing
		{BoxID: 9, SenderID: 8},  // already read
	})
	if left != 2 {
		t.Fatalf("unread = %d, want 2", left)
	}
	if got := UnreadAfter(Head{Unread: 1}, 1, []Box{{BoxID: 5, SenderID: 2}}); got != 0 {
		t.Fatalf("floor = %d", got)
	}
	due := Due([]Box{
		{UserID: 1, BoxID: 2, Date: 10, TTL: 5},
		{UserID: 1, BoxID: 2, Date: 10, TTL: 5},
		{UserID: 1, BoxID: 3, Date: 10, TTL: 0},
		{UserID: 2, BoxID: 4, Date: 10, TTL: 5},
	}, 20)
	if len(due) != 2 || due[0].UserID != 1 || due[1].UserID != 2 {
		t.Fatalf("due = %#v", due)
	}
}

func jsonMarshal(msg *mtproto.Message) (string, error) {
	b, err := json.Marshal(msg)
	return string(b), err
}
