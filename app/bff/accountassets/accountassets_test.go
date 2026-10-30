package accountassets

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func mustEncode(t *testing.T, obj interface {
	Encode(*mtproto.EncodeBuf, int32) error
}) {
	t.Helper()
	if err := obj.Encode(mtproto.NewEncodeBuf(512), 232); err != nil {
		t.Fatal(err)
	}
}

func errIs(t *testing.T, err error, code string) {
	t.Helper()
	if status.Convert(err).Message() != code {
		t.Fatalf("got %v, want %s", err, code)
	}
}

func themeFile() *mtproto.InputFile {
	return mtproto.MakeTLInputFile(&mtproto.InputFile{
		Id_INT64:    10,
		Parts:       1,
		Name:        "day.attheme",
		Md5Checksum: "d41d8cd98f00b204e9800998ecf8427e",
	}).To_InputFile()
}

func inputDoc(doc *mtproto.Document) *mtproto.InputDocument {
	return mtproto.MakeTLInputDocument(&mtproto.InputDocument{
		Id:            doc.GetId(),
		AccessHash:    doc.GetAccessHash(),
		FileReference: doc.GetFileReference(),
	}).To_InputDocument()
}

func TestThemes(t *testing.T) {
	Reset()
	if _, err := GetThemes(1, "desktop", 0); status.Convert(err).Message() != "THEME_FORMAT_INVALID" {
		t.Fatal(err)
	}
	empty, err := GetThemes(1, "android", 0)
	if err != nil || empty.GetPredicateName() != mtproto.Predicate_account_themes || len(empty.GetThemes()) != 0 || empty.GetHash() == 0 {
		t.Fatalf("empty catalog: %v %#v", err, empty)
	}
	mustEncode(t, empty)
	again, err := GetThemes(1, "android", empty.GetHash())
	if err != nil || again.GetPredicateName() != mtproto.Predicate_account_themesNotModified {
		t.Fatalf("not modified: %v %s", err, again.GetPredicateName())
	}
	mustEncode(t, again)
	if _, err := UploadTheme(1, &mtproto.TLAccountUploadTheme{
		File: themeFile(), FileName: "day.attheme", MimeType: "text/plain",
	}); status.Convert(err).Message() != "THEME_MIME_INVALID" {
		t.Fatal(err)
	}
	doc, err := UploadTheme(1, &mtproto.TLAccountUploadTheme{
		File: themeFile(), FileName: "day.attheme", MimeType: "application/x-tgtheme-android",
	})
	if err != nil || doc.GetPredicateName() != mtproto.Predicate_document {
		t.Fatal(err)
	}
	mustEncode(t, doc)
	if _, err := CreateTheme(1, &mtproto.TLAccountCreateTheme{Title: "  "}); status.Convert(err).Message() != "THEME_TITLE_INVALID" {
		t.Fatal(err)
	}
	created, err := CreateTheme(1, &mtproto.TLAccountCreateTheme{
		Title: "Day", Document: inputDoc(doc),
		Settings: []*mtproto.InputThemeSettings{
			mtproto.MakeTLInputThemeSettings(&mtproto.InputThemeSettings{
				BaseTheme:   mtproto.MakeTLBaseThemeDay(&mtproto.BaseTheme{}).To_BaseTheme(),
				AccentColor: 0x2481cc,
			}).To_InputThemeSettings(),
		},
	})
	if err != nil || created.GetSlug() == "" || !created.GetCreator() {
		t.Fatalf("create: %v %#v", err, created)
	}
	mustEncode(t, created)
	if _, err := CreateTheme(1, &mtproto.TLAccountCreateTheme{Slug: created.GetSlug(), Title: "Other", Document: inputDoc(doc)}); status.Convert(err).Message() != "THEME_SLUG_OCCUPIED" {
		t.Fatal(err)
	}
	fresh, err := GetThemes(1, "android", 0)
	if err != nil || len(fresh.GetThemes()) != 0 {
		t.Fatalf("unsaved catalog: %#v", fresh)
	}
	inTheme := mtproto.MakeTLInputTheme(&mtproto.InputTheme{Id: created.GetId(), AccessHash: created.GetAccessHash()}).To_InputTheme()
	if _, err := SaveTheme(1, inTheme, false); err != nil {
		t.Fatal(err)
	}
	listed, err := GetThemes(1, "android", 0)
	if err != nil || len(listed.GetThemes()) != 1 || listed.GetThemes()[0].GetTitle() != "Day" {
		t.Fatalf("saved: %v %#v", err, listed)
	}
	mustEncode(t, listed)
	if ios, err := GetThemes(1, "ios", 0); err != nil || len(ios.GetThemes()) != 0 {
		t.Fatalf("ios filter: %#v", ios)
	}
	got, err := GetTheme(1, "android", mtproto.MakeTLInputThemeSlug(&mtproto.InputTheme{Slug: created.GetSlug()}).To_InputTheme())
	if err != nil || got.GetId() != created.GetId() {
		t.Fatal(err)
	}
	if _, err := GetTheme(1, "android", mtproto.MakeTLInputThemeSlug(&mtproto.InputTheme{Slug: "no such"}).To_InputTheme()); status.Convert(err).Message() != "THEME_SLUG_INVALID" {
		t.Fatal(err)
	}
	updated, err := UpdateTheme(1, &mtproto.TLAccountUpdateTheme{
		Format: "android",
		Theme:  inTheme,
		Title:  wrapperspb.String("Night"),
	})
	if err != nil || updated.GetTitle() != "Night" {
		t.Fatalf("update: %v %#v", err, updated)
	}
	if _, err := InstallTheme(1, &mtproto.TLAccountInstallTheme{
		Dark:   true,
		Theme:  inTheme,
		Format: wrapperspb.String("android"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveTheme(1, inTheme, true); err != nil {
		t.Fatal(err)
	}
	second, err := CreateTheme(1, &mtproto.TLAccountCreateTheme{Title: "Second", Document: inputDoc(doc)})
	if err != nil {
		t.Fatal(err)
	}
	secondIn := mtproto.MakeTLInputTheme(&mtproto.InputTheme{Id: second.GetId(), AccessHash: second.GetAccessHash()}).To_InputTheme()
	if _, err := SaveTheme(1, inTheme, false); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveTheme(1, secondIn, false); err != nil {
		t.Fatal(err)
	}
	two, err := GetThemes(1, "android", 0)
	if err != nil || len(two.GetThemes()) != 2 {
		t.Fatalf("two themes: %v %#v", err, two)
	}
	if got, err := GetThemes(1, "android", two.GetHash()); err != nil || got.GetPredicateName() != mtproto.Predicate_account_themesNotModified {
		t.Fatalf("stable hash: %v %s", err, got.GetPredicateName())
	}
	chats := GetChatThemes(0)
	if chats.GetPredicateName() != mtproto.Predicate_account_themes || chats.GetHash() == 0 {
		t.Fatalf("chat themes: %#v", chats)
	}
	if GetChatThemes(chats.GetHash()).GetPredicateName() != mtproto.Predicate_account_themesNotModified {
		t.Fatal("chat themes not modified")
	}
	gifts := UniqueGiftChatThemes("", 20, 0)
	if gifts.GetPredicateName() != mtproto.Predicate_account_chatThemes || gifts.GetNextOffset_FLAGSTRING() != nil {
		t.Fatalf("gifts: %#v", gifts)
	}
	mustEncode(t, gifts)
	if UniqueGiftChatThemes("", 20, gifts.GetHash()).GetPredicateName() != mtproto.Predicate_account_chatThemesNotModified {
		t.Fatal("gifts not modified")
	}
}

func TestWallpapers(t *testing.T) {
	Reset()
	empty := GetWallPapers(1, 0)
	if empty.GetPredicateName() != mtproto.Predicate_account_wallPapers || empty.GetHash() == 0 {
		t.Fatalf("empty: %#v", empty)
	}
	mustEncode(t, empty)
	if GetWallPapers(1, empty.GetHash()).GetPredicateName() != mtproto.Predicate_account_wallPapersNotModified {
		t.Fatal("not modified")
	}
	file := mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: 4, Parts: 2, Name: "bg.jpg"}).To_InputFile()
	if _, err := UploadWallPaper(1, &mtproto.TLAccountUploadWallPaper{File: file, MimeType: "image/gif"}); status.Convert(err).Message() != "WALLPAPER_MIME_INVALID" {
		t.Fatal(err)
	}
	paper, err := UploadWallPaper(1, &mtproto.TLAccountUploadWallPaper{
		File: file, MimeType: "image/jpeg", ForChat: true,
		Settings: mtproto.MakeTLWallPaperSettings(&mtproto.WallPaperSettings{Blur: true}).To_WallPaperSettings(),
	})
	if err != nil || paper.GetSlug() == "" || paper.GetDocument() == nil {
		t.Fatalf("upload: %v %#v", err, paper)
	}
	mustEncode(t, paper)
	in := mtproto.MakeTLInputWallPaper(&mtproto.InputWallPaper{Id: paper.GetId(), AccessHash: paper.GetAccessHash()}).To_InputWallPaper()
	if _, err := SaveWallPaper(1, &mtproto.TLAccountSaveWallPaper{
		Wallpaper: mtproto.MakeTLInputWallPaperNoFile(&mtproto.InputWallPaper{Id: 1}).To_InputWallPaper(),
		Unsave:    mtproto.BoolFalse,
	}); status.Convert(err).Message() != "WALLPAPER_INVALID" {
		t.Fatal(err)
	}
	if _, err := InstallWallPaper(1, &mtproto.TLAccountInstallWallPaper{Wallpaper: in}); err != nil {
		t.Fatal(err)
	}
	listed := GetWallPapers(1, 0)
	if len(listed.GetWallpapers()) != 1 {
		t.Fatalf("saved: %#v", listed)
	}
	mustEncode(t, listed)
	got, err := GetWallPaper(1, mtproto.MakeTLInputWallPaperSlug(&mtproto.InputWallPaper{Slug: paper.GetSlug()}).To_InputWallPaper())
	if err != nil || got.GetId() != paper.GetId() {
		t.Fatal(err)
	}
	multi, err := GetMultiWallPapers(1, []*mtproto.InputWallPaper{in})
	if err != nil || len(multi.GetDatas()) != 1 {
		t.Fatal(err)
	}
	mustEncode(t, multi)
	if _, err := GetWallPaper(2, mtproto.MakeTLInputWallPaperSlug(&mtproto.InputWallPaper{Slug: "missing"}).To_InputWallPaper()); status.Convert(err).Message() != "WALLPAPER_INVALID" {
		t.Fatal(err)
	}
	if ResetWallPapers(1).GetPredicateName() != mtproto.Predicate_boolTrue {
		t.Fatal("reset")
	}
	if len(GetWallPapers(1, 0).GetWallpapers()) != 0 {
		t.Fatal("reset kept wallpapers")
	}
}

func TestRingtones(t *testing.T) {
	Reset()
	empty := GetSavedRingtones(1, 0)
	if empty.GetHash() == 0 || empty.GetPredicateName() != mtproto.Predicate_account_savedRingtones {
		t.Fatalf("empty: %#v", empty)
	}
	mustEncode(t, empty)
	if GetSavedRingtones(1, empty.GetHash()).GetPredicateName() != mtproto.Predicate_account_savedRingtonesNotModified {
		t.Fatal("not modified")
	}
	file := mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: 9, Parts: 1, Name: "ping.ogg"}).To_InputFile()
	if _, err := UploadRingtone(1, &mtproto.TLAccountUploadRingtone{File: file, FileName: "ping.ogg", MimeType: "audio/wav"}); status.Convert(err).Message() != "RINGTONE_MIME_INVALID" {
		t.Fatal(err)
	}
	ogg, err := UploadRingtone(1, &mtproto.TLAccountUploadRingtone{File: file, FileName: "ping.ogg", MimeType: "audio/ogg"})
	if err != nil {
		t.Fatal(err)
	}
	mustEncode(t, ogg)
	converted, err := SaveRingtone(1, &mtproto.TLAccountSaveRingtone{Id: inputDoc(ogg), Unsave: mtproto.BoolFalse})
	if err != nil || converted.GetPredicateName() != mtproto.Predicate_account_savedRingtoneConverted || converted.GetDocument().GetMimeType() != "audio/mpeg" {
		t.Fatalf("convert: %v %#v", err, converted)
	}
	mustEncode(t, converted)
	saved := GetSavedRingtones(1, 0)
	if len(saved.GetRingtones()) != 1 || saved.GetRingtones()[0].GetId() != converted.GetDocument().GetId() {
		t.Fatalf("saved: %#v", saved)
	}
	mp3, err := UploadRingtone(1, &mtproto.TLAccountUploadRingtone{File: file, FileName: "bell.mp3", MimeType: "audio/mpeg"})
	if err != nil {
		t.Fatal(err)
	}
	plain, err := SaveRingtone(1, &mtproto.TLAccountSaveRingtone{Id: inputDoc(mp3), Unsave: mtproto.BoolFalse})
	if err != nil || plain.GetPredicateName() != mtproto.Predicate_account_savedRingtone {
		t.Fatalf("mp3: %v %s", err, plain.GetPredicateName())
	}
	if _, err := SaveRingtone(1, &mtproto.TLAccountSaveRingtone{Id: inputDoc(mp3), Unsave: mtproto.BoolTrue}); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveRingtone(1, &mtproto.TLAccountSaveRingtone{
		Id:     mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: 99, AccessHash: 1}).To_InputDocument(),
		Unsave: mtproto.BoolFalse,
	}); status.Convert(err).Message() != "RINGTONE_INVALID" {
		t.Fatal(err)
	}
}

