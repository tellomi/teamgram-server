package channel

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/brand"
)

// Synthetic users only. The memory store is not the MTProto check.
const (
	failOwner int64 = 910188001
	failSame  int32 = 1_700_000_200
	failBody        = "undo-secret"
)

func TestReadParticipantsFeatureFailureFailClosed(t *testing.T) {
	svc, ctx := newTest(t)
	g := newFailGroup(t, svc, ctx)
	if _, err := svc.Invite(ctx, failOwner, 0, g.in, []int64{failGuest}, nil); err != nil {
		t.Fatal(err)
	}
	stampJoinDate(t, svc, ctx, g.ch.Id, failGuest, failSame)
	readers, err := svc.ReadParticipants(ctx, failOwner, g.ch.Id, g.oldID)
	if err != nil {
		t.Fatal(err)
	}
	if readerHas(readers, failGuest) {
		t.Fatalf("same-second join cursor listed before a feature failure: %+v", readers)
	}

	base := svc.store
	svc.store = &failFeatures{Store: base}
	readers, err = svc.ReadParticipants(ctx, failOwner, g.ch.Id, g.oldID)
	svc.store = base
	if err == nil || readerHas(readers, failGuest) {
		t.Fatalf("same-second feature read returned a read list: %+v %v", readers, err)
	}

	rights := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{ChangeInfo: true}).To_ChatAdminRights()
	if _, err = svc.EditAdmin(ctx, failOwner, 0, g.in, failGuest, rights, ""); err != nil {
		t.Fatal(err)
	}
	readers, err = svc.ReadParticipants(ctx, failOwner, g.ch.Id, g.oldID)
	if err != nil || readerHas(readers, failGuest) {
		t.Fatalf("promotion listed the admin before readHistory: %+v %v", readers, err)
	}
	if err = svc.ReadHistory(ctx, failGuest, g.in, g.oldID); err != nil {
		t.Fatal(err)
	}
	readers, err = svc.ReadParticipants(ctx, failOwner, g.ch.Id, g.oldID)
	if err != nil || !readerHas(readers, failGuest) {
		t.Fatalf("readHistory after promotion did not list the admin: %+v %v", readers, err)
	}

	svc.store = &failFeatures{Store: base}
	readers, err = svc.ReadParticipants(ctx, failOwner, g.ch.Id, g.oldID)
	svc.store = base
	if err == nil || readerHas(readers, failGuest) {
		t.Fatalf("feature read after promotion returned a read list: %+v %v", readers, err)
	}
	readers, err = svc.ReadParticipants(ctx, failOwner, g.ch.Id, g.oldID)
	if err != nil || !readerHas(readers, failGuest) {
		t.Fatalf("feature read failure changed the stored read list: %+v %v", readers, err)
	}

	svc.store = &failParticipantList{Store: base}
	readers, err = svc.ReadParticipants(ctx, failOwner, g.ch.Id, g.oldID)
	svc.store = base
	if err == nil || readerHas(readers, failGuest) {
		t.Fatalf("participant list failure returned a read list: %+v %v", readers, err)
	}
	readers, err = svc.ReadParticipants(ctx, failOwner, g.ch.Id, g.oldID)
	if err != nil || !readerHas(readers, failGuest) {
		t.Fatalf("list failure changed the stored read list: %+v %v", readers, err)
	}
}

