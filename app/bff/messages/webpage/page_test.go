package webpage

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestStickerLinkUsesTitleNotShortName(t *testing.T) {
	t.Setenv(chatbox.EnvServerLinkFetch, "")
	docs := twemojiDocs(t)
	short := fmt.Sprintf("erelpack%d", time.Now().UnixNano()%100000)
	created, err := catalog.Must().CreateSet(catalog.CreateInput{
		Self:      7,
		Title:     "erel",
		ShortName: short,
		Stickers: []*mtproto.InputStickerSetItem{
			stickerItem(docs[0], "😀"),
			stickerItem(docs[1], "😁"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.GetSet().GetTitle() != "erel" || created.GetSet().GetShortName() != short {
		t.Fatalf("pack title %q short %q", created.GetSet().GetTitle(), created.GetSet().GetShortName())
	}

	for _, raw := range []string{
		"https://tell.cc/addstickers/" + short,
		"https://www.tell.cc/addstickers/" + short,
		"https://t.me/addstickers/" + short,
		"http://telegram.me/AddStickers/" + short,
	} {
		page, _, _, err := chatbox.WebPageContext(context.Background(), raw, 0)
		if err != nil {
			t.Fatal(raw, err)
		}
		if page.GetPredicateName() != mtproto.Predicate_webPage || page.GetType().GetValue() != "telegram_stickerset" {
			t.Fatalf("%s type %s %s", raw, page.GetPredicateName(), page.GetType().GetValue())
		}
		if page.GetTitle().GetValue() != "erel" {
			t.Fatalf("%s title %q", raw, page.GetTitle().GetValue())
		}
		if !strings.Contains(page.GetUrl_STRING(), short) || strings.Contains(page.GetTitle().GetValue(), short) {
			t.Fatalf("url %q title %q", page.GetUrl_STRING(), page.GetTitle().GetValue())
		}
		if page.GetDescription().GetValue() != "Free stickerpack with 2 stickers." {
			t.Fatalf("description %q", page.GetDescription().GetValue())
		}
		if page.GetDocument() == nil || len(page.GetAttributes()) != 1 || page.GetAttributes()[0].GetPredicateName() != mtproto.Predicate_webPageAttributeStickerSet {
			t.Fatalf("attribute %#v", page.GetAttributes())
		}
		if page.GetAttributes()[0].GetEmojis() {
			t.Fatal("sticker pack marked as emoji")
		}
	}

	same, _, _, err := chatbox.WebPageContext(context.Background(), "https://tell.cc/addstickers/"+short, pageHash(t, short))
	if err != nil || same.GetPredicateName() != mtproto.Predicate_webPageNotModified {
		t.Fatalf("not modified %s %v", same.GetPredicateName(), err)
	}

	missing, _, _, err := chatbox.WebPageContext(context.Background(), "https://tell.cc/addstickers/nosuchpackzz", 0)
	if err != nil || missing.GetPredicateName() != mtproto.Predicate_webPageEmpty {
		t.Fatalf("missing %s %v", missing.GetPredicateName(), err)
	}

	media := PreviewMedia(context.Background(), "https://tell.cc/addstickers/"+short, nil)
	if media.GetWebpage().GetType().GetValue() != "telegram_stickerset" {
		t.Fatalf("send attach %#v", media)
	}
	if PreviewMedia(context.Background(), "https://www.apple.com", nil) != nil {
		t.Fatal("external url was attached on send")
	}
}

func TestEmojiPackFlag(t *testing.T) {
	docs := twemojiDocs(t)
	short := fmt.Sprintf("eremoji%d", time.Now().UnixNano()%100000)
	_, err := catalog.Must().CreateSet(catalog.CreateInput{
		Self:      7,
		Title:     "erel emoji",
		ShortName: short,
		Emojis:    true,
		Stickers:  []*mtproto.InputStickerSetItem{stickerItem(docs[2], "😀")},
	})
	if err != nil {
		t.Fatal(err)
	}
	page, _, _, err := chatbox.WebPageContext(context.Background(), "https://tell.cc/addemoji/"+short, 0)
	if err != nil {
		t.Fatal(err)
	}
	if page.GetType().GetValue() != "telegram_stickerset" || !page.GetAttributes()[0].GetEmojis() {
		t.Fatalf("%s emojis %v", page.GetType().GetValue(), page.GetAttributes())
	}
	if page.GetDescription().GetValue() != "Free emoji pack with 1 emoji." || page.GetTitle().GetValue() != "erel emoji" {
		t.Fatalf("title %q desc %q", page.GetTitle().GetValue(), page.GetDescription().GetValue())
	}
}

func TestChannelUserAndMessageCards(t *testing.T) {
	ctx := context.Background()
	svc := channel.Shared()
	suffix := time.Now().UnixNano() % 100000000
	newsName := fmt.Sprintf("n%08d", suffix)
	groupName := fmt.Sprintf("g%08d", suffix+1)

	news, err := svc.Create(ctx, channel.CreateRequest{CreatorID: 11, Title: "Erel News", About: "daily notes", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	newsChat := news.Chats[0]
	if _, err := svc.UpdateUsername(ctx, 11, 0, inputCh(newsChat), newsName); err != nil {
		t.Fatal(err)
	}
	sent, err := svc.Send(ctx, 11, newsChat.Id, 0, []channel.Outgoing{{
		RandomID: 1,
		Message:  mtproto.MakeTLMessage(&mtproto.Message{Message: "hello erel post"}).To_Message(),
	}})
	if err != nil {
		t.Fatal(err)
	}
	mid := sentID(t, sent)

	page, chats, _, err := chatbox.WebPageContext(ctx, "https://tell.cc/"+newsName, 0)
	if err != nil {
		t.Fatal(err)
	}
	if page.GetType().GetValue() != "telegram_channel" || page.GetTitle().GetValue() != "Erel News" {
		t.Fatalf("channel %s %q", page.GetType().GetValue(), page.GetTitle().GetValue())
	}
	if page.GetDescription().GetValue() != "daily notes" || len(chats) != 1 || chats[0].GetId() != newsChat.Id {
		t.Fatalf("about %q chats %d", page.GetDescription().GetValue(), len(chats))
	}

	msg, msgChats, _, err := chatbox.WebPageContext(ctx, fmt.Sprintf("https://t.me/%s/%d", newsName, mid), 0)
	if err != nil {
		t.Fatal(err)
	}
	if msg.GetType().GetValue() != "telegram_message" || msg.GetDescription().GetValue() != "hello erel post" || len(msgChats) != 1 {
		t.Fatalf("message %s %q chats %d", msg.GetType().GetValue(), msg.GetDescription().GetValue(), len(msgChats))
	}
	byID, _, _, err := chatbox.WebPageContext(ctx, fmt.Sprintf("https://tell.cc/c/%d/%d", newsChat.Id, mid), 0)
	if err != nil || byID.GetType().GetValue() != "telegram_message" {
		t.Fatalf("c path %s %v", byID.GetType().GetValue(), err)
	}

	group, err := svc.Create(ctx, channel.CreateRequest{CreatorID: 11, Title: "Erel Group", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateUsername(ctx, 11, 0, inputCh(group.Chats[0]), groupName); err != nil {
		t.Fatal(err)
	}
	gpage, _, _, err := chatbox.WebPageContext(ctx, "https://tell.cc/"+groupName, 0)
	if err != nil || gpage.GetType().GetValue() != "telegram_megagroup" || gpage.GetTitle().GetValue() != "Erel Group" {
		t.Fatalf("group %s %q %v", gpage.GetType().GetValue(), gpage.GetTitle().GetValue(), err)
	}

	secret, err := svc.Create(ctx, channel.CreateRequest{CreatorID: 11, Title: "Hidden Room", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := svc.Send(ctx, 11, secret.Chats[0].Id, 0, []channel.Outgoing{{
		RandomID: 2,
		Message:  mtproto.MakeTLMessage(&mtproto.Message{Message: "secret-erel-body"}).To_Message(),
	}})
	if err != nil {
		t.Fatal(err)
	}
	hid := sentID(t, hidden)
	priv, _, _, err := chatbox.WebPageContext(ctx, fmt.Sprintf("https://tell.cc/c/%d/%d", secret.Chats[0].Id, hid), 0)
	if err != nil || priv.GetPredicateName() != mtproto.Predicate_webPageEmpty {
		t.Fatalf("private %s %v", priv.GetPredicateName(), err)
	}
	if strings.Contains(priv.String(), "secret-erel-body") {
		t.Fatal("private message leaked into the card")
	}

	link, err := svc.ExportInvite(ctx, 11, inputCh(group.Chats[0]), channel.ExportRequest{LegacyRevokePermanent: true})
	if err != nil {
		t.Fatal(err)
	}
	inv, _, _, err := chatbox.WebPageContext(ctx, link.GetLink(), 0)
	if err != nil || inv.GetType().GetValue() != "telegram_megagroup" || inv.GetTitle().GetValue() != "Erel Group" {
		t.Fatalf("invite %s %q %v link %s", inv.GetType().GetValue(), inv.GetTitle().GetValue(), err, link.GetLink())
	}

	defer BindUserResolver(nil)
	BindUserResolver(func(context.Context, string) (UserCard, bool) {
		return UserCard{}, false
	})
	missingUser, _, _, err := chatbox.WebPageContext(ctx, "https://tell.cc/nouserzz", 0)
	if err != nil || missingUser.GetPredicateName() != mtproto.Predicate_webPageEmpty {
		t.Fatalf("missing user %s %v", missingUser.GetPredicateName(), err)
	}
	BindUserResolver(func(_ context.Context, name string) (UserCard, bool) {
		if name != "pupi" {
			return UserCard{}, false
		}
		return UserCard{
			ID:    42,
			Title: "普皮",
			About: "hello",
			User: mtproto.MakeTLUser(&mtproto.User{
				Id:        42,
				FirstName: wrapperspb.String("普皮"),
			}).To_User(),
		}, true
	})
	userPage, _, users, err := chatbox.WebPageContext(ctx, "https://tell.cc/pupi", 0)
	if err != nil {
		t.Fatal(err)
	}
	if userPage.GetType().GetValue() != "telegram_user" || userPage.GetTitle().GetValue() != "普皮" || userPage.GetDescription().GetValue() != "hello" {
		t.Fatalf("user %s %q %q", userPage.GetType().GetValue(), userPage.GetTitle().GetValue(), userPage.GetDescription().GetValue())
	}
	if len(users) != 1 || users[0].GetId() != 42 {
		t.Fatalf("users %#v", users)
	}
}

func TestClientSubmittedCard(t *testing.T) {
	longTitle := strings.Repeat("测", 200)
	in := mtproto.MakeTLInputMediaWebPage(&mtproto.InputMedia{
		Url:             "https://www.apple.com/iphone",
		Title:           longTitle,
		Description:     "A phone",
		Provider_STRING: "Apple",
		Id_INPUTPHOTO: mtproto.MakeTLInputPhoto(&mtproto.InputPhoto{
			Id:            99,
			AccessHash:    7,
			FileReference: []byte{1, 2, 3},
		}).To_InputPhoto(),
		ForceLargeMedia: true,
	}).To_InputMedia()
	if !ClientSupplied(in) {
		t.Fatal("extension not detected")
	}
	photo := mtproto.MakeTLPhoto(&mtproto.Photo{Id: 99, DcId: 2}).To_Photo()
	media, err := ClientMedia(context.Background(), in, func(context.Context, *mtproto.InputPhoto) *mtproto.Photo {
		return photo
	})
	if err != nil {
		t.Fatal(err)
	}
	page := media.GetWebpage()
	if page.GetType().GetValue() != "photo" || page.GetAuthor().GetValue() != UnverifiedAuthor {
		t.Fatalf("type %s author %s", page.GetType().GetValue(), page.GetAuthor().GetValue())
	}
	if page.GetTitle().GetValue() != strings.Repeat("测", maxTitleRunes) || page.GetSiteName().GetValue() != "Apple" {
		t.Fatalf("title len %d site %q", len([]rune(page.GetTitle().GetValue())), page.GetSiteName().GetValue())
	}
	if page.GetPhoto().GetId() != 99 || !page.GetHasLargeMedia() || !media.GetForceLargeMedia() {
		t.Fatalf("photo %#v", page.GetPhoto())
	}

	noPhoto, err := ClientMedia(context.Background(), in, func(context.Context, *mtproto.InputPhoto) *mtproto.Photo {
		return nil
	})
	if err != nil || noPhoto.GetWebpage().GetPhoto() != nil || noPhoto.GetWebpage().GetType().GetValue() != "article" {
		t.Fatalf("dropped photo %#v %v", noPhoto.GetWebpage(), err)
	}
	if noPhoto.GetWebpage().GetAuthor().GetValue() != UnverifiedAuthor {
		t.Fatal("missing unverified mark")
	}

	_, err = ClientMedia(context.Background(), mtproto.MakeTLInputMediaWebPage(&mtproto.InputMedia{
		Url:   "http://127.0.0.1/admin",
		Title: "nope",
	}).To_InputMedia(), nil)
	if err != mtproto.ErrWebpageCurlFailed {
		t.Fatalf("loopback %v", err)
	}
	skipped, err := ClientMedia(context.Background(), mtproto.MakeTLInputMediaWebPage(&mtproto.InputMedia{
		Url:      "http://127.0.0.1/admin",
		Title:    "nope",
		Optional: true,
	}).To_InputMedia(), nil)
	if err != nil || skipped != nil {
		t.Fatalf("optional %+v %v", skipped, err)
	}
	_, err = ClientMedia(context.Background(), mtproto.MakeTLInputMediaWebPage(&mtproto.InputMedia{
		Url:   "https://example.com",
		Title: strings.Repeat("a", maxFieldBytes+1),
	}).To_InputMedia(), nil)
	if err != mtproto.ErrMediaInvalid {
		t.Fatalf("overlong %v", err)
	}

	buf := mtproto.NewEncodeBuf(512)
	if err := in.Encode(buf, mtproto.Layer); err != nil {
		t.Fatal(err)
	}
	decoded := &mtproto.InputMedia{}
	if err := decoded.Decode(mtproto.NewDecodeBuf(buf.GetBuf())); err != nil {
		t.Fatal(err)
	}
	if decoded.GetTitle() != longTitle || decoded.GetDescription() != "A phone" || decoded.GetProvider_STRING() != "Apple" {
		t.Fatalf("decoded %#v", decoded)
	}
	if decoded.GetId_INPUTPHOTO().GetId() != 99 || decoded.GetId_INPUTPHOTO().GetAccessHash() != 7 || !decoded.GetForceLargeMedia() {
		t.Fatalf("photo flags %#v", decoded.GetId_INPUTPHOTO())
	}

	plain := mtproto.MakeTLInputMediaWebPage(&mtproto.InputMedia{Url: "https://www.apple.com"}).To_InputMedia()
	buf = mtproto.NewEncodeBuf(128)
	if err := plain.Encode(buf, mtproto.Layer); err != nil {
		t.Fatal(err)
	}
	official := &mtproto.InputMedia{}
	if err := official.Decode(mtproto.NewDecodeBuf(buf.GetBuf())); err != nil {
		t.Fatal(err)
	}
	if official.GetUrl() != "https://www.apple.com" || official.GetTitle() != "" || ClientSupplied(official) {
		t.Fatalf("official %#v", official)
	}
}

func pageHash(t *testing.T, short string) int32 {
	t.Helper()
	page, _, _, err := chatbox.WebPageContext(context.Background(), "https://tell.cc/addstickers/"+short, 0)
	if err != nil {
		t.Fatal(err)
	}
	return page.GetHash()
}

func twemojiDocs(t *testing.T) []*mtproto.Document {
	t.Helper()
	set, err := catalog.Must().GetSet(1, mtproto.MakeTLInputStickerSetShortName(&mtproto.InputStickerSet{
		ShortName: catalog.ShortTwemoji,
	}).To_InputStickerSet(), 0)
	if err != nil {
		t.Fatal(err)
	}
	docs := set.GetDocuments()
	if len(docs) < 3 {
		t.Fatal("twemoji is too small")
	}
	return docs
}

func stickerItem(doc *mtproto.Document, emoji string) *mtproto.InputStickerSetItem {
	return mtproto.MakeTLInputStickerSetItem(&mtproto.InputStickerSetItem{
		Document: mtproto.MakeTLInputDocument(&mtproto.InputDocument{
			Id:         doc.GetId(),
			AccessHash: doc.GetAccessHash(),
		}).To_InputDocument(),
		Emoji: emoji,
	}).To_InputStickerSetItem()
}

func inputCh(ch *mtproto.Chat) *mtproto.InputChannel {
	hash := int64(0)
	if v := ch.GetAccessHash_FLAGINT64(); v != nil {
		hash = v.Value
	}
	return mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  ch.Id,
		AccessHash: hash,
	}).To_InputChannel()
}

func sentID(t *testing.T, up *mtproto.Updates) int32 {
	t.Helper()
	for _, u := range up.GetUpdates() {
		if msg := u.GetMessage_MESSAGE(); msg != nil && msg.GetId() != 0 {
			return msg.GetId()
		}
	}
	t.Fatalf("no message id")
	return 0
}
