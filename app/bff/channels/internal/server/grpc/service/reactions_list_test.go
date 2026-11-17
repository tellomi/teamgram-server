package service

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/teamgram/proto/mtproto"
	rpmetadata "github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestBasicGroupReactionRequiresMembershipAndTheMessage(t *testing.T) {
	chatbox.ResetShared()
	ctx := withReactionUser(7, false)
	peer := mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 15}).To_InputPeer()
	outsider := ReactionsService{
		Chats:    reactionMembers{ids: []int64{4, 9}},
		Messages: reactionBox{id: 30, peer: 15, peerType: mtproto.PEER_CHAT},
	}
	if _, err := outsider.MessagesSendReaction(ctx, &mtproto.TLMessagesSendReaction{
		Peer: peer, MsgId: 30,
		Reaction_FLAGVECTORREACTION: []*mtproto.Reaction{
			mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "👍"}).To_Reaction(),
		},
	}); err != mtproto.ErrUserNotParticipant {
		t.Fatalf("outsider: %v", err)
	}
	member := ReactionsService{
		Chats:    reactionMembers{ids: []int64{7, 9}},
		Messages: reactionBox{id: 30, peer: 15, peerType: mtproto.PEER_CHAT},
	}
	if _, err := member.MessagesSendReaction(ctx, &mtproto.TLMessagesSendReaction{
		Peer: peer, MsgId: 99,
		Reaction_FLAGVECTORREACTION: []*mtproto.Reaction{
			mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "👍"}).To_Reaction(),
		},
	}); err != mtproto.ErrMsgIdInvalid {
		t.Fatalf("unseen message: %v", err)
	}
	up, err := member.MessagesSendReaction(ctx, &mtproto.TLMessagesSendReaction{
		Peer: peer, MsgId: 30,
		Reaction_FLAGVECTORREACTION: []*mtproto.Reaction{
			mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "👍"}).To_Reaction(),
		},
	})
	if err != nil || up.Updates[0].GetMsgId_INT32() != 30 {
		t.Fatalf("member reaction %#v %v", up, err)
	}
}

type reactionMembers struct{ ids []int64 }

func (m reactionMembers) ChatGetChatParticipantIdList(context.Context, *chatpb.TLChatGetChatParticipantIdList) (*chatpb.Vector_Long, error) {
	return &chatpb.Vector_Long{Datas: m.ids}, nil
}

func TestUnreadReactionDelta(t *testing.T) {
	if unreadDelta(false, true) != 1 || unreadDelta(true, false) != -1 || unreadDelta(true, true) != 0 || unreadDelta(false, false) != 0 {
		t.Fatal("unread badge delta")
	}
}

