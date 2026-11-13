package core

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/chatinvites/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/chatinvites/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/pkg/brand"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestNewInviteLink is the 管理邀请链接 → 新链接 screen: a name, 新成员审核,
// an expiry, and a usage limit. It also covers the share sheet, which calls
// exportChatInvite with legacy_revoke_permanent and then reads exported_invite.
func TestNewInviteLink(t *testing.T) {
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
	c := &ChatInvitesCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{}},
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
	if !strings.HasPrefix(primary.GetLink(), "https://tell.cc/+") || !primary.GetPermanent() {
		t.Fatalf("share sheet link %+v", primary)
	}
	full, err := svcCh.GetFullChannel(ctx, 7, mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  chat.GetId(),
		AccessHash: chat.GetAccessHash_FLAGINT64().GetValue(),
	}).To_InputChannel())
	if err != nil || full.GetFullChat().GetExportedInvite().GetLink() != primary.GetLink() {
		t.Fatalf("exported_invite %+v %v", full.GetFullChat().GetExportedInvite(), err)
	}

	expire := int32(time.Now().Unix() + 7200)
	created, err := c.MessagesExportChatInvite(&mtproto.TLMessagesExportChatInvite{
		RequestNeeded: true,
		Peer:          peer,
		ExpireDate:    &wrapperspb.Int32Value{Value: expire},
		UsageLimit:    &wrapperspb.Int32Value{Value: 10},
		Title:         &wrapperspb.StringValue{Value: "周末局"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.GetPermanent() || created.GetTitle().GetValue() != "周末局" || !created.GetRequestNeeded() ||
		created.GetExpireDate().GetValue() != expire || created.GetUsageLimit().GetValue() != 10 {
		t.Fatalf("new link %+v", created)
	}
	if !strings.HasPrefix(created.GetLink(), brand.LinkBase()+"/+") {
		t.Fatalf("host %s", created.GetLink())
	}

	edited, err := c.MessagesEditExportedChatInvite(&mtproto.TLMessagesEditExportedChatInvite{
		Peer:          peer,
		Link:          created.GetLink(),
		ExpireDate:    &wrapperspb.Int32Value{Value: expire + 60},
		UsageLimit:    &wrapperspb.Int32Value{Value: 3},
		RequestNeeded: mtproto.BoolFalse,
		Title:         &wrapperspb.StringValue{Value: "改名"},
	})
	if err != nil {
		t.Fatal(err)
	}
	inv := edited.GetInvite()
	if inv.GetTitle().GetValue() != "改名" || inv.GetRequestNeeded() || inv.GetUsageLimit().GetValue() != 3 || inv.GetExpireDate().GetValue() != expire+60 {
		t.Fatalf("edited %+v", inv)
	}

	listed, err := c.MessagesGetExportedChatInvites(&mtproto.TLMessagesGetExportedChatInvites{
		Peer:    peer,
		AdminId: mtproto.MakeTLInputUserSelf(nil).To_InputUser(),
		Limit:   20,
	})
	if err != nil || listed.GetCount() != 2 || len(listed.GetInvites()) != 2 {
		t.Fatalf("list %+v %v", listed, err)
	}
	one, err := c.MessagesGetExportedChatInvite(&mtproto.TLMessagesGetExportedChatInvite{
		Peer: peer,
		Link: created.GetLink(),
	})
	if err != nil || one.GetInvite().GetLink() != created.GetLink() {
		t.Fatalf("get one %+v %v", one, err)
	}
	admins, err := c.MessagesGetAdminsWithInvites(&mtproto.TLMessagesGetAdminsWithInvites{Peer: peer})
	if err != nil || len(admins.GetAdmins()) != 1 || admins.GetAdmins()[0].GetInvitesCount() != 2 {
		t.Fatalf("admins %+v %v", admins, err)
	}

	hash := strings.TrimPrefix(primary.GetLink(), brand.LinkBase()+"/+")
	c2 := *c
	c2.MD = &metadata.RpcMetadata{UserId: 8}
	preview, err := c2.MessagesCheckChatInvite(&mtproto.TLMessagesCheckChatInvite{Hash: hash})
	if err != nil || preview.GetPredicateName() != mtproto.Predicate_chatInvite || preview.GetTitle() != "群" {
		t.Fatalf("check %+v %v", preview, err)
	}
	updates, err := c2.MessagesImportChatInviteDE91436E(&mtproto.TLMessagesImportChatInviteDE91436E{Hash: hash})
	if err != nil || updates.GetUpdates() == nil {
		t.Fatalf("import %+v %v", updates, err)
	}
	importers, err := c.MessagesGetChatInviteImporters(&mtproto.TLMessagesGetChatInviteImporters{
		Peer:       peer,
		Link:       &wrapperspb.StringValue{Value: primary.GetLink()},
		OffsetUser: mtproto.MakeTLInputUserEmpty(nil).To_InputUser(),
		Limit:      10,
	})
	if err != nil || importers.GetCount() != 1 || importers.GetImporters()[0].GetUserId() != 8 {
		t.Fatalf("importers %+v %v", importers, err)
	}

	if _, err := c.MessagesDeleteExportedChatInvite(&mtproto.TLMessagesDeleteExportedChatInvite{
		Peer: peer,
		Link: created.GetLink(),
	}); err != nil {
		t.Fatal(err)
	}
	replaced, err := c.MessagesEditExportedChatInvite(&mtproto.TLMessagesEditExportedChatInvite{
		Revoked: true,
		Peer:    peer,
		Link:    primary.GetLink(),
	})
	if err != nil || replaced.GetNewInvite() == nil || !replaced.GetInvite().GetRevoked() {
		t.Fatalf("revoke %+v %v", replaced, err)
	}
	if _, err := c.MessagesDeleteRevokedExportedChatInvites(&mtproto.TLMessagesDeleteRevokedExportedChatInvites{
		Peer:    peer,
		AdminId: mtproto.MakeTLInputUserSelf(nil).To_InputUser(),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAndroidUnlimitedUsageLimit(t *testing.T) {
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
	c := &ChatInvitesCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 7},
	}

	// LinkEditActivity sets flags bit 1 and sends usage_limit=0 for no limit.
	created, err := c.MessagesExportChatInvite(&mtproto.TLMessagesExportChatInvite{
		Peer:       peer,
		UsageLimit: &wrapperspb.Int32Value{Value: 0},
		Title:      &wrapperspb.StringValue{Value: "不限"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.GetUsageLimit() != nil || created.GetTitle().GetValue() != "不限" {
		t.Fatalf("export %+v", created)
	}

	limited, err := c.MessagesExportChatInvite(&mtproto.TLMessagesExportChatInvite{
		Peer:       peer,
		UsageLimit: &wrapperspb.Int32Value{Value: 2},
	})
	if err != nil || limited.GetUsageLimit().GetValue() != 2 {
		t.Fatalf("limited %+v %v", limited, err)
	}
	edited, err := c.MessagesEditExportedChatInvite(&mtproto.TLMessagesEditExportedChatInvite{
		Peer:       peer,
		Link:       limited.GetLink(),
		UsageLimit: &wrapperspb.Int32Value{Value: 0},
	})
	if err != nil || edited.GetInvite().GetUsageLimit() != nil {
		t.Fatalf("edit to unlimited %+v %v", edited, err)
	}
	if _, err := c.MessagesExportChatInvite(&mtproto.TLMessagesExportChatInvite{
		Peer:       peer,
		UsageLimit: &wrapperspb.Int32Value{Value: -1},
	}); err == nil || !strings.Contains(err.Error(), "USAGE_LIMIT_INVALID") {
		t.Fatalf("negative: %v", err)
	}
	if _, err := c.MessagesEditExportedChatInvite(&mtproto.TLMessagesEditExportedChatInvite{
		Peer:       peer,
		Link:       created.GetLink(),
		UsageLimit: &wrapperspb.Int32Value{Value: 100000},
	}); err == nil || !strings.Contains(err.Error(), "USAGE_LIMIT_INVALID") {
		t.Fatalf("over max: %v", err)
	}
}
