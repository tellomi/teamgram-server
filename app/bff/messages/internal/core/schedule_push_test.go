package core

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	syncclient "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestScheduledCreatePushesOtherSessions(t *testing.T) {
	chatbox.ResetShared()
	ctx := context.Background()
	syncer := &schedSync{}
	c := &MessagesCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			SyncClient: syncer,
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 7, PermAuthKeyId: 11},
	}
	when := int32(time.Now().Unix()) + 120
	peer := mtproto.MakePeerUtil(mtproto.PEER_USER, 9)
	ups, err := c.dispatchOutbox(peer, []*msg.OutboxMessage{{
		Message:      &mtproto.Message{Message: "later"},
		ScheduleDate: wrapperspb.Int32(when),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if syncer.notMe == nil || syncer.notMe.GetPermAuthKeyId() != 11 {
		t.Fatal("other session was not pushed")
	}
	var saw bool
	for _, u := range ups.GetUpdates() {
		if u.GetPredicateName() == mtproto.Predicate_updateNewScheduledMessage && u.GetMessage_MESSAGE().GetMessage() == "later" {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("updates %+v", ups.GetUpdates())
	}
}

type schedSync struct {
	syncclient.SyncClient
	notMe *sync.TLSyncUpdatesNotMe
}

func (s *schedSync) SyncUpdatesNotMe(_ context.Context, in *sync.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	s.notMe = in
	return mtproto.MakeTLVoid(nil).To_Void(), nil
}
