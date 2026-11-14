package accesshash

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/botadmin"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	ah "github.com/teamgram/teamgram-server/pkg/accesshash"
	"google.golang.org/grpc/status"
)

type fakeUsers struct {
	rows map[int64]int64
	err  error
}

func (f fakeUsers) UserGetMutableUsers(ctx context.Context, in *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	if f.err != nil {
		return nil, f.err
	}
	var rows []*mtproto.ImmutableUser
	for _, id := range in.GetId() {
		h, ok := f.rows[id]
		if !ok {
			continue
		}
		rows = append(rows, mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
			User: &mtproto.UserData{Id: id, AccessHash: h},
		}).To_ImmutableUser())
	}
	return &userpb.Vector_ImmutableUser{Datas: rows}, nil
}

type fakeMessages struct {
	box *mtproto.MessageBox
	err error
}

func (f fakeMessages) MessageGetUserMessage(ctx context.Context, in *messagepb.TLMessageGetUserMessage) (*mtproto.MessageBox, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.box == nil || f.box.GetMessageId() != in.GetId() {
		return nil, mtproto.ErrMessageIdInvalid
	}
	return f.box, nil
}

func TestIssuedHashesStayValid(t *testing.T) {
	botadmin.Reset()
	t.Cleanup(botadmin.Reset)
	created, err := botadmin.CreateBot(context.Background(), 1, false, "Helper", "hashbot", mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: userpb.BotFatherId}).To_InputUser())
	if err != nil {
		t.Fatal(err)
	}
	botID := created.GetId()
	issued := created.GetAccessHash().GetValue()
	if _, ok := botadmin.IssuedAccessHash(botID); !ok || issued == 0 {
		t.Fatal("bot hash was not issued")
	}

	book := &liveBook{users: fakeUsers{rows: map[int64]int64{4: 44}}}
	got, err := book.UserHashes(context.Background(), 1, []int64{4, mtproto.BotVenueId, mtproto.BotGifId, botID})
	if err != nil {
		t.Fatal(err)
	}
	if !ah.Accept(44, got[4]) || ah.Accept(0, got[4]) {
		t.Fatal("stored user hash")
	}
	if !ah.Accept(catalog.VenueBotUser().GetAccessHash().GetValue(), got[mtproto.BotVenueId]) || ah.Accept(1, got[mtproto.BotVenueId]) {
		t.Fatal("venue bot")
	}
	if !ah.Accept(catalog.GifBotUser().GetAccessHash().GetValue(), got[mtproto.BotGifId]) {
		t.Fatal("gif bot")
	}
	if !ah.Accept(issued, got[botID]) || ah.Accept(0, got[botID]) {
		t.Fatal("created bot")
	}
}

func TestBoxMatchesDialogPair(t *testing.T) {
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 8, AccessHash: 1}).To_InputPeer()
	box := mtproto.MakeTLMessageBox(&mtproto.MessageBox{
		MessageId:    4,
		PeerType:     mtproto.PEER_USER,
		PeerId:       9,
		SenderUserId: 8,
		DialogId1:    8,
		DialogId2:    9,
		Message:      &mtproto.Message{PredicateName: mtproto.Predicate_message},
	}).To_MessageBox()
	if !boxMatches(box, 9, peer) {
		t.Fatal("sorted dialog pair")
	}
	box.DialogId1, box.DialogId2 = 1, 2
	if boxMatches(box, 9, peer) {
		t.Fatal("different dialog")
	}
	box.DialogId1, box.DialogId2 = 0, 0
	if !boxMatches(box, 9, peer) {
		t.Fatal("recipient copy with no dialog ids")
	}
}

func TestPrivateMessageProof(t *testing.T) {
	defer ah.SetMode(ah.ModeEnforce)()
	box := mtproto.MakeTLMessageBox(&mtproto.MessageBox{
		MessageId:    4,
		SenderUserId: 2,
		PeerType:     mtproto.PEER_USER,
		PeerId:       8,
		Message:      &mtproto.Message{PredicateName: mtproto.Predicate_message},
	}).To_MessageBox()
	book := &liveBook{messages: fakeMessages{box: box}}
	req := &mtproto.TLUsersGetFullUser{Id: mtproto.MakeTLInputUserFromMessage(&mtproto.InputUser{
		UserId: 2,
		MsgId:  4,
		Peer:   mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 8, AccessHash: 1}).To_InputPeer(),
	}).To_InputUser()}
	// The container user's hash is unknown, so the peer check fails before
	// the message is consulted.
	book.users = fakeUsers{rows: map[int64]int64{8: 1}}
	if err := ah.Check(context.Background(), 9, "users.getFullUser", req, book, nil); err != nil {
		t.Fatal(err)
	}
	req.Id.UserId = 3
	if status.Convert(ah.Check(context.Background(), 9, "users.getFullUser", req, book, nil)).Message() != "USER_ID_INVALID" {
		t.Fatal("sender is 2, not 3")
	}
	other := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 7, AccessHash: 1}).To_InputPeer()
	req.Id = mtproto.MakeTLInputUserFromMessage(&mtproto.InputUser{UserId: 2, MsgId: 4, Peer: other}).To_InputUser()
	book.users = fakeUsers{rows: map[int64]int64{7: 1}}
	if status.Convert(ah.Check(context.Background(), 9, "users.getFullUser", req, book, nil)).Message() != "USER_ID_INVALID" {
		t.Fatal("message is in a different dialog")
	}
}

func TestChannelPostMustBeVisible(t *testing.T) {
	defer ah.SetMode(ah.ModeEnforce)()
	svc := channel.New(channel.NewMemoryStore())
	prev := channel.Shared()
	channel.Use(svc)
	t.Cleanup(func() { channel.Use(prev) })

	ctx := context.Background()
	up, err := svc.Create(ctx, channel.CreateRequest{CreatorID: 1, Title: "Room", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := up.Chats[0]
	hash := ch.GetAccessHash_FLAGINT64().GetValue()
	sent, err := svc.Send(ctx, 1, ch.GetId(), 0, []channel.Outgoing{{
		RandomID: 1,
		Message: &mtproto.Message{
			PredicateName: mtproto.Predicate_message,
			FromId:        mtproto.MakePeerUser(1),
			Message:       "hi",
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	msgID := sent.Updates[0].Message_MESSAGE.Id
	book := &liveBook{}
	peer := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: ch.GetId(), AccessHash: hash}).To_InputPeer()
	req := &mtproto.TLUsersGetFullUser{Id: mtproto.MakeTLInputUserFromMessage(&mtproto.InputUser{
		UserId: 1,
		MsgId:  msgID,
		Peer:   peer,
	}).To_InputUser()}
	if err := ah.Check(ctx, 1, "users.getFullUser", req, book, nil); err != nil {
		t.Fatal(err)
	}
	// A stranger cannot use a private channel post as proof.
	if status.Convert(ah.Check(ctx, 9, "users.getFullUser", req, book, nil)).Message() != "USER_ID_INVALID" {
		t.Fatal("private channel leaked the author")
	}
	if _, err := svc.UpdateUsername(ctx, 1, 0, mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId: ch.GetId(), AccessHash: hash,
	}).To_InputChannel(), "newsproof"); err != nil {
		t.Fatal(err)
	}
	if err := ah.Check(ctx, 9, "users.getFullUser", req, book, nil); err != nil {
		t.Fatal(err)
	}
	req.Id.UserId = 2
	if status.Convert(ah.Check(ctx, 9, "users.getFullUser", req, book, nil)).Message() != "USER_ID_INVALID" {
		t.Fatal("public post does not contain user 2")
	}
}
