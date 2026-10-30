package p1rest

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
)

func TestChatTheme(t *testing.T) {
	Reset()
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 7, AccessHash: 1}).To_InputPeer()
	if _, err := SetChatTheme(1, &mtproto.TLMessagesSetChatTheme{Peer: peer}); status.Convert(err).Message() != "EMOJI_INVALID" {
		t.Fatalf("empty: %v", err)
	}
	in := &mtproto.TLMessagesSetChatTheme{
		Peer:  peer,
		Theme: mtproto.MakeTLInputChatTheme(&mtproto.InputChatTheme{Emoticon: "🎄"}).To_InputChatTheme(),
	}
	up, err := SetChatTheme(1, in)
	if err != nil || up.GetPredicateName() != mtproto.Predicate_updates {
		t.Fatalf("set: %v %#v", err, up)
	}
	if _, err := SetChatTheme(1, in); status.Convert(err).Message() != "EMOJI_NOT_MODIFIED" {
		t.Fatalf("same: %v", err)
	}
	if _, err := SetChatTheme(1, &mtproto.TLMessagesSetChatTheme{}); status.Convert(err).Message() != "PEER_ID_INVALID" {
		t.Fatalf("peer: %v", err)
	}
}

func TestWallpaper(t *testing.T) {
	Reset()
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 7}).To_InputPeer()
	if _, err := SetChatWallPaper(1, &mtproto.TLMessagesSetChatWallPaper{Peer: peer}); status.Convert(err).Message() != "WALLPAPER_INVALID" {
		t.Fatalf("missing: %v", err)
	}
	if _, err := SetChatWallPaper(1, &mtproto.TLMessagesSetChatWallPaper{Peer: peer, Revert: true}); status.Convert(err).Message() != "WALLPAPER_NOT_FOUND" {
		t.Fatalf("revert: %v", err)
	}
	up, err := SetChatWallPaper(1, &mtproto.TLMessagesSetChatWallPaper{
		Peer:      peer,
		Wallpaper: mtproto.MakeTLInputWallPaperSlug(&mtproto.InputWallPaper{Slug: "ocean"}).To_InputWallPaper(),
	})
	if err != nil || len(up.GetUpdates()) != 1 || up.Updates[0].GetPredicateName() != mtproto.Predicate_updatePeerWallpaper {
		t.Fatalf("set: %v %#v", err, up)
	}
}

func TestHistoryImport(t *testing.T) {
	Reset()
	if _, err := CheckHistoryImport("hello"); status.Convert(err).Message() != "IMPORT_FORMAT_UNRECOGNIZED" {
		t.Fatalf("bad: %v", err)
	}
	parsed, err := CheckHistoryImport("{\n \"name\": \"Family\",\n \"type\": \"private_group\",\n \"messages\": []\n}")
	if err != nil || !parsed.GetGroup() || parsed.GetTitle().GetValue() != "Family" {
		t.Fatalf("json: %v %#v", err, parsed)
	}
	pm, err := CheckHistoryImport("[31/12/2021, 5:00:00 PM] Ada: hi")
	if err != nil || !pm.GetPm() {
		t.Fatalf("whatsapp: %v %#v", err, pm)
	}
	peer := mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 9}).To_InputPeer()
	checked, err := CheckHistoryImportPeer(1, peer)
	if err != nil || checked.GetConfirmText() == "" {
		t.Fatalf("peer: %v %#v", err, checked)
	}
	if _, err := InitHistoryImport(1, &mtproto.TLMessagesInitHistoryImport{Peer: peer}); status.Convert(err).Message() != "IMPORT_FILE_INVALID" {
		t.Fatalf("file: %v", err)
	}
	job, err := InitHistoryImport(1, &mtproto.TLMessagesInitHistoryImport{
		Peer:       peer,
		File:       mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: 4, Parts: 1, Name: "chat.json"}).To_InputFile(),
		MediaCount: 1,
	})
	if err != nil || job.GetId() == 0 {
		t.Fatalf("init: %v %#v", err, job)
	}
	media, err := UploadImportedMedia(1, &mtproto.TLMessagesUploadImportedMedia{
		Peer:     peer,
		ImportId: job.GetId(),
		FileName: "a.jpg",
		Media:    mtproto.MakeTLInputMediaPhoto(&mtproto.InputMedia{}).To_InputMedia(),
	})
	if err != nil || media.GetPredicateName() != mtproto.Predicate_messageMediaEmpty {
		t.Fatalf("upload: %v %#v", err, media)
	}
	ok, err := StartHistoryImport(1, &mtproto.TLMessagesStartHistoryImport{Peer: peer, ImportId: job.GetId()})
	if err != nil || ok != mtproto.BoolTrue {
		t.Fatalf("start: %v %#v", err, ok)
	}
	if _, err := StartHistoryImport(1, &mtproto.TLMessagesStartHistoryImport{Peer: peer, ImportId: 99}); status.Convert(err).Message() != "IMPORT_ID_INVALID" {
		t.Fatalf("id: %v", err)
	}
}

