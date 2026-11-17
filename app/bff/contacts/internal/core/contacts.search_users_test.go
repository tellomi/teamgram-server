package core

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	channels_helper "github.com/teamgram/teamgram-server/app/bff/channels"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

const (
	searchViewer   int64 = 8_803_000_000_001
	searchStranger int64 = 8_803_000_000_002
	searchContact  int64 = 8_803_000_000_003
	searchBot      int64 = 8_803_000_000_005
	searchDeleted  int64 = 8_803_000_000_006
	searchFindUser int64 = 8_803_000_000_007

	searchStrangerHash int64 = 8_803_000_000_102
	searchContactHash  int64 = 8_803_000_000_103
	searchBotHash      int64 = 8_803_000_000_105
	searchFindHash     int64 = 8_803_000_000_107
	searchViewerHash   int64 = 8_803_000_000_101
)

// searchBook returns only the ids user.getMutableUsers was asked for, unioned
// with To, which is what GetCacheImmutableUserListV2 loads. It does not invent
// the viewer. Contacts.search used to ask for the hits alone, and
// GetUserListByIdList then returned no users.
type searchBook struct {
	userclient.UserClient
	mu       sync.Mutex
	byID     map[int64]*mtproto.ImmutableUser
	contacts []*mtproto.ContactData
	lastID   []int64
	lastTo   []int64
}

func newSearchBook() *searchBook {
	note := mtproto.MakeTLContactData(&mtproto.ContactData{
		UserId:        searchViewer,
		ContactUserId: searchContact,
		FirstName:     mtproto.MakeFlagsString("Notebook"),
		LastName:      mtproto.MakeFlagsString("Pal"),
		MutualContact: true,
	}).To_ContactData()
	users := []*mtproto.ImmutableUser{
		searchImm(searchViewer, searchViewerHash, "Caller", "Self", "findmeself", "10000000001", false, false, []*mtproto.ContactData{note}),
		searchImm(searchStranger, searchStrangerHash, "Zhao", "Yang", "zhaoyang", "10000000002", false, false, nil),
		searchImm(searchContact, searchContactHash, "Account", "Zhao", "notebookpal", "10000000003", false, false, nil),
		searchImm(searchBot, searchBotHash, "Find", "Bot", "findmebot", "10000000005", true, false, nil),
		searchImm(searchDeleted, 8_803_000_000_106, "Gone", "User", "findmegone", "10000000006", false, true, nil),
		searchImm(searchFindUser, searchFindHash, "Find", "User", "findmeuser", "10000000007", false, false, nil),
	}
	book := &searchBook{
		byID:     map[int64]*mtproto.ImmutableUser{},
		contacts: []*mtproto.ContactData{note},
	}
	for _, u := range users {
		book.byID[u.Id()] = u
	}
	return book
}

func searchImm(id, hash int64, first, last, username, phone string, bot, deleted bool, contacts []*mtproto.ContactData) *mtproto.ImmutableUser {
	var botData *mtproto.BotData
	if bot {
		botData = &mtproto.BotData{}
	}
	return mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
		User: &mtproto.UserData{
			Id:         id,
			AccessHash: hash,
			FirstName:  first,
			LastName:   last,
			Username:   username,
			Phone:      phone,
			Bot:        botData,
			Deleted:    deleted,
		},
		Contacts: contacts,
		KeysPrivacyRules: []*mtproto.PrivacyKeyRules{{
			Key: int32(mtproto.PHONE_NUMBER),
			Rules: []*mtproto.PrivacyRule{{
				PredicateName: mtproto.Predicate_privacyValueDisallowAll,
			}},
		}},
	}).To_ImmutableUser()
}

func (b *searchBook) UserGetContactList(context.Context, *userpb.TLUserGetContactList) (*userpb.Vector_ContactData, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return &userpb.Vector_ContactData{Datas: append([]*mtproto.ContactData(nil), b.contacts...)}, nil
}

