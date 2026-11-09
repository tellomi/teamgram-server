package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/privacysettings/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/privacysettings/internal/svc"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestSetDefaultHistoryTTLRejectsOutOfRangeAndStoreErrors(t *testing.T) {
	store := &ttlStore{}
	c := &PrivacySettingsCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: store}},
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}
	for _, period := range []int32{-1, 366*86400 + 1} {
		got, err := c.MessagesSetDefaultHistoryTTL(&mtproto.TLMessagesSetDefaultHistoryTTL{Period: period})
		if err != mtproto.ErrTtlPeriodInvalid || got != nil {
			t.Fatalf("period %d: %v %v", period, got, err)
		}
	}
	if store.calls != 0 {
		t.Fatalf("invalid period wrote %d times", store.calls)
	}

	got, err := c.MessagesSetDefaultHistoryTTL(&mtproto.TLMessagesSetDefaultHistoryTTL{Period: 86400})
	if err != nil || got == nil || !mtproto.FromBool(got) {
		t.Fatalf("save: %v %v", got, err)
	}
	if store.calls != 1 || store.userID != 42 || store.ttl != 86400 {
		t.Fatalf("stored user %d ttl %d calls %d", store.userID, store.ttl, store.calls)
	}

	store.err = errors.New("default history ttl store failed")
	got, err = c.MessagesSetDefaultHistoryTTL(&mtproto.TLMessagesSetDefaultHistoryTTL{Period: 0})
	if err == nil || got != nil {
		t.Fatalf("store failure was reported as success: %v %v", got, err)
	}
}

type ttlStore struct {
	user_client.UserClient
	err    error
	userID int64
	ttl    int32
	calls  int
}

func (s *ttlStore) UserSetDefaultHistoryTTL(_ context.Context, in *user.TLUserSetDefaultHistoryTTL) (*mtproto.Bool, error) {
	s.calls++
	s.userID = in.GetUserId()
	s.ttl = in.GetTtl()
	if s.err != nil {
		return nil, s.err
	}
	return mtproto.BoolTrue, nil
}
