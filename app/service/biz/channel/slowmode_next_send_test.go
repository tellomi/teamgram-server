package channel

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

func TestSlowmodeNextSendDate(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "Slow")
	in := inCh(ch)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2, 3, 4}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EditAdmin(ctx, 1, 0, in, 2, rights(true, false, false, false), "mod"); err != nil {
		t.Fatal(err)
	}
	const seconds int32 = 30
	if _, err := svc.ToggleSlowMode(ctx, 1, 0, in, seconds); err != nil {
		t.Fatal(err)
	}

	member := mustFull(t, svc, ctx, 3, in)
	if member.GetSlowmodeSeconds().GetValue() != seconds || member.GetSlowmodeNextSendDate() != nil {
		t.Fatalf("before send seconds=%d next=%v", member.GetSlowmodeSeconds().GetValue(), member.GetSlowmodeNextSendDate())
	}

	before := int32(time.Now().Unix())
	if _, err := svc.Send(ctx, 3, ch.Id, 0, []Outgoing{{RandomID: 1, Message: &mtproto.Message{Message: "a"}}}); err != nil {
		t.Fatal(err)
	}
	after := int32(time.Now().Unix())
	member = mustFull(t, svc, ctx, 3, in)
	got := member.GetSlowmodeNextSendDate()
	if got == nil {
		t.Fatal("member next send date missing")
	}
	next := got.GetValue()
	if next <= before || next > after+seconds || next < before+seconds {
		t.Fatalf("member next %d window (%d, %d]", next, before, after+seconds)
	}
	if mustFull(t, svc, ctx, 4, in).GetSlowmodeNextSendDate() != nil {
		t.Fatal("member who has not sent got a next date")
	}

	buf := mtproto.NewEncodeBuf(4096)
	box := mtproto.MakeTLMessagesChatFull(&mtproto.Messages_ChatFull{
		FullChat: member,
		Chats:    []*mtproto.Chat{},
		Users:    []*mtproto.User{},
	}).To_Messages_ChatFull()
	if err := box.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	decoded := &mtproto.Messages_ChatFull{}
	if err := decoded.Decode(mtproto.NewDecodeBuf(append([]byte(nil), buf.GetBuf()...))); err != nil {
		t.Fatal(err)
	}
	if decoded.GetFullChat().GetSlowmodeNextSendDate().GetValue() != next {
		t.Fatalf("roundtrip %d", decoded.GetFullChat().GetSlowmodeNextSendDate().GetValue())
	}

	if _, err := svc.Send(ctx, 3, ch.Id, 0, []Outgoing{{RandomID: 2, Message: &mtproto.Message{Message: "b"}}}); err == nil || !strings.Contains(err.Error(), "SLOWMODE_WAIT_") {
		t.Fatalf("second send: %v", err)
	}
	if again := mustFull(t, svc, ctx, 3, in).GetSlowmodeNextSendDate().GetValue(); again != next {
		t.Fatalf("rejected send moved the window %d -> %d", next, again)
	}

	if _, err := svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{RandomID: 3, Message: &mtproto.Message{Message: "admin"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{RandomID: 4, Message: &mtproto.Message{Message: "owner"}}}); err != nil {
		t.Fatal(err)
	}
	if mustFull(t, svc, ctx, 2, in).GetSlowmodeNextSendDate() != nil {
		t.Fatal("admin next send date")
	}
	if mustFull(t, svc, ctx, 1, in).GetSlowmodeNextSendDate() != nil {
		t.Fatal("creator next send date")
	}

	feat := svc.features(ctx, ch.Id)
	feat.LastSend[3] = 1
	if err := svc.saveFeatures(ctx, ch.Id, feat); err != nil {
		t.Fatal(err)
	}
	elapsed := mustFull(t, svc, ctx, 3, in)
	if elapsed.GetSlowmodeSeconds().GetValue() != seconds || elapsed.GetSlowmodeNextSendDate() != nil {
		t.Fatal("elapsed window still set")
	}
	feat = svc.features(ctx, ch.Id)
	feat.LastSend[3] = int32(time.Now().Unix()) - seconds
	if err := svc.saveFeatures(ctx, ch.Id, feat); err != nil {
		t.Fatal(err)
	}
	if mustFull(t, svc, ctx, 3, in).GetSlowmodeNextSendDate() != nil {
		t.Fatal("boundary at now is still set")
	}

	if _, err := svc.ToggleSlowMode(ctx, 1, 0, in, 0); err != nil {
		t.Fatal(err)
	}
	off := mustFull(t, svc, ctx, 3, in)
	if off.GetSlowmodeSeconds() != nil || off.GetSlowmodeNextSendDate() != nil {
		t.Fatalf("disabled seconds=%v next=%v", off.GetSlowmodeSeconds(), off.GetSlowmodeNextSendDate())
	}
	if _, err := svc.Send(ctx, 3, ch.Id, 0, []Outgoing{{RandomID: 5, Message: &mtproto.Message{Message: "c"}}}); err != nil {
		t.Fatal(err)
	}
}

func mustFull(t *testing.T, svc *Service, ctx context.Context, user int64, in *mtproto.InputChannel) *mtproto.ChatFull {
	t.Helper()
	full, err := svc.GetFullChannel(ctx, user, in)
	if err != nil || full.GetFullChat() == nil {
		t.Fatalf("getFullChannel user %d: %v", user, err)
	}
	return full.GetFullChat()
}
