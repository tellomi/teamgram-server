package channel

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type featureGate struct {
	Store
	getErr       error
	saveFeatErr  error
	saveChErr    error
	saveN        int
	failSaveFrom int
}

func (g *featureGate) GetFeatures(ctx context.Context, channelID int64) (*Features, error) {
	if g.getErr != nil {
		return nil, g.getErr
	}
	return g.Store.GetFeatures(ctx, channelID)
}

func (g *featureGate) SaveFeatures(ctx context.Context, channelID int64, f *Features) error {
	g.saveN++
	if g.failSaveFrom > 0 && g.saveN >= g.failSaveFrom {
		return errors.New("feature restore down")
	}
	if g.saveFeatErr != nil {
		return g.saveFeatErr
	}
	return g.Store.SaveFeatures(ctx, channelID, f)
}

func (g *featureGate) SaveChannel(ctx context.Context, ch *Channel) error {
	if g.saveChErr != nil {
		return g.saveChErr
	}
	return g.Store.SaveChannel(ctx, ch)
}

func TestDeactivateKeepsColorWhenFeatureReadFails(t *testing.T) {
	mem := NewMemoryStore()
	gate := &featureGate{Store: mem}
	svc := New(gate)
	ctx := context.Background()
	SetAppearanceLookup(AppearanceLookup{EmojiDocument: func(id int64) bool { return id == 99 }})
	t.Cleanup(func() { SetAppearanceLookup(AppearanceLookup{}) })
	ch := megagroup(t, svc, ctx, "Room")
	in := inCh(ch)
	if _, err := svc.UpdateUsername(ctx, 1, 0, in, "roomname"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateColor(ctx, 1, 0, &mtproto.TLChannelsUpdateColor{
		Channel:           in,
		Color_FLAGINT32:   wrapperspb.Int32(2),
		BackgroundEmojiId: wrapperspb.Int64(99),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdatePaidMessagesPrice(ctx, 1, 0, in, 15, true); err != nil {
		t.Fatal(err)
	}
	gate.getErr = errors.New("feature read down")
	if _, err := svc.DeactivateAllUsernames(ctx, 1, 0, in, nil); err == nil {
		t.Fatal("read failure returned success")
	}
	gate.getErr = nil
	full, err := svc.GetFullChannel(ctx, 1, in)
	if err != nil {
		t.Fatal(err)
	}
	chat := full.Chats[0]
	if chat.GetUsername().GetValue() != "roomname" {
		t.Fatalf("username cleared on failed read: %q", chat.GetUsername().GetValue())
	}
	if got := chat.GetColor_FLAGPEERCOLOR().GetColor(); got == nil || got.GetValue() != 2 {
		t.Fatalf("color wiped: %+v", chat.GetColor_FLAGPEERCOLOR())
	}
	if full.FullChat.GetSendPaidMessagesStars().GetValue() != 15 {
		t.Fatalf("paid stars wiped: %+v", full.FullChat.GetSendPaidMessagesStars())
	}
	stored, err := gate.Store.GetFeatures(ctx, ch.Id)
	if err != nil || !stored.ColorSet || stored.ColorID != 2 || stored.ColorEmoji != 99 || !stored.PaidMessagesSet || stored.SendPaidMessagesStars != 15 {
		t.Fatalf("feature row replaced: %+v", stored)
	}
}

func TestDeactivateKeepsFeatureRowWhenSaveFails(t *testing.T) {
	mem := NewMemoryStore()
	gate := &featureGate{Store: mem}
	svc := New(gate)
	ctx := context.Background()
	ch := megagroup(t, svc, ctx, "Room")
	in := inCh(ch)
	if _, err := svc.UpdateUsername(ctx, 1, 0, in, "roomname"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdatePaidMessagesPrice(ctx, 1, 0, in, 8, false); err != nil {
		t.Fatal(err)
	}
	gate.saveFeatErr = errors.New("feature save down")
	if _, err := svc.DeactivateAllUsernames(ctx, 1, 0, in, nil); err == nil {
		t.Fatal("save failure returned success")
	}
	gate.saveFeatErr = nil
	got, err := svc.GetChannels(ctx, 1, []*mtproto.InputChannel{in})
	if err != nil || got.Chats[0].GetUsername().GetValue() != "roomname" {
		t.Fatalf("username %+v %v", got.GetChats(), err)
	}
	stored, err := gate.Store.GetFeatures(ctx, ch.Id)
	if err != nil || !stored.PaidMessagesSet || stored.SendPaidMessagesStars != 8 {
		t.Fatalf("stored %+v %v", stored, err)
	}
	active := false
	for _, name := range stored.Usernames {
		if name.Username == "roomname" && name.Active {
			active = true
		}
	}
	if !active {
		t.Fatalf("username list cleared in the feature row: %+v", stored.Usernames)
	}
}

func TestDeactivateRestoresFeaturesWhenChannelSaveFails(t *testing.T) {
	mem := NewMemoryStore()
	gate := &featureGate{Store: mem}
	svc := New(gate)
	ctx := context.Background()
	ch := megagroup(t, svc, ctx, "Room")
	in := inCh(ch)
	if _, err := svc.UpdateUsername(ctx, 1, 0, in, "roomname"); err != nil {
		t.Fatal(err)
	}
	gate.saveChErr = errors.New("channel save down")
	if _, err := svc.DeactivateAllUsernames(ctx, 1, 0, in, nil); err == nil {
		t.Fatal("channel save failure returned success")
	}
	gate.saveChErr = nil
	stored, err := gate.Store.GetFeatures(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range stored.Usernames {
		if name.Username == "roomname" && !name.Active {
			t.Fatalf("feature usernames stayed deactivated after channel save failed: %+v", stored.Usernames)
		}
	}
	got, err := svc.GetChannels(ctx, 1, []*mtproto.InputChannel{in})
	if err != nil || got.Chats[0].GetUsername().GetValue() != "roomname" {
		t.Fatalf("channel username %+v %v", got.GetChats(), err)
	}
}

type recordedIndex struct {
	hits int
	gone map[string]bool
	err  error
}

func (r *recordedIndex) drop(_ context.Context, _ int64, names []string) error {
	r.hits++
	if r.err != nil {
		return r.err
	}
	if r.gone == nil {
		r.gone = map[string]bool{}
	}
	for _, name := range names {
		r.gone[name] = true
	}
	return nil
}

func (r *recordedIndex) contactsResolve(name string) bool {
	return r.gone == nil || !r.gone[name]
}

func TestDeactivateDoesNotTouchIndexBeforeAuth(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "Room")
	in := inCh(ch)
	if _, err := svc.UpdateUsername(ctx, 1, 0, in, "roomname"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2, 3, 5}, nil); err != nil {
		t.Fatal(err)
	}
	banOnly := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{BanUsers: true}).To_ChatAdminRights()
	if _, err := svc.EditAdmin(ctx, 1, 0, in, 3, banOnly, ""); err != nil {
		t.Fatal(err)
	}
	info := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{ChangeInfo: true}).To_ChatAdminRights()
	if _, err := svc.EditAdmin(ctx, 1, 0, in, 5, info, ""); err != nil {
		t.Fatal(err)
	}
	idx := &recordedIndex{}
	for _, userID := range []int64{2, 9, 3} {
		if _, err := svc.DeactivateAllUsernames(ctx, userID, 0, in, idx.drop); err == nil {
			t.Fatalf("user %d deactivated", userID)
		}
	}
	if idx.hits != 0 {
		t.Fatalf("index writes before auth: %d", idx.hits)
	}
	if _, err := svc.ResolveUsername(ctx, 1, "roomname"); err != nil || !idx.contactsResolve("roomname") {
		t.Fatal("name stopped resolving before an authorized deactivate")
	}
	if _, err := svc.DeactivateAllUsernames(ctx, 5, 0, in, idx.drop); err != nil {
		t.Fatal(err)
	}
	if idx.hits != 1 || idx.contactsResolve("roomname") {
		t.Fatalf("change_info index hits=%d still=%v", idx.hits, idx.contactsResolve("roomname"))
	}
	if _, err := svc.ResolveUsername(ctx, 1, "roomname"); err == nil {
		t.Fatal("change_info deactivate left the channel username resolvable")
	}

	other := megagroup(t, svc, ctx, "Other")
	otherIn := inCh(other)
	if _, err := svc.UpdateUsername(ctx, 1, 0, otherIn, "othername"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeactivateAllUsernames(ctx, 1, 0, otherIn, idx.drop); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ResolveUsername(ctx, 1, "othername"); err == nil || idx.contactsResolve("othername") {
		t.Fatal("creator deactivate left a resolvable name")
	}
}

func TestDeactivateIndexDeletedChannelColumnRemainsOnFeatureSaveError(t *testing.T) {
	mem := NewMemoryStore()
	gate := &featureGate{Store: mem}
	svc := New(gate)
	ctx := context.Background()
	ch := megagroup(t, svc, ctx, "Room")
	in := inCh(ch)
	if _, err := svc.UpdateUsername(ctx, 1, 0, in, "roomname"); err != nil {
		t.Fatal(err)
	}
	idx := &recordedIndex{}
	gate.saveFeatErr = errors.New("feature save down")
	if _, err := svc.DeactivateAllUsernames(ctx, 1, 0, in, idx.drop); err == nil {
		t.Fatal("feature save failure returned success")
	}
	if idx.contactsResolve("roomname") {
		t.Fatal("contacts index still resolves")
	}
	if _, err := svc.ResolveUsername(ctx, 1, "roomname"); err != nil {
		t.Fatalf("channel column resolve failed while the RPC failed: %v", err)
	}
	gate.saveFeatErr = nil
	if _, err := svc.DeactivateAllUsernames(ctx, 1, 0, in, idx.drop); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ResolveUsername(ctx, 1, "roomname"); err == nil || idx.contactsResolve("roomname") {
		t.Fatal("retry left a resolvable name")
	}
}

func TestDeactivateFeatureRestoreFailureStaysAnError(t *testing.T) {
	mem := NewMemoryStore()
	gate := &featureGate{Store: mem}
	svc := New(gate)
	ctx := context.Background()
	ch := megagroup(t, svc, ctx, "Room")
	in := inCh(ch)
	if _, err := svc.UpdateUsername(ctx, 1, 0, in, "roomname"); err != nil {
		t.Fatal(err)
	}
	gate.failSaveFrom = gate.saveN + 2
	gate.saveChErr = errors.New("channel save down")
	_, err := svc.DeactivateAllUsernames(ctx, 1, 0, in, nil)
	if err == nil || !strings.Contains(err.Error(), "channel save") || !strings.Contains(err.Error(), "feature restore") {
		t.Fatalf("double failure: %v", err)
	}
	if _, err = svc.ResolveUsername(ctx, 1, "roomname"); err != nil {
		t.Fatalf("channel column lost while both saves failed: %v", err)
	}
	gate.saveChErr = nil
	gate.failSaveFrom = 0
	if _, err = svc.DeactivateAllUsernames(ctx, 1, 0, in, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ResolveUsername(ctx, 1, "roomname"); err == nil {
		t.Fatal("retry left the channel username resolvable")
	}
}

func TestMySQLFeatureReadBoundaries(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := newMySQLStore(db).(*mysqlStore)
	ctx := context.Background()
	q := regexp.QuoteMeta("SELECT data FROM channel_feature WHERE channel_id=?")

	mock.ExpectQuery(q).WithArgs(int64(7)).WillReturnError(sql.ErrNoRows)
	got, err := store.GetFeatures(ctx, 7)
	if err != nil || got == nil || got.ColorSet {
		t.Fatalf("missing row %+v %v", got, err)
	}

	mock.ExpectQuery(q).WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"data"}).AddRow([]byte("{not-json")))
	if _, err = store.GetFeatures(ctx, 7); err == nil {
		t.Fatal("bad JSON returned success")
	}

	mock.ExpectQuery(q).WithArgs(int64(7)).WillReturnError(errors.New("connection reset"))
	if _, err = store.GetFeatures(ctx, 7); err == nil {
		t.Fatal("SQL read failure returned success")
	}

	mock.ExpectExec("INSERT INTO channel_feature").WithArgs(int64(7), sqlmock.AnyArg()).
		WillReturnError(errors.New("disk full"))
	if err = store.SaveFeatures(ctx, 7, &Features{ColorSet: true, ColorID: 2}); err == nil {
		t.Fatal("SQL save failure returned success")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
