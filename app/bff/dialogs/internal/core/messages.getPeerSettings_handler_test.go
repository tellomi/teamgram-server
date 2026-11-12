package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/dialogs/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/dialogs/internal/svc"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

// A missing peer-settings row is already an empty object from the user service.
// A store or RPC failure must not look like that object: the client would hide
// the add-contact and report-spam bar.
func TestMessagesGetPeerSettingsReturnsStoreError(t *testing.T) {
	storeErr := errors.New("peer settings store failed")
	users := &peerSettingsUser{err: storeErr}
	c := newPeerSettingsCore(users)

	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 20, AccessHash: 1}).To_InputPeer()
	got, err := c.MessagesGetPeerSettings(&mtproto.TLMessagesGetPeerSettings{Peer: peer})
	if err == nil || got != nil {
		t.Fatalf("store failure was reported as peer settings: %v %v", got, err)
	}
	if !errors.Is(err, storeErr) {
		t.Fatalf("error: %v", err)
	}
	if users.calls != 1 || users.userID != 42 || users.peerType != mtproto.PEER_USER || users.peerID != 20 {
		t.Fatalf("query user %d peer %d/%d calls %d", users.userID, users.peerType, users.peerID, users.calls)
	}

	users.err = nil
	users.settings = mtproto.MakeTLPeerSettings(&mtproto.PeerSettings{
		ReportSpam: true,
		AddContact: true,
	}).To_PeerSettings()
	got, err = c.MessagesGetPeerSettings(&mtproto.TLMessagesGetPeerSettings{Peer: peer})
	if err != nil || got == nil || got.GetSettings() == nil {
		t.Fatalf("saved settings: %v %v", got, err)
	}
	if !got.GetSettings().GetReportSpam() || !got.GetSettings().GetAddContact() {
		t.Fatalf("flags were cleared: %#v", got.GetSettings())
	}
	if len(got.GetChats()) != 0 || len(got.GetUsers()) != 0 {
		t.Fatalf("chats %#v users %#v", got.GetChats(), got.GetUsers())
	}

	users.settings = mtproto.MakeTLPeerSettings(&mtproto.PeerSettings{}).To_PeerSettings()
	got, err = c.MessagesGetPeerSettings(&mtproto.TLMessagesGetPeerSettings{Peer: peer})
	if err != nil || got == nil || got.GetSettings() == nil {
		t.Fatalf("missing row: %v %v", got, err)
	}
	if got.GetSettings().GetReportSpam() || got.GetSettings().GetAddContact() {
		t.Fatalf("missing row should stay empty, got %#v", got.GetSettings())
	}
	if users.calls != 3 {
		t.Fatalf("calls %d", users.calls)
	}
}

func newPeerSettingsCore(users user_client.UserClient) *DialogsCore {
	ctx := context.Background()
	return &DialogsCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: users}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}
}

type peerSettingsUser struct {
	user_client.UserClient
	err      error
	settings *mtproto.PeerSettings
	userID   int64
	peerType int32
	peerID   int64
	calls    int
}

func (u *peerSettingsUser) UserGetPeerSettings(_ context.Context, in *user.TLUserGetPeerSettings) (*mtproto.PeerSettings, error) {
	u.calls++
	u.userID = in.GetUserId()
	u.peerType = in.GetPeerType()
	u.peerID = in.GetPeerId()
	if u.err != nil {
		return nil, u.err
	}
	return u.settings, nil
}
