package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/notification/internal/reactions"
	"github.com/teamgram/teamgram-server/app/bff/notification/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

func reactionsCore(userID int64, store reactions.Store) *NotificationCore {
	return &NotificationCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Reactions: store},
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: userID},
	}
}

func TestDefaultReactionsNotifySettingsEncode(t *testing.T) {
	got := defaultReactionsNotifySettings()
	buf := mtproto.NewEncodeBuf(128)
	if err := got.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	raw := buf.GetBuf()
	ctor := uint32(raw[0]) | uint32(raw[1])<<8 | uint32(raw[2])<<16 | uint32(raw[3])<<24
	if ctor != 0x71e4ea58 {
		t.Fatalf("constructor %x", ctor)
	}
	flags := uint32(raw[4]) | uint32(raw[5])<<8 | uint32(raw[6])<<16 | uint32(raw[7])<<24
	// messages and stories are set; poll votes stay unset.
	if flags != 0x3 {
		t.Fatalf("flags %x", flags)
	}
	if got.GetSound().GetPredicateName() != mtproto.Predicate_notificationSoundDefault {
		t.Fatalf("sound %s", got.GetSound().GetPredicateName())
	}
	if !mtproto.FromBool(got.GetShowPreviews()) {
		t.Fatal("show_previews")
	}
}

func TestSetAndGetReactionsNotifySettings(t *testing.T) {
	c := reactionsCore(42, reactions.NewMemoryStore())
	first, err := c.AccountGetReactionsNotifySettings(nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.GetMessagesNotifyFrom().GetPredicateName() != mtproto.Predicate_reactionNotificationsFromContacts {
		t.Fatal("default messages")
	}
	if first.GetPollVotesNotifyFrom() != nil {
		t.Fatal("poll votes should stay disabled until set")
	}

	saved, err := c.AccountSetReactionsNotifySettings(&mtproto.TLAccountSetReactionsNotifySettings{
		Settings: &mtproto.ReactionsNotifySettings{
			MessagesNotifyFrom: mtproto.MakeTLReactionNotificationsFromAll(&mtproto.ReactionNotificationsFrom{}).To_ReactionNotificationsFrom(),
			Sound:              mtproto.MakeTLNotificationSoundNone(&mtproto.NotificationSound{}).To_NotificationSound(),
			ShowPreviews:       mtproto.BoolFalse,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.GetStoriesNotifyFrom() != nil {
		t.Fatal("omitted stories field disables story reaction notifications")
	}
	again, err := c.AccountGetReactionsNotifySettings(nil)
	if err != nil {
		t.Fatal(err)
	}
	if again.GetMessagesNotifyFrom().GetPredicateName() != mtproto.Predicate_reactionNotificationsFromAll {
		t.Fatalf("messages %s", again.GetMessagesNotifyFrom().GetPredicateName())
	}
	if again.GetSound().GetPredicateName() != mtproto.Predicate_notificationSoundNone || mtproto.FromBool(again.GetShowPreviews()) {
		t.Fatal("saved sound and previews were not returned")
	}

	other := reactionsCore(7, c.svcCtx.Reactions)
	def, err := other.AccountGetReactionsNotifySettings(nil)
	if err != nil {
		t.Fatal(err)
	}
	if def.GetMessagesNotifyFrom().GetPredicateName() != mtproto.Predicate_reactionNotificationsFromContacts {
		t.Fatal("another user must keep the default")
	}
}

// A missing user_settings row is already the default object. A store failure
// must not look like that object: the client would show contacts-only
// notifications, the default sound, and previews on.
func TestGetReactionsNotifySettingsReturnsStoreError(t *testing.T) {
	storeErr := errors.New("reactions notify store failed")
	store := &scriptedReactionsStore{err: storeErr}
	c := reactionsCore(42, store)

	got, err := c.AccountGetReactionsNotifySettings(nil)
	if err == nil || got != nil {
		t.Fatalf("store failure was reported as reaction settings: %v %v", got, err)
	}
	if !errors.Is(err, storeErr) {
		t.Fatalf("error: %v", err)
	}
	if store.calls != 1 || store.userID != 42 {
		t.Fatalf("load user %d calls %d", store.userID, store.calls)
	}

	store.err = nil
	store.ok = false
	got, err = c.AccountGetReactionsNotifySettings(nil)
	if err != nil || got == nil {
		t.Fatalf("missing row: %v %v", got, err)
	}
	if got.GetMessagesNotifyFrom().GetPredicateName() != mtproto.Predicate_reactionNotificationsFromContacts ||
		got.GetStoriesNotifyFrom().GetPredicateName() != mtproto.Predicate_reactionNotificationsFromContacts {
		t.Fatalf("missing row from %#v %#v", got.GetMessagesNotifyFrom(), got.GetStoriesNotifyFrom())
	}
	if got.GetPollVotesNotifyFrom() != nil || got.GetSound().GetPredicateName() != mtproto.Predicate_notificationSoundDefault || !mtproto.FromBool(got.GetShowPreviews()) {
		t.Fatalf("missing row should stay the default, got %#v", got)
	}

	store.ok = true
	store.raw = ""
	got, err = c.AccountGetReactionsNotifySettings(nil)
	if err != nil || got == nil || got.GetMessagesNotifyFrom().GetPredicateName() != mtproto.Predicate_reactionNotificationsFromContacts {
		t.Fatalf("empty value: %v %v", got, err)
	}

	saved := mtproto.MakeTLReactionsNotifySettings(&mtproto.ReactionsNotifySettings{
		MessagesNotifyFrom: mtproto.MakeTLReactionNotificationsFromAll(&mtproto.ReactionNotificationsFrom{}).To_ReactionNotificationsFrom(),
		Sound:              mtproto.MakeTLNotificationSoundNone(&mtproto.NotificationSound{}).To_NotificationSound(),
		ShowPreviews:       mtproto.BoolFalse,
	}).To_ReactionsNotifySettings()
	raw, err := encodeReactions(saved)
	if err != nil {
		t.Fatal(err)
	}
	store.raw = raw
	got, err = c.AccountGetReactionsNotifySettings(nil)
	if err != nil || got == nil {
		t.Fatalf("saved settings: %v %v", got, err)
	}
	if got.GetMessagesNotifyFrom().GetPredicateName() != mtproto.Predicate_reactionNotificationsFromAll {
		t.Fatalf("messages %s", got.GetMessagesNotifyFrom().GetPredicateName())
	}
	if got.GetSound().GetPredicateName() != mtproto.Predicate_notificationSoundNone || mtproto.FromBool(got.GetShowPreviews()) {
		t.Fatal("saved sound and previews were not returned")
	}
	if store.calls != 4 {
		t.Fatalf("calls %d", store.calls)
	}
}

type scriptedReactionsStore struct {
	raw    string
	ok     bool
	err    error
	userID int64
	calls  int
}

func (s *scriptedReactionsStore) Load(_ context.Context, userID int64) (string, bool, error) {
	s.calls++
	s.userID = userID
	return s.raw, s.ok, s.err
}

func (s *scriptedReactionsStore) Save(context.Context, int64, string) error {
	return nil
}
