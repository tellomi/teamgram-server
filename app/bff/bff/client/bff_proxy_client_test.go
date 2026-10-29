package bff_proxy_client

import "testing"

func TestServiceRouteMatchesServiceBoundary(t *testing.T) {
	const (
		emoji      = "/mtproto.RPCEmoji/messages_getEmojiKeywords"
		categories = "/mtproto.RPCEmojiCategories/messages_getEmojiStickerGroups"
		status     = "/mtproto.RPCEmojiStatus/account_getDefaultEmojiStatuses"
	)
	if !serviceRouteMatches(emoji, "/mtproto.RPCEmoji") {
		t.Fatal("emoji keywords should use RPCEmoji")
	}
	if serviceRouteMatches(categories, "/mtproto.RPCEmoji") {
		t.Fatal("RPCEmoji prefix must not capture RPCEmojiCategories")
	}
	if serviceRouteMatches(status, "/mtproto.RPCEmoji") {
		t.Fatal("RPCEmoji prefix must not capture RPCEmojiStatus")
	}
	if !serviceRouteMatches(categories, "/mtproto.RPCEmojiCategories") {
		t.Fatal("sticker groups should use RPCEmojiCategories")
	}
}
