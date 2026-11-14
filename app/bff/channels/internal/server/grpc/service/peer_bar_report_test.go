package service

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func TestReportClearsUserPeerBar(t *testing.T) {
	users := &barClearUser{}
	s := ReportsService{Users: users}
	ctx := context.Background()
	s.clearUserBar(ctx, 7, &mtproto.InputPeer{
		PredicateName: mtproto.Predicate_inputPeerUser,
		UserId:        20,
	})
	if users.deleted == nil || users.deleted.GetUserId() != 7 || users.deleted.GetPeerId() != 20 || users.deleted.GetPeerType() != mtproto.PEER_USER {
		t.Fatalf("deleted %#v", users.deleted)
	}
	users.deleted = nil
	s.clearUserBar(ctx, 7, &mtproto.InputPeer{
		PredicateName: mtproto.Predicate_inputPeerChannel,
		ChannelId:     4,
	})
	if users.deleted != nil {
		t.Fatalf("channel report cleared a user bar %#v", users.deleted)
	}
	(ReportsService{}).clearUserBar(ctx, 7, &mtproto.InputPeer{
		PredicateName: mtproto.Predicate_inputPeerUser,
		UserId:        20,
	})
}

type barClearUser struct {
	userclient.UserClient
	deleted *user.TLUserDeletePeerSettings
}

func (u *barClearUser) UserDeletePeerSettings(_ context.Context, in *user.TLUserDeletePeerSettings) (*mtproto.Bool, error) {
	u.deleted = in
	return mtproto.BoolTrue, nil
}
