package service

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

func TestPollVoteChecksTheCallerCanSeeTheMessage(t *testing.T) {
	t.Setenv("TELLOMI_POLLS", "1")
	chatbox.ResetShared()
	ctx := withUser(2)
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 1}).To_InputPeer()
	svc := PollsService{Messages: pollMessage{id: 44, peerType: mtproto.PEER_USER, peerID: 9, self: 2}}
	if _, err := svc.MessagesSendVote(ctx, &mtproto.TLMessagesSendVote{
		Peer: peer, MsgId: 44, Options: [][]byte{[]byte("0")},
	}); err != mtproto.ErrMessageIdInvalid {
		t.Fatalf("other chat: %v", err)
	}

	group := mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 8}).To_InputPeer()
	outsider := PollsService{
		Chats:    pollMembers{ids: []int64{1, 4}},
		Messages: pollMessage{id: 44, peerType: mtproto.PEER_CHAT, peerID: 8, self: 2},
	}
	if _, err := outsider.MessagesSendVote(ctx, &mtproto.TLMessagesSendVote{
		Peer: group, MsgId: 44, Options: [][]byte{[]byte("0")},
	}); err != mtproto.ErrUserNotParticipant {
		t.Fatalf("non-member: %v", err)
	}
}

func TestChannelVoteDoesNotWalkMembers(t *testing.T) {
	t.Setenv("TELLOMI_POLLS", "1")
	chatbox.ResetShared()
	channel.Use(channel.New(channel.NewMemoryStore()))
	ctx := withUser(9)
	created, err := channel.Shared().Create(ctx, channel.CreateRequest{CreatorID: 1, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0].GetId()
	svc := PollsService{}
	if _, err := svc.MessagesSendVote(withUser(9), &mtproto.TLMessagesSendVote{
		Peer:    mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: ch}).To_InputPeer(),
		MsgId:   1,
		Options: [][]byte{[]byte("0")},
	}); err != mtproto.ErrChannelPrivate {
		t.Fatalf("private channel outsider: %v", err)
	}

	push := &pollPush{}
	svc.Push = push
	own := mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateMessagePoll(&mtproto.Update{PollId: 5, MsgId_INT32: 1}).To_Update())
	svc.fanoutPoll(context.Background(), 1, 99, mtproto.MakePeerUtil(mtproto.PEER_CHANNEL, ch), 1, 5, own)
	if push.users != 0 || push.sessions != 1 {
		t.Fatalf("channel fanout users=%d sessions=%d", push.users, push.sessions)
	}
}

type pollMembers struct{ ids []int64 }

func (m pollMembers) ChatGetChatParticipantIdList(context.Context, *chatpb.TLChatGetChatParticipantIdList) (*chatpb.Vector_Long, error) {
	return &chatpb.Vector_Long{Datas: m.ids}, nil
}

type pollMessage struct {
	id       int32
	peerType int32
	peerID   int64
	self     int64
}

func (m pollMessage) MessageGetUserMessage(_ context.Context, in *message.TLMessageGetUserMessage) (*mtproto.MessageBox, error) {
	if in.GetId() != m.id {
		return nil, mtproto.ErrMessageIdInvalid
	}
	return &mtproto.MessageBox{
		MessageId:    m.id,
		UserId:       in.GetUserId(),
		PeerType:     m.peerType,
		PeerId:       m.peerID,
		SenderUserId: m.self,
	}, nil
}

type pollPush struct {
	users    int
	sessions int
}

func (p *pollPush) SyncPushUpdates(context.Context, *sync.TLSyncPushUpdates) (*mtproto.Void, error) {
	p.users++
	return mtproto.EmptyVoid, nil
}

func (p *pollPush) SyncUpdatesNotMe(context.Context, *sync.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	p.sessions++
	return mtproto.EmptyVoid, nil
}
