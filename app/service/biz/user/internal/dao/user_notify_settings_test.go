package dao

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestTopicNotifyCacheKeyDoesNotCollideWithTheDialog(t *testing.T) {
	dialog := genUserNotifySettingsCacheKey(1, mtproto.PEER_CHANNEL, 9)
	topic := TopicNotifySettingsCacheKey(1, mtproto.PEER_CHANNEL, 9, 3)
	other := TopicNotifySettingsCacheKey(1, mtproto.PEER_CHANNEL, 9, 4)
	if dialog == topic || topic == other {
		t.Fatalf("dialog %s topic %s other %s", dialog, topic, other)
	}
	keys := TopicNotifyCacheKeys(1, []dataobject.UserNotifySettingsDO{
		{PeerType: mtproto.PEER_CHANNEL, PeerId: 9, TopMsgId: 3},
		{PeerType: mtproto.PEER_CHANNEL, PeerId: 9, TopMsgId: 0},
		{PeerType: mtproto.PEER_CHANNEL, PeerId: 9, TopMsgId: 3},
	})
	if len(keys) != 1 || keys[0] != topic {
		t.Fatalf("keys %v", keys)
	}
}

func TestPeerNotifySettingsFromDOKeepsLegacySoundAndNewColumns(t *testing.T) {
	legacy := PeerNotifySettingsFromDO(&dataobject.UserNotifySettingsDO{
		ShowPreviews: -1,
		Silent:       -1,
		MuteUntil:    2147483647,
		Sound:        "default",
		StoriesMuted: -1,
	})
	if legacy.GetMuteUntil().GetValue() != 2147483647 {
		t.Fatalf("mute %d", legacy.GetMuteUntil().GetValue())
	}
	if legacy.GetShowPreviews() != nil || legacy.GetSilent() != nil {
		t.Fatal("unset flags were returned")
	}
	if legacy.GetAndroidSound().GetPredicateName() != mtproto.Predicate_notificationSoundDefault ||
		legacy.GetIosSound().GetPredicateName() != mtproto.Predicate_notificationSoundDefault {
		t.Fatal("a legacy sound must still be visible to current clients")
	}

	stored := PeerNotifySettingsFromDO(&dataobject.UserNotifySettingsDO{
		ShowPreviews:        1,
		Silent:              0,
		MuteUntil:           0,
		Sound:               "42",
		StoriesMuted:        1,
		StoriesHideSender:   0,
		AndroidSound:        "ringtone:42",
		StoriesAndroidSound: "none",
		IosSound:            "",
	})
	if mtproto.FromBool(stored.GetSilent()) || !mtproto.FromBool(stored.GetStoriesMuted()) || mtproto.FromBool(stored.GetStoriesHideSender()) {
		t.Fatalf("flags silent=%v stories=%v hide=%v", stored.GetSilent(), stored.GetStoriesMuted(), stored.GetStoriesHideSender())
	}
	if stored.GetAndroidSound().GetId() != 42 {
		t.Fatalf("android sound %+v", stored.GetAndroidSound())
	}
	if stored.GetIosSound() != nil {
		t.Fatal("an empty ios_sound must stay unset once a platform column is set")
	}
	if stored.GetStoriesAndroidSound().GetPredicateName() != mtproto.Predicate_notificationSoundNone {
		t.Fatalf("stories sound %+v", stored.GetStoriesAndroidSound())
	}
}