func TestTodosAndFactCheck(t *testing.T) {
	Reset()
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 3}).To_InputPeer()
	item := mtproto.MakeTLTodoItem(&mtproto.TodoItem{
		Id:    1,
		Title: mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{Text: "milk"}).To_TextWithEntities(),
	}).To_TodoItem()
	if _, err := AppendTodoList(1, &mtproto.TLMessagesAppendTodoList{Peer: peer, MsgId: 0, List: []*mtproto.TodoItem{item}}); status.Convert(err).Message() != "MESSAGE_ID_INVALID" {
		t.Fatalf("id: %v", err)
	}
	if _, err := AppendTodoList(1, &mtproto.TLMessagesAppendTodoList{Peer: peer, MsgId: 4}); status.Convert(err).Message() != "TODO_NOT_MODIFIED" {
		t.Fatalf("empty: %v", err)
	}
	if _, err := AppendTodoList(1, &mtproto.TLMessagesAppendTodoList{Peer: peer, MsgId: 4, List: []*mtproto.TodoItem{item}}); err != nil {
		t.Fatal(err)
	}
	if _, err := AppendTodoList(1, &mtproto.TLMessagesAppendTodoList{Peer: peer, MsgId: 4, List: []*mtproto.TodoItem{item}}); status.Convert(err).Message() != "TODO_ITEM_DUPLICATE" {
		t.Fatalf("dup: %v", err)
	}
	if _, err := ToggleTodoCompleted(1, &mtproto.TLMessagesToggleTodoCompleted{Peer: peer, MsgId: 4, Completed: []int32{1}}); err != nil {
		t.Fatal(err)
	}
	rows, err := GetFactCheck(1, peer, []int32{4, 5})
	if err != nil || len(rows.GetDatas()) != 2 || rows.Datas[0].GetNeedCheck() {
		t.Fatalf("facts: %v %#v", err, rows)
	}
	if _, err := EditFactCheck(1, peer, 4, item.GetTitle()); status.Convert(err).Message() != "CHAT_ACTION_FORBIDDEN" {
		t.Fatalf("edit: %v", err)
	}
	if _, err := DeleteFactCheck(1, peer, 4); status.Convert(err).Message() != "CHAT_ACTION_FORBIDDEN" {
		t.Fatalf("delete: %v", err)
	}
}

func TestComposeGameStatsSuggested(t *testing.T) {
	Reset()
	if _, err := ComposeMessageWithAI(&mtproto.TLMessagesComposeMessageWithAI{}); status.Convert(err).Message() != "MESSAGE_EMPTY" {
		t.Fatalf("empty: %v", err)
	}
	text := mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{Text: "hello there"}).To_TextWithEntities()
	if _, err := ComposeMessageWithAI(&mtproto.TLMessagesComposeMessageWithAI{Text: text}); status.Convert(err).Message() != "AI_COMPOSE_TASK_MISSING" {
		t.Fatalf("task: %v", err)
	}
	out, err := ComposeMessageWithAI(&mtproto.TLMessagesComposeMessageWithAI{Proofread: true, Text: text})
	if err != nil || out.GetResultText().GetText() != "hello there" {
		t.Fatalf("compose: %v %#v", err, out)
	}
	info := EmojiGameInfo()
	if info.GetPredicateName() != mtproto.Predicate_messages_emojiGameUnavailable {
		t.Fatalf("game: %#v", info)
	}
	channel := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 8}).To_InputPeer()
	if _, err := StatsURL(1, &mtproto.TLMessagesGetStatsURL{Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 2}).To_InputPeer()}); status.Convert(err).Message() != "PEER_ID_INVALID" {
		t.Fatalf("stats peer: %v", err)
	}
	page, err := StatsURL(1, &mtproto.TLMessagesGetStatsURL{Peer: channel, Dark: true, Params: "abc"})
	if err != nil || page.GetUrl() == "" {
		t.Fatalf("stats: %v %#v", err, page)
	}
	if _, err := ToggleSuggestedPostApproval(1, &mtproto.TLMessagesToggleSuggestedPostApproval{}); status.Convert(err).Message() != "PEER_ID_INVALID" {
		t.Fatalf("suggest: %v", err)
	}
	if _, err := ToggleSuggestedPostApproval(1, &mtproto.TLMessagesToggleSuggestedPostApproval{Peer: channel, MsgId: 3, Reject: true}); err != nil {
		t.Fatal(err)
	}
	backs := ShareThemeBackdrops(0)
	if backs.GetPredicateName() != mtproto.Predicate_shareThemeBackdrops || len(backs.GetBackdrops()) == 0 {
		t.Fatalf("backs: %#v", backs)
	}
	again := ShareThemeBackdrops(backs.GetHash())
	if again.GetPredicateName() != mtproto.Predicate_shareThemeBackdropsNotModified {
		t.Fatalf("not modified: %#v", again)
	}
	if _, err := SetShareTheme(1, channel, nil); status.Convert(err).Message() != "THEME_INVALID" {
		t.Fatalf("theme: %v", err)
	}
	if _, err := SetShareTheme(1, channel, mtproto.MakeShareThemeDefault()); err != nil {
		t.Fatal(err)
	}
}