func (b *searchBook) UserSearchUsername(_ context.Context, in *userpb.TLUserSearchUsername) (*userpb.Vector_UsernameData, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	q := strings.ToLower(in.GetQ())
	var ids []int64
	for id, u := range b.byID {
		name := strings.ToLower(u.Username())
		if name == "" || !strings.HasPrefix(name, q) {
			continue
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	datas := make([]*userpb.UsernameData, 0, len(ids))
	for _, id := range ids {
		datas = append(datas, userpb.MakeTLUsernameData(&userpb.UsernameData{
			Username: b.byID[id].Username(),
			Peer:     mtproto.MakePeerUser(id),
		}).To_UsernameData())
	}
	return &userpb.Vector_UsernameData{Datas: datas}, nil
}

func (b *searchBook) UserSearch(_ context.Context, in *userpb.TLUserSearch) (*userpb.UsersFound, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	excluded := map[int64]struct{}{}
	for _, id := range in.GetExcludedContacts() {
		excluded[id] = struct{}{}
	}
	q := strings.ToLower(in.GetQ())
	var ids []int64
	for id, u := range b.byID {
		if u.Deleted() {
			continue
		}
		if _, skip := excluded[id]; skip {
			continue
		}
		if !searchNameLike(q, u.FirstName(), u.LastName(), u.Username()) {
			continue
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return userpb.MakeTLUsersIdFound(&userpb.UsersFound{IdList: ids}).To_UsersFound(), nil
}

func (b *searchBook) UserGetMutableUsers(_ context.Context, in *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastID = append([]int64(nil), in.GetId()...)
	b.lastTo = append([]int64(nil), in.GetTo()...)
	seen := map[int64]struct{}{}
	var datas []*mtproto.ImmutableUser
	for _, id := range append(append([]int64(nil), in.GetId()...), in.GetTo()...) {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if u, ok := b.byID[id]; ok {
			datas = append(datas, u)
		}
	}
	return &userpb.Vector_ImmutableUser{Datas: datas}, nil
}

func searchNameLike(q, first, last, username string) bool {
	if q == "" {
		return false
	}
	if strings.HasPrefix(strings.ToLower(username), q) {
		return true
	}
	full := strings.ToLower(strings.TrimSpace(first + " " + last))
	packed := strings.ReplaceAll(full, " ", "")
	return strings.Contains(strings.ToLower(first), q) ||
		strings.Contains(strings.ToLower(last), q) ||
		strings.Contains(full, q) ||
		strings.Contains(packed, q)
}

func (b *searchBook) loadedViewer() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, id := range append(append([]int64(nil), b.lastID...), b.lastTo...) {
		if id == searchViewer {
			return true
		}
	}
	return false
}

func newSearchCore(book *searchBook) *ContactsCore {
	ctx := context.Background()
	return &ContactsCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{
			Dao:    &dao.Dao{UserClient: book},
			Plugin: channels_helper.Bridge{},
		},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: searchViewer},
	}
}

func refuseProductionDSN(t *testing.T) {
	t.Helper()
	t.Setenv("TEAMGRAM_MYSQL_DSN", "")
}

func useSearchChannels(t *testing.T) (groupID, channelID int64) {
	t.Helper()
	refuseProductionDSN(t)
	prev := channel.Shared()
	svc := channel.New(channel.NewMemoryStore())
	channel.Use(svc)
	t.Cleanup(func() { channel.Use(prev) })
	ctx := context.Background()
	groupID = makeSearchPublic(t, svc, ctx, "findme group", "findmegrp", true)
	channelID = makeSearchPublic(t, svc, ctx, "findme channel", "findmechn", false)
	return groupID, channelID
}

func makeSearchPublic(t *testing.T, svc *channel.Service, ctx context.Context, title, username string, mega bool) int64 {
	t.Helper()
	up, err := svc.Create(ctx, channel.CreateRequest{
		CreatorID: searchViewer,
		Title:     title,
		Broadcast: !mega,
		Megagroup: mega,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(up.Chats) == 0 {
		t.Fatal("create returned no chat")
	}
	ch := up.Chats[0]
	in := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  ch.GetId(),
		AccessHash: ch.GetAccessHash_FLAGINT64().GetValue(),
	}).To_InputChannel()
	if _, err := svc.UpdateUsername(ctx, searchViewer, 0, in, username); err != nil {
		t.Fatal(err)
	}
	return ch.GetId()
}

func runSearch(t *testing.T, c *ContactsCore, q string, broadcasts, bots bool) *mtproto.Contacts_Found {
	t.Helper()
	out, err := c.ContactsSearch(&mtproto.TLContactsSearch{
		Q:          q,
		Limit:      20,
		Broadcasts: broadcasts,
		Bots:       bots,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSearchRefs(t, out)
	if peerListed(out.GetMyResults(), searchViewer) || peerListed(out.GetResults(), searchViewer) || userListed(out.GetUsers(), searchViewer) {
		t.Fatalf("caller listed for %q: my=%v results=%v users=%v", q, peerIDs(out.GetMyResults()), peerIDs(out.GetResults()), userIDs(out.GetUsers()))
	}
	return out
}

func assertSearchRefs(t *testing.T, found *mtproto.Contacts_Found) {
	t.Helper()
	users := map[int64]*mtproto.User{}
	for _, u := range found.GetUsers() {
		if u.GetId() == 0 {
			t.Fatal("user without id")
		}
		if _, ok := users[u.GetId()]; ok {
			t.Fatalf("duplicate user %d", u.GetId())
		}
		users[u.GetId()] = u
	}
	chats := map[int64]*mtproto.Chat{}
	for _, ch := range found.GetChats() {
		if ch.GetId() == 0 {
			t.Fatal("chat without id")
		}
		chats[ch.GetId()] = ch
	}
	referencedUsers := map[int64]struct{}{}
	referencedChats := map[int64]struct{}{}
	check := func(bucket string, peers []*mtproto.Peer) {
		t.Helper()
		for _, p := range peers {
			switch p.GetPredicateName() {
			case mtproto.Predicate_peerUser:
				id := p.GetUserId()
				referencedUsers[id] = struct{}{}
				if _, ok := users[id]; !ok {
					t.Fatalf("%s user %d missing from users", bucket, id)
				}
			case mtproto.Predicate_peerChannel:
				id := p.GetChannelId()
				referencedChats[id] = struct{}{}
				if _, ok := chats[id]; !ok {
					t.Fatalf("%s channel %d missing from chats", bucket, id)
				}
			default:
				t.Fatalf("%s peer %s", bucket, p.GetPredicateName())
			}
		}
	}
	check("my_results", found.GetMyResults())
	check("results", found.GetResults())
	for id := range users {
		if _, ok := referencedUsers[id]; !ok {
			t.Fatalf("user %d is not in my_results or results", id)
		}
	}
	for id := range chats {
		if _, ok := referencedChats[id]; !ok {
			t.Fatalf("chat %d is not in results", id)
		}
	}
}

func peerListed(peers []*mtproto.Peer, id int64) bool {
	for _, p := range peers {
		if p.GetUserId() == id || p.GetChannelId() == id {
			return true
		}
	}
	return false
}

func userListed(users []*mtproto.User, id int64) bool {
	for _, u := range users {
		if u.GetId() == id {
			return true
		}
	}
	return false
}

func peerIDs(peers []*mtproto.Peer) []int64 {
	out := make([]int64, 0, len(peers))
	for _, p := range peers {
		if p.GetPredicateName() == mtproto.Predicate_peerChannel {
			out = append(out, p.GetChannelId())
			continue
		}
		out = append(out, p.GetUserId())
	}
	return out
}

func userIDs(users []*mtproto.User) []int64 {
	out := make([]int64, 0, len(users))
	for _, u := range users {
		out = append(out, u.GetId())
	}
	return out
}

func foundUser(t *testing.T, found *mtproto.Contacts_Found, id int64) *mtproto.User {
	t.Helper()
	for _, u := range found.GetUsers() {
		if u.GetId() == id {
			return u
		}
	}
	t.Fatalf("missing user %d in %v", id, userIDs(found.GetUsers()))
	return nil
}

func assertPhoneHidden(t *testing.T, u *mtproto.User) {
	t.Helper()
	if u.GetPhone().GetValue() != "" {
		t.Fatalf("phone leaked for %d: %q", u.GetId(), u.GetPhone().GetValue())
	}
}

func TestContactsSearchUsersEmptyWithoutViewer(t *testing.T) {
	stranger := searchImm(searchStranger, searchStrangerHash, "Zhao", "Yang", "zhaoyang", "10000000002", false, false, nil)
	hitsOnly := &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{stranger}}
	var ids []int64
	hitsOnly.Visit(func(it *mtproto.ImmutableUser) {
		if it.Deleted() || it.Id() == searchViewer {
			return
		}
		ids = append(ids, it.Id())
	})
	if len(ids) != 1 || ids[0] != searchStranger {
		t.Fatalf("hits = %v", ids)
	}
	if got := hitsOnly.GetUserListByIdList(searchViewer, ids...); len(got) != 0 {
		t.Fatalf("users without viewer = %#v", got)
	}

	viewer := searchImm(searchViewer, searchViewerHash, "Caller", "Self", "findmeself", "10000000001", false, false, nil)
	loaded := &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{viewer, stranger}}
	got := loaded.GetUserListByIdList(searchViewer, ids...)
	if len(got) != 1 || got[0].GetId() != searchStranger {
		t.Fatalf("users with viewer = %v", userIDs(got))
	}
	if userListed(got, searchViewer) {
		t.Fatal("rendering the viewer put the caller into users")
	}
	if got[0].GetAccessHash().GetValue() != searchStrangerHash {
		t.Fatalf("access hash %d", got[0].GetAccessHash().GetValue())
	}
}

func TestSearchUserLoadIDs(t *testing.T) {
	ids, to := searchUserLoadIDs(searchViewer, nil)
	if len(ids) != 0 || len(to) != 0 {
		t.Fatalf("empty hits = %v %v", ids, to)
	}
	ids, to = searchUserLoadIDs(0, []int64{searchStranger})
	if len(ids) != 1 || ids[0] != searchStranger || len(to) != 0 {
		t.Fatalf("no viewer = %v %v", ids, to)
	}
	ids, to = searchUserLoadIDs(searchViewer, []int64{searchStranger})
	if len(ids) != 2 || ids[0] != searchViewer || ids[1] != searchStranger || len(to) != 1 || to[0] != searchViewer {
		t.Fatalf("prepend = %v %v", ids, to)
	}
	ids, to = searchUserLoadIDs(searchViewer, []int64{searchViewer})
	if len(ids) != 1 || ids[0] != searchViewer || len(to) != 1 || to[0] != searchViewer {
		t.Fatalf("already loaded = %v %v", ids, to)
	}
}

func TestContactsSearchUsers(t *testing.T) {
	groupID, channelID := useSearchChannels(t)
	book := newSearchBook()
	c := newSearchCore(book)

	t.Run("at and plain username", func(t *testing.T) {
		for _, q := range []string{"@zhaoyang", "zhaoyang"} {
			out := runSearch(t, c, q, false, false)
			if !book.loadedViewer() {
				t.Fatal("viewer was not loaded")
			}
			if !peerListed(out.GetResults(), searchStranger) || peerListed(out.GetMyResults(), searchStranger) {
				t.Fatalf("%s buckets my=%v results=%v", q, peerIDs(out.GetMyResults()), peerIDs(out.GetResults()))
			}
			u := foundUser(t, out, searchStranger)
			if u.GetAccessHash().GetValue() != searchStrangerHash {
				t.Fatalf("access hash %d", u.GetAccessHash().GetValue())
			}
			if u.GetContact() || u.GetMutualContact() || u.GetBot() {
				t.Fatalf("stranger flags contact=%v mutual=%v bot=%v", u.GetContact(), u.GetMutualContact(), u.GetBot())
			}
			if u.GetFirstName().GetValue() != "Zhao" || u.GetLastName().GetValue() != "Yang" {
				t.Fatalf("name %s %s", u.GetFirstName().GetValue(), u.GetLastName().GetValue())
			}
			assertPhoneHidden(t, u)
		}
	})

	t.Run("contact note", func(t *testing.T) {
		for _, q := range []string{"Notebook Pal", "Pal Notebook"} {
			out := runSearch(t, c, q, false, false)
			if !peerListed(out.GetMyResults(), searchContact) || peerListed(out.GetResults(), searchContact) {
				t.Fatalf("%s my=%v results=%v", q, peerIDs(out.GetMyResults()), peerIDs(out.GetResults()))
			}
			u := foundUser(t, out, searchContact)
			if !u.GetContact() || !u.GetMutualContact() {
				t.Fatalf("contact flags contact=%v mutual=%v", u.GetContact(), u.GetMutualContact())
			}
			if u.GetFirstName().GetValue() != "Notebook" || u.GetLastName().GetValue() != "Pal" {
				t.Fatalf("note name %s %s", u.GetFirstName().GetValue(), u.GetLastName().GetValue())
			}
			if u.GetAccessHash().GetValue() != searchContactHash {
				t.Fatalf("access hash %d", u.GetAccessHash().GetValue())
			}
			assertPhoneHidden(t, u)
		}
	})

	t.Run("multiword account name", func(t *testing.T) {
		out := runSearch(t, c, "Zhao Yang", false, false)
		if !peerListed(out.GetResults(), searchStranger) || peerListed(out.GetMyResults(), searchStranger) {
			t.Fatalf("my=%v results=%v", peerIDs(out.GetMyResults()), peerIDs(out.GetResults()))
		}
		u := foundUser(t, out, searchStranger)
		if u.GetContact() || u.GetFirstName().GetValue() != "Zhao" || u.GetLastName().GetValue() != "Yang" {
			t.Fatalf("rendered %#v", u)
		}
		assertPhoneHidden(t, u)
		miss := runSearch(t, c, "Yang Zhao", false, false)
		if peerListed(miss.GetResults(), searchStranger) || peerListed(miss.GetMyResults(), searchStranger) {
			t.Fatal("reversed account name matched")
		}
	})

	t.Run("unrelated and self", func(t *testing.T) {
		none := runSearch(t, c, "zzzzqqq", false, false)
		if len(none.GetMyResults()) != 0 || len(none.GetResults()) != 0 || len(none.GetUsers()) != 0 || len(none.GetChats()) != 0 {
			t.Fatalf("unrelated %#v", none)
		}
		self := runSearch(t, c, "findmeself", false, false)
		if len(self.GetMyResults()) != 0 || len(self.GetResults()) != 0 || len(self.GetUsers()) != 0 {
			t.Fatalf("self query my=%v results=%v users=%v", peerIDs(self.GetMyResults()), peerIDs(self.GetResults()), userIDs(self.GetUsers()))
		}
		if !book.loadedViewer() {
			t.Fatal("self hit did not load the viewer")
		}
	})

	t.Run("flags bots deleted channels", func(t *testing.T) {
		all := runSearch(t, c, "findme", false, false)
		if !peerListed(all.GetResults(), searchFindUser) || !peerListed(all.GetResults(), searchBot) {
			t.Fatalf("users %v", peerIDs(all.GetResults()))
		}
		if !peerListed(all.GetResults(), groupID) || !peerListed(all.GetResults(), channelID) {
			t.Fatalf("chats %v want %d %d", peerIDs(all.GetResults()), groupID, channelID)
		}
		if peerListed(all.GetResults(), searchDeleted) || userListed(all.GetUsers(), searchDeleted) {
			t.Fatal("deleted user was returned")
		}
		find := foundUser(t, all, searchFindUser)
		bot := foundUser(t, all, searchBot)
		if find.GetBot() || !bot.GetBot() {
			t.Fatalf("bot flags user=%v bot=%v", find.GetBot(), bot.GetBot())
		}
		if find.GetAccessHash().GetValue() != searchFindHash {
			t.Fatalf("find hash %d", find.GetAccessHash().GetValue())
		}
		assertPhoneHidden(t, find)
		assertPhoneHidden(t, bot)
		for _, ch := range all.GetChats() {
			switch ch.GetId() {
			case groupID:
				if !ch.GetMegagroup() {
					t.Fatal("group was not a megagroup")
				}
			case channelID:
				if !ch.GetBroadcast() || ch.GetMegagroup() {
					t.Fatal("channel flags")
				}
			}
		}

		botsOnly := runSearch(t, c, "findme", false, true)
		if !peerListed(botsOnly.GetResults(), searchBot) || peerListed(botsOnly.GetResults(), searchFindUser) || peerListed(botsOnly.GetResults(), groupID) || peerListed(botsOnly.GetResults(), channelID) {
			t.Fatalf("bots %v", peerIDs(botsOnly.GetResults()))
		}
		if !foundUser(t, botsOnly, searchBot).GetBot() {
			t.Fatal("bots flag dropped the bot user")
		}

		channelsOnly := runSearch(t, c, "findme", true, false)
		if !peerListed(channelsOnly.GetResults(), channelID) || peerListed(channelsOnly.GetResults(), groupID) || peerListed(channelsOnly.GetResults(), searchBot) || peerListed(channelsOnly.GetResults(), searchFindUser) {
			t.Fatalf("broadcasts %v", peerIDs(channelsOnly.GetResults()))
		}
		if len(channelsOnly.GetUsers()) != 0 {
			t.Fatalf("broadcasts users %v", userIDs(channelsOnly.GetUsers()))
		}

		both := runSearch(t, c, "findme", true, true)
		if !peerListed(both.GetResults(), searchBot) || !peerListed(both.GetResults(), channelID) || peerListed(both.GetResults(), groupID) || peerListed(both.GetResults(), searchFindUser) {
			t.Fatalf("both %v", peerIDs(both.GetResults()))
		}
	})

	t.Run("short query", func(t *testing.T) {
		_, err := c.ContactsSearch(&mtproto.TLContactsSearch{Q: "ab", Limit: 20})
		if err == nil || !strings.Contains(err.Error(), "QUERY_TOO_SHORT") {
			t.Fatalf("short query err %v", err)
		}
	})
}

func TestContactsSearchUsersRace(t *testing.T) {
	refuseProductionDSN(t)
	prev := channel.Shared()
	channel.Use(channel.New(channel.NewMemoryStore()))
	t.Cleanup(func() { channel.Use(prev) })

	var wg sync.WaitGroup
	errCh := make(chan error, 3)
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			book := newSearchBook()
			c := newSearchCore(book)
			for range 20 {
				out, err := c.ContactsSearch(&mtproto.TLContactsSearch{Q: "@zhaoyang", Limit: 20})
				if err != nil {
					errCh <- err
					return
				}
				if !peerListed(out.GetResults(), searchStranger) || peerListed(out.GetMyResults(), searchViewer) || peerListed(out.GetResults(), searchViewer) || userListed(out.GetUsers(), searchViewer) {
					errCh <- fmt.Errorf("race buckets my=%v results=%v users=%v", peerIDs(out.GetMyResults()), peerIDs(out.GetResults()), userIDs(out.GetUsers()))
					return
				}
				if !userListed(out.GetUsers(), searchStranger) {
					errCh <- fmt.Errorf("race missing user %v", userIDs(out.GetUsers()))
					return
				}
				if !book.loadedViewer() {
					errCh <- fmt.Errorf("race did not load viewer")
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}