func TestNotifySettingsCacheKeysCoverPeerGlobalAndStoryScopes(t *testing.T) {
	userID := int64(42)
	rows := []dataobject.UserNotifySettingsDO{
		{PeerType: mtproto.PEER_USER, PeerId: 99},
		{PeerType: mtproto.PEER_CHANNEL, PeerId: 7},
		{PeerType: mtproto.PEER_USERS, PeerId: 0},
	}
	got := NotifySettingsCacheKeys(userID, rows)
	want := []string{
		genUserNotifySettingsCacheKey(userID, mtproto.PEER_USER, 99),
		genUserNotifySettingsCacheKey(userID, mtproto.PEER_CHANNEL, 7),
		genUserNotifySettingsCacheKey(userID, mtproto.PEER_USERS, 0),
		genUserNotifySettingsCacheKey(userID, mtproto.PEER_CHATS, 0),
		genUserNotifySettingsCacheKey(userID, mtproto.PEER_BROADCASTS, 0),
	}
	if len(got) != len(want) {
		t.Fatalf("keys %v", got)
	}
	have := map[string]struct{}{}
	for _, key := range got {
		have[key] = struct{}{}
	}
	for _, key := range want {
		if _, ok := have[key]; !ok {
			t.Fatalf("missing %s in %v", key, got)
		}
	}
	// The users/chats/broadcasts keys are the story scopes: stories_muted
	// and the story sounds live on those same cached objects.
	if _, ok := have[genUserNotifySettingsCacheKey(userID, mtproto.PEER_USERS, 0)]; !ok {
		t.Fatal("global users key, which carries story notify settings, was omitted")
	}
}

func TestDropNotifySettingsCacheRemovesStaleMute(t *testing.T) {
	mr := miniredis.RunT(t)
	rds, err := redis.NewRedis(redis.RedisConf{Host: mr.Addr(), Type: redis.NodeType})
	if err != nil {
		t.Fatal(err)
	}
	d := &Dao{CachedConn: sqlc.NewNodeConn(nil, rds)}
	ctx := context.Background()
	userID := int64(42)
	rows := []dataobject.UserNotifySettingsDO{
		{PeerType: mtproto.PEER_USER, PeerId: 99},
		{PeerType: mtproto.PEER_USERS, PeerId: 0},
	}
	keys := NotifySettingsCacheKeys(userID, rows)
	for _, key := range keys {
		if err := d.CachedConn.SetCache(ctx, key, "mute-until-max"); err != nil {
			t.Fatal(err)
		}
	}
	other := genUserNotifySettingsCacheKey(43, mtproto.PEER_USER, 99)
	if err := d.CachedConn.SetCache(ctx, other, "keep"); err != nil {
		t.Fatal(err)
	}

	if err := d.DropNotifySettingsCache(ctx, userID, rows); err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		var stale string
		if err := d.CachedConn.GetCache(ctx, key, &stale); err == nil {
			t.Fatalf("key %s still holds %q", key, stale)
		}
	}
	var kept string
	if err := d.CachedConn.GetCache(ctx, other, &kept); err != nil || kept != "keep" {
		t.Fatalf("other user cache %q %v", kept, err)
	}
}

func TestMuteOnlyInsertStoresUnsetSound(t *testing.T) {
	updates, insertOnly := notifyWriteColumns(mtproto.MakeTLPeerNotifySettings(&mtproto.PeerNotifySettings{
		MuteUntil: wrapperspb.Int32(10),
	}).To_PeerNotifySettings())
	if _, ok := updates["sound"]; ok {
		t.Fatal("a mute-only update must not overwrite a stored sound")
	}
	if insertOnly["sound"] != "-1" {
		t.Fatalf("insert sound %v, want -1", insertOnly["sound"])
	}
	if updates["mute_until"] != int32(10) {
		t.Fatalf("mute column %v", updates["mute_until"])
	}
	got := PeerNotifySettingsFromDO(&dataobject.UserNotifySettingsDO{
		ShowPreviews: -1,
		Silent:       -1,
		MuteUntil:    10,
		Sound:        "-1",
		StoriesMuted: -1,
	})
	if got.GetSound() != nil || got.GetAndroidSound() != nil || got.GetIosSound() != nil || got.GetOtherSound() != nil {
		t.Fatalf("unset sound was returned as a value: %+v", got)
	}
	withSound, insertOnly := notifyWriteColumns(mtproto.MakeTLPeerNotifySettings(&mtproto.PeerNotifySettings{
		Sound: &wrapperspb.StringValue{Value: "default"},
	}).To_PeerNotifySettings())
	if withSound["sound"] != "default" || len(insertOnly) != 0 {
		t.Fatalf("explicit sound updates=%v insertOnly=%v", withSound, insertOnly)
	}
}
