package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestCreateChannelKeepsGeoPoint is ISSUE-S-GEO-CREATE-COORDS-001 (row 269).
// Phone 9996610457, code 11111.
func TestCreateChannelKeepsGeoPoint(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	owner, _ := dialPhone(t, ctx, cfg, "9996610457", "Own457")
	created, err := owner.Invoke(ctx, &mtproto.TLChannelsCreateChannel{
		Constructor: mtproto.CRC32_channels_createChannel_91006707,
		Megagroup:   true,
		Title:       "GeoRoom",
		About:       "",
		GeoPoint: mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{
			Lat:  30.57,
			Long: 104.06,
		}).To_InputGeoPoint(),
		Address: wrapperspb.String("Chengdu"),
	})
	if err != nil {
		t.Fatal(err)
	}
	id, hash := megagroupOf(t, created)
	full := setupFull(t, ctx, owner, id, hash)
	pt := full.GetLocation().GetGeoPoint()
	if pt.GetLat() != 30.57 || pt.GetLong() != 104.06 || full.GetLocation().GetAddress() != "Chengdu" {
		t.Fatalf("location %+v", full.GetLocation())
	}
	if !setupChat(t, ctx, owner, id, hash).GetHasGeo() {
		t.Fatal("has_geo")
	}
}

// TestMigrateKeepsAdmin is ISSUE-S-MIGRATE-ADMIN-001 (row 188).
// Phones 9996610458–459. Photo copy is covered by TestMigrateKeepsAdminAndPhoto.
func TestMigrateKeepsAdmin(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	owner, _ := dialPhone(t, ctx, cfg, "9996610458", "Own458")
	_, _ = dialPhone(t, ctx, cfg, "9996610459", "Mem459")
	memberID, memberHash := setupImport(t, ctx, owner, "9996610459", "Mem459")
	chatID, err := owner.CreateChat(ctx, "BasicRoom", inputUser(memberID, memberHash))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = owner.Invoke(ctx, &mtproto.TLMessagesEditChatAdmin{
		Constructor: mtproto.CRC32_messages_editChatAdmin,
		ChatId:      chatID,
		UserId:      inputUser(memberID, memberHash),
		IsAdmin:     mtproto.ToBool(true),
	}); err != nil {
		t.Fatal(err)
	}
	migrated, err := owner.Invoke(ctx, &mtproto.TLMessagesMigrateChat{
		Constructor: mtproto.CRC32_messages_migrateChat,
		ChatId:      chatID,
	})
	if err != nil {
		t.Fatal(err)
	}
	channelID, channelHash := megagroupOf(t, migrated)
	part, err := owner.Invoke(ctx, &mtproto.TLChannelsGetParticipant{
		Constructor: mtproto.CRC32_channels_getParticipant,
		Channel:     inputChannel(channelID, channelHash),
		Participant: inputPeerUser(memberID, memberHash),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := participantName(t, part); got != mtproto.Predicate_channelParticipantAdmin {
		t.Fatalf("migrated admin %s", got)
	}
}

// TestForumGeneralStaysSingle is ISSUE-S-TOPIC-GENERAL-DUP-001 (row 260).
// Phone 9996610457.
func TestForumGeneralStaysSingle(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	owner, _ := dialPhone(t, ctx, cfg, "9996610457", "Own457")
	id, hash, err := owner.CreateMegagroup(ctx, "TopicRoom")
	if err != nil {
		t.Fatal(err)
	}
	channel := inputChannel(id, hash)
	peer := inputPeerChannel(id, hash)
	toggleForum(t, ctx, owner, channel, true)
	editGeneralHidden(t, ctx, owner, peer, true)
	editGeneralHidden(t, ctx, owner, peer, false)
	toggleForum(t, ctx, owner, channel, false)
	toggleForum(t, ctx, owner, channel, true)
	topics := forumTopicList(t, ctx, owner, peer)
	n := 0
	for _, topic := range topics {
		if topic.GetTitle() == "General" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("general topics %d", n)
	}
}

func setupImport(t *testing.T, ctx context.Context, c *Client, number, name string) (int64, int64) {
	t.Helper()
	id, hash, err := c.ImportPhone(ctx, number, name)
	if err != nil {
		t.Fatal(err)
	}
	return id, hash
}

func setupFull(t *testing.T, ctx context.Context, c *Client, id, hash int64) *mtproto.ChatFull {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLChannelsGetFullChannel{
		Constructor: mtproto.CRC32_channels_getFullChannel,
		Channel:     inputChannel(id, hash),
	})
	if err != nil {
		t.Fatal(err)
	}
	return asChatFull(t, obj)
}

func setupChat(t *testing.T, ctx context.Context, c *Client, id, hash int64) *mtproto.Chat {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLChannelsGetFullChannel{
		Constructor: mtproto.CRC32_channels_getFullChannel,
		Channel:     inputChannel(id, hash),
	})
	if err != nil {
		t.Fatal(err)
	}
	switch v := obj.(type) {
	case *mtproto.TLMessagesChatFull:
		for _, chat := range v.To_Messages_ChatFull().GetChats() {
			if chat.GetId() == id {
				return chat
			}
		}
	case *mtproto.Messages_ChatFull:
		for _, chat := range v.GetChats() {
			if chat.GetId() == id {
				return chat
			}
		}
	}
	t.Fatal("chat missing")
	return nil
}

