package chatbox

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

func testBot() BotProfile {
	return BotProfile{
		ID: 70, Name: "Helper", Username: "helper", Inline: true, Placeholder: "search",
		AttachMenu: true, MenuURL: "https://bot.example/menu", MainURL: "https://bot.example/app",
		Apps: []BotAppSpec{{
			ID: 5, ShortName: "play", Title: "Play", Description: "A game",
			URL: "https://bot.example/play",
		}},
	}
}

func userIn(id int64) *mtproto.InputUser {
	return mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: id}).To_InputUser()
}

func peerIn(id int64) *mtproto.InputPeer {
	return mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: id}).To_InputPeer()
}

func article(id, text string) *mtproto.BotInlineResult {
	return mtproto.MakeTLBotInlineResult(&mtproto.BotInlineResult{
		Id:   id,
		Type: "article",
		SendMessage: mtproto.MakeTLBotInlineMessageText(&mtproto.BotInlineMessage{
			Message: text,
		}).To_BotInlineMessage(),
	}).To_BotInlineResult()
}

func TestInlineQuerySendAndExpiry(t *testing.T) {
	b := newBook(nil)
	b.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	ctx := context.Background()
	if _, err := b.InlineResults(ctx, 3, userIn(70), peerIn(9), "hi", ""); err != mtproto.ErrBotInvalid {
		t.Fatalf("missing bot: %v", err)
	}
	if err := b.PutBot(ctx, testBot()); err != nil {
		t.Fatal(err)
	}
	off := testBot()
	off.ID = 71
	off.Inline = false
	if err := b.PutBot(ctx, off); err != nil {
		t.Fatal(err)
	}
	if _, err := b.InlineResults(ctx, 3, userIn(71), peerIn(9), "hi", ""); err != mtproto.ErrBotInlineDisabled {
		t.Fatalf("inline off: %v", err)
	}
	empty, err := b.InlineResults(ctx, 3, userIn(70), peerIn(9), "hi", "")
	if err != nil || empty.QueryId == 0 || len(empty.Results) != 0 || empty.Users[0].GetId() != 70 {
		t.Fatalf("empty %+v %v", empty, err)
	}
	if _, _, err := b.TakeInlineResult(ctx, 3, peerIn(9), empty.QueryId, "missing", 1, false, false, false, 0, nil); err != mtproto.ErrInlineResultExpired {
		t.Fatalf("missing result: %v", err)
	}
	if err := b.SetInlineAnswer(ctx, 70, "hi", false, 60, []*mtproto.BotInlineResult{article("a", "hello")}); err != nil {
		t.Fatal(err)
	}
	got, err := b.InlineResults(ctx, 3, userIn(70), peerIn(9), "hi", "")
	if err != nil || len(got.Results) != 1 || got.Results[0].GetId() != "a" || got.CacheTime != 60 {
		t.Fatalf("cached %+v %v", got, err)
	}
	req, ready, err := b.TakeInlineResult(ctx, 3, peerIn(9), got.QueryId, "a", 11, false, true, false, 0, nil)
	if err != nil || ready != nil || req.Message.GetMessage() != "hello" || req.Message.GetViaBotId().GetValue() != 70 || !req.Message.GetSilent() {
		t.Fatalf("send %+v %+v %v", req, ready, err)
	}
	b.now = func() time.Time { return time.Unix(1_700_000_000, 0).Add(inlineQueryTTL + time.Second) }
	if _, _, err := b.TakeInlineResult(ctx, 3, peerIn(9), got.QueryId, "a", 12, false, false, false, 0, nil); err != mtproto.ErrInlineResultExpired {
		t.Fatalf("expired: %v", err)
	}
	if _, err := b.InlineResults(ctx, 3, userIn(70), peerIn(9), "way too long "+string(make([]byte, 300)), ""); err != mtproto.ErrMessageTooLong {
		t.Fatalf("long query: %v", err)
	}
}