func TestConnectedBots(t *testing.T) {
	Reset()
	empty := GetConnectedBots(1)
	if len(empty.GetConnectedBots()) != 0 || empty.GetPredicateName() != mtproto.Predicate_account_connectedBots {
		t.Fatalf("empty: %#v", empty)
	}
	mustEncode(t, empty)
	self := mtproto.MakeTLInputUserSelf(&mtproto.InputUser{}).To_InputUser()
	if _, err := UpdateConnectedBot(1, &mtproto.TLAccountUpdateConnectedBot{Bot: self}); status.Convert(err).Message() != "BOT_BUSINESS_MISSING" {
		t.Fatal(err)
	}
	bot := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 42, AccessHash: 7}).To_InputUser()
	if _, err := UpdateConnectedBot(1, &mtproto.TLAccountUpdateConnectedBot{Bot: bot}); status.Convert(err).Message() != "BUSINESS_RECIPIENTS_EMPTY" {
		t.Fatal(err)
	}
	recipients := mtproto.MakeTLInputBusinessBotRecipients(&mtproto.InputBusinessBotRecipients{Contacts: true}).To_InputBusinessBotRecipients()
	up, err := UpdateConnectedBot(1, &mtproto.TLAccountUpdateConnectedBot{
		Bot: bot, Recipients_INPUTBUSINESSBOTRECIPIENTS: recipients,
		Rights: mtproto.MakeTLBusinessBotRights(&mtproto.BusinessBotRights{Reply: true}).To_BusinessBotRights(),
	})
	if err != nil || up.GetPredicateName() != mtproto.Predicate_updates || len(up.GetUsers()) != 1 || !up.GetUsers()[0].GetBotBusiness() {
		t.Fatalf("connect: %v %#v", err, up)
	}
	mustEncode(t, up)
	listed := GetConnectedBots(1)
	if len(listed.GetConnectedBots()) != 1 || listed.GetConnectedBots()[0].GetBotId() != 42 {
		t.Fatalf("list: %#v", listed)
	}
	mustEncode(t, listed)
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 9}).To_InputPeer()
	if _, err := ToggleConnectedBotPaused(1, nil, true); status.Convert(err).Message() != "PEER_ID_INVALID" {
		t.Fatal(err)
	}
	if _, err := ToggleConnectedBotPaused(1, peer, true); err != nil {
		t.Fatal(err)
	}
	if _, err := DisablePeerConnectedBot(2, peer); status.Convert(err).Message() != "BOT_NOT_CONNECTED_YET" {
		t.Fatal(err)
	}
	if _, err := DisablePeerConnectedBot(1, peer); err != nil {
		t.Fatal(err)
	}
	if _, err := DisablePeerConnectedBot(1, peer); status.Convert(err).Message() != "BOT_ALREADY_DISABLED" {
		t.Fatal(err)
	}
	if _, err := UpdateConnectedBot(1, &mtproto.TLAccountUpdateConnectedBot{Deleted: true, Bot: bot}); err != nil {
		t.Fatal(err)
	}
	if len(GetConnectedBots(1).GetConnectedBots()) != 0 {
		t.Fatal("bot still connected")
	}
}

