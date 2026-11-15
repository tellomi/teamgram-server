package archive

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type flagUser struct {
	userclient.UserClient
	cur   *mtproto.PeerSettings
	saved *user.TLUserAddPeerSettings
}

func (f *flagUser) UserGetPeerSettings(context.Context, *user.TLUserGetPeerSettings) (*mtproto.PeerSettings, error) {
	return f.cur, nil
}

func (f *flagUser) UserAddPeerSettings(_ context.Context, in *user.TLUserAddPeerSettings) (*mtproto.Bool, error) {
	f.saved = in
	return mtproto.BoolTrue, nil
}

func TestSetAutoarchivedKeepsStrangerBar(t *testing.T) {
	users := &flagUser{cur: &mtproto.PeerSettings{ReportSpam: true, AddContact: true, BlockContact: true}}
	err := (Clients{User: users}).SetAutoarchived(context.Background(), 1, mtproto.PEER_USER, 2, true)
	if err != nil {
		t.Fatal(err)
	}
	got := users.saved.GetSettings()
	if !got.GetAutoarchived() || !got.GetReportSpam() || !got.GetAddContact() || !got.GetBlockContact() {
		t.Fatalf("%#v", got)
	}
}
