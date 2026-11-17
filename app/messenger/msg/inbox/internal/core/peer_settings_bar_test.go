package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/internal/svc"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dao"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestStrangerBarWanted(t *testing.T) {
	if !strangerBarWanted(20, false, false) {
		t.Fatal("stranger")
	}
	for _, from := range []int64{0, 777000, 42777, 424000} {
		if strangerBarWanted(from, false, false) {
			t.Fatalf("service user %d", from)
		}
	}
	if strangerBarWanted(20, true, false) || strangerBarWanted(20, false, true) {
		t.Fatal("contact or bot")
	}
	flags := strangerPeerSettings()
	if !flags.GetReportSpam() || !flags.GetAddContact() || !flags.GetBlockContact() {
		t.Fatalf("flags %#v", flags)
	}
}

func TestOpenStrangerBarWritesRecipientSettings(t *testing.T) {
	users := &barUser{}
	c := newBarCore(users)
	got := c.openStrangerBar(7, 20, mtproto.PEER_USER, false)
	if got == nil || !got.GetReportSpam() || !got.GetAddContact() || !got.GetBlockContact() {
		t.Fatalf("settings %#v", got)
	}
	if users.added == nil || users.added.GetUserId() != 7 || users.added.GetPeerId() != 20 || users.added.GetPeerType() != mtproto.PEER_USER {
		t.Fatalf("stored %#v", users.added)
	}
	if users.botChecks != 1 || users.contactChecks != 1 {
		t.Fatalf("bot %d contact %d", users.botChecks, users.contactChecks)
	}

	users.added = nil
	if c.openStrangerBar(7, 20, mtproto.PEER_USER, true) != nil || users.added != nil {
		t.Fatal("existing dialog reopened the bar")
	}
	if c.openStrangerBar(7, 20, mtproto.PEER_CHAT, false) != nil {
		t.Fatal("group message opened a user bar")
	}
	if c.openStrangerBar(7, 777000, mtproto.PEER_USER, false) != nil || users.botChecks != 1 {
		t.Fatal("service user was queried")
	}
	users.contact = true
	if c.openStrangerBar(7, 20, mtproto.PEER_USER, false) != nil || users.added != nil {
		t.Fatal("contact opened the bar")
	}
	users.contact = false
	users.bot = true
	if c.openStrangerBar(7, 20, mtproto.PEER_USER, false) != nil || users.added != nil {
		t.Fatal("bot opened the bar")
	}
}

func newBarCore(users userclient.UserClient) *InboxCore {
	ctx := context.Background()
	return &InboxCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: users}},
		Logger: logx.WithContext(ctx),
	}
}

type barUser struct {
	userclient.UserClient
	contact       bool
	bot           bool
	added         *user.TLUserAddPeerSettings
	botChecks     int
	contactChecks int
}

func (u *barUser) UserIsBot(context.Context, *user.TLUserIsBot) (*mtproto.Bool, error) {
	u.botChecks++
	return mtproto.ToBool(u.bot), nil
}

func (u *barUser) UserCheckContact(context.Context, *user.TLUserCheckContact) (*mtproto.Bool, error) {
	u.contactChecks++
	return mtproto.ToBool(u.contact), nil
}

func (u *barUser) UserAddPeerSettings(_ context.Context, in *user.TLUserAddPeerSettings) (*mtproto.Bool, error) {
	u.added = in
	return mtproto.BoolTrue, nil
}
