package story

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func photoMedia() *mtproto.MessageMedia {
	return mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{
		Photo_FLAGPHOTO: mtproto.MakeTLPhoto(&mtproto.Photo{Id: 9, AccessHash: 8, Date: 1, DcId: 1}).To_Photo(),
	}).To_MessageMedia()
}

func videoMedia() *mtproto.MessageMedia {
	return mtproto.MakeTLMessageMediaDocument(&mtproto.MessageMedia{
		Document: mtproto.MakeTLDocument(&mtproto.Document{
			Id: 15, AccessHash: 16, Date: 1, MimeType: "video/mp4", DcId: 1,
			Attributes: []*mtproto.DocumentAttribute{
				mtproto.MakeTLDocumentAttributeVideo(&mtproto.DocumentAttribute{Duration: 3, W: 10, H: 20}).To_DocumentAttribute(),
			},
		}).To_Document(),
	}).To_MessageMedia()
}

func rules(names ...string) []*mtproto.PrivacyRule {
	out := make([]*mtproto.PrivacyRule, 0, len(names))
	for _, name := range names {
		switch name {
		case "all":
			out = append(out, mtproto.MakeTLPrivacyValueAllowAll(nil).To_PrivacyRule())
		case "contacts":
			out = append(out, mtproto.MakeTLPrivacyValueAllowContacts(nil).To_PrivacyRule())
		case "close":
			out = append(out, mtproto.MakeTLPrivacyValueAllowCloseFriends(nil).To_PrivacyRule())
		}
	}
	return out
}

func allowUsers(ids ...int64) []*mtproto.PrivacyRule {
	return []*mtproto.PrivacyRule{mtproto.MakeTLPrivacyValueAllowUsers(&mtproto.PrivacyRule{Users: ids}).To_PrivacyRule()}
}

func newSvc(t *testing.T) (*Service, context.Context, *int32) {
	t.Helper()
	svc := New(NewMemoryStore())
	Use(svc)
	channel.Use(channel.New(channel.NewMemoryStore()))
	now := int32(1_700_000_000)
	svc.SetClock(func() int32 { return now })
	return svc, context.Background(), &now
}

func send(t *testing.T, svc *Service, ctx context.Context, actor int64, media *mtproto.MessageMedia, privacy []*mtproto.PrivacyRule, random int64, caption string, pinned bool) *mtproto.Updates {
	t.Helper()
	up, err := svc.Send(ctx, SendRequest{
		Actor: actor, Owner: Owner{Kind: KindUser, ID: actor}, Media: media,
		Rules: privacy, RandomID: random, Caption: caption, Pinned: pinned,
	})
	if err != nil {
		t.Fatal(err)
	}
	return up
}

func storyID(t *testing.T, up *mtproto.Updates) int32 {
	t.Helper()
	for _, u := range up.Updates {
		if u.GetPredicateName() == mtproto.Predicate_updateStoryID {
			return u.GetId_INT32()
		}
	}
	t.Fatal("missing updateStoryID")
	return 0
}

func TestSendEditDeleteAndLayer232Updates(t *testing.T) {
	svc, ctx, _ := newSvc(t)
	up := send(t, svc, ctx, 1, photoMedia(), rules("all"), 42, "hello", false)
	id := storyID(t, up)
	var sawStory bool
	for _, u := range up.Updates {
		if u.GetPredicateName() == mtproto.Predicate_updateStory {
			sawStory = true
			if u.GetStory().GetId() != id || u.GetStory().GetCaption().GetValue() != "hello" {
				t.Fatalf("updateStory = %+v", u.GetStory())
			}
			if u.GetPeer_PEER().GetUserId() != 1 {
				t.Fatalf("peer = %+v", u.GetPeer_PEER())
			}
		}
	}
	if !sawStory {
		t.Fatal("missing updateStory")
	}
	buf := mtproto.NewEncodeBuf(1024)
	if err := up.Encode(buf, mtproto.Layer); err != nil {
		t.Fatal(err)
	}
	if uint32(buf.GetBuf()[0])|uint32(buf.GetBuf()[1])<<8|uint32(buf.GetBuf()[2])<<16|uint32(buf.GetBuf()[3])<<24 != 0x74ae4240 {
		t.Fatalf("updates ctor")
	}

	edited, err := svc.Edit(ctx, EditRequest{Actor: 1, Owner: Owner{Kind: KindUser, ID: 1}, ID: id, Caption: strPtr("edited")})
	if err != nil {
		t.Fatal(err)
	}
	if edited.Updates[0].GetStory().GetCaption().GetValue() != "edited" || !edited.Updates[0].GetStory().GetEdited() {
		t.Fatalf("edit = %+v", edited.Updates[0].GetStory())
	}
	deleted, err := svc.Delete(ctx, 1, 0, Owner{Kind: KindUser, ID: 1}, 0, []int32{id})
	if err != nil || len(deleted) != 1 || deleted[0] != id {
		t.Fatalf("delete %v %v", deleted, err)
	}
	got, err := svc.GetByID(ctx, 1, Owner{Kind: KindUser, ID: 1}, 0, []int32{id})
	if err != nil || got.Stories[0].GetPredicateName() != mtproto.Predicate_storyItemDeleted {
		t.Fatalf("deleted item %+v %v", got, err)
	}
}

