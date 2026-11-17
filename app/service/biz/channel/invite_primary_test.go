package channel

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/brand"
)

func TestPrimaryInviteWithoutExport(t *testing.T) {
	svc, ctx := newTest(t)
	for _, kind := range []string{"megagroup", "broadcast"} {
		t.Run(kind, func(t *testing.T) {
			created, err := svc.Create(ctx, CreateRequest{
				CreatorID: 1,
				Title:     kind,
				Megagroup: kind == "megagroup",
				Broadcast: kind == "broadcast",
			})
			if err != nil {
				t.Fatal(err)
			}
			ch := created.Chats[0]
			in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
			link, hash := requirePrimary(t, svc, ctx, 1, in)
			again, err := svc.GetFullChannel(ctx, 1, in)
			if err != nil || again.GetFullChat().GetExportedInvite().GetLink() != link {
				t.Fatalf("second read %+v %v", again.GetFullChat().GetExportedInvite(), err)
			}
			if n := len(activePermanents(t, svc, ctx, ch.Id, 1)); n != 1 {
				t.Fatalf("permanent rows %d", n)
			}
			list, err := svc.ExportedInvites(ctx, 1, in, 1, false)
			if err != nil || len(list) != 1 || list[0].GetLink() != link || !list[0].GetPermanent() {
				t.Fatalf("list %+v %v", list, err)
			}
			preview, _, err := svc.CheckInvite(ctx, 2, hash)
			if err != nil || preview.GetPredicateName() != mtproto.Predicate_chatInvite || preview.GetTitle() != kind {
				t.Fatalf("preview %+v %v", preview, err)
			}
			if kind == "broadcast" && !preview.GetBroadcast() {
				t.Fatalf("broadcast preview %+v", preview)
			}
			if kind == "megagroup" && !preview.GetMegagroup() {
				t.Fatalf("megagroup preview %+v", preview)
			}
			if _, err := svc.ImportInvite(ctx, 2, 0, hash); err != nil {
				t.Fatal(err)
			}
			member, err := svc.GetFullChannel(ctx, 2, in)
			if err != nil {
				t.Fatal(err)
			}
			if member.GetFullChat().GetExportedInvite() != nil {
				t.Fatalf("member exported_invite %+v", member.GetFullChat().GetExportedInvite())
			}
			buf := mtproto.NewEncodeBuf(2048)
			if err := member.GetFullChat().Encode(buf, 232); err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(buf.GetBuf(), []byte(hash)) {
				t.Fatal("member full chat contains the invite hash")
			}
			if _, err := svc.ExportedInvites(ctx, 2, in, 2, false); err == nil || !strings.Contains(err.Error(), "CHAT_ADMIN_INVITE_REQUIRED") {
				t.Fatalf("member list: %v", err)
			}
			if _, err := svc.ExportInvite(ctx, 2, in, ExportRequest{LegacyRevokePermanent: true}); err == nil || !strings.Contains(err.Error(), "CHAT_ADMIN_INVITE_REQUIRED") {
				t.Fatalf("member export: %v", err)
			}
			if n := len(activePermanents(t, svc, ctx, ch.Id, 1)); n != 1 {
				t.Fatalf("member read created a primary: %d", n)
			}
			if len(activePermanents(t, svc, ctx, ch.Id, 2)) != 0 {
				t.Fatal("member received a primary row")
			}
		})
	}
}

func TestPrimaryInviteRecoversEmptyChannel(t *testing.T) {
	st := NewMemoryStore()
	svc := New(st)
	ctx := context.Background()
	ch := &Channel{
		AccessHash:        99,
		CreatorID:         1,
		Title:             "old-empty",
		Megagroup:         true,
		Date:              1,
		DefaultBanned:     defaultChannelBanned(),
		ParticipantsCount: 1,
		Version:           1,
	}
	if err := st.CreateChannel(ctx, ch, &Participant{
		UserID:      1,
		Role:        RoleCreator,
		State:       StateNormal,
		AdminRights: allAdminRights(),
		Date:        1,
	}); err != nil {
		t.Fatal(err)
	}
	in := input(ch.ID, ch.AccessHash)
	revoked, err := svc.ExportedInvites(ctx, 1, in, 1, true)
	if err != nil || len(revoked) != 0 {
		t.Fatalf("revoked list %+v %v", revoked, err)
	}
	if rows, err := st.ListInvites(ctx, ch.ID); err != nil || len(rows) != 0 {
		t.Fatalf("revoked read wrote %+v %v", rows, err)
	}
	link, hash := requirePrimary(t, svc, ctx, 1, in)
	if n := len(activePermanents(t, svc, ctx, ch.ID, 1)); n != 1 {
		t.Fatalf("recovered rows %d", n)
	}
	if err := svc.DeleteInvite(ctx, 1, in, link); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.CheckInvite(ctx, 3, hash); err == nil || !strings.Contains(err.Error(), "INVITE_HASH") {
		t.Fatalf("deleted hash: %v", err)
	}
	fresh, freshHash := requirePrimary(t, svc, ctx, 1, in)
	if fresh == link || freshHash == hash {
		t.Fatalf("recovery reused %s", fresh)
	}
	if n := len(activePermanents(t, svc, ctx, ch.ID, 1)); n != 1 {
		t.Fatalf("after recovery %d", n)
	}
	if _, err := svc.ImportInvite(ctx, 3, 0, freshHash); err != nil {
		t.Fatal(err)
	}
}

