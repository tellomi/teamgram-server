package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestStripConstructor(t *testing.T) {
	cases := map[string]string{
		"sendCode":              "sendCode",
		"getAppConfig61E3F854":  "getAppConfig",
		"deleteHistory9BAA9647": "deleteHistory",
		"verifyEmail32DA4CF":    "verifyEmail",
		"createChat34A818":      "createChat",
		"addChatUserCBC6D107":   "addChatUser",
		"getAccountTTL":         "getAccountTTL",
		"getMessagesViews":      "getMessagesViews",
	}
	for in, want := range cases {
		if got := stripConstructor(in); got != want {
			t.Errorf("stripConstructor(%s) = %s, want %s", in, got, want)
		}
	}
	if got := canonicalGRPC("account_getAppConfig61E3F854"); got != "account.getAppConfig" {
		t.Fatal(got)
	}
}

func TestModuleAndPhase(t *testing.T) {
	if moduleOf("messages.getDialogFilters") != "folders" {
		t.Fatal(moduleOf("messages.getDialogFilters"))
	}
	if moduleOf("messages.getAllStickers") != "stickers" {
		t.Fatal(moduleOf("messages.getAllStickers"))
	}
	if moduleOf("phone.getCallConfig") != "phone/calls" {
		t.Fatal(moduleOf("phone.getCallConfig"))
	}
	if moduleOf("payments.getPaymentForm") != "premium/payments" {
		t.Fatal(moduleOf("payments.getPaymentForm"))
	}
	if phaseOf("messages", true, false) != "P0" {
		t.Fatal("startup should be P0")
	}
	if phaseOf("messages", false, false) != "P1" {
		t.Fatal("messages should be P1")
	}
	if phaseOf("stories", false, false) != "P2" {
		t.Fatal("stories should be P2")
	}
}

func TestWhen(t *testing.T) {
	st, bg := whenOf("State/AccountStateManager.swift", "applyDifference", "updates.getDifference")
	if !st || !bg {
		t.Fatalf("state file: startup=%v background=%v", st, bg)
	}
	st, bg = whenOf("State/PendingMessageManager.swift", "sendMessageContent", "messages.sendMessage")
	if st || bg {
		t.Fatalf("send pipeline is interactive, startup=%v background=%v", st, bg)
	}
	st, bg = whenOf("api/api_updates.cpp", "getDifference", "updates.getDifference")
	if !st || !bg {
		t.Fatalf("updates: startup=%v background=%v", st, bg)
	}
	st, bg = whenOf("PendingMessages/StandaloneSendMessage.swift", "sendMessageContent", "messages.sendMessage")
	if st || bg {
		t.Fatalf("send should be interactive, startup=%v background=%v", st, bg)
	}
	st, _ = whenOf("export/export_api_wrap.cpp", "requestDialogs", "messages.getDialogs")
	if st {
		t.Fatal("export must not be startup")
	}
	st, _ = whenOf("ui/LoginActivity.java", "onNextPressed", "payments.getPaymentForm")
	if st {
		t.Fatal("payment form inside login must not be P0")
	}
	st, _ = whenOf("ui/LoginActivity.java", "onNextPressed", "auth.sendCode")
	if !st {
		t.Fatal("auth.sendCode from login is startup")
	}
	st, _ = whenOf("ChatActivity.java", "getDialogId", "messages.sendQuickReplyMessages")
	if st {
		t.Fatal("getDialogId must not mark a method as loading dialogs")
	}
}