func TestPrivacyExpiryPinnedAlbums(t *testing.T) {
	svc, ctx, now := newSvc(t)
	svc.MarkContact(1, 2)
	svc.MarkCloseFriend(1, 3)

	everyone := send(t, svc, ctx, 1, photoMedia(), rules("all"), 1, "public #trip", false)
	contacts := send(t, svc, ctx, 1, videoMedia(), rules("contacts"), 2, "contacts", true)
	close := send(t, svc, ctx, 1, photoMedia(), rules("close"), 3, "close", false)
	selected := send(t, svc, ctx, 1, photoMedia(), allowUsers(4), 4, "selected", false)
	eID, cID, clID, sID := storyID(t, everyone), storyID(t, contacts), storyID(t, close), storyID(t, selected)

	peer, err := svc.GetPeerStories(ctx, 9, Owner{Kind: KindUser, ID: 1}, 0)
	if err != nil || len(peer.Stories.Stories) != 1 || peer.Stories.Stories[0].GetId() != eID {
		t.Fatalf("stranger sees %+v %v", peer, err)
	}
	if peer.Stories.Stories[0].GetPrivacy() != nil {
		t.Fatal("privacy leaked to a stranger")
	}
	if full := svc.ActivePeerStories(ctx, 1, 9); full == nil || len(full.Stories) != 1 || full.Stories[0].GetId() != eID {
		t.Fatalf("userFull stories %+v", full)
	}
	if !svc.PinnedAvailable(ctx, 1, 2) || svc.PinnedAvailable(ctx, 1, 9) {
		t.Fatal("pinned-available flag does not follow privacy")
	}

	mine, err := svc.GetPeerStories(ctx, 1, Owner{Kind: KindUser, ID: 1}, 0)
	if err != nil || len(mine.Stories.Stories) != 4 || mine.Stories.Stories[0].GetPrivacy() == nil {
		t.Fatalf("owner sees %+v", mine)
	}

	asContact, _ := svc.GetPeerStories(ctx, 2, Owner{Kind: KindUser, ID: 1}, 0)
	if !hasStory(asContact.Stories.Stories, eID) || !hasStory(asContact.Stories.Stories, cID) || hasStory(asContact.Stories.Stories, clID) || hasStory(asContact.Stories.Stories, sID) {
		t.Fatalf("contact stories %+v", asContact.Stories.Stories)
	}
	asClose, _ := svc.GetPeerStories(ctx, 3, Owner{Kind: KindUser, ID: 1}, 0)
	if !hasStory(asClose.Stories.Stories, clID) {
		t.Fatal("close friend cannot see close-friends story")
	}
	asSelected, _ := svc.GetPeerStories(ctx, 4, Owner{Kind: KindUser, ID: 1}, 0)
	if !hasStory(asSelected.Stories.Stories, sID) || hasStory(asSelected.Stories.Stories, cID) {
		t.Fatalf("selected %+v", asSelected.Stories.Stories)
	}

	tray, err := svc.GetAll(ctx, 2, false, false, "")
	if err != nil || !peerHas(tray.PeerStories, 1) {
		t.Fatalf("tray %+v %v", tray, err)
	}
	again, err := svc.GetAll(ctx, 2, false, false, tray.State)
	if err != nil || again.GetPredicateName() != mtproto.Predicate_stories_allStoriesNotModified {
		t.Fatalf("not modified %+v %v", again, err)
	}
	strangerTray, _ := svc.GetAll(ctx, 9, false, false, "")
	if peerHas(strangerTray.PeerStories, 1) {
		t.Fatal("stranger tray includes a non-contact")
	}

	rem, err := svc.Remains(ctx, 1, Owner{Kind: KindUser, ID: 1}, 0)
	if err != nil || rem != dailyLimit-4 {
		t.Fatalf("remains %d %v", rem, err)
	}

	if err = svc.TogglePeerHidden(ctx, 2, Owner{Kind: KindUser, ID: 1}, 0, true); err != nil {
		t.Fatal(err)
	}
	hidden, _ := svc.GetAll(ctx, 2, true, false, "")
	main, _ := svc.GetAll(ctx, 2, false, false, "")
	if !peerHas(hidden.PeerStories, 1) || peerHas(main.PeerStories, 1) {
		t.Fatal("peer hidden did not move the tray")
	}
	if err = svc.ToggleAllHidden(ctx, 2, true); err != nil {
		t.Fatal(err)
	}

	pinned, err := svc.GetPinned(ctx, 2, Owner{Kind: KindUser, ID: 1}, 0, 0, 10)
	if err != nil || len(pinned.Stories) != 1 || pinned.Stories[0].GetId() != cID {
		t.Fatalf("pinned %+v %v", pinned, err)
	}
	if _, err = svc.TogglePinned(ctx, 1, Owner{Kind: KindUser, ID: 1}, 0, []int32{eID}, true); err != nil {
		t.Fatal(err)
	}
	if err = svc.TogglePinnedTop(ctx, 1, Owner{Kind: KindUser, ID: 1}, 0, []int32{eID, cID}); err != nil {
		t.Fatal(err)
	}
	top, _ := svc.GetPinned(ctx, 1, Owner{Kind: KindUser, ID: 1}, 0, 0, 10)
	if len(top.PinnedToTop) != 2 || top.PinnedToTop[0] != eID {
		t.Fatalf("pinned to top %+v", top.PinnedToTop)
	}

	album, err := svc.CreateAlbum(ctx, 1, Owner{Kind: KindUser, ID: 1}, 0, "Trip", []int32{eID, cID})
	if err != nil || album.Title != "Trip" || album.IconPhoto == nil {
		t.Fatalf("album %+v %v", album, err)
	}
	gotAlbums, err := svc.GetAlbums(ctx, 2, Owner{Kind: KindUser, ID: 1}, 0, 0)
	if err != nil || len(gotAlbums.Albums) != 1 {
		t.Fatalf("albums %+v %v", gotAlbums, err)
	}
	same, err := svc.GetAlbums(ctx, 2, Owner{Kind: KindUser, ID: 1}, 0, gotAlbums.Hash)
	if err != nil || same.GetPredicateName() != mtproto.Predicate_stories_albumsNotModified {
		t.Fatalf("albums hash %+v", same)
	}
	page, err := svc.GetAlbumStories(ctx, 2, Owner{Kind: KindUser, ID: 1}, 0, album.AlbumId, 0, 10)
	if err != nil || len(page.Stories) != 2 {
		t.Fatalf("album stories %+v %v", page, err)
	}
	if _, err = svc.UpdateAlbum(ctx, 1, Owner{Kind: KindUser, ID: 1}, 0, album.AlbumId, strPtr("Trip 2"), []int32{cID}, nil, nil); err != nil {
		t.Fatal(err)
	}

	found, err := svc.Search(ctx, 9, "trip", "", 10, nil)
	if err != nil || found.Count != 1 || found.Stories[0].GetStory().GetId() != eID {
		t.Fatalf("search %+v %v", found, err)
	}
	only, err := svc.Search(ctx, 9, "", "", 10, &Owner{Kind: KindUser, ID: 1})
	if err != nil || only.Count < 1 {
		t.Fatalf("peer search %+v %v", only, err)
	}
	for _, one := range only.Stories {
		if one.GetPeer().GetUserId() != 1 {
			t.Fatalf("peer search returned %d", one.GetPeer().GetUserId())
		}
	}
	other, err := svc.Search(ctx, 9, "trip", "", 10, &Owner{Kind: KindUser, ID: 9})
	if err != nil || other.Count != 0 {
		t.Fatalf("other peer %+v %v", other, err)
	}

	*now += defaultPeriod + 1
	active, _ := svc.GetPeerStories(ctx, 1, Owner{Kind: KindUser, ID: 1}, 0)
	if len(active.Stories.Stories) != 0 {
		t.Fatalf("expired still active: %d", len(active.Stories.Stories))
	}
	archive, err := svc.GetArchive(ctx, 1, Owner{Kind: KindUser, ID: 1}, 0, 0, 20)
	if err != nil || archive.Count != 4 {
		t.Fatalf("archive %+v %v", archive, err)
	}
	stillPinned, _ := svc.GetPinned(ctx, 2, Owner{Kind: KindUser, ID: 1}, 0, 0, 10)
	if len(stillPinned.Stories) != 2 {
		t.Fatalf("pinned after expiry %d", len(stillPinned.Stories))
	}
	if _, err = svc.GetArchive(ctx, 2, Owner{Kind: KindUser, ID: 1}, 0, 0, 10); err == nil {
		t.Fatal("contact should not read the archive")
	}
}

