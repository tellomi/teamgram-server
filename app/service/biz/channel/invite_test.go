package channel

import (
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/brand"
)

func TestInviteLinkFamily(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "Weekend")
	in := inCh(ch)

	primary, err := svc.ExportInvite(ctx, 1, in, ExportRequest{LegacyRevokePermanent: true})
	if err != nil {
		t.Fatal(err)
	}
	if !primary.GetPermanent() || primary.GetRevoked() || !strings.HasPrefix(primary.GetLink(), brand.LinkBase()+"/+") {
		t.Fatalf("primary %+v", primary)
	}
	if strings.Contains(primary.GetLink(), "t.me") {
		t.Fatalf("link host %s", primary.GetLink())
	}
	hash := strings.TrimPrefix(primary.GetLink(), brand.LinkBase()+"/+")
	if len(hash) != 20 || (hash[0] >= '0' && hash[0] <= '9') {
		t.Fatalf("channel hash %q", hash)
	}
	buf := mtproto.NewEncodeBuf(256)
	if err := primary.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}

	full, err := svc.GetFullChannel(ctx, 1, in)
	if err != nil || full.GetFullChat().GetExportedInvite().GetLink() != primary.GetLink() {
		t.Fatalf("full exported_invite %+v %v", full.GetFullChat().GetExportedInvite(), err)
	}

	// The "新链接" screen: name, approval, expiry, and a usage limit.
	expire := int32(time.Now().Unix() + 3600)
	extra, err := svc.ExportInvite(ctx, 1, in, ExportRequest{
		RequestNeeded: true,
		Expire:        expire,
		Usage:         10,
		UsageSet:      true,
		Title:         "周末局",
		TitleSet:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if extra.GetPermanent() || !extra.GetRequestNeeded() || extra.GetTitle().GetValue() != "周末局" ||
		extra.GetExpireDate().GetValue() != expire || extra.GetUsageLimit().GetValue() != 10 {
		t.Fatalf("additional %+v", extra)
	}
	edited, err := svc.EditInvite(ctx, 1, in, EditRequest{
		Link:          extra.GetLink(),
		Expire:        int32ptr(expire + 3600),
		Usage:         int32ptr(25),
		RequestNeeded: boolptr(false),
		Title:         strptr("改过的链接"),
	})
	if err != nil || len(edited) != 1 {
		t.Fatalf("edit %+v %v", edited, err)
	}
	got := edited[0]
	if got.GetTitle().GetValue() != "改过的链接" || got.GetRequestNeeded() || got.GetUsageLimit().GetValue() != 25 || got.GetExpireDate().GetValue() != expire+3600 {
		t.Fatalf("edited %+v", got)
	}

	if _, err := svc.ExportInvite(ctx, 1, in, ExportRequest{LegacyRevokePermanent: true, Title: "x", TitleSet: true}); err == nil || !strings.Contains(err.Error(), "CHAT_INVITE_PERMANENT") {
		t.Fatalf("legacy with title: %v", err)
	}
	if _, err := svc.ExportInvite(ctx, 1, in, ExportRequest{Subscription: true}); err == nil || !strings.Contains(err.Error(), "PAYMENT_UNSUPPORTED") {
		t.Fatalf("subscription: %v", err)
	}
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ExportInvite(ctx, 2, in, ExportRequest{LegacyRevokePermanent: true}); err == nil || !strings.Contains(err.Error(), "CHAT_ADMIN_INVITE_REQUIRED") {
		t.Fatalf("member export: %v", err)
	}

	list, err := svc.ExportedInvites(ctx, 1, in, 1, false)
	if err != nil || len(list) != 2 {
		t.Fatalf("list %d %v", len(list), err)
	}
	one, err := svc.ExportedInvite(ctx, 1, in, extra.GetLink())
	if err != nil || one.GetLink() != extra.GetLink() {
		t.Fatalf("one %+v %v", one, err)
	}
	admins, err := svc.AdminsWithInvites(ctx, 1, in)
	if err != nil || len(admins) != 1 || admins[0].GetAdminId() != 1 || admins[0].GetInvitesCount() != 2 {
		t.Fatalf("admins %+v %v", admins, err)
	}

	preview, ids, err := svc.CheckInvite(ctx, 3, hash)
	if err != nil || preview.GetPredicateName() != mtproto.Predicate_chatInvite || !preview.GetMegagroup() || preview.GetTitle() != "Weekend" {
		t.Fatalf("preview %+v ids %v %v", preview, ids, err)
	}
	if err := preview.Encode(mtproto.NewEncodeBuf(256), 232); err != nil {
		t.Fatal(err)
	}
	joined, err := svc.ImportInvite(ctx, 3, 0, hash)
	if err != nil || joined == nil {
		t.Fatal(err)
	}
	again, _, err := svc.CheckInvite(ctx, 3, hash)
	if err != nil || again.GetPredicateName() != mtproto.Predicate_chatInviteAlready {
		t.Fatalf("already %+v %v", again, err)
	}
	importers, total, err := svc.InviteImporters(ctx, 1, in, false, primary.GetLink(), 0, 0, 50)
	if err != nil || total != 1 || len(importers) != 1 || importers[0].GetUserId() != 3 {
		t.Fatalf("importers %+v total %d %v", importers, total, err)
	}

	ask, err := svc.ExportInvite(ctx, 1, in, ExportRequest{RequestNeeded: true, Title: "审核", TitleSet: true})
	if err != nil {
		t.Fatal(err)
	}
	askHash := strings.TrimPrefix(ask.GetLink(), brand.LinkBase()+"/+")
	if _, err := svc.ImportInvite(ctx, 4, 0, askHash); err == nil || !strings.Contains(err.Error(), "INVITE_REQUEST_SENT") {
		t.Fatalf("request: %v", err)
	}
	pending, _, err := svc.InviteImporters(ctx, 1, in, true, ask.GetLink(), 0, 0, 10)
	if err != nil || len(pending) != 1 || pending[0].GetUserId() != 4 {
		t.Fatalf("pending %+v %v", pending, err)
	}
	if _, err := svc.HideJoinRequest(ctx, 1, 0, in, 4, true); err != nil {
		t.Fatal(err)
	}
	member, err := svc.GetParticipant(ctx, 1, in, mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 4}).To_InputPeer())
	if err != nil || member.GetParticipant().GetUserId() != 4 {
		t.Fatalf("approved %+v %v", member, err)
	}
	if _, err := svc.HideAllJoins(ctx, 1, 0, in, false, ""); err != nil {
		t.Fatal(err)
	}

	revoked, err := svc.EditInvite(ctx, 1, in, EditRequest{Link: primary.GetLink(), Revoked: true})
	if err != nil || len(revoked) != 2 || !revoked[0].GetRevoked() || !revoked[1].GetPermanent() || revoked[1].GetRevoked() {
		t.Fatalf("revoke primary %+v %v", revoked, err)
	}
	full, err = svc.GetFullChannel(ctx, 1, in)
	if err != nil || full.GetFullChat().GetExportedInvite().GetLink() != revoked[1].GetLink() {
		t.Fatalf("replaced primary %+v %v", full.GetFullChat().GetExportedInvite(), err)
	}
	if err := svc.DeleteInvite(ctx, 1, in, extra.GetLink()); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteRevokedInvites(ctx, 1, in, 1); err != nil {
		t.Fatal(err)
	}
	revokedList, err := svc.ExportedInvites(ctx, 1, in, 1, true)
	if err != nil || len(revokedList) != 0 {
		t.Fatalf("revoked left %+v %v", revokedList, err)
	}

	limited, err := svc.ExportInvite(ctx, 1, in, ExportRequest{Usage: 1, UsageSet: true})
	if err != nil {
		t.Fatal(err)
	}
	limHash := strings.TrimPrefix(limited.GetLink(), brand.LinkBase()+"/+")
	if _, err := svc.ImportInvite(ctx, 5, 0, limHash); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ImportInvite(ctx, 6, 0, limHash); err == nil || !strings.Contains(err.Error(), "INVITE_HASH_EXPIRED") {
		t.Fatalf("usage: %v", err)
	}

	news, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Feed", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ExportInvite(ctx, 1, inCh(news.Chats[0]), ExportRequest{LegacyRevokePermanent: true}); err != nil {
		t.Fatal(err)
	}
}

func int32ptr(v int32) *int32 { return &v }
func boolptr(v bool) *bool    { return &v }
func strptr(v string) *string { return &v }