func TestSessionIDMapsMatch(t *testing.T) {
	root, err := findRoot("")
	if err != nil {
		t.Fatal(err)
	}
	var a, b serverModel
	if err := a.parseIDMap(filepath.Join(root, "teamgramd/etc/session.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := b.parseIDMap(filepath.Join(root, "teamgramd/etc2/session.yaml")); err != nil {
		t.Fatal(err)
	}
	if len(a.Routed) != len(b.Routed) {
		t.Fatalf("etc has %d routes, etc2 has %d", len(a.Routed), len(b.Routed))
	}
	for k := range a.Routed {
		if !b.Routed[k] {
			t.Errorf("routed in etc but not etc2: %s", k)
		}
	}
}

func TestServerSamples(t *testing.T) {
	root, err := findRoot("")
	if err != nil {
		t.Fatal(err)
	}
	srv, err := loadServer(root)
	if err != nil {
		t.Fatal(err)
	}
	if !srv.Routed["RPCConfiguration"] {
		t.Fatal("configuration should be routed")
	}
	if !srv.Routed["RPCVoipCalls"] {
		t.Fatal("voip calls should be routed so phone.* reaches the bff")
	}
	if !srv.Routed["RPCSecretChats"] {
		t.Fatal("secret chats should be routed so messages.getDhConfig reaches the phone handler")
	}
	if !srv.Routed["RPCSeamless"] || !srv.Registered["RPCSeamless"] {
		t.Fatal("seamless URL auth must be routed and registered")
	}
	if srv.Routed["RPCGroupCalls"] {
		t.Fatal("group calls stay unrouted until the SFU path is a separate slice")
	}
	if !srv.Routed["RPCStories"] || !srv.Registered["RPCStories"] {
		t.Fatal("stories must be routed and registered")
	}
	assertStatus := func(method, status, effect string) {
		t.Helper()
		st, ef, _, _, notes, _ := srv.resolve(method)
		if st != status || ef != effect {
			t.Errorf("%s status=%s effect=%s notes=%s, want %s/%s", method, st, ef, notes, status, effect)
		}
	}
	assertStatus("help.getConfig", statusImplemented, effectOK)
	assertStatus("phone.getCallConfig", statusImplemented, effectOK)
	assertStatus("phone.requestCall", statusImplemented, effectOK)
	assertStatus("phone.sendSignalingData", statusImplemented, effectOK)
	assertStatus("messages.getDhConfig", statusImplemented, effectOK)
	assertStatus("messages.receivedQueue", statusImplemented, effectOK)
	assertStatus("messages.getDialogs", statusImplemented, effectOK)
	assertStatus("messages.getHistory", statusImplemented, effectOK)
	// Routed sticker method with a real catalog read.
	assertStatus("messages.getAllStickers", statusImplemented, effectOK)
	assertStatus("messages.getMaskStickers", statusImplemented, effectOK)
	assertStatus("messages.getArchivedStickers", statusImplemented, effectOK)
	assertStatus("messages.getSavedGifs", statusImplemented, effectOK)
	assertStatus("messages.saveGif", statusImplemented, effectOK)
	assertStatus("stickers.createStickerSet", statusImplemented, effectOK)
	assertStatus("stickers.checkShortName", statusImplemented, effectOK)
	assertStatus("help.getTimezonesList", statusImplemented, effectOK)
	assertStatus("help.getUserInfo", statusImplemented, effectOK)
	assertStatus("help.editUserInfo", statusImplemented, effectOK)
	if !srv.Routed["RPCGifs"] || !srv.Registered["RPCGifs"] {
		t.Fatal("saved gifs must be routed and registered")
	}
	if !srv.Routed["RPCTimezones"] || !srv.Registered["RPCTimezones"] {
		t.Fatal("timezones must be routed and registered")
	}
	if !srv.Routed["RPCTsf"] || !srv.Registered["RPCTsf"] {
		t.Fatal("support user info must be routed and registered")
	}
	// P0 startup calls that must not come back as ENTERPRISE_IS_BLOCKED.
	assertStatus("auth.finishPasskeyLogin", statusImplemented, effectOK)
	assertStatus("messages.getPaidReactionPrivacy", statusImplemented, effectOK)
	assertStatus("messages.requestEncryption", statusImplemented, effectOK)
	assertStatus("messages.acceptEncryption", statusImplemented, effectOK)
	assertStatus("messages.sendEncrypted", statusImplemented, effectOK)
	assertStatus("messages.sendEncryptedFile", statusImplemented, effectOK)
	assertStatus("messages.requestUrlAuth", statusImplemented, effectOK)
	assertStatus("messages.acceptUrlAuth", statusImplemented, effectOK)
	assertStatus("messages.sendPaidReaction", statusImplemented, effectOK)
	assertStatus("messages.getSplitRanges", statusImplemented, effectOK)
	assertStatus("messages.deletePhoneCallHistory", statusImplemented, effectOK)
	assertStatus("messages.getEmojiGroups", statusImplemented, effectOK)
	assertStatus("messages.discardEncryption", statusImplemented, effectOK)
	assertStatus("messages.setEncryptedTyping", statusImplemented, effectOK)
	assertStatus("messages.readEncryptedHistory", statusImplemented, effectOK)
	assertStatus("messages.sendEncryptedService", statusImplemented, effectOK)
	assertStatus("messages.reportEncryptedSpam", statusImplemented, effectOK)
	assertStatus("messages.togglePaidReactionPrivacy", statusImplemented, effectOK)
	assertStatus("messages.declineUrlAuth", statusImplemented, effectOK)
	assertStatus("messages.checkUrlAuthMatchCode", statusImplemented, effectOK)
	assertStatus("messages.setChatTheme", statusImplemented, effectOK)
	assertStatus("messages.setChatWallPaper", statusImplemented, effectOK)
	assertStatus("messages.setShareTheme", statusImplemented, effectOK)
	assertStatus("messages.getShareThemeBackdrops", statusImplemented, effectOK)
	assertStatus("messages.initHistoryImport", statusImplemented, effectOK)
	assertStatus("messages.checkHistoryImport", statusImplemented, effectOK)
	assertStatus("messages.appendTodoList", statusImplemented, effectOK)
	assertStatus("messages.getFactCheck", statusImplemented, effectOK)
	assertStatus("messages.composeMessageWithAI", statusImplemented, effectOK)
	assertStatus("messages.getEmojiGameInfo", statusImplemented, effectOK)
	assertStatus("messages.getStatsURL", statusImplemented, effectOK)
	assertStatus("messages.toggleSuggestedPostApproval", statusImplemented, effectOK)
	assertStatus("account.getThemes", statusImplemented, effectOK)
	assertStatus("account.getWallPapers", statusImplemented, effectOK)
	assertStatus("account.createTheme", statusImplemented, effectOK)
	assertStatus("account.getConnectedBots", statusImplemented, effectOK)
	assertStatus("account.getSavedRingtones", statusImplemented, effectOK)
	assertStatus("account.getDefaultProfilePhotoEmojis", statusImplemented, effectOK)
	assertStatus("account.getCollectibleEmojiStatuses", statusImplemented, effectOK)
	assertStatus("account.getWebAuthorizations", statusImplemented, effectOK)
	assertStatus("help.getPromoData", statusImplemented, effectOK)
	assertStatus("help.hidePromoData", statusImplemented, effectOK)
	for _, method := range []string{
		"stories.activateStealthMode",
		"stories.canSendStory",
		"stories.createAlbum",
		"stories.deleteAlbum",
		"stories.deleteStories",
		"stories.editStory",
		"stories.exportStoryLink",
		"stories.getAlbumStories",
		"stories.getAlbums",
		"stories.getAllReadPeerStories",
		"stories.getAllStories",
		"stories.getChatsToSend",
		"stories.getPeerMaxIDs",
		"stories.getPeerStories",
		"stories.getPinnedStories",
		"stories.getStoriesArchive",
		"stories.getStoriesByID",
		"stories.getStoriesViews",
		"stories.getStoryReactionsList",
		"stories.getStoryViewsList",
		"stories.incrementStoryViews",
		"stories.readStories",
		"stories.reorderAlbums",
		"stories.report",
		"stories.searchPosts",
		"stories.sendReaction",
		"stories.sendStory",
		"stories.startLive",
		"stories.togglePeerStoriesHidden",
		"stories.togglePinned",
		"stories.togglePinnedToTop",
		"stories.updateAlbum",
	} {
		assertStatus(method, statusImplemented, effectOK)
		_, _, _, _, notes, _ := srv.resolve(method)
		if strings.Contains(notes, "layer variants disagree") || strings.Contains(notes, "ENTERPRISE_IS_BLOCKED") {
			t.Errorf("%s notes=%s", method, notes)
		}
	}
	if !srv.Routed["RPCPasskey"] || !srv.Registered["RPCPasskey"] {
		t.Fatal("passkey login must be routed and registered")
	}
	if !srv.Routed["RPCWebBrowser"] || !srv.Registered["RPCWebBrowser"] {
		t.Fatal("web browser settings must be routed and registered")
	}
	assertStatus("account.getPasskeys", statusImplemented, effectOK)
	assertStatus("account.initPasskeyRegistration", statusImplemented, effectOK)
	assertStatus("account.registerPasskey", statusImplemented, effectOK)
	assertStatus("account.deletePasskey", statusImplemented, effectOK)
	assertStatus("auth.initPasskeyLogin", statusImplemented, effectOK)
	assertStatus("account.getWebBrowserSettings", statusImplemented, effectOK)
	assertStatus("account.updateWebBrowserSettings", statusImplemented, effectOK)
	assertStatus("account.toggleWebBrowserSettingsException", statusImplemented, effectOK)
	assertStatus("account.deleteWebBrowserSettingsExceptions", statusImplemented, effectOK)
	assertStatus("account.initTakeoutSession", statusImplemented, effectOK)
	assertStatus("account.finishTakeoutSession", statusImplemented, effectOK)
	assertStatus("account.reorderUsernames", statusImplemented, effectOK)
	assertStatus("account.toggleUsername", statusImplemented, effectOK)
	assertStatus("account.getPaidMessagesRevenue", statusImplemented, effectOK)
	assertStatus("account.toggleNoPaidMessagesException", statusImplemented, effectOK)
	if _, ok := canonicalClientMethod("account.reorderProfileTabs"); ok {
		t.Fatal("commented profile-tab reorder must stay out of the matrix")
	}
	if got, ok := canonicalClientMethod("account.initPasskeyLogin"); !ok || got != "auth.initPasskeyLogin" {
		t.Fatalf("android passkey login alias: %s %v", got, ok)
	}
	if !srv.Routed["RPCPromoData"] || !srv.Registered["RPCPromoData"] {
		t.Fatal("promo data must be routed and registered")
	}
}
