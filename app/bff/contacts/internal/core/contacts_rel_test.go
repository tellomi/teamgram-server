package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/accountrest"
	"github.com/teamgram/teamgram-server/app/bff/contacts/book"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/svc"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func relCore(user int64, saved dao.SavedBook) *ContactsCore {
	b := book.New(nil)
	book.Install(b)
	return &ContactsCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			Book:  b,
			Saved: saved,
		}},
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: user},
	}
}

func TestSavedExportAndReset(t *testing.T) {
	accountrest.Reset()
	mem := &dao.MemSaved{
		Matched:      map[int64][]dao.SavedPhone{4: {{Phone: "8613800138000", FirstName: "Ada", Date: 10}}},
		Unregistered: map[int64][]dao.SavedPhone{4: {{Phone: "8613900139000", FirstName: "Bea", Date: 11}}},
	}
	c := relCore(4, mem)
	if _, err := c.ContactsGetSaved(&mtproto.TLContactsGetSaved{}); status.Convert(err).Message() != "TAKEOUT_REQUIRED" {
		t.Fatal(err)
	}
	opened, err := accountrest.InitTakeoutSession(4, &mtproto.TLAccountInitTakeoutSession{MessageUsers: true})
	if err != nil {
		t.Fatal(err)
	}
	c.MD.Takeout = &metadata.Takeout{Id: opened.GetId()}
	if _, err := c.ContactsGetSaved(&mtproto.TLContactsGetSaved{}); status.Convert(err).Message() != "TAKEOUT_REQUIRED" {
		t.Fatal(err)
	}
	opened, err = accountrest.InitTakeoutSession(4, &mtproto.TLAccountInitTakeoutSession{Contacts: true})
	if err != nil {
		t.Fatal(err)
	}
	c.MD.Takeout = &metadata.Takeout{Id: opened.GetId()}
	got, err := c.ContactsGetSaved(&mtproto.TLContactsGetSaved{})
	if err != nil || len(got.GetDatas()) != 2 || got.GetDatas()[0].GetPhone() != "8613800138000" {
		t.Fatalf("%v %#v", err, got)
	}
	if _, err := c.ContactsResetSaved(&mtproto.TLContactsResetSaved{}); err != nil {
		t.Fatal(err)
	}
	got, err = c.ContactsGetSaved(&mtproto.TLContactsGetSaved{})
	if err != nil || len(got.GetDatas()) != 1 {
		t.Fatalf("after reset %#v %v", got, err)
	}
	if n, err := c.savedCount(); err != nil || n != 0 {
		t.Fatalf("saved_count %d %v", n, err)
	}
}

func TestTopPeersHandler(t *testing.T) {
	c := relCore(4, nil)
	testDialogHits = func() []book.Hit {
		return []book.Hit{{Peer: book.Peer{Type: mtproto.PEER_USER, ID: 8}, Date: 50}}
	}
	t.Cleanup(func() { testDialogHits = nil })
	if _, err := c.ContactsGetTopPeers(&mtproto.TLContactsGetTopPeers{}); status.Convert(err).Message() != "TYPES_EMPTY" {
		t.Fatal(err)
	}
	got, err := c.ContactsGetTopPeers(&mtproto.TLContactsGetTopPeers{Correspondents: true, Limit: 10})
	if err != nil || len(got.GetCategories()) != 1 || got.GetCategories()[0].GetPeers()[0].GetPeer().GetUserId() != 8 {
		t.Fatalf("%v %#v", err, got)
	}
	hash := book.HashRanks(c.relBook().Rank(c.ctx, 4, []string{book.CatCorrespondents}, c.topHits(), 0, 10))
	again, err := c.ContactsGetTopPeers(&mtproto.TLContactsGetTopPeers{Correspondents: true, Limit: 10, Hash: hash})
	if err != nil || again.GetPredicateName() != mtproto.Predicate_contacts_topPeersNotModified {
		t.Fatalf("hash %v %#v", err, again)
	}
	if _, err := c.ContactsToggleTopPeers(&mtproto.TLContactsToggleTopPeers{Enabled: mtproto.BoolFalse}); err != nil {
		t.Fatal(err)
	}
	off, err := c.ContactsGetTopPeers(&mtproto.TLContactsGetTopPeers{Correspondents: true})
	if err != nil || off.GetPredicateName() != mtproto.Predicate_contacts_topPeersDisabled {
		t.Fatalf("disabled %#v %v", off, err)
	}
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 8}).To_InputPeer()
	cat := mtproto.MakeTLTopPeerCategoryCorrespondents(&mtproto.TopPeerCategory{}).To_TopPeerCategory()
	if _, err := c.ContactsToggleTopPeers(&mtproto.TLContactsToggleTopPeers{Enabled: mtproto.BoolTrue}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ContactsResetTopPeerRating(&mtproto.TLContactsResetTopPeerRating{Category: cat, Peer: peer}); err != nil {
		t.Fatal(err)
	}
	cleared, err := c.ContactsGetTopPeers(&mtproto.TLContactsGetTopPeers{Correspondents: true, Limit: 10})
	if err != nil || len(cleared.GetCategories()) != 0 {
		t.Fatalf("reset %#v %v", cleared, err)
	}
	if _, err := c.ContactsResetTopPeerRating(&mtproto.TLContactsResetTopPeerRating{
		Category: cat,
		Peer:     mtproto.MakeTLInputPeerEmpty(&mtproto.InputPeer{}).To_InputPeer(),
	}); status.Convert(err).Message() != "PEER_ID_INVALID" {
		t.Fatal(err)
	}
}