func TestPrimaryInviteConcurrent(t *testing.T) {
	st := NewMemoryStore()
	svc := New(st)
	ctx := context.Background()
	ch := &Channel{
		AccessHash:        100,
		CreatorID:         1,
		Title:             "race",
		Megagroup:         true,
		Date:              1,
		DefaultBanned:     defaultChannelBanned(),
		ParticipantsCount: 1,
		Version:           1,
	}
	if err := st.CreateChannel(ctx, ch, &Participant{
		UserID: 1, Role: RoleCreator, State: StateNormal, AdminRights: allAdminRights(), Date: 1,
	}); err != nil {
		t.Fatal(err)
	}
	in := input(ch.ID, ch.AccessHash)
	const n = 24
	var wg sync.WaitGroup
	links := make([]string, n)
	errs := make([]error, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				full, err := svc.GetFullChannel(ctx, 1, in)
				errs[i] = err
				if err == nil {
					links[i] = full.GetFullChat().GetExportedInvite().GetLink()
				}
				return
			}
			list, err := svc.ExportedInvites(ctx, 1, in, 1, false)
			errs[i] = err
			if err == nil && len(list) > 0 {
				for _, inv := range list {
					if inv.GetPermanent() {
						links[i] = inv.GetLink()
					}
				}
			}
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if links[i] == "" || links[i] != links[0] {
			t.Fatalf("link %d %q want %q", i, links[i], links[0])
		}
	}
	if n := len(activePermanents(t, svc, ctx, ch.ID, 1)); n != 1 {
		t.Fatalf("concurrent repair rows %d", n)
	}

	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				_, errs[i] = svc.GetFullChannel(ctx, 1, in)
				return
			}
			_, errs[i] = svc.ExportInvite(ctx, 1, in, ExportRequest{LegacyRevokePermanent: true})
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("revoke race %d: %v", i, err)
		}
	}
	if n := len(activePermanents(t, svc, ctx, ch.ID, 1)); n != 1 {
		t.Fatalf("concurrent revoke rows %d", n)
	}
}

