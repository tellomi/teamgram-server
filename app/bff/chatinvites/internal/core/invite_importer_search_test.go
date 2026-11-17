package core

import (
	"context"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/chatinvites/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/chatinvites/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/brand"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestInviteImporterNameSearch is ISSUE-S-JOINREQ-SEARCH-001 and
// ISSUE-S-INVITE-IMPORTERS-USERS-001. A name query used to drop every row
// because the admin was not loaded with the importers, and the users vector
// came back empty so the client could not build offset_user.
func TestInviteImporterNameSearch(t *testing.T) {
	svcCh := channel.New(channel.NewMemoryStore())
	prev := channel.Shared()
	channel.Use(svcCh)
	t.Cleanup(func() { channel.Use(prev) })

	ctx := context.Background()
	up, err := svcCh.Create(ctx, channel.CreateRequest{CreatorID: 7, Title: "群", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	chat := up.Chats[0]
	peer := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{
		ChannelId:  chat.GetId(),
		AccessHash: chat.GetAccessHash_FLAGINT64().GetValue(),
	}).To_InputPeer()
	users := &importerUsers{}
	c := &ChatInvitesCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			UserClient: users,
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 7},
	}
	primary, err := c.MessagesExportChatInvite(&mtproto.TLMessagesExportChatInvite{
		LegacyRevokePermanent: true,
		Peer:                  peer,
	})
	if err != nil {
		t.Fatal(err)
	}
	hash := strings.TrimPrefix(primary.GetLink(), brand.LinkBase()+"/+")
	joiner := *c
	joiner.MD = &metadata.RpcMetadata{UserId: 8}
	if _, err := joiner.MessagesImportChatInviteDE91436E(&mtproto.TLMessagesImportChatInviteDE91436E{Hash: hash}); err != nil {
		t.Fatal(err)
	}

	// Official search sets q and does not set link. Both together are
	// SEARCH_WITH_LINK_NOT_SUPPORTED.
	found, err := c.MessagesGetChatInviteImporters(&mtproto.TLMessagesGetChatInviteImporters{
		Peer:       peer,
		Q:          &wrapperspb.StringValue{Value: "ada"},
		OffsetUser: mtproto.MakeTLInputUserEmpty(nil).To_InputUser(),
		Limit:      10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if found.GetCount() != 1 || len(found.GetImporters()) != 1 || found.GetImporters()[0].GetUserId() != 8 {
		t.Fatalf("search %+v", found)
	}
	if len(found.GetUsers()) != 1 || found.GetUsers()[0].GetId() != 8 {
		t.Fatalf("users %+v", found.GetUsers())
	}
	if len(users.to) != 1 || users.to[0] != 7 {
		t.Fatalf("viewer was not loaded, to=%v", users.to)
	}

	miss, err := c.MessagesGetChatInviteImporters(&mtproto.TLMessagesGetChatInviteImporters{
		Peer:       peer,
		Q:          &wrapperspb.StringValue{Value: "zzz"},
		OffsetUser: mtproto.MakeTLInputUserEmpty(nil).To_InputUser(),
		Limit:      10,
	})
	if err != nil || miss.GetCount() != 0 || len(miss.GetImporters()) != 0 {
		t.Fatalf("miss %+v %v", miss, err)
	}
}

func TestPageImportersByName(t *testing.T) {
	rows := []*mtproto.ChatInviteImporter{
		mtproto.MakeTLChatInviteImporter(&mtproto.ChatInviteImporter{UserId: 1, Date: 10}).To_ChatInviteImporter(),
		mtproto.MakeTLChatInviteImporter(&mtproto.ChatInviteImporter{UserId: 2, Date: 20}).To_ChatInviteImporter(),
		mtproto.MakeTLChatInviteImporter(&mtproto.ChatInviteImporter{UserId: 3, Date: 30}).To_ChatInviteImporter(),
	}
	page := pageImporters(rows, 10, 1, 1)
	if len(page) != 1 || page[0].GetUserId() != 2 {
		t.Fatalf("page %+v", page)
	}
}

type importerUsers struct {
	userclient.UserClient
	to []int64
}

func (u *importerUsers) UserGetMutableUsers(_ context.Context, in *user.TLUserGetMutableUsers) (*user.Vector_ImmutableUser, error) {
	u.to = append([]int64{}, in.GetTo()...)
	ids := append([]int64{}, in.GetId()...)
	ids = append(ids, in.GetTo()...)
	seen := map[int64]struct{}{}
	rows := make([]*mtproto.ImmutableUser, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		first := "Owner"
		name := "owner"
		if id == 8 {
			first = "Ada"
			name = "ada"
		}
		rows = append(rows, &mtproto.ImmutableUser{User: &mtproto.UserData{
			Id:         id,
			FirstName:  first,
			Username:   name,
			AccessHash: 1,
		}})
	}
	return &user.Vector_ImmutableUser{Datas: rows}, nil
}
