package wipe

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

type pushed struct {
	user int64
	pred string
	ids  []int32
	chat int64
}

type note struct {
	items []pushed
}

func (n *note) Push(_ context.Context, userID int64, updates []*mtproto.Update) error {
	for _, upd := range updates {
		n.items = append(n.items, pushed{
			user: userID,
			pred: upd.GetPredicateName(),
			ids:  append([]int32(nil), upd.GetMessages()...),
			chat: upd.GetChatId_INT64(),
		})
	}
	return nil
}

func TestWipePrivateBothSidesKeepsGroupMessages(t *testing.T) {
	mem := NewMem()
	const me = int64(10)
	const peer = int64(20)
	mem.AddBox(me, Peer{Type: mtproto.PEER_USER, ID: peer}, Box{ID: 1, PhotoID: 100})
	mem.AddBox(me, Peer{Type: mtproto.PEER_USER, ID: peer}, Box{ID: 2, DocumentID: 200})
	mem.AddBox(peer, Peer{Type: mtproto.PEER_USER, ID: me}, Box{ID: 5, PhotoID: 100})
	mem.AddBox(me, Peer{Type: mtproto.PEER_SELF, ID: me}, Box{ID: 3})
	mem.AddBox(me, Peer{Type: mtproto.PEER_CHAT, ID: 7}, Box{ID: 9, DocumentID: 300})
	mem.SetObjects("photo", 100, []Object{{Kind: "photo", ID: 100, Class: "photos", Path: "m/100.dat"}})
	mem.SetObjects("document", 200, []Object{{Kind: "document", ID: 200, Class: "documents", Path: "200.dat"}})
	mem.SetObjects("document", 300, []Object{{Kind: "document", ID: 300, Class: "documents", Path: "300.dat"}})
	mem.SetRefs("document", 300, 1)
	mem.SetProfile(me, memProfile{Username: "ada", Name: "Ada", About: "hi", Birthday: "1990-01-01", Photo: 9, Photos: []int64{9}})
	mem.SetObjects("avatar", 9, []Object{{Kind: "avatar", ID: 9, Class: "avatars", Path: "a/9.dat"}})
	mem.SetObjects("photo", 9, []Object{{Kind: "photo", ID: 9, Class: "photos", Path: "a/9.dat"}})
	mem.AddContact(me, peer)
	mem.AddContact(peer, me)
	mem.AddContact(30, me)
	mem.SetSettings(me, 4)
	mem.AddGroup(false, memGroup{
		ID: 7, Creator: me, Version: 1,
		Members: []Member{
			{UserID: me, Role: mtproto.ChatMemberCreator, State: 0, Date: 1},
			{UserID: peer, Role: mtproto.ChatMemberNormal, State: 0, Date: 2},
			{UserID: 21, Role: mtproto.ChatMemberAdmin, State: 0, Date: 5},
			{UserID: 22, Role: mtproto.ChatMemberAdmin, State: 0, Date: 4},
		},
	})
	mem.AddGroup(true, memGroup{
		ID: 8, Creator: me,
		Members: []Member{
			{UserID: me, Role: 1, State: 0, Date: 1},
		},
	})
	var cache []string
	n := &note{}
	eng := &Engine{
		Store:  mem,
		Notify: n,
		Cache:  delCache{fn: func(keys ...string) { cache = append(cache, keys...) }},
		Batch:  1,
		Now:    func() int64 { return 100 },
		Secrets: func(context.Context, int64) ([]SecretEvent, []int64, error) {
			return []SecretEvent{{
				UserID: peer,
				Update: mtproto.MakeTLUpdateEncryption(&mtproto.Update{}).To_Update(),
			}}, []int64{77}, nil
		},
	}
	mem.SetObjects("encrypted", 77, []Object{{Kind: "encrypted", ID: 77, Class: "encryptedfiles", Path: "77.dat"}})

	res, err := eng.Run(context.Background(), Request{UserID: me, Phone: "+8613800138000", Username: "ada"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Boxes != 4 || res.SecretChats != 1 || res.GroupsLeft != 1 || res.Deactivated != 1 {
		t.Fatalf("result = %+v", res)
	}
	if mem.LiveBoxes(me, Peer{mtproto.PEER_USER, peer}) != 0 || mem.LiveBoxes(peer, Peer{mtproto.PEER_USER, me}) != 0 {
		t.Fatal("private boxes should be gone on both sides")
	}
	if mem.LiveBoxes(me, Peer{mtproto.PEER_CHAT, 7}) != 1 {
		t.Fatal("group message must stay")
	}
	if mem.DialogLive(me, Peer{mtproto.PEER_USER, peer}) || mem.DialogLive(peer, Peer{mtproto.PEER_USER, me}) {
		t.Fatal("private dialogs should be removed")
	}
	g := mem.Group(7)
	if g.Creator != 22 || g.Deactivated {
		t.Fatalf("basic group creator=%d deactivated=%v", g.Creator, g.Deactivated)
	}
	left := false
	for _, m := range g.Members {
		if m.UserID == me && m.State == mtproto.ChatMemberStateLeft {
			left = true
		}
	}
	if !left {
		t.Fatal("user should be left in the basic group")
	}
	ch := mem.Group(8)
	if !ch.Deactivated {
		t.Fatal("sole channel should be deactivated")
	}
	p := mem.Profile(me)
	if p.Name != "" || p.About != "" || p.Birthday != "" || p.Username != "" || p.Photo != 0 {
		t.Fatalf("profile = %+v", p)
	}
	if mem.ContactLive(me, peer) || mem.ContactLive(peer, me) || mem.ContactLive(30, me) {
		t.Fatal("contacts should be removed both ways")
	}
	if mem.Settings(me) != 0 {
		t.Fatal("settings should be deleted")
	}
	if len(mem.PtsRows(peer)) == 0 {
		t.Fatal("peer should have a pts row for updateDeleteMessages")
	}
	sawDelete, sawLeft, sawSecret := false, false, false
	for _, item := range n.items {
		if item.user == peer && item.pred == mtproto.Predicate_updateDeleteMessages && len(item.ids) == 1 && item.ids[0] == 5 {
			sawDelete = true
		}
		if item.pred == mtproto.Predicate_updateChatParticipantDelete && item.chat == 7 && item.user == peer {
			sawLeft = true
		}
		if item.pred == mtproto.Predicate_updateEncryption && item.user == peer {
			sawSecret = true
		}
	}
	if !sawDelete || !sawLeft || !sawSecret {
		t.Fatalf("pushes = %+v", n.items)
	}
	queued := map[string]bool{}
	for _, q := range mem.Queue() {
		queued[q.Class+"/"+q.Path] = true
	}
	for _, want := range []string{"photos/m/100.dat", "documents/200.dat", "avatars/a/9.dat", "encryptedfiles/77.dat"} {
		if !queued[want] {
			t.Fatalf("queue missing %s in %+v", want, mem.Queue())
		}
	}
	if queued["documents/300.dat"] {
		t.Fatal("group-only document must not be queued from the private wipe")
	}
	if len(cache) == 0 {
		t.Fatal("cache keys should be dropped")
	}

	again, err := eng.Run(context.Background(), Request{UserID: me, Phone: "+8613800138000", Username: "ada"})
	if err != nil {
		t.Fatal(err)
	}
	if again.Boxes != 0 || again.GroupsLeft != 0 {
		t.Fatalf("retry = %+v", again)
	}
	if mem.LiveBoxes(me, Peer{mtproto.PEER_CHAT, 7}) != 1 {
		t.Fatal("retry removed the group message")
	}
}

func TestGCKeepsReferencedAndTrashesTheRest(t *testing.T) {
	mem := NewMem()
	mem.queue = []GCItem{
		{ID: 1, Kind: "photo", MediaID: 100, Class: "photos", Path: "m/100.dat", Status: statusPending},
		{ID: 2, Kind: "document", MediaID: 300, Class: "documents", Path: "300.dat", Status: statusPending},
		{ID: 3, Kind: "document", MediaID: 400, Class: "documents", Path: "400.dat", Status: statusPending},
		{ID: 4, Kind: "avatar", MediaID: 9, Class: "avatars", Path: "a/9.dat", Status: statusTrashed, TrashedAt: 1},
	}
	mem.SetRefs("document", 300, 2)
	mem.SetExempt(400)
	mv := &fakeMover{}
	cfg := GCConfig{DryRun: true, Keep: time.Hour, Batch: 10, Endpoint: "https://example.r2.cloudflarestorage.com"}
	later := time.Unix(1+int64(time.Hour/time.Second)+1, 0)
	rep, err := RunGC(context.Background(), mem, mv, cfg, later)
	if err != nil {
		t.Fatal(err)
	}
	if rep.WouldTrash != 1 || rep.Referenced != 1 || rep.Exempt != 1 || rep.WouldDelete != 1 || len(mv.trashed) != 0 {
		t.Fatalf("dry report %+v mover %+v", rep, mv)
	}
	if !rep.DryRun || rep.Endpoint == "" {
		t.Fatalf("report should name the endpoint: %s", rep)
	}
	cfg.DryRun = false
	rep, err = RunGC(context.Background(), mem, mv, cfg, later)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Trashed != 1 || rep.Deleted != 1 || len(mv.trashed) != 1 || len(mv.deleted) != 1 {
		t.Fatalf("live report %+v mover %+v", rep, mv)
	}
	if mem.Queue()[0].Status != statusTrashed || mem.Queue()[1].Status != statusPending || mem.Queue()[2].Status != statusSkipped {
		t.Fatalf("queue = %+v", mem.Queue())
	}
	// Inside the keep window the trashed photo is not deleted again.
	rep, err = RunGC(context.Background(), mem, mv, cfg, later.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if rep.Deleted != 0 {
		t.Fatalf("keep window deleted %+v", rep)
	}
}

type fakeMover struct {
	trashed []string
	deleted []string
}

func (f *fakeMover) MoveToTrash(_ context.Context, class, path string) error {
	f.trashed = append(f.trashed, class+"/"+path)
	return nil
}

func (f *fakeMover) DeleteTrash(_ context.Context, class, path string) error {
	f.deleted = append(f.deleted, class+"/"+path)
	return nil
}

type delCache struct {
	fn func(...string)
}

func (d delCache) Del(keys ...string) (int, error) {
	d.fn(keys...)
	return len(keys), nil
}

func TestPickSuccessorBasicGroup(t *testing.T) {
	members := []Member{
		{UserID: 1, Role: mtproto.ChatMemberCreator, State: 0, Date: 1},
		{UserID: 2, Role: mtproto.ChatMemberNormal, State: 0, Date: 2},
		{UserID: 4, Role: mtproto.ChatMemberAdmin, State: 0, Date: 9},
		{UserID: 3, Role: mtproto.ChatMemberAdmin, State: 0, Date: 8},
	}
	got, ok := PickSuccessor(members, 1)
	if !ok || got != 3 {
		t.Fatalf("got %d %v", got, ok)
	}
}

func TestGCKeepsObjectHeldByAnotherBlobRef(t *testing.T) {
	mem := NewMem()
	mem.queue = []GCItem{
		{ID: 1, Kind: "document", MediaID: 22, Class: "documents", Path: "11.dat", Status: statusPending},
	}
	mem.SetShared("documents", "11.dat")
	mv := &fakeMover{}
	cfg := GCConfig{DryRun: true, Keep: time.Hour, Batch: 10}
	rep, err := RunGC(context.Background(), mem, mv, cfg, time.Unix(10, 0))
	if err != nil {
		t.Fatal(err)
	}
	if rep.WouldTrash != 0 || rep.Referenced != 1 || len(mv.trashed) != 0 {
		t.Fatalf("dry %+v", rep)
	}
	cfg.DryRun = false
	rep, err = RunGC(context.Background(), mem, mv, cfg, time.Unix(10, 0))
	if err != nil {
		t.Fatal(err)
	}
	if rep.Trashed != 0 || rep.Referenced != 1 || len(mv.trashed) != 0 {
		t.Fatalf("live %+v mover %+v", rep, mv)
	}
	if mem.Queue()[0].Status != statusPending {
		t.Fatalf("queue %+v", mem.Queue())
	}
}

func TestGCFromEnvDefaultsDryRun(t *testing.T) {
	t.Setenv(EnvDryRun, "")
	t.Setenv(EnvKeepDays, "")
	cfg := GCFromEnv()
	if !cfg.DryRun || cfg.Keep != 30*24*time.Hour {
		t.Fatalf("%+v", cfg)
	}
	t.Setenv(EnvDryRun, "false")
	t.Setenv(EnvKeepDays, "7")
	cfg = GCFromEnv()
	if cfg.DryRun || cfg.Keep != 7*24*time.Hour {
		t.Fatalf("%+v", cfg)
	}
}
