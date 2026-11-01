package service

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/teamgram/proto/mtproto"
	rpmetadata "github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

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