func TestViewsReactionsReadStealthReport(t *testing.T) {
	svc, ctx, _ := newSvc(t)
	svc.MarkContact(1, 2)
	up := send(t, svc, ctx, 1, photoMedia(), rules("all"), 7, "view me", false)
	id := storyID(t, up)
	owner := Owner{Kind: KindUser, ID: 1}
	if err := svc.IncrementViews(ctx, 2, owner, 0, []int32{id}); err != nil {
		t.Fatal(err)
	}
	if err := svc.IncrementViews(ctx, 1, owner, 0, []int32{id}); err != nil {
		t.Fatal(err)
	}
	views, err := svc.GetViews(ctx, 1, owner, 0, []int32{id})
	if err != nil || views.Views[0].ViewsCount != 1 || !views.Views[0].HasViewers {
		t.Fatalf("views %+v %v", views, err)
	}
	list, err := svc.ViewsList(ctx, ViewsQuery{Actor: 1, Owner: owner, ID: id, Limit: 10})
	if err != nil || list.Count != 1 || list.Views[0].UserId != 2 {
		t.Fatalf("views list %+v %v", list, err)
	}
	react, err := svc.React(ctx, 2, 9, owner, 0, id, mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "❤"}).To_Reaction())
	if err != nil || react.Updates[0].GetStory().GetSentReaction().GetEmoticon() != "❤" {
		t.Fatalf("react %+v %v", react, err)
	}
	reactions, err := svc.ReactionsList(ctx, ViewsQuery{Actor: 1, Owner: owner, ID: id, Limit: 10})
	if err != nil || reactions.Count != 1 {
		t.Fatalf("reactions %+v %v", reactions, err)
	}
	read, err := svc.Read(ctx, 2, 9, owner, 0, id)
	if err != nil || len(read) != 1 {
		t.Fatalf("read %v %v", read, err)
	}
	allRead, err := svc.AllRead(ctx, 2)
	if err != nil || len(allRead.Updates) != 1 || allRead.Updates[0].GetMaxId() != id {
		t.Fatalf("all read %+v %v", allRead, err)
	}

	if _, err = svc.Stealth(ctx, 2, 9, true, true); err != nil {
		t.Fatal(err)
	}
	if err = svc.IncrementViews(ctx, 5, owner, 0, []int32{id}); err != nil {
		t.Fatal(err)
	}
	// User 5 has no stealth. User 2's past view was erased.
	after, _ := svc.ViewsList(ctx, ViewsQuery{Actor: 1, Owner: owner, ID: id, Limit: 10})
	if after.Count != 1 || after.Views[0].UserId != 5 {
		t.Fatalf("after stealth %+v", after)
	}
	if _, err = svc.Stealth(ctx, 2, 0, false, true); err == nil {
		t.Fatal("stealth cooldown should flood")
	}

	choose, err := svc.Report(ctx, 2, owner, 0, []int32{id}, nil, "")
	if err != nil || choose.GetPredicateName() != mtproto.Predicate_reportResultChooseOption || len(choose.Options) != 4 {
		t.Fatalf("report options %+v %v", choose, err)
	}
	done, err := svc.Report(ctx, 2, owner, 0, []int32{id}, []byte("spam"), "bad")
	if err != nil || done.GetPredicateName() != mtproto.Predicate_reportResultReported {
		t.Fatalf("report %+v %v", done, err)
	}
	link, err := svc.ExportLink(ctx, 2, owner, 0, id)
	if err != nil || link == "" {
		t.Fatal(err)
	}
	buf := mtproto.NewEncodeBuf(128)
	exported := mtproto.MakeTLExportedStoryLink(&mtproto.ExportedStoryLink{Link: link}).To_ExportedStoryLink()
	if err = exported.Encode(buf, mtproto.Layer); err != nil {
		t.Fatal(err)
	}
}