func TestReactionHandlersPrivateChannelAndBot(t *testing.T) {
	chatbox.ResetShared()
	channel.Use(channel.New(channel.NewMemoryStore()))
	ctx := withReactionUser(7, false)
	svc := ReactionsService{}

	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 8}).To_InputPeer()
	if _, err := chatbox.Shared().React(ctx, 7, mtproto.MakePeerUtil(mtproto.PEER_USER, 8), 4, []string{"👍"}, nil, false); err != nil {
		t.Fatal(err)
	}
	list, err := svc.MessagesGetMessageReactionsList(ctx, &mtproto.TLMessagesGetMessageReactionsList{
		Peer:  peer,
		Id:    4,
		Limit: 10,
	})
	if err != nil || list.Count != 1 || list.Reactions[0].GetPeerId().GetUserId() != 7 || list.Users[0].GetId() != 7 {
		t.Fatalf("private list %#v %v", list, err)
	}
	refresh, err := svc.MessagesGetMessagesReactions(ctx, &mtproto.TLMessagesGetMessagesReactions{
		Peer: peer,
		Id:   []int32{4},
	})
	if err != nil || !refresh.Updates[0].GetReactions_MESSAGEREACTIONS().GetCanSeeList() || refresh.Users[0].GetId() != 7 {
		t.Fatalf("refresh %#v %v", refresh, err)
	}

	wired := ReactionsService{Messages: reactionBox{id: 4, peer: 8, peerType: mtproto.PEER_USER}}
	if _, err := wired.MessagesGetMessageReactionsList(ctx, &mtproto.TLMessagesGetMessageReactionsList{
		Peer: peer, Id: 5, Limit: 10,
	}); err != mtproto.ErrMsgIdInvalid {
		t.Fatalf("missing private message: %v", err)
	}
	kept, err := wired.MessagesGetMessagesReactions(ctx, &mtproto.TLMessagesGetMessagesReactions{
		Peer: peer, Id: []int32{4, 5},
	})
	if err != nil || len(kept.Updates) != 1 || kept.Updates[0].GetMsgId_INT32() != 4 {
		t.Fatalf("kept %#v %v", kept, err)
	}

	up, err := channel.Shared().Create(ctx, channel.CreateRequest{CreatorID: 7, Title: "Group", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := up.Chats[0]
	sent, err := channel.Shared().Send(ctx, 7, ch.Id, 0, []channel.Outgoing{{
		RandomID: 1, Message: &mtproto.Message{Message: "hi"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	msgID := sent.Updates[0].Message_MESSAGE.Id
	if _, err := channel.Shared().React(ctx, 7, ch.Id, msgID, []string{"🔥"}, false); err != nil {
		t.Fatal(err)
	}
	chPeer := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: ch.Id}).To_InputPeer()
	got, err := svc.MessagesGetMessageReactionsList(ctx, &mtproto.TLMessagesGetMessageReactionsList{
		Peer: chPeer, Id: msgID, Limit: 10,
		Reaction_FLAGREACTION: mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "🔥"}).To_Reaction(),
	})
	if err != nil || got.Count != 1 || !got.Reactions[0].GetMy() {
		t.Fatalf("channel list %#v %v", got, err)
	}

	news, err := channel.Shared().Create(ctx, channel.CreateRequest{CreatorID: 7, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	post, err := channel.Shared().Send(ctx, 7, news.Chats[0].Id, 0, []channel.Outgoing{{
		RandomID: 2, Message: &mtproto.Message{Message: "post"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := channel.Shared().React(ctx, 7, news.Chats[0].Id, post.Updates[0].Message_MESSAGE.Id, []string{"👍"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.MessagesGetMessageReactionsList(ctx, &mtproto.TLMessagesGetMessageReactionsList{
		Peer:  mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: news.Chats[0].Id}).To_InputPeer(),
		Id:    post.Updates[0].Message_MESSAGE.Id,
		Limit: 10,
	}); err != mtproto.ErrBroadcastForbidden {
		t.Fatalf("broadcast: %v", err)
	}

	if _, err := svc.MessagesGetMessageReactionsList(withReactionUser(7, true), &mtproto.TLMessagesGetMessageReactionsList{
		Peer: peer, Id: 4, Limit: 10,
	}); err != mtproto.ErrBotMethodInvalid {
		t.Fatalf("bot: %v", err)
	}
	if _, err := svc.MessagesGetMessagesReactions(ctx, &mtproto.TLMessagesGetMessagesReactions{
		Peer: mtproto.MakeTLInputPeerEmpty(&mtproto.InputPeer{}).To_InputPeer(),
		Id:   []int32{4},
	}); err != mtproto.ErrPeerIdInvalid {
		t.Fatalf("empty peer: %v", err)
	}
	legacy, err := svc.MessagesGetMessageReactionsList(ctx, &mtproto.TLMessagesGetMessageReactionsList{
		Peer:                peer,
		Id:                  4,
		Limit:               10,
		Reaction_FLAGSTRING: &wrapperspb.StringValue{Value: "👍"},
	})
	if err != nil || legacy.Count != 1 {
		t.Fatalf("legacy filter %#v %v", legacy, err)
	}
}

type reactionBox struct {
	id       int32
	peer     int64
	peerType int32
}

func (f reactionBox) MessageGetUserMessage(_ context.Context, in *message.TLMessageGetUserMessage) (*mtproto.MessageBox, error) {
	if in.GetId() != f.id {
		return nil, mtproto.ErrMessageIdInvalid
	}
	return &mtproto.MessageBox{MessageId: f.id, UserId: in.GetUserId(), PeerType: f.peerType, PeerId: f.peer}, nil
}

func withReactionUser(user int64, bot bool) context.Context {
	raw, err := proto.Marshal(&rpmetadata.RpcMetadata{UserId: user, IsBot: bot})
	if err != nil {
		panic(err)
	}
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"rpc_metadata", base64.StdEncoding.EncodeToString(raw),
	))
}

// TestBoxPeerMatchesIncomingPrivate covers the recipient's copy of a private
// message: prod rows store peer_id = the recipient itself, sender = the chat
// partner, and the dialog as the sorted id pair.
func TestBoxPeerMatchesIncomingPrivate(t *testing.T) {
	const me, them = int64(136907725), int64(136907717)
	peer := mtproto.MakePeerUtil(mtproto.PEER_USER, them)
	incoming := &mtproto.MessageBox{MessageId: 110, PeerType: mtproto.PEER_USER, PeerId: me, SenderUserId: them, DialogId1: them, DialogId2: me}
	if !boxPeerMatches(incoming, peer, me) {
		t.Fatal("incoming private message did not match its chat")
	}
	legacy := &mtproto.MessageBox{MessageId: 110, PeerType: mtproto.PEER_USER, PeerId: me, SenderUserId: them}
	if !boxPeerMatches(legacy, peer, me) {
		t.Fatal("incoming row without dialog ids did not match")
	}
	other := &mtproto.MessageBox{MessageId: 111, PeerType: mtproto.PEER_USER, PeerId: me, SenderUserId: 42, DialogId1: 42, DialogId2: me}
	if boxPeerMatches(other, peer, me) {
		t.Fatal("message from another chat matched")
	}
	outgoing := &mtproto.MessageBox{MessageId: 112, PeerType: mtproto.PEER_USER, PeerId: them, SenderUserId: me, DialogId1: them, DialogId2: me}
	if !boxPeerMatches(outgoing, peer, me) {
		t.Fatal("outgoing private message did not match")
	}
}
