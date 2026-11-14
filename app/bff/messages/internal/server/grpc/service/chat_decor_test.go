package service

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/messages/p1rest"
	msg_client "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
)

func TestSetChatThemePostsTheServiceMessage(t *testing.T) {
	p1rest.Reset()
	posted := &decorMsg{}
	svc := ThemesService{Msg: posted}
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 7, AccessHash: 1}).To_InputPeer()
	up, err := svc.MessagesSetChatTheme(withUser(1), &mtproto.TLMessagesSetChatTheme{
		Peer:  peer,
		Theme: mtproto.MakeTLInputChatTheme(&mtproto.InputChatTheme{Emoticon: "🎄"}).To_InputChatTheme(),
	})
	if err != nil || up == nil || posted.action.GetPredicateName() != mtproto.Predicate_messageActionSetChatTheme {
		t.Fatalf("theme %v action %#v", err, posted.action)
	}
	if posted.peer != mtproto.PEER_USER || posted.peerID != 7 {
		t.Fatalf("peer %d %d", posted.peer, posted.peerID)
	}

	p1rest.Reset()
	walls := WallpapersService{Msg: posted}
	posted.action = nil
	paper, err := walls.MessagesSetChatWallPaper(withUser(1), &mtproto.TLMessagesSetChatWallPaper{
		Peer:      peer,
		ForBoth:   true,
		Wallpaper: mtproto.MakeTLInputWallPaperSlug(&mtproto.InputWallPaper{Slug: "ocean"}).To_InputWallPaper(),
	})
	if err != nil || paper == nil || !posted.action.GetForBoth() || posted.action.GetPredicateName() != mtproto.Predicate_messageActionSetChatWallPaper {
		t.Fatalf("wallpaper %v action %#v", err, posted.action)
	}
}

type decorMsg struct {
	msg_client.MsgClient
	action *mtproto.MessageAction
	peer   int32
	peerID int64
}

func (m *decorMsg) MsgSendMessageV2(_ context.Context, in *msg.TLMsgSendMessageV2) (*mtproto.Updates, error) {
	m.peer = in.GetPeerType()
	m.peerID = in.GetPeerId()
	if len(in.GetMessage()) > 0 {
		m.action = in.GetMessage()[0].GetMessage().GetAction()
	}
	return mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateNewMessage(&mtproto.Update{
		Message_MESSAGE: in.GetMessage()[0].GetMessage(),
	}).To_Update()), nil
}
