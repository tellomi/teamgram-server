package e2e

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Sheet rows 149, 320, 334–341. Phones stay in 9996610381–9996610389.

func TestAcceptContactSharesPhone(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(3 * time.Minute)
	defer cancel()
	a, aSelf := dialPhone(t, ctx, cfg, "9996610381", "ShareA")
	e, eSelf := dialPhone(t, ctx, cfg, "9996610382", "ShareE")
	if err := setPhoneNobody(ctx, a); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = setPhoneEveryone(ctx, a) }()
	aID, aHash, err := e.ImportPhone(ctx, "9996610381", "ShareA")
	if err != nil {
		t.Fatal(err)
	}
	if aID != aSelf.ID {
		t.Fatalf("imported %d want %d", aID, aSelf.ID)
	}
	if _, err = e.SendText(ctx, inputPeerUser(aSelf.ID, aHash), "hello from e", nil); err != nil {
		t.Fatal(err)
	}
	if got := userPhone(t, ctx, e, aSelf.ID, aHash); got != "" {
		t.Fatalf("phone visible before accept: %q", got)
	}
	eHash := waitUserHash(t, ctx, a, eSelf.ID)
	if flags := waitPeerFlags(t, ctx, a, eSelf.ID, eHash, true); !flags.GetAddContact() {
		t.Fatalf("bar %#v", flags)
	}
	if _, err = a.Invoke(ctx, &mtproto.TLContactsAcceptContact{
		Constructor: mtproto.CRC32_contacts_acceptContact,
		Id:          inputUser(eSelf.ID, eHash),
	}); err != nil {
		t.Fatal(err)
	}
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		return userPhone(t, ctx, e, aSelf.ID, aHash) != "", nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestChatThemeWallpaperOnFullUser(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(3 * time.Minute)
	defer cancel()
	a, aSelf := dialPhone(t, ctx, cfg, "9996610383", "ThemeA")
	b, bSelf := dialPhone(t, ctx, cfg, "9996610384", "ThemeB")
	bID, bHash, err := a.ImportPhone(ctx, "9996610384", "ThemeB")
	if err != nil {
		t.Fatal(err)
	}
	peerB := inputPeerUser(bID, bHash)
	if _, err = a.SendText(ctx, peerB, "hi", nil); err != nil {
		t.Fatal(err)
	}
	aHash := waitUserHash(t, ctx, b, aSelf.ID)
	if _, err = a.Invoke(ctx, &mtproto.TLMessagesSetChatTheme{
		Constructor: mtproto.CRC32_messages_setChatTheme_81202c9,
		Peer:        peerB,
		Theme:       mtproto.MakeTLInputChatTheme(&mtproto.InputChatTheme{Emoticon: "🎄"}).To_InputChatTheme(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = a.Invoke(ctx, &mtproto.TLMessagesSetChatWallPaper{
		Constructor: mtproto.CRC32_messages_setChatWallPaper,
		Peer:        peerB,
		ForBoth:     true,
		Wallpaper:   mtproto.MakeTLInputWallPaperNoFile(&mtproto.InputWallPaper{Id: 88001}).To_InputWallPaper(),
		Settings: mtproto.MakeTLWallPaperSettings(&mtproto.WallPaperSettings{
			BackgroundColor: wrapperspb.Int32(0x336699),
		}).To_WallPaperSettings(),
	}); err != nil {
		t.Fatal(err)
	}
	peerA := inputPeerUser(aSelf.ID, aHash)
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		return historyHasAction(t, b, peerA, mtproto.Predicate_messageActionSetChatWallPaper), nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, side := range []struct {
		c    *Client
		id   int64
		hash int64
	}{
		{a, bID, bHash},
		{b, aSelf.ID, aHash},
		{b, bSelf.ID, bSelf.AccessHash},
	} {
		if side.id == bSelf.ID {
			continue
		}
		full := loadFullUser(t, ctx, side.c, side.id, side.hash)
		// Layer 232 userFull carries theme:ChatTheme. theme_emoticon is not on that constructor.
		if full.GetTheme().GetEmoticon() != "🎄" {
			t.Fatalf("theme %#v", full.GetTheme())
		}
		if full.GetWallpaper().GetId() == 0 || full.GetWallpaper().GetSettings().GetBackgroundColor().GetValue() != 0x336699 {
			t.Fatalf("wallpaper %#v", full.GetWallpaper())
		}
	}
}

func TestChannelOwnPostEditAndDelete(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(3 * time.Minute)
	defer cancel()
	owner, _ := dialPhone(t, ctx, cfg, "9996610385", "PostOwner")
	poster, _ := dialPhone(t, ctx, cfg, "9996610386", "Poster")
	// Broadcast channel: only post_messages, own posts stay editable.
	created, err := owner.Invoke(ctx, &mtproto.TLChannelsCreateChannel{
		Constructor: mtproto.CRC32_channels_createChannel_91006707,
		Broadcast:   true,
		Title:       "zt-broadcast",
	})
	if err != nil {
		t.Fatal(err)
	}
	var bID, bHash int64
	for _, chat := range chatsOf(created) {
		if chat.GetBroadcast() {
			bID, bHash = chat.GetId(), chatAccessHash(chat)
		}
	}
	if bID == 0 {
		t.Fatalf("channel %T", created)
	}
	posterID, posterHash, err := owner.ImportPhone(ctx, "9996610386", "Poster")
	if err != nil {
		t.Fatal(err)
	}
	channel := inputChannel(bID, bHash)
	if err = owner.InviteToChannel(ctx, channel, inputUser(posterID, posterHash)); err != nil {
		t.Fatal(err)
	}
	if _, err = owner.Invoke(ctx, &mtproto.TLChannelsEditAdmin{
		Constructor:     mtproto.CRC32_channels_editAdmin_d33c8902,
		Channel:         channel,
		UserId:          inputUser(posterID, posterHash),
		AdminRights:     mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{PostMessages: true}).To_ChatAdminRights(),
		Rank_STRING:     "poster",
		Rank_FLAGSTRING: wrapperspb.String("poster"),
	}); err != nil {
		t.Fatal(err)
	}
	var memberHash int64
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		hash, ok, err := poster.ChatAccess(ctx, bID)
		if err != nil || !ok {
			return false, err
		}
		memberHash = hash
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	peer := inputPeerChannel(bID, memberHash)
	msgID, err := poster.SendText(ctx, peer, "own post", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = poster.Invoke(ctx, &mtproto.TLMessagesEditMessage{
		Constructor: mtproto.CRC32_messages_editMessage_48f71778,
		Peer:        peer,
		Id:          msgID,
		Message:     wrapperspb.String("own post edited"),
	}); err != nil {
		t.Fatalf("edit own post: %v", err)
	}
	if _, err = poster.Invoke(ctx, &mtproto.TLChannelsDeleteMessages{
		Constructor: mtproto.CRC32_channels_deleteMessages,
		Channel:     inputChannel(bID, memberHash),
		Id:          []int32{msgID},
	}); err != nil {
		t.Fatalf("delete own post: %v", err)
	}
}

func TestEditReturnsNotModified(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(2 * time.Minute)
	defer cancel()
	a, _ := dialPhone(t, ctx, cfg, "9996610387", "EditA")
	_, bSelf := dialPhone(t, ctx, cfg, "9996610388", "EditB")
	bID, bHash, err := a.ImportPhone(ctx, "9996610388", "EditB")
	if err != nil {
		t.Fatal(err)
	}
	_ = bSelf
	peer := inputPeerUser(bID, bHash)
	id, err := a.SendText(ctx, peer, "original", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.Invoke(ctx, &mtproto.TLMessagesEditMessage{
		Constructor: mtproto.CRC32_messages_editMessage_48f71778,
		Peer:        peer,
		Id:          id,
		Message:     wrapperspb.String("changed"),
	}); err != nil {
		t.Fatal(err)
	}
	_, err = a.Invoke(ctx, &mtproto.TLMessagesEditMessage{
		Constructor: mtproto.CRC32_messages_editMessage_48f71778,
		Peer:        peer,
		Id:          id,
		Message:     wrapperspb.String("changed"),
	})
	if err == nil || !strings.Contains(err.Error(), "MESSAGE_NOT_MODIFIED") {
		t.Fatalf("same edit: %v", err)
	}
}

func TestUserReplyMarkupIgnored(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(2 * time.Minute)
	defer cancel()
	a, _ := dialPhone(t, ctx, cfg, "9996610387", "MarkA")
	b, _ := dialPhone(t, ctx, cfg, "9996610388", "MarkB")
	bID, bHash, err := a.ImportPhone(ctx, "9996610388", "MarkB")
	if err != nil {
		t.Fatal(err)
	}
	aHash := waitUserHash(t, ctx, b, mustSelf(t, ctx, a).ID)
	markup := mtproto.MakeTLReplyInlineMarkup(&mtproto.ReplyMarkup{
		Rows_VECTORKEYBOARDINLINEBUTTONROW: []*mtproto.KeyboardInlineButtonRow{
			mtproto.MakeTLKeyboardInlineButtonRow(&mtproto.KeyboardInlineButtonRow{
				Buttons: []*mtproto.KeyboardInlineButton{
					mtproto.MakeTLKeyboardInlineButton(&mtproto.KeyboardInlineButton{
						Text: "go",
						Type: mtproto.MakeTLInlineButtonTypeUrl(&mtproto.InlineButtonType{Url: "https://example.com/zt"}).To_InlineButtonType(),
					}).To_KeyboardInlineButton(),
				},
			}).To_KeyboardInlineButtonRow(),
		},
	}).To_ReplyMarkup()
	if _, err = sendWithMarkup(ctx, a, inputPeerUser(bID, bHash), "button text", markup); err != nil {
		t.Fatal(err)
	}
	// Private chats keep a box id per user. The sender's id is not the recipient's id.
	msg := historyByText(t, ctx, b, inputPeerUser(mustSelf(t, ctx, a).ID, aHash), "button text")
	if msg.GetReplyMarkup() != nil {
		t.Fatalf("markup leaked %#v", msg.GetReplyMarkup())
	}
}

func TestTextURLSchemeDropped(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(2 * time.Minute)
	defer cancel()
	a, aSelf := dialPhone(t, ctx, cfg, "9996610387", "UrlA")
	b, _ := dialPhone(t, ctx, cfg, "9996610388", "UrlB")
	bID, bHash, err := a.ImportPhone(ctx, "9996610388", "UrlB")
	if err != nil {
		t.Fatal(err)
	}
	aHash := waitUserHash(t, ctx, b, aSelf.ID)
	text := "click"
	if _, err = a.SendText(ctx, inputPeerUser(bID, bHash), text, []*mtproto.MessageEntity{
		mtproto.MakeTLMessageEntityTextUrl(&mtproto.MessageEntity{Offset: 0, Length: 5, Url: "javascript:alert(1)"}).To_MessageEntity(),
	}); err != nil {
		t.Fatal(err)
	}
	msg := historyByText(t, ctx, b, inputPeerUser(aSelf.ID, aHash), text)
	for _, ent := range msg.GetEntities() {
		if ent.GetPredicateName() == mtproto.Predicate_messageEntityTextUrl {
			t.Fatalf("javascript url kept %#v", ent)
		}
	}
}

func TestMessageAndCaptionLength(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(3 * time.Minute)
	defer cancel()
	a, aSelf := dialPhone(t, ctx, cfg, "9996610387", "LenA")
	b, _ := dialPhone(t, ctx, cfg, "9996610388", "LenB")
	bID, bHash, err := a.ImportPhone(ctx, "9996610388", "LenB")
	if err != nil {
		t.Fatal(err)
	}
	aHash := waitUserHash(t, ctx, b, aSelf.ID)
	peer := inputPeerUser(bID, bHash)
	long := strings.Repeat("a", 4096)
	if _, err = a.SendText(ctx, peer, long, nil); err != nil {
		t.Fatal(err)
	}
	msg := historyByText(t, ctx, b, inputPeerUser(aSelf.ID, aHash), long)
	if len(msg.GetMessage()) != 4096 {
		t.Fatalf("4096 stored as %d", len(msg.GetMessage()))
	}
	_, err = a.SendText(ctx, peer, strings.Repeat("a", 4097), nil)
	if err == nil || !strings.Contains(err.Error(), "MESSAGE_TOO_LONG") {
		t.Fatalf("4097: %v", err)
	}
	fileID := uploadBytes(t, a, tinyJPEG(t))
	if _, err = sendCaption(ctx, a, peer, fileID, strings.Repeat("c", 1024)); err != nil {
		t.Fatalf("caption 1024: %v", err)
	}
	_, err = sendCaption(ctx, a, peer, fileID, strings.Repeat("c", 1025))
	if err == nil || !strings.Contains(err.Error(), "MEDIA_CAPTION_TOO_LONG") {
		t.Fatalf("caption 1025: %v", err)
	}
}

func TestStoriesHiddenOnUser(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(2 * time.Minute)
	defer cancel()
	a, _ := dialPhone(t, ctx, cfg, "9996610387", "StoryA")
	_, bSelf := dialPhone(t, ctx, cfg, "9996610388", "StoryB")
	bID, bHash, err := a.ImportPhone(ctx, "9996610388", "StoryB")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.Invoke(ctx, &mtproto.TLContactsAddContact{
		Constructor: mtproto.CRC32_contacts_addContact_d9ba2e54,
		Id:          inputUser(bID, bHash),
		FirstName:   "StoryB",
		LastName:    "Hidden",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = a.Invoke(ctx, &mtproto.TLStoriesTogglePeerStoriesHidden{
		Constructor: mtproto.CRC32_stories_togglePeerStoriesHidden,
		Peer:        inputPeerUser(bID, bHash),
		Hidden:      mtproto.ToBool(true),
	}); err != nil {
		t.Fatal(err)
	}
	if !userStoriesHidden(t, ctx, a, bID, bHash) {
		t.Fatal("stories_hidden stayed false")
	}
	if _, err = a.Invoke(ctx, &mtproto.TLStoriesTogglePeerStoriesHidden{
		Constructor: mtproto.CRC32_stories_togglePeerStoriesHidden,
		Peer:        inputPeerUser(bID, bHash),
		Hidden:      mtproto.ToBool(false),
	}); err != nil {
		t.Fatal(err)
	}
	if userStoriesHidden(t, ctx, a, bID, bHash) {
		t.Fatal("stories_hidden stayed true")
	}
	_ = bSelf
}

func TestChannelTitleCountsRunes(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(2 * time.Minute)
	defer cancel()
	a, _ := dialPhone(t, ctx, cfg, "9996610389", "TitleA")
	emoji := "zt" + strings.Repeat("🙂", 80)
	created, err := a.Invoke(ctx, &mtproto.TLChannelsCreateChannel{
		Constructor: mtproto.CRC32_channels_createChannel_91006707,
		Megagroup:   true,
		Title:       emoji,
	})
	if err != nil {
		t.Fatalf("emoji title: %v", err)
	}
	var id, hash int64
	var title string
	for _, chat := range chatsOf(created) {
		if chat.GetMegagroup() {
			id, hash, title = chat.GetId(), chatAccessHash(chat), chat.GetTitle()
		}
	}
	if title != emoji {
		t.Fatalf("emoji title stored %q", title)
	}
	long := strings.Repeat("群", 200)
	edited, err := a.Invoke(ctx, &mtproto.TLChannelsEditTitle{
		Constructor: mtproto.CRC32_channels_editTitle,
		Channel:     inputChannel(id, hash),
		Title:       long,
	})
	if err != nil {
		t.Fatalf("chinese title: %v", err)
	}
	got := ""
	for _, chat := range chatsOf(edited) {
		if chat.GetId() == id {
			got = chat.GetTitle()
		}
	}
	if len([]rune(got)) != 128 {
		t.Fatalf("title runes %d %q", len([]rune(got)), got)
	}
}

func setPhoneEveryone(ctx context.Context, c *Client) error {
	_, err := c.Invoke(ctx, &mtproto.TLAccountSetPrivacy{
		Constructor: mtproto.CRC32_account_setPrivacy,
		Key:         mtproto.MakeTLInputPrivacyKeyPhoneNumber(&mtproto.InputPrivacyKey{}).To_InputPrivacyKey(),
		Rules: []*mtproto.InputPrivacyRule{
			mtproto.MakeTLInputPrivacyValueAllowAll(&mtproto.InputPrivacyRule{}).To_InputPrivacyRule(),
		},
	})
	return err
}

func loadFullUser(t *testing.T, ctx context.Context, c *Client, id, hash int64) *mtproto.UserFull {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLUsersGetFullUser{
		Constructor: mtproto.CRC32_users_getFullUser,
		Id:          inputUser(id, hash),
	})
	if err != nil {
		t.Fatal(err)
	}
	return fullUserOf(t, obj)
}

func historyByText(t *testing.T, ctx context.Context, c *Client, peer *mtproto.InputPeer, text string) *mtproto.Message {
	t.Helper()
	var found *mtproto.Message
	if err := waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		page, err := c.History(ctx, peer, 40)
		if err != nil {
			return false, err
		}
		for _, m := range page {
			if m.GetMessage() == text {
				found = m
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		t.Fatalf("history missing %q: %v", text, err)
	}
	return found
}

func historyHasAction(t *testing.T, c *Client, peer *mtproto.InputPeer, name string) bool {
	t.Helper()
	ctx, cancel := contextTimeout(8 * time.Second)
	defer cancel()
	ok := false
	// The wallpaper service message is written by the inbox consumer after
	// setChatWallPaper returns. One getHistory error is not the failure.
	_ = waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		res, err := c.Invoke(ctx, &mtproto.TLMessagesGetHistory{
			Constructor: mtproto.CRC32_messages_getHistory,
			Peer:        peer,
			Limit:       20,
		})
		if err != nil || ctx.Err() != nil {
			return false, nil
		}
		msgs, good := messagesOf(res)
		if !good {
			return false, nil
		}
		for _, m := range msgs {
			if m.GetAction().GetPredicateName() == name {
				ok = true
				return true, nil
			}
		}
		return false, nil
	})
	return ok
}

func sendWithMarkup(ctx context.Context, c *Client, peer *mtproto.InputPeer, text string, markup *mtproto.ReplyMarkup) (int32, error) {
	res, err := c.Invoke(ctx, &mtproto.TLMessagesSendMessage{
		Constructor: mtproto.CRC32_messages_sendMessage_fef48f62,
		Peer:        peer,
		Message:     text,
		RandomId:    randomID(),
		ReplyMarkup: markup,
	})
	if err != nil {
		return 0, err
	}
	id := messageID(res)
	if id == 0 {
		return 0, fmt.Errorf("send with markup returned %T", res)
	}
	return id, nil
}

func sendCaption(ctx context.Context, c *Client, peer *mtproto.InputPeer, fileID int64, caption string) (int32, error) {
	res, err := c.Invoke(ctx, &mtproto.TLMessagesSendMedia{
		Constructor: mtproto.CRC32_messages_sendMedia_7852834e,
		Peer:        peer,
		Media: mtproto.MakeTLInputMediaUploadedPhoto(&mtproto.InputMedia{
			File: mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: fileID, Parts: 1, Name: "cap.jpg"}).To_InputFile(),
		}).To_InputMedia(),
		Message:  caption,
		RandomId: randomID(),
	})
	if err != nil {
		return 0, err
	}
	return messageID(res), nil
}

func tinyJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	img.Set(0, 0, color.RGBA{R: 20, G: 40, B: 80, A: 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 50}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func userStoriesHidden(t *testing.T, ctx context.Context, c *Client, id, hash int64) bool {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLUsersGetUsers{
		Constructor: mtproto.CRC32_users_getUsers,
		Id:          []*mtproto.InputUser{inputUser(id, hash)},
	})
	if err != nil {
		t.Fatal(err)
	}
	user := userByID(usersOf(obj), id)
	return user.GetStoriesHidden()
}

func mustSelf(t *testing.T, ctx context.Context, c *Client) Self {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLUsersGetUsers{
		Constructor: mtproto.CRC32_users_getUsers,
		Id:          []*mtproto.InputUser{inputUserSelf()},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, user := range usersOf(obj) {
		if user.GetSelf() {
			return Self{ID: user.GetId(), AccessHash: user.GetAccessHash().GetValue()}
		}
	}
	t.Fatal("self user missing")
	return Self{}
}
