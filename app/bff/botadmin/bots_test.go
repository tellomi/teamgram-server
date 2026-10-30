package botadmin

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"

	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func codeOf(err error) string {
	if err == nil {
		return ""
	}
	if st, ok := status.FromError(err); ok {
		return st.Message()
	}
	return err.Error()
}

func manager() *mtproto.InputUser {
	return mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: userpb.BotFatherId}).To_InputUser()
}

func asUser(id, hash int64) *mtproto.InputUser {
	return mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: id, AccessHash: hash}).To_InputUser()
}

func photo(raw string) *mtproto.InputMedia {
	return mtproto.MakeTLInputMediaPhotoExternal(&mtproto.InputMedia{Url: raw}).To_InputMedia()
}

func TestCheckAndCreateBot(t *testing.T) {
	Reset()
	ctx := context.Background()
	if _, err := CheckUsername(ctx, 0, false, "newsbot"); codeOf(err) != "AUTH_KEY_UNREGISTERED" {
		t.Fatal(codeOf(err))
	}
	if _, err := CheckUsername(ctx, 7, false, "hello"); codeOf(err) != "USERNAME_SUFFIX_MISSING" {
		t.Fatal(codeOf(err))
	}
	if _, err := CheckUsername(ctx, 7, false, "bot"); codeOf(err) != "USERNAME_INVALID" {
		t.Fatal(codeOf(err))
	}
	if _, err := CheckUsername(ctx, 7, true, "news"); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckUsername(ctx, 7, false, "newsbot"); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateBot(ctx, 7, false, "", "newsbot", manager()); codeOf(err) != "NAME_INVALID" {
		t.Fatal(codeOf(err))
	}
	if _, err := CreateBot(ctx, 7, false, "News", "newsbot", asUser(99, 1)); codeOf(err) != "MANAGER_INVALID" {
		t.Fatal(codeOf(err))
	}
	got, err := CreateBot(ctx, 7, true, "News", "newsbot", manager())
	if err != nil {
		t.Fatal(err)
	}
	if !got.GetBot() || got.GetUsername().GetValue() != "newsbot" || got.GetFirstName().GetValue() != "News" {
		t.Fatalf("user %+v", got)
	}
	if _, err := CheckUsername(ctx, 7, false, "NewsBot"); codeOf(err) != "USERNAME_OCCUPIED" {
		t.Fatal(codeOf(err))
	}
	if _, err := CreateBot(ctx, 7, false, "Other", "newsbot", manager()); codeOf(err) != "USERNAME_OCCUPIED" {
		t.Fatal(codeOf(err))
	}
	// A bot without bot_can_manage_bots cannot be the manager.
	if _, err := CreateBot(ctx, 8, false, "Child", "childbot", asUser(got.GetId(), got.GetAccessHash().GetValue())); codeOf(err) != "MANAGER_PERMISSION_MISSING" {
		t.Fatal(codeOf(err))
	}
	grantManager(got.GetId())
	child, err := CreateBot(ctx, 8, false, "Child", "childbot", asUser(got.GetId(), got.GetAccessHash().GetValue()))
	if err != nil {
		t.Fatal(err)
	}
	if child.GetId() == got.GetId() {
		t.Fatal("ids collided")
	}
	list, err := AdminedBots(7)
	if err != nil || len(list.GetDatas()) != 1 || list.GetDatas()[0].GetId() != got.GetId() {
		t.Fatalf("admined %+v %v", list, err)
	}
	if _, err := GetBotInfo(ctx, 7, asUser(got.GetId(), got.GetAccessHash().GetValue()), "en"); err != nil {
		t.Fatal(err)
	}
	if _, err := SetBotInfo(ctx, 7, asUser(got.GetId(), got.GetAccessHash().GetValue()), "en", wrapperspb.String("News EN"), wrapperspb.String(strings.Repeat("a", 121)), nil); codeOf(err) != "ABOUT_TOO_LONG" {
		t.Fatal(codeOf(err))
	}
	if _, err := SetBotInfo(ctx, 7, asUser(got.GetId(), got.GetAccessHash().GetValue()), "en", wrapperspb.String("News EN"), wrapperspb.String("about"), wrapperspb.String("desc")); err != nil {
		t.Fatal(err)
	}
	info, err := GetBotInfo(ctx, 7, asUser(got.GetId(), got.GetAccessHash().GetValue()), "en")
	if err != nil || info.GetName() != "News EN" || info.GetAbout() != "about" || info.GetDescription() != "desc" {
		t.Fatalf("info %+v %v", info, err)
	}
	if _, err := GetBotInfo(ctx, 9, asUser(got.GetId(), got.GetAccessHash().GetValue()), ""); codeOf(err) != "BOT_INVALID" {
		t.Fatal(codeOf(err))
	}
}