func TestNoteAndStoriesAndLocated(t *testing.T) {
	c := relCore(4, nil)
	testIsContact = func(self, peer int64) (bool, error) { return peer == 8, nil }
	testEnsureUsers = func(ids []int64) error { return nil }
	testMutableUsers = func(_ context.Context, in *userpb.TLUserGetMutableUsersV2) (*mtproto.MutableUsers, error) {
		var users []*mtproto.ImmutableUser
		for _, id := range in.GetId() {
			users = append(users, &mtproto.ImmutableUser{User: &mtproto.UserData{Id: id, FirstName: "Ada"}})
		}
		return &mtproto.MutableUsers{Users: users}, nil
	}
	t.Cleanup(func() {
		testIsContact = nil
		testEnsureUsers = nil
		testMutableUsers = nil
	})
	self := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 4}).To_InputUser()
	other := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 8}).To_InputUser()
	if _, err := c.ContactsUpdateContactNote(&mtproto.TLContactsUpdateContactNote{Id: self, Note: &mtproto.TextWithEntities{Text: "x"}}); status.Convert(err).Message() != "CONTACT_ID_INVALID" {
		t.Fatal(err)
	}
	if _, err := c.ContactsUpdateContactNote(&mtproto.TLContactsUpdateContactNote{Id: other, Note: &mtproto.TextWithEntities{Text: "x"}}); err != nil {
		t.Fatal(err)
	}
	if book.Shared().Note(c.ctx, 4, 8).GetText() != "x" {
		t.Fatal("missing note")
	}
	stranger := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 9}).To_InputUser()
	if _, err := c.ContactsUpdateContactNote(&mtproto.TLContactsUpdateContactNote{Id: stranger, Note: &mtproto.TextWithEntities{Text: "x"}}); status.Convert(err).Message() != "CONTACT_MISSING" {
		t.Fatal(err)
	}

	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 8}).To_InputPeer()
	if _, err := c.ContactsSetBlocked(&mtproto.TLContactsSetBlocked{MyStoriesFrom: true, Id: []*mtproto.InputPeer{peer}, Limit: 1}); err != nil {
		t.Fatal(err)
	}
	if !book.Shared().StoryBlocked(c.ctx, 4, 8) {
		t.Fatal("story block")
	}
	page, err := c.ContactsGetBlocked(&mtproto.TLContactsGetBlocked{MyStoriesFrom: true, Limit: 10})
	if err != nil || len(page.GetBlocked()) != 1 || page.GetBlocked()[0].GetPeerId().GetUserId() != 8 {
		t.Fatalf("%v %#v", err, page)
	}
	if _, err := c.ContactsBlock(&mtproto.TLContactsBlock{MyStoriesFrom: true, Id: peer}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ContactsUnblock(&mtproto.TLContactsUnblock{MyStoriesFrom: true, Id: peer}); err != nil {
		t.Fatal(err)
	}
	if book.Shared().StoryBlocked(c.ctx, 4, 8) {
		t.Fatal("unblocked")
	}

	// Publishing requires a profile photo. The mutable-user hook above returns
	// nothing, so the publish is rejected and a plain lookup still works.
	geo := &mtproto.InputGeoPoint{PredicateName: mtproto.Predicate_inputGeoPoint, Lat: 31.2, Long: 121.5}
	if _, err := c.ContactsGetLocated(&mtproto.TLContactsGetLocated{
		GeoPoint:    geo,
		SelfExpires: wrapperspb.Int32(60),
	}); status.Convert(err).Message() != "USERPIC_UPLOAD_REQUIRED" {
		t.Fatal(err)
	}
	if _, err := c.ContactsGetLocated(&mtproto.TLContactsGetLocated{}); status.Convert(err).Message() != "GEO_POINT_INVALID" {
		t.Fatal(err)
	}
	if err := c.relBook().Publish(c.ctx, 5, 31.21, 121.5, book.ExpiresUnix(nowUnix(), 120)); err != nil {
		t.Fatal(err)
	}
	upd, err := c.ContactsGetLocated(&mtproto.TLContactsGetLocated{GeoPoint: geo})
	if err != nil || len(upd.GetUpdates()) != 1 || len(upd.GetUpdates()[0].GetPeers()) != 1 {
		t.Fatalf("%v %#v", err, upd)
	}
	if upd.GetUpdates()[0].GetPeers()[0].GetDistance() <= 0 {
		t.Fatalf("distance %#v", upd.GetUpdates()[0].GetPeers()[0])
	}
}
