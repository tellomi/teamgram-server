package main

import (
	"path/filepath"
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
	if !srv.Registered["RPCStories"] {
		t.Fatal("stories handler is registered on the bff process")
	}
	if srv.Routed["RPCStories"] {
		t.Fatal("stories are not in the session IDMap")
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
	// Routed sticker method that returns an empty set.
	assertStatus("messages.getMaskStickers", statusStub, effectEmpty)
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
	assertStatus("account.getThemes", statusStub, effectEmpty)
	assertStatus("account.getWallPapers", statusStub, effectEmpty)
	assertStatus("account.getWebAuthorizations", statusImplemented, effectOK)
	assertStatus("help.getPromoData", statusImplemented, effectOK)
	assertStatus("help.hidePromoData", statusImplemented, effectOK)
	if !srv.Routed["RPCPasskey"] || !srv.Registered["RPCPasskey"] {
		t.Fatal("passkey login must be routed and registered")
	}
	// Other passkey RPCs stay enterprise errors. Routing the service must not
	// mark those wrappers as implemented.
	assertStatus("account.getPasskeys", statusStub, effectError)
	assertStatus("auth.initPasskeyLogin", statusStub, effectError)
	if !srv.Routed["RPCPromoData"] || !srv.Registered["RPCPromoData"] {
		t.Fatal("promo data must be routed and registered")
	}
}