func TestUsernamesPreviewsAndMiniApp(t *testing.T) {
	Reset()
	ctx := context.Background()
	bot, err := CreateBot(ctx, 7, false, "News", "newsbot", manager())
	if err != nil {
		t.Fatal(err)
	}
	in := asUser(bot.GetId(), bot.GetAccessHash().GetValue())
	if _, err := AddUsername(ctx, 7, in, "extra"); err != nil {
		t.Fatal(err)
	}
	if _, err := ReorderUsernames(ctx, 7, in, []string{"extra"}); codeOf(err) != "ORDER_INVALID" {
		t.Fatal(codeOf(err))
	}
	if _, err := ReorderUsernames(ctx, 7, in, []string{"extra", "newsbot"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ToggleUsername(ctx, 7, in, "extra", false); err != nil {
		t.Fatal(err)
	}
	if _, err := ToggleUsername(ctx, 7, in, "extra", false); codeOf(err) != "USERNAME_NOT_MODIFIED" {
		t.Fatal(codeOf(err))
	}
	if _, err := RemoveUsername(ctx, 7, in, "newsbot"); codeOf(err) != "USERNAME_INVALID" {
		t.Fatal(codeOf(err))
	}
	if _, err := RemoveUsername(ctx, 7, in, "extra"); err != nil {
		t.Fatal(err)
	}

	a := photo("https://cdn.example/a.jpg")
	b := photo("https://cdn.example/b.jpg")
	first, err := AddPreviewMedia(ctx, 7, in, "", a)
	if err != nil || first.GetMedia() == nil {
		t.Fatal(err)
	}
	if _, err := AddPreviewMedia(ctx, 7, in, "en", b); err != nil {
		t.Fatal(err)
	}
	edited, err := EditPreviewMedia(ctx, 7, in, "", a, photo("https://cdn.example/c.jpg"))
	if err != nil || edited.GetMedia().GetPhoto_FLAGPHOTO().GetId() == 0 && edited.GetMedia() == nil {
		t.Fatal(err)
	}
	info, err := PreviewInfo(ctx, 7, in, "")
	if err != nil || len(info.GetMedia()) != 1 || len(info.GetLangCodes()) != 2 {
		t.Fatalf("preview info %+v %v", info, err)
	}
	medias, err := PreviewMedias(ctx, 3, in)
	if err != nil || len(medias.GetDatas()) != 1 {
		t.Fatal(err)
	}
	if _, err := ReorderPreviewMedias(ctx, 7, in, "en", []*mtproto.InputMedia{a}); codeOf(err) != "ORDER_INVALID" {
		t.Fatal(codeOf(err))
	}
	if _, err := DeletePreviewMedia(ctx, 7, in, "en", []*mtproto.InputMedia{b}); err != nil {
		t.Fatal(err)
	}
	pop, err := PopularAppBots(7, "", 10)
	if err != nil || len(pop.GetUsers()) != 1 || pop.GetUsers()[0].GetId() != bot.GetId() {
		t.Fatalf("popular %+v %v", pop, err)
	}
	rec, err := Recommendations(ctx, 3, in)
	if err != nil || len(rec.GetUsers()) != 0 {
		t.Fatalf("recs %+v %v", rec, err)
	}

	can, err := CanSendMessage(ctx, 3, in)
	if err != nil || mtproto.FromBool(can) {
		t.Fatal(can, err)
	}
	ups, err := AllowSendMessage(ctx, 3, in)
	if err != nil || len(ups.GetUpdates()) != 1 || len(ups.GetUsers()) != 1 {
		t.Fatalf("allow %+v %v", ups, err)
	}
	can, err = CanSendMessage(ctx, 3, in)
	if err != nil || !mtproto.FromBool(can) {
		t.Fatal(can, err)
	}
	ok, err := CheckDownloadFileParams(ctx, 3, in, "pic.jpg", "https://cdn.example/pic.jpg")
	if err != nil || !mtproto.FromBool(ok) {
		t.Fatal(ok, err)
	}
	ok, err = CheckDownloadFileParams(ctx, 3, in, "../x", "https://cdn.example/pic.jpg")
	if err != nil || mtproto.FromBool(ok) {
		t.Fatal("bad name should be boolFalse", ok, err)
	}
	if _, err := InvokeWebViewCustomMethod(ctx, 3, in, "", &mtproto.DataJSON{Data: `{}`}); codeOf(err) != "METHOD_INVALID" {
		t.Fatal(codeOf(err))
	}
	raw, err := InvokeWebViewCustomMethod(ctx, 3, in, "ping", mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: `{"n":1}`}).To_DataJSON())
	if err != nil || raw.GetData() != `{"ok":true}` {
		t.Fatalf("custom %v %v", raw, err)
	}

	req, err := RequestWebViewButton(ctx, bot.GetId(), asUser(3, 1), mtproto.MakeTLKeyboardButton(&mtproto.KeyboardButton{Text: "Open"}).To_KeyboardButton())
	if err != nil || req.GetWebappReqId() == "" {
		t.Fatal(err)
	}
	btn, err := RequestedWebViewButton(ctx, 3, in, req.GetWebappReqId())
	if err != nil || btn.GetText() != "Open" {
		t.Fatal(btn, err)
	}
	if _, err := RequestedWebViewButton(ctx, 3, in, "missing"); codeOf(err) != "QUERY_ID_INVALID" {
		t.Fatal(codeOf(err))
	}
}