func TestStartCallbackAttachAndWeb(t *testing.T) {
	b := newBook(nil)
	b.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	ctx := context.Background()
	if err := b.PutBot(ctx, testBot()); err != nil {
		t.Fatal(err)
	}
	if _, err := b.StartBot(ctx, 3, userIn(70), peerIn(9), 4, "bad param"); err != mtproto.ErrStartParamInvalid {
		t.Fatalf("param: %v", err)
	}
	start, err := b.StartBot(ctx, 3, userIn(70), peerIn(9), 4, "ref")
	if err != nil || start.Message.GetMessage() != "/start ref" || start.RandomID != 4 {
		t.Fatalf("start %+v %v", start, err)
	}
	if _, err := b.CallbackAnswer(ctx, 3, peerIn(9), 0, []byte("x"), false); err != mtproto.ErrMessageIdInvalid {
		t.Fatalf("msg id: %v", err)
	}
	if _, err := b.CallbackAnswer(ctx, 3, peerIn(9), 8, make([]byte, 65), false); err != mtproto.ErrButtonDataInvalid {
		t.Fatalf("data: %v", err)
	}
	ans, err := b.CallbackAnswer(ctx, 3, peerIn(9), 8, []byte("go"), false)
	if err != nil || ans.GetMessage() != nil || ans.Alert {
		t.Fatalf("empty callback %+v %v", ans, err)
	}
	if err := b.SetCallbackAnswer(ctx, 3, peerIn(9), 8, []byte("go"), true, "Done", "", 5); err != nil {
		t.Fatal(err)
	}
	ans, err = b.CallbackAnswer(ctx, 3, peerIn(9), 8, []byte("go"), false)
	if err != nil || !ans.Alert || ans.GetMessage().GetValue() != "Done" || ans.CacheTime != 5 {
		t.Fatalf("stored callback %+v %v", ans, err)
	}
	if _, err := b.CallbackAnswer(ctx, 3, peerIn(9), 8, []byte("go"), true); err != mtproto.ErrPasswordHashInvalid {
		t.Fatalf("password: %v", err)
	}

	list, err := b.AttachBots(ctx, 3, 0)
	if err != nil || len(list.Bots) != 0 {
		t.Fatalf("empty attach %+v %v", list, err)
	}
	if _, err := b.ToggleAttach(ctx, 3, userIn(70), true, true); err != nil {
		t.Fatal(err)
	}
	list, err = b.AttachBots(ctx, 3, 0)
	if err != nil || len(list.Bots) != 1 || list.Bots[0].GetBotId() != 70 || !list.Users[0].GetAttachMenuEnabled() {
		t.Fatalf("enabled %+v %v", list, err)
	}
	again, err := b.AttachBots(ctx, 3, list.Hash)
	if err != nil || again.GetPredicateName() != mtproto.Predicate_attachMenuBotsNotModified {
		t.Fatalf("hash %+v %v", again, err)
	}
	one, err := b.AttachBot(ctx, 3, userIn(70))
	if err != nil || one.Bot.GetInactive() || one.Bot.GetShortName() != "helper" {
		t.Fatalf("one %+v %v", one, err)
	}

	page, err := b.OpenWebView(ctx, WebOpen{
		Self: 3, BotID: 70, Peer: mtproto.MakePeerUtil(mtproto.PEER_USER, 70),
		Kind: "menu", URL: "https://evil.example/",
	})
	if err != mtproto.ErrUrlInvalid {
		t.Fatalf("foreign url: %+v %v", page, err)
	}
	page, err = b.OpenWebView(ctx, WebOpen{
		Self: 3, BotID: 70, Peer: mtproto.MakePeerUtil(mtproto.PEER_USER, 70),
		Kind: "main", StartParam: "go", Fullscreen: true,
	})
	if err != nil || page.GetQueryId_FLAGINT64().GetValue() == 0 || page.Url == "" || !page.Fullscreen {
		t.Fatalf("main %+v %v", page, err)
	}
	if _, err := b.ProlongWebView(ctx, 3, 70, page.GetQueryId_FLAGINT64().GetValue(), mtproto.MakePeerUtil(mtproto.PEER_USER, 70)); err != nil {
		t.Fatal(err)
	}
	if _, err := b.ProlongWebView(ctx, 3, 70, 1, nil); err != mtproto.ErrQueryIdInvalid {
		t.Fatalf("prolong: %v", err)
	}
	app, err := b.GetBotApp(ctx, 3, mtproto.MakeTLInputBotAppShortName(&mtproto.InputBotApp{
		BotId: userIn(70), ShortName: "play",
	}).To_InputBotApp(), 0)
	if err != nil || app.App.GetShortName() != "play" || app.App.GetPhoto() == nil {
		t.Fatalf("app %+v %v", app, err)
	}
	cached, err := b.GetBotApp(ctx, 3, mtproto.MakeTLInputBotAppShortName(&mtproto.InputBotApp{
		BotId: userIn(70), ShortName: "play",
	}).To_InputBotApp(), app.App.GetHash())
	if err != nil || cached.App.GetPredicateName() != mtproto.Predicate_botAppNotModified {
		t.Fatalf("app hash %+v %v", cached, err)
	}
	plain := testBot()
	plain.MainURL = ""
	plain.ID = 72
	plain.Apps = nil
	if err := b.PutBot(ctx, plain); err != nil {
		t.Fatal(err)
	}
	if _, err := b.OpenWebView(ctx, WebOpen{Self: 3, BotID: 72, Kind: "main"}); err != mtproto.ErrBotAppInvalid {
		t.Fatalf("no main: %v", err)
	}
}

