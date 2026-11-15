package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestChannelToggleUsername is channels.toggleUsername: active=false stops
// contacts.resolveUsername, username.active reads back false, another session
// receives updateChannel, and active=true succeeds.
func TestChannelToggleUsername(t *testing.T) {
	cfg := requireE2E(t)
	owner, _ := loginNamed(t, cfg, ProfileIOS, "chan_user_a", "ChanA")
	other, _ := loginNamed(t, cfg, ProfileAndroid, "chan_user_a", "ChanA")
	viewer, _ := loginNamed(t, cfg, ProfileAndroid, "chan_user_b", "ChanB")
	ctx, cancel := contextTimeout(90 * time.Second)
	defer cancel()

	id, hash, err := owner.CreateMegagroup(ctx, "LinkBoard")
	if err != nil {
		t.Fatal(err)
	}
	name := "lb" + randomUsername()
	channel := inputChannel(id, hash)
	if _, err := owner.Invoke(ctx, &mtproto.TLChannelsUpdateUsername{
		Constructor: mtproto.CRC32_channels_updateUsername,
		Channel:     channel,
		Username:    name,
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := other.State(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Invoke(ctx, &mtproto.TLChannelsToggleUsername{
		Constructor: mtproto.CRC32_channels_toggleUsername,
		Channel:     channel,
		Username:    name,
		Active:      mtproto.BoolFalse,
	}); err != nil {
		t.Fatal(err)
	}

	// updateChannel has no pts. This server delivers it on the live socket
	// (sync.pushUpdatesIfNot). updates.getDifference does not replay it.
	if err := waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		return pushedUpdateChannel(other.Pushed(), id), nil
	}); err != nil {
		t.Fatalf("updateChannel for %d: %v", id, err)
	}

	chat := channelFrom(t, invokeOK(t, owner, &mtproto.TLChannelsGetChannels{
		Constructor: mtproto.CRC32_channels_getChannels,
		Id:          []*mtproto.InputChannel{channel},
	}), id)
	if chat.GetUsername().GetValue() != "" {
		t.Fatalf("primary username still %q", chat.GetUsername().GetValue())
	}
	found, active := usernameState(chat, name)
	if !found || active {
		t.Fatalf("username.active after deactivate found=%v active=%v names=%v", found, active, chat.GetUsernames())
	}
	_, err = viewer.Invoke(ctx, &mtproto.TLContactsResolveUsername{
		Constructor: mtproto.CRC32_contacts_resolveUsername_725afbbc,
		Username:    name,
	})
	if rpc, ok := asRPCError(err); !ok || rpc.Message != "USERNAME_NOT_OCCUPIED" {
		t.Fatalf("resolve while inactive: %v", err)
	}

	if _, err := owner.Invoke(ctx, &mtproto.TLChannelsToggleUsername{
		Constructor: mtproto.CRC32_channels_toggleUsername,
		Channel:     channel,
		Username:    name,
		Active:      mtproto.BoolTrue,
	}); err != nil {
		t.Fatal(err)
	}
	resolved, err := viewer.Invoke(ctx, &mtproto.TLContactsResolveUsername{
		Constructor: mtproto.CRC32_contacts_resolveUsername_725afbbc,
		Username:    name,
	})
	if err != nil {
		t.Fatal(err)
	}
	peer, _ := resolved.(*mtproto.TLContactsResolvedPeer)
	if peer == nil || peer.GetPeer().GetChannelId() != id {
		t.Fatalf("resolve after activate %T %+v", resolved, resolved)
	}
	again := channelFrom(t, invokeOK(t, owner, &mtproto.TLChannelsGetChannels{
		Constructor: mtproto.CRC32_channels_getChannels,
		Id:          []*mtproto.InputChannel{channel},
	}), id)
	found, active = usernameState(again, name)
	if !found || !active || again.GetUsername().GetValue() != name {
		t.Fatalf("reactivated chat username=%q names=%v", again.GetUsername().GetValue(), again.GetUsernames())
	}
	_, err = owner.Invoke(ctx, &mtproto.TLChannelsToggleUsername{
		Constructor: mtproto.CRC32_channels_toggleUsername,
		Channel:     channel,
		Username:    name,
		Active:      mtproto.BoolTrue,
	})
	if rpc, ok := asRPCError(err); !ok || rpc.Message != "USERNAME_NOT_MODIFIED" {
		t.Fatalf("second activate: %v", err)
	}
}

// TestMegagroupStickersReadBack is channels.setStickers: the owner and a
// member both read channelFull.stickerset, and only the owner has can_set_stickers.
func TestMegagroupStickersReadBack(t *testing.T) {
	cfg := requireE2E(t)
	owner, self := loginNamed(t, cfg, ProfileIOS, "chan_user_a", "ChanA")
	member, memberSelf := loginNamed(t, cfg, ProfileAndroid, "chan_user_b", "ChanB")
	ctx, cancel := contextTimeout(90 * time.Second)
	defer cancel()

	id, hash, err := owner.CreateMegagroup(ctx, "PackBoard")
	if err != nil {
		t.Fatal(err)
	}
	channel := inputChannel(id, hash)
	if err := owner.InviteToChannel(ctx, channel, inputUser(memberSelf.ID, memberSelf.AccessHash)); err != nil {
		t.Fatal(err)
	}
	doc := uploadStickerDocument(t, owner, "image/webp", "sticker.webp", tinyStickerWEBP)
	created := createStickerSet(t, owner, self, "", []*mtproto.InputStickerSetItem{stickerItem(doc, "🫥")})
	set := created.GetSet()
	if _, err := owner.Invoke(ctx, &mtproto.TLChannelsSetStickers{
		Constructor: mtproto.CRC32_channels_setStickers,
		Channel:     channel,
		Stickerset: mtproto.MakeTLInputStickerSetID(&mtproto.InputStickerSet{
			Id:         set.GetId(),
			AccessHash: set.GetAccessHash(),
		}).To_InputStickerSet(),
	}); err != nil {
		t.Fatal(err)
	}

	ownerFull := asChatFull(t, invokeOK(t, owner, &mtproto.TLChannelsGetFullChannel{
		Constructor: mtproto.CRC32_channels_getFullChannel,
		Channel:     channel,
	}))
	if !ownerFull.GetCanSetStickers() || ownerFull.GetStickerset().GetId() != set.GetId() || ownerFull.GetStickerset().GetShortName() == "" {
		t.Fatalf("owner full can=%v set=%+v", ownerFull.GetCanSetStickers(), ownerFull.GetStickerset())
	}
	memberFull := asChatFull(t, invokeOK(t, member, &mtproto.TLChannelsGetFullChannel{
		Constructor: mtproto.CRC32_channels_getFullChannel,
		Channel:     channel,
	}))
	if memberFull.GetCanSetStickers() || memberFull.GetStickerset().GetId() != set.GetId() {
		t.Fatalf("member full can=%v set=%+v", memberFull.GetCanSetStickers(), memberFull.GetStickerset())
	}
}

// TestSearchResultsCalendarMediaFilters is messages.getSearchResultsCalendar
// and getSearchResultsPositions. Media filters are accepted. A url message
// shows up in both.
func TestSearchResultsCalendarMediaFilters(t *testing.T) {
	cfg := requireE2E(t)
	c, _ := loginNamed(t, cfg, ProfileIOS, "search_cal", "SearchCal")
	ctx, cancel := contextTimeout(60 * time.Second)
	defer cancel()

	text := "notes https://example.com/packboard"
	url := "https://example.com/packboard"
	msgID, err := c.SendText(ctx, inputPeerSelf(), text, []*mtproto.MessageEntity{
		mtproto.MakeTLMessageEntityUrl(&mtproto.MessageEntity{
			Offset: int32(strings.Index(text, url)),
			Length: int32(len(url)),
		}).To_MessageEntity(),
	})
	if err != nil {
		t.Fatal(err)
	}
	peer := inputPeerSelf()
	filters := []*mtproto.MessagesFilter{
		mtproto.MakeTLInputMessagesFilterPhotos(nil).To_MessagesFilter(),
		mtproto.MakeTLInputMessagesFilterVideo(nil).To_MessagesFilter(),
		mtproto.MakeTLInputMessagesFilterPhotoVideo(nil).To_MessagesFilter(),
		mtproto.MakeTLInputMessagesFilterDocument(nil).To_MessagesFilter(),
		mtproto.MakeTLInputMessagesFilterMusic(nil).To_MessagesFilter(),
		mtproto.MakeTLInputMessagesFilterVoice(nil).To_MessagesFilter(),
		mtproto.MakeTLInputMessagesFilterGif(nil).To_MessagesFilter(),
		mtproto.MakeTLInputMessagesFilterUrl(nil).To_MessagesFilter(),
		{Constructor: mtproto.CRC32_inputMessagesFilterPhotos},
	}
	for _, filter := range filters {
		if _, err := c.Invoke(ctx, &mtproto.TLMessagesGetSearchResultsCalendar{
			Constructor: mtproto.CRC32_messages_getSearchResultsCalendar_6aa3f6bd,
			Peer:        peer,
			Filter:      filter,
		}); err != nil {
			t.Fatalf("calendar %s: %v", filter.GetPredicateName(), err)
		}
		if _, err := c.Invoke(ctx, &mtproto.TLMessagesGetSearchResultsPositions{
			Constructor: mtproto.CRC32_messages_getSearchResultsPositions_9c7f2f10,
			Peer:        peer,
			Filter:      filter,
			Limit:       10,
		}); err != nil {
			t.Fatalf("positions %s: %v", filter.GetPredicateName(), err)
		}
	}
	_, err = c.Invoke(ctx, &mtproto.TLMessagesGetSearchResultsCalendar{
		Constructor: mtproto.CRC32_messages_getSearchResultsCalendar_6aa3f6bd,
		Peer:        peer,
		Filter:      mtproto.MakeTLInputMessagesFilterEmpty(nil).To_MessagesFilter(),
	})
	if rpc, ok := asRPCError(err); !ok || rpc.Message != "FILTER_NOT_SUPPORTED" {
		t.Fatalf("empty filter: %v", err)
	}

	urlFilter := mtproto.MakeTLInputMessagesFilterUrl(nil).To_MessagesFilter()
	cal := searchCalendar(t, invokeOK(t, c, &mtproto.TLMessagesGetSearchResultsCalendar{
		Constructor: mtproto.CRC32_messages_getSearchResultsCalendar_6aa3f6bd,
		Peer:        peer,
		Filter:      urlFilter,
	}))
	if cal.GetCount() < 1 || !calendarHas(cal, msgID) {
		t.Fatalf("calendar count=%d messages=%v", cal.GetCount(), cal.GetMessages())
	}
	pos := searchPositions(t, invokeOK(t, c, &mtproto.TLMessagesGetSearchResultsPositions{
		Constructor: mtproto.CRC32_messages_getSearchResultsPositions_9c7f2f10,
		Peer:        peer,
		Filter:      urlFilter,
		Limit:       10,
	}))
	if pos.GetCount() < 1 || !positionsHave(pos, msgID) {
		t.Fatalf("positions count=%d %+v", pos.GetCount(), pos.GetPositions())
	}
}

func usernameState(chat *mtproto.Chat, name string) (found, active bool) {
	for _, row := range chat.GetUsernames() {
		if row != nil && strings.EqualFold(row.GetUsername(), name) {
			return true, row.GetActive()
		}
	}
	return false, false
}

func channelFrom(t *testing.T, obj mtproto.TLObject, id int64) *mtproto.Chat {
	t.Helper()
	for _, chat := range chatsIn(obj) {
		if chat != nil && chat.GetId() == id {
			return chat
		}
	}
	t.Fatalf("channel %d missing in %T", id, obj)
	return nil
}

func chatsIn(obj mtproto.TLObject) []*mtproto.Chat {
	if chats := chatsOf(obj); len(chats) > 0 {
		return chats
	}
	switch v := obj.(type) {
	case *mtproto.TLMessagesChats:
		return v.To_Messages_Chats().GetChats()
	case *mtproto.TLMessagesChatsSlice:
		return v.To_Messages_Chats().GetChats()
	case *mtproto.TLMessagesChatFull:
		return v.To_Messages_ChatFull().GetChats()
	case *mtproto.Messages_Chats:
		return v.GetChats()
	case *mtproto.Messages_ChatFull:
		return v.GetChats()
	default:
		return nil
	}
}

func pushedUpdateChannel(objs []mtproto.TLObject, channelID int64) bool {
	for _, obj := range objs {
		if differenceHasUpdateChannel(obj, channelID) {
			return true
		}
		for _, up := range updatesOf(obj) {
			if up != nil && up.GetPredicateName() == mtproto.Predicate_updateChannel && up.GetChannelId() == channelID {
				return true
			}
		}
	}
	return false
}

func differenceHasUpdateChannel(obj mtproto.TLObject, channelID int64) bool {
	var ups []*mtproto.Update
	switch v := obj.(type) {
	case *mtproto.TLUpdatesDifference:
		ups = v.GetOtherUpdates()
	case *mtproto.TLUpdatesDifferenceSlice:
		ups = v.GetOtherUpdates()
	case *mtproto.Updates_Difference:
		ups = v.GetOtherUpdates()
	default:
		ups = updatesOf(obj)
	}
	for _, up := range ups {
		if up != nil && up.GetPredicateName() == mtproto.Predicate_updateChannel && up.GetChannelId() == channelID {
			return true
		}
	}
	return false
}

func searchCalendar(t *testing.T, obj mtproto.TLObject) *mtproto.Messages_SearchResultsCalendar {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.TLMessagesSearchResultsCalendar:
		return v.To_Messages_SearchResultsCalendar()
	case *mtproto.Messages_SearchResultsCalendar:
		return v
	default:
		t.Fatalf("calendar %T", obj)
		return nil
	}
}

func searchPositions(t *testing.T, obj mtproto.TLObject) *mtproto.Messages_SearchResultsPositions {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.TLMessagesSearchResultsPositions:
		return v.To_Messages_SearchResultsPositions()
	case *mtproto.Messages_SearchResultsPositions:
		return v
	default:
		t.Fatalf("positions %T", obj)
		return nil
	}
}

func calendarHas(cal *mtproto.Messages_SearchResultsCalendar, id int32) bool {
	for _, msg := range cal.GetMessages() {
		if msg.GetId() == id {
			return true
		}
	}
	for _, period := range cal.GetPeriods() {
		if period.GetMinMsgId() <= id && id <= period.GetMaxMsgId() {
			return true
		}
	}
	return false
}

func positionsHave(pos *mtproto.Messages_SearchResultsPositions, id int32) bool {
	for _, row := range pos.GetPositions() {
		if row.GetMsgId() == id {
			return true
		}
	}
	return pos.GetCount() == 1
}
