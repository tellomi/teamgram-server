package service

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	rpcmd "github.com/teamgram/proto/mtproto/rpc/metadata"
	syncclient "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"google.golang.org/grpc/metadata"
)

func TestDeleteScheduledPushesOtherSession(t *testing.T) {
	chatbox.ResetShared()
	ctx := context.Background()
	peer := mtproto.MakePeerUtil(mtproto.PEER_USER, 9)
	when := int32(time.Now().Unix()) + 180
	ups, err := chatbox.Shared().Schedule(ctx, 7, peer, when, []*mtproto.Message{{
		Message: "later",
	}})
	if err != nil || len(ups.GetUpdates()) == 0 {
		t.Fatal(err)
	}
	id := ups.GetUpdates()[0].GetMessage_MESSAGE().GetId()
	out, err := rpcmd.RpcMetadataToOutgoing(context.Background(), &rpcmd.RpcMetadata{UserId: 7, PermAuthKeyId: 11})
	if err != nil {
		t.Fatal(err)
	}
	md, _ := metadata.FromOutgoingContext(out)
	ctx = metadata.NewIncomingContext(context.Background(), md)
	syncer := &deleteSchedSync{}
	got, err := (ScheduledService{Sync: syncer}).MessagesDeleteScheduledMessages(ctx, &mtproto.TLMessagesDeleteScheduledMessages{
		Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 9}).To_InputPeer(),
		Id:   []int32{id},
	})
	if err != nil {
		t.Fatal(err)
	}
	if syncer.notMe == nil || syncer.notMe.GetUserId() != 7 || syncer.notMe.GetPermAuthKeyId() != 11 {
		t.Fatal("other session was not pushed")
	}
	if got.GetUpdates()[0].GetPredicateName() != mtproto.Predicate_updateDeleteScheduledMessages {
		t.Fatalf("update %+v", got.GetUpdates())
	}
}

type deleteSchedSync struct {
	syncclient.SyncClient
	notMe *sync.TLSyncUpdatesNotMe
}

func (s *deleteSchedSync) SyncUpdatesNotMe(_ context.Context, in *sync.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	s.notMe = in
	return mtproto.MakeTLVoid(nil).To_Void(), nil
}
