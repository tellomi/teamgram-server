package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestChannelFullChatPhoto is ISSUE-S-CHAN-FULLPHOTO-001 (问题清单第 358 行).
// After channels.editPhoto, channels.getFullChannel.chat_photo is the stored
// photo for the owner and a member, and the same id as chat.photo.photo_id.
// inputChatPhotoEmpty makes it photoEmpty. messages.getFullChat already
// loads the basic-group photo the same way; this checks that path too.
// Phones 9996610801 and 9996610802, code 11111.
func TestChannelFullChatPhoto(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	owner, _ := dialPhone(t, ctx, cfg, "9996610801", "PhotoOwn")
	member, memberSelf := dialPhone(t, ctx, cfg, "9996610802", "PhotoMem")
	memberID, memberHash, err := owner.ImportPhone(ctx, "9996610802", "PhotoMem")
	if err != nil {
		t.Fatal(err)
	}

	groupID, groupHash, err := owner.CreateMegagroup(ctx, "FullPhoto")
	if err != nil {
		t.Fatal(err)
	}
	group := inputChannel(groupID, groupHash)
	if err = owner.InviteToChannel(ctx, group, inputUser(memberID, memberHash)); err != nil {
		t.Fatal(err)
	}
	memberGroup := waitChannel(t, ctx, member, groupID)

	file := uploadChatJPEG(t, ctx, owner, "group.jpg", 1)
	updated, err := owner.Invoke(ctx, &mtproto.TLChannelsEditPhoto{
		Constructor: mtproto.CRC32_channels_editPhoto,
		Channel:     group,
		Photo: mtproto.MakeTLInputChatUploadedPhoto(&mtproto.InputChatPhoto{
			File: file,
		}).To_InputChatPhoto(),
	})
	if err != nil {
		t.Fatal(err)
	}
	photoID := editPhotoID(t, updated)
	assertFullPhoto(t, ctx, owner, group, groupID, photoID)
	assertFullPhoto(t, ctx, member, memberGroup, groupID, photoID)

	if _, err = owner.Invoke(ctx, &mtproto.TLChannelsEditPhoto{
		Constructor: mtproto.CRC32_channels_editPhoto,
		Channel:     group,
		Photo:       mtproto.MakeTLInputChatPhotoEmpty(nil).To_InputChatPhoto(),
	}); err != nil {
		t.Fatal(err)
	}
	assertFullPhoto(t, ctx, owner, group, groupID, 0)
	assertFullPhoto(t, ctx, member, memberGroup, groupID, 0)

	chatID, err := owner.CreateChat(ctx, "FullPhotoBasic", inputUser(memberSelf.ID, memberSelf.AccessHash))
	if err != nil {
		t.Fatal(err)
	}
	basicFile := uploadChatJPEG(t, ctx, owner, "basic.jpg", 2)
	basicUpdated, err := owner.Invoke(ctx, &mtproto.TLMessagesEditChatPhoto{
		Constructor: mtproto.CRC32_messages_editChatPhoto,
		ChatId:      chatID,
		Photo: mtproto.MakeTLInputChatUploadedPhoto(&mtproto.InputChatPhoto{
			File: basicFile,
		}).To_InputChatPhoto(),
	})
	if err != nil {
		t.Fatal(err)
	}
	basicID := editPhotoID(t, basicUpdated)
	assertBasicFullPhoto(t, ctx, owner, chatID, basicID)
	assertBasicFullPhoto(t, ctx, member, chatID, basicID)
	if _, err = owner.Invoke(ctx, &mtproto.TLMessagesEditChatPhoto{
		Constructor: mtproto.CRC32_messages_editChatPhoto,
		ChatId:      chatID,
		Photo:       mtproto.MakeTLInputChatPhotoEmpty(nil).To_InputChatPhoto(),
	}); err != nil {
		t.Fatal(err)
	}
	assertBasicFullPhoto(t, ctx, owner, chatID, 0)
	assertBasicFullPhoto(t, ctx, member, chatID, 0)
}

