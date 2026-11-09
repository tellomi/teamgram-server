package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestGlobalSearchFree is the client global search: a free post quota,
// Chinese post search, media filters on messages.searchGlobal, and
// contacts.search titles plus the broadcasts and bots flags.
func TestGlobalSearchFree(t *testing.T) {
	cfg := requireE2E(t)
	c, _ := loginNamed(t, cfg, ProfileIOS, "search", "Search")
	ctx, cancel := contextTimeout(90 * time.Second)
	defer cancel()

	before := floodOf(t, invokeOK(t, c, &mtproto.TLChannelsCheckSearchPostsFlood{
		Constructor: mtproto.CRC32_channels_checkSearchPostsFlood,
		Query:       &wrapperspb.StringValue{Value: "你好"},
	}))
	if !before.GetQueryIsFree() || before.GetStarsAmount() != 0 || before.GetTotalDaily() != 100 || before.GetRemains() != 100 || before.GetWaitTill() != nil {
		t.Fatalf("flood %+v", before)
	}

	created, err := c.Invoke(ctx, &mtproto.TLChannelsCreateChannel{
		Constructor: mtproto.CRC32_channels_createChannel_91006707,
		Broadcast:   true,
		Title:       "中文新闻台",
		About:       "",
	})
	if err != nil {
		t.Fatal(err)
	}
	var chID, chHash int64
	for _, chat := range chatsOf(created) {
		if chat != nil && chat.GetBroadcast() && chat.GetTitle() == "中文新闻台" {
			chID, chHash = chat.GetId(), chatAccessHash(chat)
		}
	}
	if chID == 0 {
		t.Fatalf("create channel %T", created)
	}
	username := "zh" + randomUsername()
	if _, err := c.Invoke(ctx, &mtproto.TLChannelsUpdateUsername{
		Constructor: mtproto.CRC32_channels_updateUsername,
		Channel:     inputChannel(chID, chHash),
		Username:    username,
	}); err != nil {
		t.Fatal(err)
	}
	channelPeer := inputPeerChannel(chID, chHash)
	if _, err := c.SendText(ctx, channelPeer, "上海你好 #新闻", nil); err != nil {
		t.Fatal(err)
	}
	posts := messagesOfResult(t, invokeOK(t, c, &mtproto.TLChannelsSearchPosts{
		Constructor: mtproto.CRC32_channels_searchPosts_f2c4f24d,
		Query:       &wrapperspb.StringValue{Value: "海"},
		OffsetPeer:  inputPeerEmpty(),
		Limit:       20,
	}))
	if !messageHas(posts, "上海你好 #新闻") {
		t.Fatalf("single character posts %+v", messageTexts(posts))
	}
	if len(posts.GetChats()) == 0 || len(posts.GetUsers()) == 0 {
		var peer *mtproto.Peer
		if len(posts.GetMessages()) > 0 {
			peer = posts.GetMessages()[0].GetPeerId()
		}
		t.Fatalf("vectors chats=%d users=%d msgs=%d peer=%v", len(posts.GetChats()), len(posts.GetUsers()), len(posts.GetMessages()), peer)
	}
	if posts.GetSearchFlood().GetStarsAmount() != 0 || !posts.GetSearchFlood().GetQueryIsFree() || posts.GetSearchFlood().GetRemains() != 99 {
		t.Fatalf("search flood %+v", posts.GetSearchFlood())
	}
	again := messagesOfResult(t, invokeOK(t, c, &mtproto.TLChannelsSearchPosts{
		Constructor:        mtproto.CRC32_channels_searchPosts_f2c4f24d,
		Hashtag_FLAGSTRING: &wrapperspb.StringValue{Value: "新闻"},
		Hashtag_STRING:     "新闻",
		OffsetPeer:         inputPeerEmpty(),
		Limit:              20,
	}))
	if !messageHas(again, "上海你好 #新闻") {
		t.Fatalf("hashtag %+v", messageTexts(again))
	}
	full := messagesOfResult(t, invokeOK(t, c, &mtproto.TLChannelsSearchPosts{
		Constructor: mtproto.CRC32_channels_searchPosts_f2c4f24d,
		Query:       &wrapperspb.StringValue{Value: "你好"},
		OffsetPeer:  inputPeerEmpty(),
		Limit:       20,
	}))
	if !messageHas(full, "上海你好 #新闻") {
		t.Fatalf("query %+v", messageTexts(full))
	}

	after := floodOf(t, invokeOK(t, c, &mtproto.TLChannelsCheckSearchPostsFlood{
		Constructor: mtproto.CRC32_channels_checkSearchPostsFlood,
	}))
	if after.GetRemains() != 97 || after.GetStarsAmount() != 0 || !after.GetQueryIsFree() || after.GetWaitTill() != nil {
		t.Fatalf("after searches %+v", after)
	}

	if _, err := c.SendText(ctx, inputPeerSelf(), "看这里 https://example.com/中文", nil); err != nil {
		t.Fatal(err)
	}
	_, err = c.Invoke(ctx, &mtproto.TLMessagesSearchGlobal{
		Constructor: mtproto.CRC32_messages_searchGlobal_6126a43c,
		Q:           "",
		Filter:      mtproto.MakeTLInputMessagesFilterEmpty(nil).To_MessagesFilter(),
		OffsetPeer:  inputPeerEmpty(),
		Limit:       10,
	})
	if rpc, ok := asRPCError(err); !ok || rpc.Message != "SEARCH_QUERY_EMPTY" {
		t.Fatalf("empty text search %v", err)
	}
	urls := messagesOfResult(t, invokeOK(t, c, &mtproto.TLMessagesSearchGlobal{
		Constructor: mtproto.CRC32_messages_searchGlobal_6126a43c,
		Q:           "",
		Filter:      mtproto.MakeTLInputMessagesFilterUrl(nil).To_MessagesFilter(),
		OffsetPeer:  inputPeerEmpty(),
		Limit:       20,
	}))
	if !messageHas(urls, "看这里 https://example.com/中文") {
		t.Fatalf("empty url filter %+v", messageTexts(urls))
	}
	zh := messagesOfResult(t, invokeOK(t, c, &mtproto.TLMessagesSearchGlobal{
		Constructor: mtproto.CRC32_messages_searchGlobal_6126a43c,
		UsersOnly:   true,
		Q:           "中文",
		Filter:      mtproto.MakeTLInputMessagesFilterEmpty(nil).To_MessagesFilter(),
		OffsetPeer:  inputPeerEmpty(),
		Limit:       20,
	}))
	if !messageHas(zh, "看这里 https://example.com/中文") {
		t.Fatalf("users_only %+v", messageTexts(zh))
	}
	broad := messagesOfResult(t, invokeOK(t, c, &mtproto.TLMessagesSearchGlobal{
		Constructor:    mtproto.CRC32_messages_searchGlobal_6126a43c,
		BroadcastsOnly: true,
		Q:              "中文",
		Filter:         mtproto.MakeTLInputMessagesFilterEmpty(nil).To_MessagesFilter(),
		OffsetPeer:     inputPeerEmpty(),
		Limit:          20,
	}))
	if messageHas(broad, "看这里 https://example.com/中文") {
		t.Fatalf("broadcasts_only kept a user message %+v", messageTexts(broad))
	}

	found := foundOf(t, invokeOK(t, c, &mtproto.TLContactsSearch{
		Constructor: mtproto.CRC32_contacts_search_5f58d0f,
		Broadcasts:  true,
		Q:           "中文新闻",
		Limit:       20,
	}))
	if !chatHas(found, "中文新闻台") {
		t.Fatalf("broadcasts title %+v", chatTitles(found))
	}
	bots := foundOf(t, invokeOK(t, c, &mtproto.TLContactsSearch{
		Constructor: mtproto.CRC32_contacts_search_5f58d0f,
		Bots:        true,
		Q:           "中文新闻",
		Limit:       20,
	}))
	if chatHas(bots, "中文新闻台") {
		t.Fatalf("bots flag returned a channel %+v", chatTitles(bots))
	}
}