func TestEmojiCatalogs(t *testing.T) {
	list := EmojiList(0)
	if list.GetPredicateName() != mtproto.Predicate_emojiList || list.GetHash() == 0 || len(list.GetDocumentId()) != 0 {
		t.Fatalf("emoji: %#v", list)
	}
	mustEncode(t, list)
	if EmojiList(list.GetHash()).GetPredicateName() != mtproto.Predicate_emojiListNotModified {
		t.Fatal("emoji not modified")
	}
	statuses := CollectibleStatuses(0)
	if statuses.GetPredicateName() != mtproto.Predicate_account_emojiStatuses || len(statuses.GetStatuses()) != 0 {
		t.Fatalf("statuses: %#v", statuses)
	}
	mustEncode(t, statuses)
	if CollectibleStatuses(statuses.GetHash()).GetPredicateName() != mtproto.Predicate_account_emojiStatusesNotModified {
		t.Fatal("statuses not modified")
	}
}

func TestHashZeroReturnsTheList(t *testing.T) {
	Reset()
	if EmptyHash == 0 {
		t.Fatal("empty hash")
	}
	first, err := GetThemes(3, "tdesktop", 0)
	if err != nil || first.GetPredicateName() != mtproto.Predicate_account_themes {
		t.Fatal(err)
	}
	second, err := GetThemes(3, "tdesktop", 0)
	if err != nil || second.GetPredicateName() != mtproto.Predicate_account_themes {
		t.Fatal("hash 0 must return the list")
	}
}

func TestUploadThemeNil(t *testing.T) {
	Reset()
	_, err := UploadTheme(3, nil)
	errIs(t, err, "THEME_FILE_INVALID")
}