func TestIncompleteJoinDoesNotGrantMembership(t *testing.T) {
	t.Run("invite", func(t *testing.T) {
		svc, ctx := newTest(t)
		g := newFailGroup(t, svc, ctx)
		fault, restore := armJoinFault(svc, failGuest)
		_, err := svc.Invite(ctx, failOwner, 0, g.in, []int64{failGuest}, nil)
		restore()
		if err == nil || !strings.Contains(err.Error(), "inbox down") {
			t.Fatalf("invite err=%v writes=%d undos=%d", err, fault.normalWrites, fault.undoWrites)
		}
		assertNoActiveMember(t, svc, ctx, g, failGuest, true)
		if _, err = svc.Invite(ctx, failOwner, 0, g.in, []int64{failGuest}, nil); err != nil {
			t.Fatal(err)
		}
		assertJoinedHidesSameSecond(t, svc, ctx, g, failGuest)
		top := joinTop(svc.features(ctx, g.ch.Id), failGuest)
		if _, err = svc.Invite(ctx, failOwner, 0, g.in, []int64{failGuest}, nil); err != nil {
			t.Fatal(err)
		}
		if joinTop(svc.features(ctx, g.ch.Id), failGuest) != top {
			t.Fatal("retry of a finished join rewrote the join top")
		}
	})

	t.Run("join", func(t *testing.T) {
		svc, ctx := newTest(t)
		g := newFailGroup(t, svc, ctx)
		if _, err := svc.UpdateUsername(ctx, failOwner, 0, g.in, "undopubjoin"); err != nil {
			t.Fatal(err)
		}
		fault, restore := armJoinFault(svc, failGuest)
		_, err := svc.Join(ctx, failGuest, 0, g.in)
		restore()
		if err == nil || !strings.Contains(err.Error(), "inbox down") {
			t.Fatalf("join err=%v writes=%d undos=%d", err, fault.normalWrites, fault.undoWrites)
		}
		assertNoActiveMember(t, svc, ctx, g, failGuest, false)
		if _, err = svc.Join(ctx, failGuest, 0, g.in); err != nil {
			t.Fatal(err)
		}
		assertJoinedHidesSameSecond(t, svc, ctx, g, failGuest)
	})

	t.Run("import", func(t *testing.T) {
		svc, ctx := newTest(t)
		g := newFailGroup(t, svc, ctx)
		exported, err := svc.ExportInvite(ctx, failOwner, g.in, ExportRequest{LegacyRevokePermanent: true})
		if err != nil {
			t.Fatal(err)
		}
		hash := strings.TrimPrefix(exported.GetLink(), brand.LinkBase()+"/+")
		fault, restore := armJoinFault(svc, failGuest)
		_, err = svc.ImportInvite(ctx, failGuest, 0, hash)
		restore()
		if err == nil || !strings.Contains(err.Error(), "inbox down") {
			t.Fatalf("import err=%v writes=%d undos=%d", err, fault.normalWrites, fault.undoWrites)
		}
		assertNoActiveMember(t, svc, ctx, g, failGuest, true)
		if _, err = svc.ImportInvite(ctx, failGuest, 0, hash); err != nil {
			t.Fatal(err)
		}
		assertJoinedHidesSameSecond(t, svc, ctx, g, failGuest)
	})

	t.Run("membership write after cursor", func(t *testing.T) {
		svc, ctx := newTest(t)
		g := newFailGroup(t, svc, ctx)
		base := svc.store
		fault := &joinFault{Store: base, user: failGuest, normalLeft: 0, undoErr: errors.New("participant write failed")}
		svc.store = fault
		_, err := svc.Invite(ctx, failOwner, 0, g.in, []int64{failGuest}, nil)
		svc.store = base
		if err == nil || !strings.Contains(err.Error(), "participant write failed") {
			t.Fatalf("invite err=%v", err)
		}
		assertNoActiveMember(t, svc, ctx, g, failGuest, true)
		feat, ferr := svc.mutableFeatures(ctx, g.ch.Id)
		if ferr != nil {
			t.Fatal(ferr)
		}
		if !feat.HiddenPrehistory || feat.JoinTop[failGuest] < g.oldID {
			t.Fatalf("failed member write lost the committed join top: %+v", feat)
		}
		inbox, rerr := svc.store.GetRead(ctx, g.ch.Id, failGuest)
		if rerr != nil || inbox == nil || inbox.ReadInboxMaxID < g.oldID {
			t.Fatalf("failed member write lost the committed inbox cursor: %+v %v", inbox, rerr)
		}
		if _, err = svc.Invite(ctx, failOwner, 0, g.in, []int64{failGuest}, nil); err != nil {
			t.Fatal(err)
		}
		assertJoinedHidesSameSecond(t, svc, ctx, g, failGuest)
	})

	t.Run("no config is not a read error", func(t *testing.T) {
		svc, ctx := newTest(t)
		g := newFailGroup(t, svc, ctx)
		clearFeatures(t, svc, g.ch.Id)
		if _, err := svc.Invite(ctx, failOwner, 0, g.in, []int64{failGuest}, nil); err != nil {
			t.Fatal(err)
		}
		p, err := svc.store.GetParticipant(ctx, g.ch.Id, failGuest)
		if err != nil || p == nil || !p.InChannel() {
			t.Fatalf("missing feature row blocked the join: %+v %v", p, err)
		}
		feat, err := svc.mutableFeatures(ctx, g.ch.Id)
		if err != nil || feat.loadFailed || joinTop(feat, failGuest) < g.oldID {
			t.Fatalf("empty config did not record the join top: %+v %v", feat, err)
		}

		svc, ctx = newTest(t)
		g = newFailGroup(t, svc, ctx)
		base := svc.store
		svc.store = &failFeatures{Store: base}
		_, err = svc.Invite(ctx, failOwner, 0, g.in, []int64{failGuest}, nil)
		svc.store = base
		if err == nil {
			t.Fatal("feature read error was treated as an empty config")
		}
		assertNoActiveMember(t, svc, ctx, g, failGuest, true)
		kept, err := svc.mutableFeatures(ctx, g.ch.Id)
		if err != nil || !kept.HiddenPrehistory || joinTop(kept, failGuest) != 0 {
			t.Fatalf("failed feature read changed the stored config: %+v %v", kept, err)
		}
	})

	t.Run("nil member is not a read error", func(t *testing.T) {
		svc, ctx := newTest(t)
		g := newFailGroup(t, svc, ctx)
		const absent int64 = 910188005
		got, err := svc.store.GetParticipant(ctx, g.ch.Id, absent)
		if err != nil || got != nil {
			t.Fatalf("legitimate nil member: %+v %v", got, err)
		}
		gate := svc.PrehistoryGate(ctx, absent, g.ch.Id)
		if !gate.Hidden || gate.Unknown || gate.Admin {
			t.Fatalf("nil member became a failed read: %+v", gate)
		}
		if _, err = svc.Invite(ctx, failOwner, 0, g.in, []int64{absent}, nil); err != nil {
			t.Fatal(err)
		}
		if p, err := svc.store.GetParticipant(ctx, g.ch.Id, absent); err != nil || p == nil || !p.InChannel() {
			t.Fatalf("nil member was not invited: %+v %v", p, err)
		}

		const unreadable int64 = 910188006
		base := svc.store
		svc.store = &errParticipant{Store: base, user: unreadable}
		_, err = svc.Invite(ctx, failOwner, 0, g.in, []int64{unreadable}, nil)
		if err == nil || !strings.Contains(err.Error(), "participant read failed") {
			svc.store = base
			t.Fatalf("participant read err=%v", err)
		}
		gate = svc.PrehistoryGate(ctx, unreadable, g.ch.Id)
		svc.store = base
		if !gate.Hidden || !gate.Unknown {
			t.Fatalf("participant read error was treated as a nil member: %+v", gate)
		}
		if p, perr := svc.store.GetParticipant(ctx, g.ch.Id, unreadable); perr != nil || (p != nil && p.InChannel()) {
			t.Fatalf("failed participant read left a member: %+v %v", p, perr)
		}
	})

	t.Run("join top write", func(t *testing.T) {
		svc, ctx := newTest(t)
		g := newFailGroup(t, svc, ctx)
		base := svc.store
		svc.store = &failFeatureSave{Store: base}
		_, err := svc.Invite(ctx, failOwner, 0, g.in, []int64{failGuest}, nil)
		svc.store = base
		if err == nil || !strings.Contains(err.Error(), "features save failed") {
			t.Fatalf("join top write err=%v", err)
		}
		assertNoActiveMember(t, svc, ctx, g, failGuest, true)
		kept, err := svc.mutableFeatures(ctx, g.ch.Id)
		if err != nil || !kept.HiddenPrehistory || joinTop(kept, failGuest) != 0 {
			t.Fatalf("failed join-top write changed the stored config: %+v %v", kept, err)
		}
	})

	t.Run("inbox read error", func(t *testing.T) {
		svc, ctx := newTest(t)
		g := newFailGroup(t, svc, ctx)
		base := svc.store
		svc.store = &errReadState{Store: base, user: failGuest}
		_, err := svc.Invite(ctx, failOwner, 0, g.in, []int64{failGuest}, nil)
		svc.store = base
		if err == nil || !strings.Contains(err.Error(), "inbox read failed") {
			t.Fatalf("inbox read err=%v", err)
		}
		assertNoActiveMember(t, svc, ctx, g, failGuest, true)
	})
}