func uploadChatJPEG(t *testing.T, ctx context.Context, c *Client, name string, seed byte) *mtproto.InputFile {
	t.Helper()
	fileID := randomID()
	if _, err := c.Invoke(ctx, &mtproto.TLUploadSaveFilePart{
		Constructor: mtproto.CRC32_upload_saveFilePart,
		FileId:      fileID,
		FilePart:    0,
		Bytes:       testJPEG(t, 800, 800, seed),
	}); err != nil {
		t.Fatal(err)
	}
	return mtproto.MakeTLInputFile(&mtproto.InputFile{
		Id_INT64: fileID,
		Parts:    1,
		Name:     name,
	}).To_InputFile()
}

func editPhotoID(t *testing.T, obj mtproto.TLObject) int64 {
	t.Helper()
	var fromAction, fromCard int64
	for _, msg := range allMessages(obj) {
		act := msg.GetAction()
		if act.GetPredicateName() == mtproto.Predicate_messageActionChatEditPhoto && act.GetPhoto().GetId() != 0 {
			fromAction = act.GetPhoto().GetId()
			break
		}
	}
	for _, chat := range chatsOf(obj) {
		if id := chat.GetPhoto().GetPhotoId(); id != 0 {
			fromCard = id
			break
		}
	}
	if fromAction == 0 && fromCard == 0 {
		t.Fatalf("editPhoto updates %T have no chat photo", obj)
	}
	if fromAction != 0 && fromCard != 0 && fromAction != fromCard {
		t.Fatalf("service photo %d, chat.photo %d", fromAction, fromCard)
	}
	if fromAction != 0 {
		return fromAction
	}
	return fromCard
}

func assertFullPhoto(t *testing.T, ctx context.Context, c *Client, channel *mtproto.InputChannel, channelID, photoID int64) {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLChannelsGetFullChannel{
		Constructor: mtproto.CRC32_channels_getFullChannel,
		Channel:     channel,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertChatFullPhoto(t, messagesChatFull(t, obj), channelID, photoID)
}

func assertBasicFullPhoto(t *testing.T, ctx context.Context, c *Client, chatID, photoID int64) {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLMessagesGetFullChat{
		Constructor: mtproto.CRC32_messages_getFullChat,
		ChatId:      chatID,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertChatFullPhoto(t, messagesChatFull(t, obj), chatID, photoID)
}

func messagesChatFull(t *testing.T, obj mtproto.TLObject) *mtproto.Messages_ChatFull {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.TLMessagesChatFull:
		return v.To_Messages_ChatFull()
	case *mtproto.Messages_ChatFull:
		return v
	default:
		t.Fatalf("messages.chatFull %T", obj)
		return nil
	}
}

func assertChatFullPhoto(t *testing.T, box *mtproto.Messages_ChatFull, chatID, photoID int64) {
	t.Helper()
	full := box.GetFullChat()
	var card *mtproto.Chat
	for _, chat := range box.GetChats() {
		if chat.GetId() == chatID {
			card = chat
			break
		}
	}
	if full == nil || card == nil {
		t.Fatalf("full chat %d missing (%v card)", chatID, card != nil)
	}
	photo := full.GetChatPhoto()
	if photoID == 0 {
		if photo.GetPredicateName() != mtproto.Predicate_photoEmpty {
			t.Fatalf("chat %d chat_photo %s id %d", chatID, photo.GetPredicateName(), photo.GetId())
		}
		return
	}
	if photo.GetPredicateName() != mtproto.Predicate_photo {
		t.Fatalf("chat %d chat_photo %s", chatID, photo.GetPredicateName())
	}
	if len(photo.GetSizes()) == 0 && len(photo.GetVideoSizes()) == 0 {
		t.Fatalf("chat %d chat_photo %d has no sizes", chatID, photo.GetId())
	}
	if photo.GetId() != photoID || card.GetPhoto().GetPhotoId() != photoID {
		t.Fatalf("chat %d chat_photo %d card %d want %d", chatID, photo.GetId(), card.GetPhoto().GetPhotoId(), photoID)
	}
}
