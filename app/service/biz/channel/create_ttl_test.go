package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestCreateTTLZeroWritesNoServiceMessage(t *testing.T) {
	svc, ctx := newTest(t)
	up, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Off", Megagroup: true, TtlPeriod: 0})
	if err != nil {
		t.Fatal(err)
	}
	in := inCh(up.Chats[0])
	full, err := svc.GetFullChannel(ctx, 1, in)
	if err != nil || full.GetFullChat().GetTtlPeriod() != nil {
		t.Fatalf("full ttl = %+v %v", full.GetFullChat().GetTtlPeriod(), err)
	}
	hist, err := svc.History(ctx, 1, in, HistoryQuery{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range hist.GetMessages() {
		if m.GetAction().GetPredicateName() == mtproto.Predicate_messageActionSetMessagesTTL {
			t.Fatalf("ttl 0 wrote %#v", m.GetAction())
		}
	}
}

func TestCreateTTLPositiveAnnouncesWithoutGlobalFlag(t *testing.T) {
	svc, ctx := newTest(t)
	up, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Day", Megagroup: true, TtlPeriod: 86400})
	if err != nil {
		t.Fatal(err)
	}
	in := inCh(up.Chats[0])
	full, err := svc.GetFullChannel(ctx, 1, in)
	if err != nil || full.GetFullChat().GetTtlPeriod().GetValue() != 86400 {
		t.Fatalf("full ttl = %+v %v", full.GetFullChat().GetTtlPeriod(), err)
	}
	var found bool
	for _, u := range up.GetUpdates() {
		act := u.GetMessage_MESSAGE().GetAction()
		if act.GetPredicateName() != mtproto.Predicate_messageActionSetMessagesTTL {
			continue
		}
		found = true
		if act.GetPeriod() != 86400 || act.GetAutoSettingFrom() != nil {
			t.Fatalf("create ttl action = %#v", act)
		}
	}
	if !found {
		t.Fatal("positive ttl did not announce messageActionSetMessagesTTL")
	}
}

func TestCreateTTLRejectsOutOfRange(t *testing.T) {
	svc, ctx := newTest(t)
	if _, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Bad", Megagroup: true, TtlPeriod: -1}); err != mtproto.ErrTtlPeriodInvalid {
		t.Fatalf("err = %v", err)
	}
}