func TestPrimaryInviteKeepsRevokeAndExtra(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "keep", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	primary, _ := requirePrimary(t, svc, ctx, 1, in)
	extra, err := svc.ExportInvite(ctx, 1, in, ExportRequest{Title: "附加", TitleSet: true, Usage: 4, UsageSet: true})
	if err != nil {
		t.Fatal(err)
	}
	if extra.GetPermanent() || extra.GetLink() == primary {
		t.Fatalf("additional %+v", extra)
	}
	still, err := svc.GetFullChannel(ctx, 1, in)
	if err != nil || still.GetFullChat().GetExportedInvite().GetLink() != primary {
		t.Fatalf("primary changed %+v %v", still.GetFullChat().GetExportedInvite(), err)
	}
	replaced, err := svc.ExportInvite(ctx, 1, in, ExportRequest{LegacyRevokePermanent: true})
	if err != nil {
		t.Fatal(err)
	}
	if !replaced.GetPermanent() || replaced.GetLink() == primary {
		t.Fatalf("revoked replacement %+v", replaced)
	}
	full, err := svc.GetFullChannel(ctx, 1, in)
	if err != nil || full.GetFullChat().GetExportedInvite().GetLink() != replaced.GetLink() {
		t.Fatalf("full after revoke %+v %v", full.GetFullChat().GetExportedInvite(), err)
	}
	if n := len(activePermanents(t, svc, ctx, ch.Id, 1)); n != 1 {
		t.Fatalf("active primaries %d", n)
	}
	old := strings.TrimPrefix(primary, brand.LinkBase()+"/+")
	if _, _, err := svc.CheckInvite(ctx, 4, old); err == nil || !strings.Contains(err.Error(), "INVITE_HASH") {
		t.Fatalf("old primary still live: %v", err)
	}
	list, err := svc.ExportedInvites(ctx, 1, in, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	var sawExtra, sawPrimary bool
	for _, inv := range list {
		if inv.GetLink() == extra.GetLink() && !inv.GetPermanent() && inv.GetTitle().GetValue() == "附加" {
			sawExtra = true
		}
		if inv.GetLink() == replaced.GetLink() && inv.GetPermanent() {
			sawPrimary = true
		}
	}
	if !sawExtra || !sawPrimary || len(list) != 2 {
		t.Fatalf("list %+v", list)
	}
	other, err := svc.ExportedInvites(ctx, 1, in, 9, false)
	if err != nil || len(other) != 0 {
		t.Fatalf("listing an admin with no links %+v %v", other, err)
	}
}

func TestInviteAdminGetsOwnPrimary(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "admins", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	creatorLink, _ := requirePrimary(t, svc, ctx, 1, in)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EditAdmin(ctx, 1, 0, in, 2, mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{
		ChangeInfo: true,
	}).To_ChatAdminRights(), ""); err != nil {
		t.Fatal(err)
	}
	limited, err := svc.GetFullChannel(ctx, 2, in)
	if err != nil || limited.GetFullChat().GetExportedInvite() != nil {
		t.Fatalf("admin without invite %+v %v", limited.GetFullChat().GetExportedInvite(), err)
	}
	if _, err := svc.ExportedInvites(ctx, 2, in, 2, false); err == nil || !strings.Contains(err.Error(), "CHAT_ADMIN_INVITE_REQUIRED") {
		t.Fatalf("no-invite list: %v", err)
	}
	if len(activePermanents(t, svc, ctx, ch.Id, 2)) != 0 {
		t.Fatal("admin without invite_users got a row")
	}
	listed, err := svc.ExportedInvites(ctx, 1, in, 2, false)
	if err != nil || len(listed) != 0 {
		t.Fatalf("creator listing the other admin %+v %v", listed, err)
	}
	if len(activePermanents(t, svc, ctx, ch.Id, 2)) != 0 {
		t.Fatal("listing another admin created their primary")
	}
	if _, err := svc.EditAdmin(ctx, 1, 0, in, 2, mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{
		InviteUsers: true,
	}).To_ChatAdminRights(), ""); err != nil {
		t.Fatal(err)
	}
	own, ownHash := requirePrimary(t, svc, ctx, 2, in)
	if own == creatorLink {
		t.Fatal("invite admin was given the creator link")
	}
	creatorStill, err := svc.GetFullChannel(ctx, 1, in)
	if err != nil || creatorStill.GetFullChat().GetExportedInvite().GetLink() != creatorLink {
		t.Fatalf("creator link changed %+v %v", creatorStill.GetFullChat().GetExportedInvite(), err)
	}
	if _, err := svc.EditAdmin(ctx, 1, 0, in, 2, mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{}).To_ChatAdminRights(), ""); err != nil {
		t.Fatal(err)
	}
	demoted, err := svc.GetFullChannel(ctx, 2, in)
	if err != nil || demoted.GetFullChat().GetExportedInvite() != nil {
		t.Fatalf("demoted %+v %v", demoted.GetFullChat().GetExportedInvite(), err)
	}
	if _, _, err := svc.CheckInvite(ctx, 3, ownHash); err == nil || !strings.Contains(err.Error(), "INVITE_HASH") {
		t.Fatalf("demoted link still joins: %v", err)
	}
	creatorHash := strings.TrimPrefix(creatorLink, brand.LinkBase()+"/+")
	if _, err := svc.ImportInvite(ctx, 3, 0, creatorHash); err != nil {
		t.Fatalf("creator link: %v", err)
	}
}

func requirePrimary(t *testing.T, svc *Service, ctx context.Context, user int64, in *mtproto.InputChannel) (string, string) {
	t.Helper()
	full, err := svc.GetFullChannel(ctx, user, in)
	if err != nil {
		t.Fatal(err)
	}
	inv := full.GetFullChat().GetExportedInvite()
	if inv == nil || !inv.GetPermanent() || inv.GetRevoked() || inv.GetRequestNeeded() ||
		inv.GetTitle() != nil || inv.GetExpireDate() != nil || inv.GetUsageLimit() != nil {
		t.Fatalf("primary %+v", inv)
	}
	prefix := brand.LinkBase() + "/+"
	if !strings.HasPrefix(inv.GetLink(), prefix) || strings.Contains(inv.GetLink(), "t.me") {
		t.Fatalf("link %s", inv.GetLink())
	}
	hash := strings.TrimPrefix(inv.GetLink(), prefix)
	if len(hash) != 20 || (hash[0] >= '0' && hash[0] <= '9') {
		t.Fatalf("hash %q", hash)
	}
	buf := mtproto.NewEncodeBuf(256)
	if err := inv.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	return inv.GetLink(), hash
}

func activePermanents(t *testing.T, svc *Service, ctx context.Context, channelID, adminID int64) []*StoredInvite {
	t.Helper()
	rows, err := svc.store.ListInvites(ctx, channelID)
	if err != nil {
		t.Fatal(err)
	}
	var out []*StoredInvite
	for _, row := range rows {
		if row.AdminID == adminID && row.Permanent && !row.Revoked {
			out = append(out, row)
		}
	}
	return out
}