func TestPreparedRequestedAndWebData(t *testing.T) {
	b := newBook(nil)
	b.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	ctx := context.Background()
	if err := b.PutBot(ctx, testBot()); err != nil {
		t.Fatal(err)
	}
	saved, err := b.SavePrepared(ctx, 70, 3, mtproto.MakeTLInputBotInlineResult(&mtproto.InputBotInlineResult{
		Id:   "p",
		Type: "article",
		SendMessage: mtproto.MakeTLInputBotInlineMessageText(&mtproto.InputBotInlineMessage{
			Message: "prepared",
		}).To_InputBotInlineMessage(),
	}).To_InputBotInlineResult(), nil)
	if err != nil || saved.Id == "" || saved.ExpireDate == 0 {
		t.Fatalf("save %+v %v", saved, err)
	}
	got, err := b.PreparedInline(ctx, 3, userIn(70), saved.Id)
	if err != nil || got.Result.GetSendMessage().GetMessage() != "prepared" || got.QueryId == 0 {
		t.Fatalf("get %+v %v", got, err)
	}
	if _, err := b.PreparedInline(ctx, 4, userIn(70), saved.Id); err != mtproto.ErrInlineResultExpired {
		t.Fatalf("other user: %v", err)
	}
	send, ready, err := b.TakeInlineResult(ctx, 3, peerIn(70), got.QueryId, "p", 21, false, false, false, 0, nil)
	if err != nil || ready != nil || send.Message.GetMessage() != "prepared" {
		t.Fatalf("send prepared %+v %+v %v", send, ready, err)
	}
	req, err := b.RequestedPeers(ctx, 3, peerIn(70), 2, 9, []*mtproto.InputPeer{peerIn(8)})
	if err != nil || req.Message.GetAction().GetButtonId() != 9 || req.Message.GetAction().GetPeers_VECTORPEER()[0].GetUserId() != 8 {
		t.Fatalf("peers %+v %v", req, err)
	}
	if _, err := b.RequestedPeers(ctx, 3, peerIn(70), 2, 9, nil); err != mtproto.ErrPeerIdInvalid {
		t.Fatalf("no peers: %v", err)
	}
	data, err := b.WebViewData(ctx, 3, userIn(70), 15, "Send", "payload")
	if err != nil || data.Message.GetAction().GetText() != "Send" || data.Message.GetAction().GetData() != "payload" || data.RandomID != 15 {
		t.Fatalf("webview data %+v %v", data, err)
	}
	if _, err := b.WebViewData(ctx, 3, userIn(70), 16, "  ", "x"); err != mtproto.ErrMessageEmpty {
		t.Fatalf("empty button: %v", err)
	}
}