func TestStarRefVerificationEmojiAndCommands(t *testing.T) {
	Reset()
	ctx := context.Background()
	bot, err := CreateBot(ctx, 7, false, "News", "newsbot", manager())
	if err != nil {
		t.Fatal(err)
	}
	in := asUser(bot.GetId(), bot.GetAccessHash().GetValue())
	prog, err := UpdateStarRefProgram(ctx, 7, in, 100, wrapperspb.Int32(3))
	if err != nil || prog.GetCommissionPermille() != 100 || prog.GetDurationMonths().GetValue() != 3 {
		t.Fatalf("%+v %v", prog, err)
	}
	if _, err := UpdateStarRefProgram(ctx, 7, in, 50, nil); codeOf(err) != "STARREF_PERMILLE_TOO_LOW" {
		t.Fatal(codeOf(err))
	}
	if _, err := UpdateStarRefProgram(ctx, 7, in, 100, wrapperspb.Int32(1)); codeOf(err) != "STARREF_PERMILLE_INVALID" {
		t.Fatal(codeOf(err))
	}
	if _, err := UpdateStarRefProgram(ctx, 7, in, 200, nil); err != nil {
		t.Fatal(err)
	}
	ended, err := UpdateStarRefProgram(ctx, 7, in, 0, nil)
	if err != nil || ended.GetEndDate().GetValue() == 0 {
		t.Fatalf("end %+v %v", ended, err)
	}
	if _, err := UpdateStarRefProgram(ctx, 7, in, 100, nil); codeOf(err) != "STARREF_AWAITING_END" {
		t.Fatal(codeOf(err))
	}
	mu.Lock()
	w.bots[bot.GetId()].star.end = time.Now().Add(-time.Hour).Unix()
	mu.Unlock()
	if _, err := UpdateStarRefProgram(ctx, 7, in, 80, nil); err != nil {
		t.Fatal(err)
	}

	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 42, AccessHash: 1}).To_InputPeer()
	if _, err := SetCustomVerification(ctx, 7, true, in, peer, "ok"); codeOf(err) != "BOT_VERIFIER_FORBIDDEN" {
		t.Fatal(codeOf(err))
	}
	if _, err := SetCustomVerification(ctx, 7, true, in, nil, ""); codeOf(err) != "PEER_ID_INVALID" {
		t.Fatal(codeOf(err))
	}
	grantVerifier(bot.GetId())
	if _, err := SetCustomVerification(ctx, 7, true, in, peer, "checked"); err != nil {
		t.Fatal(err)
	}

	if _, err := ToggleEmojiPermission(ctx, 4, in, true); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateUserEmojiStatus(ctx, 4, asUser(7, 1), nil); codeOf(err) != "USER_BOT_INVALID" {
		t.Fatal(codeOf(err))
	}
	if _, err := UpdateUserEmojiStatus(ctx, bot.GetId(), asUser(4, 1), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateUserEmojiStatus(ctx, bot.GetId(), asUser(5, 1), nil); codeOf(err) != "USER_PRIVACY_RESTRICTED" {
		t.Fatal(codeOf(err))
	}

	cmds := []*mtproto.BotCommand{
		mtproto.MakeTLBotCommand(&mtproto.BotCommand{Command: "start", Description: "Start"}).To_BotCommand(),
	}
	if _, err := SetCommands(ctx, 7, nil, "", cmds); codeOf(err) != "USER_BOT_INVALID" {
		t.Fatal(codeOf(err))
	}
	if _, err := SetCommands(ctx, bot.GetId(), nil, "", cmds); err != nil {
		t.Fatal(err)
	}
	got, err := Commands(ctx, bot.GetId(), nil, "")
	if err != nil || len(got.GetDatas()) != 1 || got.GetDatas()[0].GetCommand() != "start" {
		t.Fatalf("%+v %v", got, err)
	}
	tok, err := ExportToken(ctx, 7, in, false)
	if err != nil || !strings.Contains(tok.GetToken(), ":") {
		t.Fatal(tok, err)
	}
	again, err := ExportToken(ctx, 7, in, true)
	if err != nil || again.GetToken() == tok.GetToken() {
		t.Fatal("revoke should rotate the token")
	}
	if _, err := SetJoinResult(ctx, bot.GetId(), 99, mtproto.MakeTLJoinChatBotResultApproved(nil).To_JoinChatBotResult()); err != nil {
		t.Fatal(err)
	}
	access, err := AccessSettings(ctx, 7, in)
	if err != nil || access.GetRestricted() {
		t.Fatal(access, err)
	}
	if _, err := EditAccessSettings(ctx, 7, in, true, []*mtproto.InputUser{asUser(4, 1)}); err != nil {
		t.Fatal(err)
	}
	access, err = AccessSettings(ctx, 7, in)
	if err != nil || !access.GetRestricted() || len(access.GetAddUsers()) != 1 {
		t.Fatalf("%+v %v", access, err)
	}
}

func TestCreateLimitAndBadHash(t *testing.T) {
	Reset()
	ctx := context.Background()
	var last *mtproto.User
	for i := 0; i < botsCreateLimit; i++ {
		name := "n" + strings.Repeat("x", 2) + string(rune('a'+i%26)) + "bot"
		// Usernames must be unique and end in bot. Build them from the index.
		uname := "u" + pad(i) + "bot"
		u, err := CreateBot(ctx, 7, false, "N", uname, manager())
		if err != nil {
			t.Fatal(i, err)
		}
		last = u
		_ = name
	}
	if _, err := CreateBot(ctx, 7, false, "N", "overflowbot", manager()); codeOf(err) != "BOT_CREATE_LIMIT_EXCEEDED" {
		t.Fatal(codeOf(err))
	}
	if _, err := GetBotInfo(ctx, 7, asUser(last.GetId(), last.GetAccessHash().GetValue()+1), ""); codeOf(err) != "BOT_INVALID" {
		t.Fatal(codeOf(err))
	}
}

func pad(i int) string {
	if i < 10 {
		return "0" + string(rune('0'+i))
	}
	return string(rune('0'+i/10)) + string(rune('0'+i%10))
}