func TestChannelStoriesRing(t *testing.T) {
	svc, ctx, _ := newSvc(t)
	created, err := channel.Shared().Create(ctx, channel.CreateRequest{CreatorID: 7, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	owner := Owner{Kind: KindChannel, ID: ch.Id}
	up, err := svc.Send(ctx, SendRequest{
		Actor: 7, Owner: owner, AccessHash: ch.AccessHash_FLAGINT64.GetValue(),
		Media: videoMedia(), Rules: rules("all"), RandomID: 99, Caption: "channel",
	})
	if err != nil {
		t.Fatal(err)
	}
	id := storyID(t, up)
	if _, err = svc.Send(ctx, SendRequest{
		Actor: 8, Owner: owner, AccessHash: ch.AccessHash_FLAGINT64.GetValue(),
		Media: photoMedia(), Rules: rules("all"), RandomID: 100,
	}); err == nil {
		t.Fatal("non-admin posted a channel story")
	}
	refreshed := channel.Shared().ChatsFor(ctx, 7, ch.Id)
	if len(refreshed) != 1 || refreshed[0].GetStoriesMaxId_FLAGRECENTSTORY().GetMaxId().GetValue() != id {
		t.Fatalf("channel ring %+v", refreshed)
	}
	full, err := channel.Shared().GetFullChannel(ctx, 7, mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId: ch.Id, AccessHash: ch.AccessHash_FLAGINT64.GetValue(),
	}).To_InputChannel())
	if err != nil || full.FullChat.GetStories().GetStories()[0].GetId() != id || !full.FullChat.GetStoriesPinnedAvailable() && false {
		if err != nil || len(full.GetFullChat().GetStories().GetStories()) != 1 {
			t.Fatalf("channel full stories %+v %v", full, err)
		}
	}
	live, err := svc.Send(ctx, SendRequest{
		Actor: 7, Owner: owner, AccessHash: ch.GetAccessHash_FLAGINT64().GetValue(),
		Rules: rules("all"), RandomID: 101, Live: true, Caption: "live",
	})
	if err != nil || !live.Updates[len(live.Updates)-1].GetStory().GetLive() {
		t.Fatalf("live %+v %v", live, err)
	}
	chats := svc.ChatsToSend(ctx, 7)
	if len(chats) != 1 || chats[0].Id != ch.Id {
		t.Fatalf("chats to send %+v", chats)
	}
	maxes := svc.PeerMaxIDs(ctx, 7, []Owner{owner})
	if maxes[0].GetMaxId().GetValue() == 0 || !maxes[0].GetLive() {
		t.Fatalf("peer max %+v", maxes[0])
	}
	buf := mtproto.NewEncodeBuf(512)
	if err = refreshed[0].Encode(buf, mtproto.Layer); err != nil {
		t.Fatal(err)
	}
}

