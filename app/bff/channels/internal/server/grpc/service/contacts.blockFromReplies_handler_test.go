package service

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	rpcmd "github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func repliesCtx() context.Context {
	out, err := rpcmd.RpcMetadataToOutgoing(context.Background(), &rpcmd.RpcMetadata{UserId: 3, PermAuthKeyId: 9})
	if err != nil {
		panic(err)
	}
	md, _ := metadata.FromOutgoingContext(out)
	return metadata.NewIncomingContext(context.Background(), md)
}

func TestBlockFromReplies(t *testing.T) {
	ctx := repliesCtx()
	box := &mtproto.MessageBox{
		UserId:       3,
		MessageId:    40,
		SenderUserId: repliesUserID,
		PeerType:     mtproto.PEER_USER,
		PeerId:       repliesUserID,
		Message: &mtproto.Message{
			FwdFrom: &mtproto.MessageFwdHeader{
				FromId: mtproto.MakeTLPeerUser(&mtproto.Peer{UserId: 15}).To_Peer(),
			},
		},
	}
	var blocked int64
	var deleted []int32
	var synced bool
	s := ThreadsService{
		Messages: repliesMsgFake{box: box, history: []*mtproto.MessageBox{
			box,
			{
				MessageId:    30,
				SenderUserId: repliesUserID,
				PeerType:     mtproto.PEER_USER,
				PeerId:       repliesUserID,
				Message: &mtproto.Message{FwdFrom: &mtproto.MessageFwdHeader{
					FromId: mtproto.MakeTLPeerUser(&mtproto.Peer{UserId: 15}).To_Peer(),
				}},
			},
			{
				MessageId: 20,
				PeerType:  mtproto.PEER_USER,
				PeerId:    repliesUserID,
				Message: &mtproto.Message{FwdFrom: &mtproto.MessageFwdHeader{
					FromId: mtproto.MakeTLPeerUser(&mtproto.Peer{UserId: 99}).To_Peer(),
				}},
			},
		}},
		Users:  repliesUserFake{blocked: &blocked},
		Outbox: repliesOutFake{deleted: &deleted},
		Sync:   repliesSyncFake{called: &synced},
	}
	if _, err := s.ContactsBlockFromReplies(ctx, &mtproto.TLContactsBlockFromReplies{MsgId: 0}); status.Convert(err).Message() != "MSG_ID_INVALID" {
		t.Fatalf("zero id %v", err)
	}
	up, err := s.ContactsBlockFromReplies(ctx, &mtproto.TLContactsBlockFromReplies{
		DeleteHistory: true,
		ReportSpam:    true,
		MsgId:         40,
	})
	if err != nil {
		t.Fatal(err)
	}
	if blocked != 15 {
		t.Fatalf("blocked %d", blocked)
	}
	if len(deleted) != 2 || !synced {
		t.Fatalf("deleted %#v synced %v", deleted, synced)
	}
	if up == nil || len(up.GetUpdates()) != 1 || up.GetUpdates()[0].GetPeerId().GetUserId() != 15 {
		t.Fatalf("updates %#v", up)
	}
	if _, err := s.ContactsBlockFromReplies(ctx, &mtproto.TLContactsBlockFromReplies{MsgId: 7}); status.Convert(err).Message() != "MSG_ID_INVALID" {
		t.Fatalf("missing %v", err)
	}
}

type repliesMsgFake struct {
	box     *mtproto.MessageBox
	history []*mtproto.MessageBox
}

func (f repliesMsgFake) MessageGetUserMessage(_ context.Context, in *message.TLMessageGetUserMessage) (*mtproto.MessageBox, error) {
	if f.box != nil && in.GetId() == f.box.GetMessageId() {
		return f.box, nil
	}
	return nil, mtproto.ErrMsgIdInvalid
}

func (f repliesMsgFake) MessageGetHistoryMessages(context.Context, *message.TLMessageGetHistoryMessages) (*message.Vector_MessageBox, error) {
	return &message.Vector_MessageBox{Datas: f.history}, nil
}

type repliesUserFake struct{ blocked *int64 }

func (f repliesUserFake) UserGetMutableUsers(_ context.Context, in *user.TLUserGetMutableUsers) (*user.Vector_ImmutableUser, error) {
	var datas []*mtproto.ImmutableUser
	for _, id := range in.GetId() {
		datas = append(datas, &mtproto.ImmutableUser{User: &mtproto.UserData{Id: id, FirstName: "n"}})
	}
	return &user.Vector_ImmutableUser{Datas: datas}, nil
}

func (f repliesUserFake) UserBlockPeer(_ context.Context, in *user.TLUserBlockPeer) (*mtproto.Bool, error) {
	*f.blocked = in.GetPeerId()
	return mtproto.BoolTrue, nil
}

type repliesOutFake struct{ deleted *[]int32 }

func (f repliesOutFake) MsgDeleteMessages(_ context.Context, in *msg.TLMsgDeleteMessages) (*mtproto.Messages_AffectedMessages, error) {
	*f.deleted = append([]int32{}, in.GetId()...)
	return &mtproto.Messages_AffectedMessages{}, nil
}

type repliesSyncFake struct{ called *bool }

func (f repliesSyncFake) SyncUpdatesNotMe(context.Context, *sync.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	*f.called = true
	return mtproto.MakeTLVoid(nil).To_Void(), nil
}
