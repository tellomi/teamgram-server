package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	message_client "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestGetHistoryPassesOfficialLimitDownstream(t *testing.T) {
	h := &historyMsg{}
	c := &MessagesCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			MessageClient: h,
			UserClient:    historyUsers{},
		}},
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 7},
	}
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 9}).To_InputPeer()
	for _, tc := range []struct {
		in, want int32
	}{
		{100, 100},
		{101, 100},
		{50, 50},
	} {
		h.got = nil
		_, err := c.MessagesGetHistory(&mtproto.TLMessagesGetHistory{
			Peer:  peer,
			Limit: tc.in,
		})
		if err != nil {
			t.Fatal(err)
		}
		if h.got == nil || h.got.Limit != tc.want {
			t.Fatalf("limit %d downstream %#v want %d", tc.in, h.got, tc.want)
		}
	}
}

type historyMsg struct {
	message_client.MessageClient
	got *message.TLMessageGetHistoryMessages
}

func (h *historyMsg) MessageGetHistoryMessages(_ context.Context, in *message.TLMessageGetHistoryMessages) (*message.Vector_MessageBox, error) {
	h.got = in
	return &message.Vector_MessageBox{}, nil
}

func (h *historyMsg) MessageGetHistoryMessagesCount(context.Context, *message.TLMessageGetHistoryMessagesCount) (*mtproto.Int32, error) {
	return &mtproto.Int32{V: 0}, nil
}

type historyUsers struct {
	user_client.UserClient
}

func (historyUsers) UserGetMutableUsers(context.Context, *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	return &userpb.Vector_ImmutableUser{}, nil
}

func TestGetHistoryEmptyDialogIsMessagesNotAnError(t *testing.T) {
	asked := &askedHistoryUsers{}
	c := &MessagesCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			MessageClient: &historyMsg{},
			UserClient:    asked,
		}},
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 7},
	}
	res, err := c.MessagesGetHistory(&mtproto.TLMessagesGetHistory{
		Peer:  mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 9}).To_InputPeer(),
		Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.GetPredicateName() != mtproto.Predicate_messages_messages {
		t.Fatalf("constructor %s", res.GetPredicateName())
	}
	if len(res.GetMessages()) != 0 {
		t.Fatalf("messages %+v", res.GetMessages())
	}
	if !asked.saw(9) {
		t.Fatalf("peer user was not loaded: %v", asked.ids)
	}
}

type askedHistoryUsers struct {
	user_client.UserClient
	ids []int64
}

func (a *askedHistoryUsers) UserGetMutableUsers(_ context.Context, in *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	a.ids = append(a.ids, in.GetId()...)
	return &userpb.Vector_ImmutableUser{}, nil
}

func (a *askedHistoryUsers) saw(id int64) bool {
	for _, v := range a.ids {
		if v == id {
			return true
		}
	}
	return false
}