const failGuest int64 = 910188002

type failGroup struct {
	ch    *mtproto.Chat
	in    *mtproto.InputChannel
	oldID int32
}

func newFailGroup(t *testing.T, svc *Service, ctx context.Context) failGroup {
	t.Helper()
	created, err := svc.Create(ctx, CreateRequest{CreatorID: failOwner, Title: "Undo", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := inCh(ch)
	sent, err := svc.Send(ctx, failOwner, ch.Id, 0, []Outgoing{{RandomID: failOwner + 17, Message: &mtproto.Message{
		Message: failBody,
		Date:    failSame,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	oldID := sent.Updates[0].GetMessage_MESSAGE().GetId()
	if oldID == 0 {
		t.Fatal("secret id is 0")
	}
	if _, err = svc.TogglePreHistory(ctx, failOwner, 0, in, true); err != nil {
		t.Fatal(err)
	}
	return failGroup{ch: ch, in: in, oldID: oldID}
}

func armJoinFault(svc *Service, user int64) (*joinFault, func()) {
	base := svc.store
	fault := &joinFault{
		Store:      base,
		user:       user,
		saveRead:   errors.New("inbox down"),
		normalLeft: 1,
		undoErr:    errors.New("participant write failed"),
	}
	svc.store = fault
	return fault, func() { svc.store = base }
}

func assertNoActiveMember(t *testing.T, svc *Service, ctx context.Context, g failGroup, user int64, private bool) {
	t.Helper()
	p, err := svc.store.GetParticipant(ctx, g.ch.Id, user)
	if err != nil {
		t.Fatal(err)
	}
	if p != nil && p.State == StateNormal {
		feat, _ := svc.mutableFeatures(ctx, g.ch.Id)
		t.Fatalf("StateNormal without a finished join, joinTop=%d", joinTop(feat, user))
	}
	if p != nil {
		p.Date = failSame
		if err = svc.store.PutParticipant(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	hist, _ := svc.History(ctx, user, g.in, HistoryQuery{Limit: 50})
	got, _ := svc.GetMessages(ctx, user, g.in, []int32{g.oldID})
	if private && (prehistHas(historyMessages(hist), failBody) || prehistHas(historyMessages(got), failBody)) {
		t.Fatalf("private history returned %q while the join was unfinished", failBody)
	}
	if p != nil && p.State == StateNormal && (prehistHas(historyMessages(hist), failBody) || prehistHas(historyMessages(got), failBody)) {
		t.Fatal("active member received the pre-join body")
	}
}

func assertJoinedHidesSameSecond(t *testing.T, svc *Service, ctx context.Context, g failGroup, user int64) {
	t.Helper()
	stampJoinDate(t, svc, ctx, g.ch.Id, user, failSame)
	p, err := svc.store.GetParticipant(ctx, g.ch.Id, user)
	if err != nil || p == nil || p.State != StateNormal {
		t.Fatalf("finished join is not a member: %+v %v", p, err)
	}
	feat, err := svc.mutableFeatures(ctx, g.ch.Id)
	if err != nil || joinTop(feat, user) < g.oldID {
		t.Fatalf("finished join has no join top: %+v %v", feat, err)
	}
	hist, err := svc.History(ctx, user, g.in, HistoryQuery{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if prehistHas(hist.Messages, failBody) {
		t.Fatalf("same-second history returned %q", failBody)
	}
	got, err := svc.GetMessages(ctx, user, g.in, []int32{g.oldID})
	if err != nil {
		t.Fatal(err)
	}
	if prehistHas(got.Messages, failBody) || got.Messages[0].GetPredicateName() != mtproto.Predicate_messageEmpty {
		t.Fatalf("getMessages returned the pre-join body: %+v", got.Messages)
	}
}

func historyMessages(box *mtproto.Messages_Messages) []*mtproto.Message {
	if box == nil {
		return nil
	}
	return box.Messages
}

func stampJoinDate(t *testing.T, svc *Service, ctx context.Context, channelID, user int64, date int32) {
	t.Helper()
	p, err := svc.store.GetParticipant(ctx, channelID, user)
	if err != nil || p == nil {
		t.Fatal(err)
	}
	p.Date = date
	if err = svc.store.PutParticipant(ctx, p); err != nil {
		t.Fatal(err)
	}
}

func clearFeatures(t *testing.T, svc *Service, channelID int64) {
	t.Helper()
	mem, ok := svc.store.(*memoryStore)
	if !ok {
		t.Fatal("expected the memory store")
	}
	mem.mu.Lock()
	delete(mem.features, channelID)
	mem.mu.Unlock()
}

// joinFault lets one StateNormal write succeed and then fails SaveRead and
// any later participant write. That is the old order: grant membership,
// fail the inbox, and fail the undo. The user must not stay StateNormal.
type joinFault struct {
	Store
	user         int64
	saveRead     error
	normalLeft   int
	undoErr      error
	normalWrites int
	undoWrites   int
}

func (f *joinFault) SaveRead(ctx context.Context, st *ReadState) error {
	if st != nil && st.UserID == f.user && f.saveRead != nil {
		return f.saveRead
	}
	return f.Store.SaveRead(ctx, st)
}

func (f *joinFault) PutParticipant(ctx context.Context, p *Participant) error {
	if p != nil && p.UserID == f.user && p.State == StateNormal {
		if f.normalLeft <= 0 {
			return errors.New("participant write failed")
		}
		f.normalLeft--
		f.normalWrites++
	} else if p != nil && p.UserID == f.user {
		f.undoWrites++
		if f.undoErr != nil {
			return f.undoErr
		}
	}
	return f.Store.PutParticipant(ctx, p)
}

type failParticipantList struct{ Store }

func (f *failParticipantList) ListParticipants(ctx context.Context, channelID int64) ([]*Participant, error) {
	return nil, errors.New("participant list failed")
}

type errParticipant struct {
	Store
	user int64
}

func (e *errParticipant) GetParticipant(ctx context.Context, channelID, userID int64) (*Participant, error) {
	if userID == e.user {
		return nil, errors.New("participant read failed")
	}
	return e.Store.GetParticipant(ctx, channelID, userID)
}

type errReadState struct {
	Store
	user int64
}

func (e *errReadState) GetRead(ctx context.Context, channelID, userID int64) (*ReadState, error) {
	if userID == e.user {
		return nil, errors.New("inbox read failed")
	}
	return e.Store.GetRead(ctx, channelID, userID)
}
