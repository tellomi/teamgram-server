package channel

import (
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/chattitle"
)

func TestOwnPostEditWithoutEditRight(t *testing.T) {
	svc, ctx := newTest(t)
	up, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Posts", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := up.Chats[0]
	in := inCh(ch)
	if _, err = svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	post := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{PostMessages: true}).To_ChatAdminRights()
	if _, err = svc.EditAdmin(ctx, 1, 0, in, 2, post, "poster"); err != nil {
		t.Fatal(err)
	}
	sent, err := svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{RandomID: 8, Message: &mtproto.Message{Message: "mine", Date: 10}}})
	if err != nil {
		t.Fatal(err)
	}
	msgID := sent.Updates[0].GetMessage_MESSAGE().GetId()
	if _, err = svc.Edit(ctx, 2, ch.Id, 0, msgID, func(m *mtproto.Message) {
		m.Message = "mine-edited"
	}); err != nil {
		t.Fatalf("own edit: %v", err)
	}
	if _, err = svc.Edit(ctx, 2, ch.Id, 0, msgID, func(m *mtproto.Message) {
		m.Message = "mine-edited"
	}); err != mtproto.ErrMessageNotModified {
		t.Fatalf("unchanged edit: %v", err)
	}
	owner, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{RandomID: 9, Message: &mtproto.Message{Message: "owner", Date: 11}}})
	if err != nil {
		t.Fatal(err)
	}
	ownerID := owner.Updates[0].GetMessage_MESSAGE().GetId()
	if _, err = svc.Edit(ctx, 2, ch.Id, 0, ownerID, func(m *mtproto.Message) {
		m.Message = "nope"
	}); err != mtproto.ErrMessageAuthorRequired {
		t.Fatalf("other post: %v", err)
	}
	if _, _, err = svc.Delete(ctx, 2, ch.Id, 0, []int32{msgID}); err != nil {
		t.Fatalf("own delete: %v", err)
	}
}

func TestMessageBodyReturnsStoredMedia(t *testing.T) {
	svc, ctx := newTest(t)
	up, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Media", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := up.Chats[0]
	photo := mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{}).To_MessageMedia()
	sent, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{RandomID: 3, Message: &mtproto.Message{Message: "cap", Media: photo, Date: 10}}})
	if err != nil {
		t.Fatal(err)
	}
	msgID := sent.Updates[0].GetMessage_MESSAGE().GetId()
	body, err := svc.MessageBody(ctx, ch.Id, msgID)
	if err != nil || body.GetMedia().GetPredicateName() != photo.GetPredicateName() {
		t.Fatalf("body %v %v", body.GetMedia().GetPredicateName(), err)
	}
	if _, err = svc.MessageBody(ctx, ch.Id, msgID+99); err != mtproto.ErrMessageIdInvalid {
		t.Fatalf("missing post: %v", err)
	}
}

func TestTitleCountsRunes(t *testing.T) {
	svc, ctx := newTest(t)
	emoji := "zt" + strings.Repeat("🙂", 80)
	up, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: emoji, Megagroup: true})
	if err != nil {
		t.Fatalf("emoji title: %v", err)
	}
	if up.Chats[0].GetTitle() != emoji {
		t.Fatalf("stored %q", up.Chats[0].GetTitle())
	}
	long := strings.Repeat("群", 200)
	edited, err := svc.EditTitle(ctx, 1, 0, inCh(up.Chats[0]), long)
	if err != nil {
		t.Fatalf("chinese title: %v", err)
	}
	var title string
	for _, chat := range edited.Chats {
		if chat.GetId() == up.Chats[0].GetId() {
			title = chat.GetTitle()
		}
	}
	if len([]rune(title)) != chattitle.MaxRunes {
		t.Fatalf("title runes %d %q", len([]rune(title)), title)
	}
}