func floodOf(t *testing.T, obj mtproto.TLObject) *mtproto.SearchPostsFlood {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.TLSearchPostsFlood:
		return v.To_SearchPostsFlood()
	case *mtproto.SearchPostsFlood:
		return v
	default:
		t.Fatalf("flood %T", obj)
		return nil
	}
}

func messagesOfResult(t *testing.T, obj mtproto.TLObject) *mtproto.Messages_Messages {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.TLMessagesMessages:
		return v.To_Messages_Messages()
	case *mtproto.TLMessagesMessagesSlice:
		return v.To_Messages_Messages()
	case *mtproto.TLMessagesChannelMessages:
		return v.To_Messages_Messages()
	default:
		t.Fatalf("messages %T", obj)
		return nil
	}
}

func foundOf(t *testing.T, obj mtproto.TLObject) *mtproto.Contacts_Found {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.TLContactsFound:
		return v.To_Contacts_Found()
	case *mtproto.Contacts_Found:
		return v
	default:
		t.Fatalf("found %T", obj)
		return nil
	}
}

func messageTexts(res *mtproto.Messages_Messages) []string {
	var out []string
	for _, m := range res.GetMessages() {
		if m != nil && m.GetMessage() != "" {
			out = append(out, m.GetMessage())
		}
	}
	return out
}

func messageHas(res *mtproto.Messages_Messages, want string) bool {
	for _, text := range messageTexts(res) {
		if strings.Contains(text, want) {
			return true
		}
	}
	return false
}

func chatTitles(found *mtproto.Contacts_Found) []string {
	var out []string
	for _, ch := range found.GetChats() {
		if ch != nil {
			out = append(out, ch.GetTitle())
		}
	}
	return out
}

func chatHas(found *mtproto.Contacts_Found, title string) bool {
	for _, got := range chatTitles(found) {
		if got == title {
			return true
		}
	}
	return false
}