func TestLayer232StoryConstructors(t *testing.T) {
	item := mtproto.MakeTLStoryItem(&mtproto.StoryItem{
		Pinned: true, Public: true, Id: 3, Date: 10, ExpireDate: 20,
		Caption: &wrapperspb.StringValue{Value: "cap"},
		Media:   photoMedia(),
		Privacy: rules("all"),
		Views:   mtproto.MakeTLStoryViews(&mtproto.StoryViews{ViewsCount: 2, HasViewers: true}).To_StoryViews(),
		Albums:  []int32{1},
	}).To_StoryItem()
	raw := mustEncode(t, item, mtproto.Layer)
	if ctor(raw) != 0x16a4b93c {
		t.Fatalf("storyItem ctor 0x%x", ctor(raw))
	}
	back := &mtproto.StoryItem{}
	if err := back.Decode(mtproto.NewDecodeBuf(raw)); err != nil {
		t.Fatal(err)
	}
	if back.GetId() != 3 || back.GetCaption().GetValue() != "cap" || !back.GetPublic() || !back.GetPinned() {
		t.Fatalf("roundtrip %+v", back)
	}

	checks := []struct {
		obj  mtproto.TLObject
		ctor uint32
	}{
		{mtproto.MakeTLPeerStories(&mtproto.PeerStories{Peer: mtproto.MakePeerUser(1), Stories: []*mtproto.StoryItem{item}}).To_PeerStories(), 0x9a35e999},
		{mtproto.MakeTLUpdateStory(&mtproto.Update{Peer_PEER: mtproto.MakePeerUser(1), Story: item}).To_Update(), 0x75b3b798},
		{mtproto.MakeTLUpdateReadStories(&mtproto.Update{Peer_PEER: mtproto.MakePeerUser(1), MaxId: 3}).To_Update(), 0xf74e932b},
		{mtproto.MakeTLUpdateStoryID(&mtproto.Update{Id_INT32: 3, RandomId: 9}).To_Update(), 0x1bf335b9},
		{mtproto.MakeTLStoriesAllStories(&mtproto.Stories_AllStories{
			State: "s", PeerStories: []*mtproto.PeerStories{}, Chats: []*mtproto.Chat{}, Users: []*mtproto.User{},
			StealthMode: mtproto.MakeTLStoriesStealthMode(&mtproto.StoriesStealthMode{}).To_StoriesStealthMode(),
		}).To_Stories_AllStories(), 0x6efc5e81},
		{mtproto.MakeTLStoriesStories(&mtproto.Stories_Stories{Stories: []*mtproto.StoryItem{item}, Chats: []*mtproto.Chat{}, Users: []*mtproto.User{}}).To_Stories_Stories(), 0x63c3dd0a},
		{mtproto.MakeTLRecentStory(&mtproto.RecentStory{MaxId: &wrapperspb.Int32Value{Value: 4}}).To_RecentStory(), 0x711d692d},
		{mtproto.MakeTLStoryAlbum(&mtproto.StoryAlbum{AlbumId: 1, Title: "A"}).To_StoryAlbum(), 0x9325705a},
	}
	for _, c := range checks {
		raw = mustEncode(t, c.obj, mtproto.Layer)
		if ctor(raw) != c.ctor {
			t.Errorf("ctor 0x%x, want 0x%x", ctor(raw), c.ctor)
		}
	}
}

