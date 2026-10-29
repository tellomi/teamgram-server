package mtproto

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestUnregisteredServiceDoesNotBecomeInternal(t *testing.T) {
	err := status.Error(codes.Unimplemented, "unknown service mtproto.RPCStories")
	if !IsUnregisteredServiceError(err) {
		t.Fatal("expected unknown service")
	}
	got := NewRpcError(err)
	if got.GetErrorCode() != int32(ErrBadRequest) || got.GetErrorMessage() != "METHOD_NOT_IMPL" {
		t.Fatalf("rpc error = %d %s", got.GetErrorCode(), got.GetErrorMessage())
	}

	stickers, ok := StartupRPCResult(&TLMessagesGetAllStickers{})
	if !ok {
		t.Fatal("getAllStickers should have a typed fallback")
	}
	if stickers.(*Messages_AllStickers).GetPredicateName() != Predicate_messages_allStickersNotModified {
		t.Fatalf("stickers = %s", stickers.(*Messages_AllStickers).GetPredicateName())
	}
	mustEncodeTL(t, stickers)

	fallback := FallbackForUnregisteredService(&TLAccountUpdateColor{})
	rpcErr, ok := fallback.(*TLRpcError)
	if !ok || rpcErr.GetErrorMessage() != "METHOD_NOT_IMPL" || rpcErr.GetErrorCode() != 400 {
		t.Fatalf("fallback = %#v", fallback)
	}

	deadline := NewRpcError(status.Error(codes.DeadlineExceeded, "context deadline exceeded"))
	if deadline.GetErrorCode() != 500 || deadline.GetErrorMessage() != "INTERNAL" {
		t.Fatalf("deadline = %d %s", deadline.GetErrorCode(), deadline.GetErrorMessage())
	}
}

func TestStartupFallbacksEncode(t *testing.T) {
	samples := []TLObject{
		&TLHelpGetConfig{},
		&TLHelpGetNearestDc{},
		&TLHelpGetCdnConfig{},
		&TLHelpTest{},
		&TLHelpSaveAppLog{},
		&TLHelpGetAppUpdate{},
		&TLHelpGetPremiumPromo{},
		&TLHelpGetAppConfig61E3F854{},
		&TLHelpGetAppConfig98914110{},
		&TLPhoneGetCallConfig{},
		&TLLangpackGetDifference{LangCode: "en"},
		&TLAccountGetAutoDownloadSettings{},
		&TLUpdatesGetState{},
		&TLUpdatesGetDifference{},
		&TLUpdatesGetChannelDifference{Pts: 9},
		&TLMessagesReceivedQueue{},
		&TLMessagesReceivedMessages{},
		&TLMessagesGetDialogs{},
		&TLMessagesGetPinnedDialogs{},
		&TLMessagesGetPeerDialogs{},
		&TLMessagesGetPinnedSavedDialogs{},
		&TLMessagesGetDefaultHistoryTTL{},
		&TLMessagesGetAllStickers{},
		&TLMessagesGetEmojiStickers{},
		&TLMessagesGetMaskStickers{},
		&TLMessagesGetStickerSet{},
		&TLMessagesGetRecentStickers{},
		&TLMessagesGetFavedStickers{},
		&TLMessagesGetSavedGifs{},
		&TLMessagesGetFeaturedStickers{},
		&TLMessagesGetEmojiKeywords{LangCode: "en"},
		&TLMessagesGetEmojiKeywordsDifference{LangCode: "en", FromVersion: 3},
		&TLMessagesGetEmojiKeywordsLanguages{},
		&TLMessagesGetEmojiGroups{},
		&TLMessagesGetEmojiStatusGroups{},
		&TLMessagesGetEmojiProfilePhotoGroups{},
		&TLMessagesGetEmojiStickerGroups{},
		&TLAccountGetDefaultEmojiStatuses{},
		&TLAccountGetRecentEmojiStatuses{},
		&TLAccountGetCollectibleEmojiStatuses{},
		&TLAccountGetChannelDefaultEmojiStatuses{},
		&TLAccountGetDefaultProfilePhotoEmojis{},
		&TLAccountGetDefaultGroupPhotoEmojis{},
		&TLAccountGetDefaultBackgroundEmojis{},
		&TLAccountGetChannelRestrictedStatusEmojis{},
		&TLMessagesGetTopReactions{},
		&TLMessagesGetRecentReactions{},
		&TLMessagesGetDefaultTagReactions{},
		&TLMessagesGetSavedReactionTags{},
		&TLMessagesGetEmojiGameInfo{},
		&TLAccountGetNotifySettings{},
		&TLAccountGetContactSignUpNotification{},
		&TLAccountGetReactionsNotifySettings{},
		&TLUsersGetFullUser{},
		&TLContactsGetContacts{},
		&TLMessagesGetUnreadMentions{},
		&TLMessagesGetScheduledMessages{},
		&TLMessagesGetQuickReplyMessages{},
		&TLStoriesGetAllStories{},
		&TLStoriesGetPinnedStories{},
		&TLStoriesSendStory{},
		&TLStoriesGetPeerMaxIDs78499170{},
	}
	for _, sample := range samples {
		result, ok := StartupRPCResult(sample)
		if !ok || result == nil {
			t.Fatalf("%T: missing startup fallback", sample)
		}
		mustEncodeTL(t, result)
	}
}

func mustEncodeTL(t *testing.T, obj TLObject) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic encoding %T: %v", obj, r)
		}
	}()
	buf := NewEncodeBuf(256)
	if err := obj.Encode(buf, 232); err != nil {
		t.Fatalf("%T: %v", obj, err)
	}
	if buf.GetOffset() < 4 {
		t.Fatalf("%T: empty buffer", obj)
	}
}
