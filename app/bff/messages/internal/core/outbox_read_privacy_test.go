package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	messageclient "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
)

func TestOutboxReadDateHonorsHideReadMarks(t *testing.T) {
	users := &readPrivacyUsers{}
	msgs := &readPrivacyMessages{date: 1_700_000_000}
	c := readDateCore(users, msgs)

	got, err := c.MessagesGetOutboxReadDate(readDateRequest(2, 9))
	if err != nil || got.GetDate() != msgs.date {
		t.Fatalf("open date %v %v", got, err)
	}
	if msgs.lookups != 1 {
		t.Fatalf("lookups %d", msgs.lookups)
	}

	users.peer = true
	msgs.lookups = 0
	if _, err = c.MessagesGetOutboxReadDate(readDateRequest(2, 9)); status.Convert(err).Message() != "USER_PRIVACY_RESTRICTED" {
		t.Fatalf("peer hide: %v", err)
	}
	if msgs.lookups != 0 {
		t.Fatal("privacy failure still fetched the read date")
	}

	users.peer = false
	users.self = true
	if _, err = c.MessagesGetOutboxReadDate(readDateRequest(2, 9)); status.Convert(err).Message() != "YOUR_PRIVACY_RESTRICTED" {
		t.Fatalf("self hide: %v", err)
	}

	users.premium = true
	got, err = c.MessagesGetOutboxReadDate(readDateRequest(2, 9))
	if err != nil || got.GetDate() != msgs.date {
		t.Fatalf("premium caller %v %v", got, err)
	}
	users.peer = true
	if _, err = c.MessagesGetOutboxReadDate(readDateRequest(2, 9)); status.Convert(err).Message() != "USER_PRIVACY_RESTRICTED" {
		t.Fatalf("premium vs peer hide: %v", err)
	}
}

func readDateCore(users *readPrivacyUsers, msgs *readPrivacyMessages) *MessagesCore {
	return &MessagesCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			UserClient:    users,
			MessageClient: msgs,
		}},
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 1},
	}
}

func readDateRequest(peer int64, msgID int32) *mtproto.TLMessagesGetOutboxReadDate {
	return &mtproto.TLMessagesGetOutboxReadDate{
		Peer:  mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: peer}).To_InputPeer(),
		MsgId: msgID,
	}
}

type readPrivacyUsers struct {
	userclient.UserClient
	self, peer, premium bool
}

func (u *readPrivacyUsers) UserGetGlobalPrivacySettings(_ context.Context, in *user.TLUserGetGlobalPrivacySettings) (*mtproto.GlobalPrivacySettings, error) {
	hide := u.peer && in.GetUserId() != 1
	if in.GetUserId() == 1 {
		hide = u.self
	}
	return mtproto.MakeTLGlobalPrivacySettings(&mtproto.GlobalPrivacySettings{HideReadMarks: hide}).To_GlobalPrivacySettings(), nil
}

func (u *readPrivacyUsers) UserGetImmutableUser(context.Context, *user.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	return &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 1, Premium: u.premium}}, nil
}

type readPrivacyMessages struct {
	messageclient.MessageClient
	date    int32
	lookups int
}

func (m *readPrivacyMessages) MessageGetUserMessage(_ context.Context, in *message.TLMessageGetUserMessage) (*mtproto.MessageBox, error) {
	return &mtproto.MessageBox{UserId: in.GetUserId(), MessageId: in.GetId(), PeerType: mtproto.PEER_USER, PeerId: 2}, nil
}

func (m *readPrivacyMessages) MessageGetOutboxReadDate(context.Context, *message.TLMessageGetOutboxReadDate) (*message.Vector_ReadParticipantDate, error) {
	m.lookups++
	return &message.Vector_ReadParticipantDate{Datas: []*mtproto.ReadParticipantDate{{Date: m.date}}}, nil
}
