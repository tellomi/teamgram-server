package mtproto

import (
	"encoding/binary"
	"testing"
	"time"

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
		&TLAccountGetPasswordSettings{},
		&TLAccountGetTmpPassword{Period: 60},
		&TLAuthFinishPasskeyLogin{},
		&TLMessagesGetDialogFiltersEFD48C89{},
		&TLMessagesGetDialogFiltersF19ED96D{},
		&TLMessagesGetSuggestedDialogFilters{},
		&TLMessagesGetPaidReactionPrivacy{},
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

func TestP0StartupMethods(t *testing.T) {
	const (
		crcPasswordSettings           = 0x9a5c33e5
		crcTmpPassword                = 0xdb64fd34
		crcReactionsNotify            = 0x71e4ea58
		crcAuthSignUpRequired         = 0x44747e9a
		crcDialogFilters              = 0x2ad93719
		crcVector                     = 0x1cb5c415
		crcUpdates                    = 0x74ae4240
		crcUpdatePaidReactionPrivacy  = 0x8b725fce
		crcPaidReactionPrivacyDefault = 0x206ad49e
	)
	for _, sample := range []struct {
		in  TLObject
		msg string
	}{
		{&TLAccountUpdatePasswordSettings{}, "PASSWORD_SETTINGS_UNAVAILABLE"},
		{&TLAccountConfirmPasswordEmail{}, "CODE_INVALID"},
		{&TLAccountResendPasswordEmail{}, "EMAIL_NOT_SETUP"},
		{&TLAccountCancelPasswordEmail{}, "EMAIL_NOT_SETUP"},
		{&TLAccountDeclinePasswordReset{}, "PASSWORD_RECOVERY_NA"},
	} {
		got, ok := StartupRPCResult(sample.in)
		if !ok {
			t.Fatalf("%T: missing result", sample.in)
		}
		rpc, isErr := got.(*TLRpcError)
		if !isErr || rpc.GetErrorCode() != 400 || rpc.GetErrorMessage() != sample.msg {
			t.Fatalf("%T: %#v", sample.in, got)
		}
		mustEncodeTL(t, got)
	}
	for _, sample := range []TLObject{
		&TLMessagesUpdateDialogFilter{},
		&TLMessagesUpdateDialogFiltersOrder{},
		&TLMessagesToggleDialogFilterTags{},
		&TLAccountSetReactionsNotifySettings{},
	} {
		if _, ok := StartupRPCResult(sample); ok {
			t.Fatalf("%T: unauthenticated fallback must not acknowledge a write", sample)
		}
	}

	settings := mustP0(t, &TLAccountGetPasswordSettings{})
	if settings.(*Account_PasswordSettings).GetPredicateName() != Predicate_account_passwordSettings {
		t.Fatal(settings.(*Account_PasswordSettings).GetPredicateName())
	}
	if settings.(*Account_PasswordSettings).GetEmail() != nil || settings.(*Account_PasswordSettings).GetSecureSettings() != nil {
		t.Fatal("password settings should carry no email and no secure settings")
	}
	assertCtor(t, &TLAccountGetPasswordSettings{}, settings, crcPasswordSettings)

	before := int32(time.Now().Unix())
	tmp := mustP0(t, &TLAccountGetTmpPassword{Period: 120})
	tmpPass := tmp.(*Account_TmpPassword)
	if tmpPass.GetPredicateName() != Predicate_account_tmpPassword {
		t.Fatal(tmpPass.GetPredicateName())
	}
	if len(tmpPass.GetTmpPassword()) != 0 {
		t.Fatalf("tmp password = %q", tmpPass.GetTmpPassword())
	}
	if tmpPass.GetValidUntil() < before+120 || tmpPass.GetValidUntil() > before+130 {
		t.Fatalf("valid_until = %d, now = %d", tmpPass.GetValidUntil(), before)
	}
	assertCtor(t, &TLAccountGetTmpPassword{}, tmp, crcTmpPassword)

	notify := mustP0(t, &TLAccountGetReactionsNotifySettings{})
	if notify.(*ReactionsNotifySettings).GetPredicateName() != Predicate_reactionsNotifySettings {
		t.Fatal(notify.(*ReactionsNotifySettings).GetPredicateName())
	}
	assertCtor(t, &TLAccountGetReactionsNotifySettings{}, notify, crcReactionsNotify)

	auth := mustP0(t, &TLAuthFinishPasskeyLogin{})
	if auth.(*Auth_Authorization).GetPredicateName() != Predicate_auth_authorizationSignUpRequired {
		t.Fatal(auth.(*Auth_Authorization).GetPredicateName())
	}
	if auth.(*Auth_Authorization).GetUser() != nil {
		t.Fatal("finishPasskeyLogin must not return a user")
	}
	assertCtor(t, &TLAuthFinishPasskeyLogin{}, auth, crcAuthSignUpRequired)

	filters := mustP0(t, &TLMessagesGetDialogFiltersEFD48C89{})
	dialogFilters := filters.(*Messages_DialogFilters)
	if dialogFilters.GetPredicateName() != Predicate_messages_dialogFilters {
		t.Fatal(dialogFilters.GetPredicateName())
	}
	if dialogFilters.GetTagsEnabled() || len(dialogFilters.GetFilters()) != 0 {
		t.Fatalf("filters = tags %v len %d", dialogFilters.GetTagsEnabled(), len(dialogFilters.GetFilters()))
	}
	assertCtor(t, &TLMessagesGetDialogFiltersEFD48C89{}, filters, crcDialogFilters)

	oldFilters := mustP0(t, &TLMessagesGetDialogFiltersF19ED96D{})
	if len(oldFilters.(*Vector_DialogFilter).GetDatas()) != 0 {
		t.Fatal("legacy getDialogFilters should be an empty vector")
	}
	assertCtor(t, &TLMessagesGetDialogFiltersF19ED96D{}, oldFilters, crcVector)

	suggested := mustP0(t, &TLMessagesGetSuggestedDialogFilters{})
	if len(suggested.(*Vector_DialogFilterSuggested).GetDatas()) != 0 {
		t.Fatal("suggested filters should be empty")
	}
	assertCtor(t, &TLMessagesGetSuggestedDialogFilters{}, suggested, crcVector)

	queue := mustP0(t, &TLMessagesReceivedQueue{MaxQts: 10})
	if len(queue.(*Vector_Long).GetDatas()) != 0 {
		t.Fatal("receivedQueue should be an empty vector")
	}
	assertCtor(t, &TLMessagesReceivedQueue{}, queue, crcVector)

	privacy := mustP0(t, &TLMessagesGetPaidReactionPrivacy{})
	up := privacy.(*Updates)
	if up.GetPredicateName() != Predicate_updates {
		t.Fatal(up.GetPredicateName())
	}
	if len(up.GetUpdates()) != 1 || up.GetUpdates()[0].GetPredicateName() != Predicate_updatePaidReactionPrivacy {
		t.Fatalf("updates = %#v", up.GetUpdates())
	}
	if up.GetUpdates()[0].GetPrivate_PAIDREACTIONPRIVACY().GetPredicateName() != Predicate_paidReactionPrivacyDefault {
		t.Fatal(up.GetUpdates()[0].GetPrivate_PAIDREACTIONPRIVACY().GetPredicateName())
	}
	buf := assertCtor(t, &TLMessagesGetPaidReactionPrivacy{}, privacy, crcUpdates)
	if !containsCtor(buf, crcUpdatePaidReactionPrivacy) || !containsCtor(buf, crcPaidReactionPrivacyDefault) {
		t.Fatal("paid reaction privacy update was not encoded")
	}
}

func mustP0(t *testing.T, sample TLObject) TLObject {
	t.Helper()
	got, ok := StartupRPCResult(sample)
	if !ok || got == nil {
		t.Fatalf("%T: missing startup result", sample)
	}
	if _, isErr := got.(*TLRpcError); isErr {
		t.Fatalf("%T: rpc error", sample)
	}
	mustEncodeTL(t, got)
	return got
}

func assertCtor(t *testing.T, sample, got TLObject, want uint32) []byte {
	t.Helper()
	buf := NewEncodeBuf(256)
	if err := got.Encode(buf, 232); err != nil {
		t.Fatalf("%T: %v", sample, err)
	}
	raw := buf.GetBuf()
	if len(raw) < 4 {
		t.Fatalf("%T: short buffer", sample)
	}
	if gotCtor := binary.LittleEndian.Uint32(raw[:4]); gotCtor != want {
		t.Fatalf("%T constructor = %#x, want %#x", sample, gotCtor, want)
	}
	return raw
}

func containsCtor(buf []byte, want uint32) bool {
	for i := 0; i+4 <= len(buf); i++ {
		if binary.LittleEndian.Uint32(buf[i:i+4]) == want {
			return true
		}
	}
	return false
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