func megagroupOf(t *testing.T, obj mtproto.TLObject) (int64, int64) {
	t.Helper()
	for _, chat := range allChats(obj) {
		if chat != nil && chat.GetMegagroup() {
			return chat.GetId(), chatAccessHash(chat)
		}
	}
	t.Fatalf("no megagroup in %T", obj)
	return 0, 0
}

func allChats(obj mtproto.TLObject) []*mtproto.Chat {
	if chats := chatsOf(obj); len(chats) > 0 {
		return chats
	}
	switch v := obj.(type) {
	case *mtproto.TLUpdates:
		return v.GetChats()
	case *mtproto.Updates:
		return v.GetChats()
	default:
		return nil
	}
}

func participantName(t *testing.T, obj mtproto.TLObject) string {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.TLChannelsChannelParticipant:
		return v.GetParticipant().GetPredicateName()
	case *mtproto.Channels_ChannelParticipant:
		return v.GetParticipant().GetPredicateName()
	default:
		t.Fatalf("participant %T", obj)
		return ""
	}
}

func toggleForum(t *testing.T, ctx context.Context, c *Client, channel *mtproto.InputChannel, enabled bool) {
	t.Helper()
	if _, err := c.Invoke(ctx, &mtproto.TLChannelsToggleForum{
		Constructor: mtproto.CRC32_channels_toggleForum_3ff75734,
		Channel:     channel,
		Enabled:     mtproto.ToBool(enabled),
		Tabs:        mtproto.ToBool(false),
	}); err != nil {
		t.Fatal(err)
	}
}

func editGeneralHidden(t *testing.T, ctx context.Context, c *Client, peer *mtproto.InputPeer, hidden bool) {
	t.Helper()
	if _, err := c.Invoke(ctx, &mtproto.TLMessagesEditForumTopic{
		Constructor: mtproto.CRC32_messages_editForumTopic,
		Peer:        peer,
		TopicId:     1,
		Hidden:      mtproto.ToBool(hidden),
	}); err != nil {
		t.Fatal(err)
	}
}

func forumTopicList(t *testing.T, ctx context.Context, c *Client, peer *mtproto.InputPeer) []*mtproto.ForumTopic {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLMessagesGetForumTopics{
		Constructor: mtproto.CRC32_messages_getForumTopics,
		Peer:        peer,
		Limit:       50,
	})
	if err != nil {
		t.Fatal(err)
	}
	switch v := obj.(type) {
	case *mtproto.TLMessagesForumTopics:
		return v.GetTopics()
	case *mtproto.Messages_ForumTopics:
		return v.GetTopics()
	default:
		t.Fatalf("topics %T", obj)
		return nil
	}
}
