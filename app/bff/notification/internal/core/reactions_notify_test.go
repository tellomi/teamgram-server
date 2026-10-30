package core

import (
	"context"
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
