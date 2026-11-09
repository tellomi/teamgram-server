package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestSearchPostsChineseHashtagPageAndHidden(t *testing.T) {
	svc, ctx := newTest(t)
	news, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "中文新闻", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	newsChat := news.Chats[0]
	newsIn := input(newsChat.Id, newsChat.GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.UpdateUsername(ctx, 1, 0, newsIn, "news_zh"); err != nil {
		t.Fatal(err)
	}
	other, err := svc.Create(ctx, CreateRequest{CreatorID: 2, Title: "别人的频道", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	otherIn := input(other.Chats[0].Id, other.Chats[0].GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.UpdateUsername(ctx, 2, 0, otherIn, "othernews"); err != nil {
		t.Fatal(err)
	}
	joined, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "私密群", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	stranger, err := svc.Create(ctx, CreateRequest{CreatorID: 9, Title: "未加入群", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	strangerIn := input(stranger.Chats[0].Id, stranger.Chats[0].GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.UpdateUsername(ctx, 9, 0, strangerIn, "publicgroup"); err != nil {
		t.Fatal(err)
	}
	hidden := []struct {
		title string
		mark  func(*Channel)
	}{
		{"已删除", func(ch *Channel) { ch.Deleted = true }},
		{"已封禁", func(ch *Channel) { ch.Banned = true }},
		{"受限制", func(ch *Channel) { ch.Restricted = true }},
	}
	for i, h := range hidden {
		up, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: h.title, Broadcast: true})
		if err != nil {
			t.Fatal(err)
		}
		in := input(up.Chats[0].Id, up.Chats[0].GetAccessHash_FLAGINT64().GetValue())
		if _, err := svc.UpdateUsername(ctx, 1, 0, in, "hide"+string(rune('a'+i))); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Send(ctx, 1, up.Chats[0].Id, 0, []Outgoing{{
			RandomID: int64(50 + i),
			Message:  &mtproto.Message{Message: "上海你好 " + h.title, Date: 50},
		}}); err != nil {
			t.Fatal(err)
		}
		raw, err := svc.store.GetChannel(ctx, up.Chats[0].Id)
		if err != nil {
			t.Fatal(err)
		}
		h.mark(raw)
		if err := svc.store.SaveChannel(ctx, raw); err != nil {
			t.Fatal(err)
		}
	}
	kicked, err := svc.Create(ctx, CreateRequest{CreatorID: 3, Title: "踢出", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	kickedIn := input(kicked.Chats[0].Id, kicked.Chats[0].GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.UpdateUsername(ctx, 3, 0, kickedIn, "kickednews"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Join(ctx, 1, 0, kickedIn); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EditBanned(ctx, 3, 0, kickedIn, 1, mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{
		ViewMessages: true,
	}).To_ChatBannedRights()); err != nil {
		// A viewer who cannot be banned this way is still covered by the channel flags above.
		p, _ := svc.store.GetParticipant(ctx, kicked.Chats[0].Id, 1)
		if p != nil {
			p.Role = RoleBanned
			p.State = StateKicked
			_ = svc.store.PutParticipant(ctx, p)
		}
	}

	send := func(channelID int64, random int64, text string, date int32) {
		t.Helper()
		if _, err := svc.Send(ctx, 1, channelID, 0, []Outgoing{{
			RandomID: random,
			Message:  &mtproto.Message{Message: text, Date: date},
		}}); err != nil {
			t.Fatal(err)
		}
	}
	send(newsChat.Id, 1, "上海你好 #新闻", 300)
	send(newsChat.Id, 2, "第二条 中", 200)
	send(newsChat.Id, 3, "100% off", 100)
	if _, err := svc.Send(ctx, 2, other.Chats[0].Id, 0, []Outgoing{{
		RandomID: 4,
		Message:  &mtproto.Message{Message: "上海你好", Date: 250},
	}}); err != nil {
		t.Fatal(err)
	}
	send(joined.Chats[0].Id, 5, "上海你好 群里", 150)
	if _, err := svc.Send(ctx, 9, stranger.Chats[0].Id, 0, []Outgoing{{
		RandomID: 8,
		Message:  &mtproto.Message{Message: "上海你好 公开群", Date: 180},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Send(ctx, 3, kicked.Chats[0].Id, 0, []Outgoing{{
		RandomID: 9,
		Message:  &mtproto.Message{Message: "上海你好 踢出", Date: 160},
	}}); err != nil {
		t.Fatal(err)
	}

	page, err := svc.SearchPosts(ctx, 1, PostQuery{Needle: "海", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 2 || page.Messages[0].GetMessage() == "" {
		t.Fatalf("first page %#v", texts(page))
	}
	if page.Messages[0].GetDate() < page.Messages[1].GetDate() {
		t.Fatalf("order %v", texts(page))
	}
	if len(page.Chats) == 0 || len(page.Users) == 0 {
		t.Fatalf("vectors chats=%d users=%d", len(page.Chats), len(page.Users))
	}
	if page.GetNextRate().GetValue() == 0 {
		t.Fatal("missing next_rate")
	}
	last := page.Messages[len(page.Messages)-1]
	next, err := svc.SearchPosts(ctx, 1, PostQuery{
		Needle:     "海",
		OffsetRate: last.GetDate(),
		OffsetPeer: last.GetPeerId().GetChannelId(),
		OffsetID:   last.GetId(),
		Limit:      10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Messages) == 0 {
		t.Fatal("empty second page")
	}
	for _, m := range next.Messages {
		if m.GetDate() > last.GetDate() {
			t.Fatalf("newer on next page %s", m.GetMessage())
		}
		if m.GetMessage() == last.GetMessage() && m.GetId() == last.GetId() {
			t.Fatalf("duplicated %s", m.GetMessage())
		}
	}
	joinedTexts := append(texts(page), texts(next)...)
	for _, want := range []string{"上海你好 #新闻", "上海你好", "上海你好 群里"} {
		if !hasText(joinedTexts, want) {
			t.Fatalf("missing %q in %v", want, joinedTexts)
		}
	}
	for _, drop := range []string{"上海你好 公开群", "上海你好 踢出", "上海你好 已删除", "上海你好 已封禁", "上海你好 受限制"} {
		if hasText(joinedTexts, drop) {
			t.Fatalf("included %q in %v", drop, joinedTexts)
		}
	}

	one, err := svc.SearchPosts(ctx, 1, PostQuery{Needle: "中", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if !hasText(texts(one), "第二条 中") {
		t.Fatalf("single character %#v", texts(one))
	}
	tag, err := svc.SearchPosts(ctx, 1, PostQuery{Needle: "#新闻", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(tag.Messages) != 1 || tag.Messages[0].GetMessage() != "上海你好 #新闻" {
		t.Fatalf("hashtag %#v", texts(tag))
	}
	pct, err := svc.SearchPosts(ctx, 1, PostQuery{Needle: "100%", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(pct.Messages) != 1 || pct.Messages[0].GetMessage() != "100% off" {
		t.Fatalf("percent %#v", texts(pct))
	}
	under, err := svc.SearchPosts(ctx, 1, PostQuery{Needle: "_", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(under.Messages) != 0 {
		t.Fatalf("underscore wildcard %#v", texts(under))
	}
}

func TestSearchPublicChineseTitle(t *testing.T) {
	svc, ctx := newTest(t)
	news, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "中文频道", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	in := input(news.Chats[0].Id, news.Chats[0].GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.UpdateUsername(ctx, 1, 0, in, "zhongwen"); err != nil {
		t.Fatal(err)
	}
	group, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "中文群组", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	gin := input(group.Chats[0].Id, group.Chats[0].GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.UpdateUsername(ctx, 1, 0, gin, "zhongqun"); err != nil {
		t.Fatal(err)
	}
	priv, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "中文私密", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	_ = priv
	banned, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "中文封禁频道", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	bin := input(banned.Chats[0].Id, banned.Chats[0].GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.UpdateUsername(ctx, 1, 0, bin, "bannedzh"); err != nil {
		t.Fatal(err)
	}
	raw, _ := svc.store.GetChannel(ctx, banned.Chats[0].Id)
	raw.Banned = true
	if err := svc.store.SaveChannel(ctx, raw); err != nil {
		t.Fatal(err)
	}

	hits, err := svc.SearchPublic(ctx, 8, "中文", 20)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, ch := range hits {
		got[ch.Title] = true
	}
	if !got["中文频道"] || !got["中文群组"] || got["中文私密"] || got["中文封禁频道"] {
		t.Fatalf("hits %#v", got)
	}
	one, err := svc.SearchPublic(ctx, 8, "频", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(one) != 1 || one[0].Title != "中文频道" {
		t.Fatalf("single rune %#v", one)
	}
}

func TestLikeContainsEscapes(t *testing.T) {
	if got := likeContains(`100%_a\b`); got != `%100\%\_a\\b%` {
		t.Fatalf("pattern %q", got)
	}
}

func texts(res *mtproto.Messages_Messages) []string {
	var out []string
	for _, m := range res.GetMessages() {
		out = append(out, m.GetMessage())
	}
	return out
}

func hasText(all []string, want string) bool {
	for _, s := range all {
		if s == want {
			return true
		}
	}
	return false
}