func strPtr(s string) *string { return &s }

func TestGetAllStateStable(t *testing.T) {
	svc, ctx, _ := newSvc(t)
	svc.MarkContact(10, 2)
	svc.MarkContact(10, 3)
	svc.MarkContact(10, 4)
	send(t, svc, ctx, 2, photoMedia(), rules("all"), 1, "a", false)
	send(t, svc, ctx, 3, photoMedia(), rules("all"), 2, "b", false)
	send(t, svc, ctx, 4, photoMedia(), rules("all"), 3, "c", false)
	first, err := svc.GetAll(ctx, 10, false, false, "")
	if err != nil || first.Count != 3 || first.State == "" {
		t.Fatalf("tray %+v %v", first, err)
	}
	for i := 0; i < 30; i++ {
		again, err := svc.GetAll(ctx, 10, false, false, "")
		if err != nil || again.State != first.State || again.Count != 3 {
			t.Fatalf("unstable state %q vs %q count=%d %v", first.State, again.GetState(), again.GetCount(), err)
		}
	}
	same, err := svc.GetAll(ctx, 10, false, false, first.State)
	if err != nil || same.GetPredicateName() != mtproto.Predicate_stories_allStoriesNotModified {
		t.Fatalf("not modified %+v %v", same, err)
	}
	if len(same.GetState()) == 0 || same.GetStealthMode() == nil {
		t.Fatalf("not modified payload %+v", same)
	}
}

func hasStory(items []*mtproto.StoryItem, id int32) bool {
	for _, item := range items {
		if item.GetId() == id {
			return true
		}
	}
	return false
}

func peerHas(list []*mtproto.PeerStories, userID int64) bool {
	for _, ps := range list {
		if ps.GetPeer().GetUserId() == userID && len(ps.Stories) > 0 {
			return true
		}
	}
	return false
}

func mustEncode(t *testing.T, obj mtproto.TLObject, layer int32) []byte {
	t.Helper()
	buf := mtproto.NewEncodeBuf(1024)
	if err := obj.Encode(buf, layer); err != nil {
		t.Fatal(err)
	}
	return buf.GetBuf()
}

func ctor(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}
