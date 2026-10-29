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
	if srv.Routed["RPCVoipCalls"] {
		t.Fatal("voip calls are commented out of session.yaml")
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
	assertStatus("messages.getDialogs", statusImplemented, effectOK)
	assertStatus("messages.getHistory", statusImplemented, effectOK)
	// Routed sticker method with a real catalog read.
	assertStatus("messages.getAllStickers", statusImplemented, effectOK)
	// Routed sticker method that returns an empty set.
	assertStatus("messages.getMaskStickers", statusStub, effectEmpty)
	// Passkeys are implemented as an enterprise error and the service is not routed.
	st, ef, _, _, notes, _ := srv.resolve("account.getPasskeys")
	if st != statusMissing || ef != effectError {
		t.Fatalf("getPasskeys %s %s %s", st, ef, notes)
	}
	if !strings.Contains(notes, "not routed") && !strings.Contains(notes, "no route") {
		t.Fatalf("notes should mention the missing route: %s", notes)
	}
}
