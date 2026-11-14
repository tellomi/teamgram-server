package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestEditMessageKeepsBold(t *testing.T) {
	cfg := requireE2E(t)
	a, aSelf := loginNamed(t, cfg, ProfileIOS, "nsem_a", "NsemA")
	b, bSelf := loginNamed(t, cfg, ProfileAndroid, "nsem_b", "NsemB")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	peerForA := inputPeerUser(bSelf.ID, bSelf.AccessHash)
	text := fmt.Sprintf("bold-%d", time.Now().UnixNano())
	id, err := a.SendText(ctx, peerForA, text, nil)
	if err != nil {
		t.Fatal(err)
	}
	edited, err := a.Invoke(ctx, &mtproto.TLMessagesEditMessage{
		Constructor: mtproto.CRC32_messages_editMessage_b106e66c,
		Peer:        peerForA,
		Id:          id,
		Message:     &wrapperspb.StringValue{Value: text},
		Entities:    []*mtproto.MessageEntity{entityBold(0, int32(len(text)))},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !messageHasBold(allMessages(edited), text) {
		t.Fatalf("edit result dropped bold: %T", edited)
	}
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		msgs, err := a.History(ctx, peerForA, 20)
		if err != nil {
			return false, err
		}
		return messageHasBold(msgs, text), nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		msgs, err := b.History(ctx, inputPeerUser(aSelf.ID, aSelf.AccessHash), 20)
		if err != nil {
			return false, err
		}
		return messageHasBold(msgs, text), nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSupergroupCannotEditOthers(t *testing.T) {
	cfg := requireE2E(t)
	a, _ := loginNamed(t, cfg, ProfileIOS, "nsem_a", "NsemA")
	b, bSelf := loginNamed(t, cfg, ProfileAndroid, "nsem_b", "NsemB")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	title := fmt.Sprintf("edit-room-%d", time.Now().UnixNano())
	chID, chHash, err := a.CreateMegagroup(ctx, title)
	if err != nil {
		t.Fatal(err)
	}
	channel := inputChannel(chID, chHash)
	if err = a.InviteToChannel(ctx, channel, inputUser(bSelf.ID, bSelf.AccessHash)); err != nil {
		t.Fatal(err)
	}
	peer := inputPeerChannel(chID, chHash)
	text := fmt.Sprintf("member-%d", time.Now().UnixNano())
	var id int32
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		id, err = b.SendText(ctx, peer, text, nil)
		return err == nil && id != 0, err
	}); err != nil {
		t.Fatal(err)
	}
	_, err = a.Invoke(ctx, &mtproto.TLMessagesEditMessage{
		Constructor: mtproto.CRC32_messages_editMessage_b106e66c,
		Peer:        peer,
		Id:          id,
		Message:     &wrapperspb.StringValue{Value: text + "!"},
	})
	wantRPC(t, err, 403, "MESSAGE_AUTHOR_REQUIRED")
	if _, err = b.Invoke(ctx, &mtproto.TLMessagesEditMessage{
		Constructor: mtproto.CRC32_messages_editMessage_b106e66c,
		Peer:        peer,
		Id:          id,
		Message:     &wrapperspb.StringValue{Value: text + "!"},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestExpiredMuteIsUnrestricted(t *testing.T) {
	cfg := requireE2E(t)
	a, _ := loginNamed(t, cfg, ProfileIOS, "nsem_a", "NsemA")
	b, bSelf := loginNamed(t, cfg, ProfileAndroid, "nsem_b", "NsemB")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	title := fmt.Sprintf("mute-room-%d", time.Now().UnixNano())
	chID, chHash, err := a.CreateMegagroup(ctx, title)
	if err != nil {
		t.Fatal(err)
	}
	channel := inputChannel(chID, chHash)
	if err = a.InviteToChannel(ctx, channel, inputUser(bSelf.ID, bSelf.AccessHash)); err != nil {
		t.Fatal(err)
	}
	until := int32(time.Now().Unix()) + 2
	if _, err = a.Invoke(ctx, &mtproto.TLChannelsEditBanned{
		Constructor: mtproto.CRC32_channels_editBanned,
		Channel:     channel,
		Participant: inputPeerUser(bSelf.ID, bSelf.AccessHash),
		BannedRights: mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{
			SendMessages: true,
			UntilDate:    until,
		}).To_ChatBannedRights(),
	}); err != nil {
		t.Fatal(err)
	}
	peer := inputPeerChannel(chID, chHash)
	if _, err = b.SendText(ctx, peer, "while-muted", nil); err == nil {
		t.Fatal("mute still accepted a message")
	}
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-time.After(3 * time.Second):
	}
	full, err := b.Invoke(ctx, &mtproto.TLChannelsGetFullChannel{
		Constructor: mtproto.CRC32_channels_getFullChannel,
		Channel:     channel,
	})
	if err != nil {
		t.Fatal(err)
	}
	if br := bannedRightsOf(full, chID); br != nil && br.GetSendMessages() {
		t.Fatalf("expired banned_rights = %#v", br)
	}
	if _, err = b.SendText(ctx, peer, fmt.Sprintf("after-mute-%d", time.Now().UnixNano()), nil); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateColorRejectsUnknown(t *testing.T) {
	cfg := requireE2E(t)
	a, _ := loginNamed(t, cfg, ProfileIOS, "color_a", "ColorA")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	colors, err := a.Invoke(ctx, &mtproto.TLHelpGetPeerColors{
		Constructor: mtproto.CRC32_help_getPeerColors,
		Hash:        0,
	})
	if err != nil {
		t.Fatal(err)
	}
	list, ok := colors.(*mtproto.TLHelpPeerColors)
	if !ok || list.GetData2() == nil {
		t.Fatalf("help.getPeerColors %T", colors)
	}
	var saw0 bool
	for _, opt := range list.GetData2().GetColors() {
		if opt.GetColorId() == 999 {
			t.Fatal("help.getPeerColors listed color 999")
		}
		if opt.GetColorId() == 0 {
			saw0 = true
		}
	}
	if !saw0 {
		t.Fatal("help.getPeerColors did not list color 0")
	}
	_, err = a.Invoke(ctx, &mtproto.TLAccountUpdateColor{
		Constructor: mtproto.CRC32_account_updateColor_684d214e,
		Color_FLAGPEERCOLOR: mtproto.MakeTLPeerColor(&mtproto.PeerColor{
			Color: &wrapperspb.Int32Value{Value: 999},
		}).To_PeerColor(),
	})
	wantRPC(t, err, 400, "COLOR_INVALID")
	if _, err = a.Invoke(ctx, &mtproto.TLAccountUpdateColor{
		Constructor: mtproto.CRC32_account_updateColor_684d214e,
		Color_FLAGPEERCOLOR: mtproto.MakeTLPeerColor(&mtproto.PeerColor{
			Color: &wrapperspb.Int32Value{Value: 0},
		}).To_PeerColor(),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestStoryViewBlockedFlag(t *testing.T) {
	cfg := requireE2E(t)
	a, aSelf := loginNamed(t, cfg, ProfileIOS, "storyblk_a", "StoryA")
	b, bSelf := loginNamed(t, cfg, ProfileAndroid, "storyblk_b", "StoryB")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	peerForA := inputPeerUser(bSelf.ID, bSelf.AccessHash)
	t.Cleanup(func() {
		clean, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_, _ = a.Invoke(clean, &mtproto.TLContactsSetBlocked{
			Constructor:   mtproto.CRC32_contacts_setBlocked,
			MyStoriesFrom: true,
			Id:            []*mtproto.InputPeer{},
			Limit:         0,
		})
	})
	fileID := uploadBytes(t, a, testJPEG(t, 320, 240, 41))
	up := invokeOK(t, a, &mtproto.TLStoriesSendStory{
		Constructor: mtproto.CRC32_stories_sendStory_8f9e6898,
		Peer:        inputPeerSelf(),
		Media: mtproto.MakeTLInputMediaUploadedPhoto(&mtproto.InputMedia{
			File: mtproto.MakeTLInputFile(&mtproto.InputFile{
				Id_INT64: fileID,
				Parts:    1,
				Name:     "story.jpg",
			}).To_InputFile(),
		}).To_InputMedia(),
		PrivacyRules: []*mtproto.InputPrivacyRule{
			mtproto.MakeTLInputPrivacyValueAllowAll(nil).To_InputPrivacyRule(),
		},
		RandomId: randomID(),
		Period:   &wrapperspb.Int32Value{Value: 86400},
	})
	storyID := nativeStoryID(t, up)
	if _, err := b.Invoke(ctx, &mtproto.TLStoriesIncrementStoryViews{
		Constructor: mtproto.CRC32_stories_incrementStoryViews_b2028afb,
		Peer:        inputPeerUser(aSelf.ID, aSelf.AccessHash),
		Id:          []int32{storyID},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Invoke(ctx, &mtproto.TLContactsSetBlocked{
		Constructor:   mtproto.CRC32_contacts_setBlocked,
		MyStoriesFrom: true,
		Id:            []*mtproto.InputPeer{peerForA},
		Limit:         1,
	}); err != nil {
		t.Fatal(err)
	}
	var flagged bool
	if err := waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		res, err := a.Invoke(ctx, &mtproto.TLStoriesGetStoryViewsList{
			Constructor: mtproto.CRC32_stories_getStoryViewsList_7ed23c57,
			Peer:        inputPeerSelf(),
			Id:          storyID,
			Limit:       20,
		})
		if err != nil {
			return false, err
		}
		list, ok := res.(*mtproto.TLStoriesStoryViewsList)
		if !ok || list.GetData2() == nil {
			return false, fmt.Errorf("stories.getStoryViewsList %T", res)
		}
		for _, view := range list.GetData2().GetViews() {
			if view.GetUserId() == bSelf.ID {
				flagged = view.GetBlockedMyStoriesFrom()
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}
	if !flagged {
		t.Fatal("storyView.blocked_my_stories_from is false")
	}
}

func TestHistoryTTLPeriodStaysFixed(t *testing.T) {
	cfg := requireE2E(t)
	a, aSelf := loginNamed(t, cfg, ProfileIOS, "ttlfix_a", "TtlA")
	b, bSelf := loginNamed(t, cfg, ProfileAndroid, "ttlfix_b", "TtlB")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	peerForA := inputPeerUser(bSelf.ID, bSelf.AccessHash)
	if err := setHistoryTTL(ctx, a, peerForA, 86400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		clean, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = setHistoryTTL(clean, a, peerForA, 0)
	})
	text := fmt.Sprintf("ttl-fixed-%d", time.Now().UnixNano())
	if _, err := a.SendText(ctx, peerForA, text, nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-time.After(2 * time.Second):
	}
	msgs, err := a.History(ctx, peerForA, 20)
	if err != nil {
		t.Fatal(err)
	}
	period := ttlOfText(msgs, text)
	if period != 86400 {
		t.Fatalf("sender ttl_period = %d, want 86400", period)
	}
	msgs, err = b.History(ctx, inputPeerUser(aSelf.ID, aSelf.AccessHash), 20)
	if err != nil {
		t.Fatal(err)
	}
	if period = ttlOfText(msgs, text); period != 86400 {
		t.Fatalf("recipient ttl_period = %d, want 86400", period)
	}
}

func nativeStoryID(t *testing.T, obj mtproto.TLObject) int32 {
	t.Helper()
	var updates []*mtproto.Update
	switch u := obj.(type) {
	case *mtproto.TLUpdates:
		updates = u.GetUpdates()
	case *mtproto.TLUpdatesCombined:
		updates = u.GetUpdates()
	default:
		t.Fatalf("stories.sendStory returned %T", obj)
	}
	for _, up := range updates {
		if up.GetPredicateName() == mtproto.Predicate_updateStoryID && up.GetId_INT32() != 0 {
			return up.GetId_INT32()
		}
	}
	t.Fatal("stories.sendStory did not return updateStoryID")
	return 0
}

func messageHasBold(msgs []*mtproto.Message, text string) bool {
	for _, msg := range msgs {
		if msg.GetMessage() != text {
			continue
		}
		for _, entity := range msg.GetEntities() {
			if entity.GetPredicateName() == mtproto.Predicate_messageEntityBold && entity.GetLength() == int32(len(text)) {
				return true
			}
		}
	}
	return false
}

func ttlOfText(msgs []*mtproto.Message, text string) int32 {
	for _, msg := range msgs {
		if msg.GetMessage() == text {
			return msg.GetTtlPeriod().GetValue()
		}
	}
	return -1
}

func bannedRightsOf(obj mtproto.TLObject, channelID int64) *mtproto.ChatBannedRights {
	for _, chat := range chatsOf(obj) {
		if chat.GetId() == channelID {
			return chat.GetBannedRights()
		}
	}
	return nil
}
