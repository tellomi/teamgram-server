package e2e

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestStoryMentionWritesAPrivateMessage covers spreadsheet row 249.
// Phones 9996610721 (author) and 9996610722 (mentioned).
func TestStoryMentionWritesAPrivateMessage(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	author, authorSelf := dialPhone(t, ctx, cfg, "9996610721", "storyE")
	mentioned, mentionedSelf := dialPhone(t, ctx, cfg, "9996610722", "storyD")
	name := claimUsername(t, ctx, mentioned)
	caption := "hi @" + name
	off := int32(len(utf16.Encode([]rune("hi "))))
	length := int32(len(utf16.Encode([]rune("@" + name))))

	photo := storyPhoto(t, ctx, author)
	up := invokeOK(t, author, &mtproto.TLStoriesSendStory{
		Constructor: mtproto.CRC32_stories_sendStory_8f9e6898,
		Peer:        inputPeerSelf(),
		Media: mtproto.MakeTLInputMediaPhoto(&mtproto.InputMedia{
			Id_INPUTPHOTO: mtproto.MakeTLInputPhoto(&mtproto.InputPhoto{
				Id:            photo.GetId(),
				AccessHash:    photo.GetAccessHash(),
				FileReference: photo.GetFileReference(),
			}).To_InputPhoto(),
		}).To_InputMedia(),
		Caption: &wrapperspb.StringValue{Value: caption},
		Entities: []*mtproto.MessageEntity{
			mtproto.MakeTLInputMessageEntityMentionName(&mtproto.MessageEntity{
				Offset:           0,
				Length:           2,
				UserId_INPUTUSER: inputUser(mentionedSelf.ID, mentionedSelf.AccessHash),
			}).To_MessageEntity(),
			mtproto.MakeTLMessageEntityMention(&mtproto.MessageEntity{
				Offset: off,
				Length: length,
			}).To_MessageEntity(),
		},
		PrivacyRules: []*mtproto.InputPrivacyRule{
			mtproto.MakeTLInputPrivacyValueAllowAll(nil).To_InputPrivacyRule(),
		},
		RandomId: randomID(),
		Period:   &wrapperspb.Int32Value{Value: 86400},
	})
	storyID := storyIDIn(t, up)
	peer := inputPeerUser(authorSelf.ID, authorSelf.AccessHash)
	if err := waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		hist, err := mentioned.History(ctx, peer, 20)
		if err != nil {
			return false, err
		}
		for _, m := range hist {
			media := m.GetMedia()
			if media.GetPredicateName() == mtproto.Predicate_messageMediaStory && media.GetId() == storyID && media.GetViaMention() {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		t.Fatalf("mentioned user has no story message: %v", err)
	}
}

func storyPhoto(t *testing.T, ctx context.Context, c *Client) *mtproto.Photo {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 8), 40, uint8(y * 8), 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 60}); err != nil {
		t.Fatal(err)
	}
	fileID := uploadBytes(t, c, buf.Bytes())
	res, err := c.Invoke(ctx, &mtproto.TLMessagesUploadMedia{
		Constructor: mtproto.CRC32_messages_uploadMedia_14967978,
		Peer:        inputPeerSelf(),
		Media: mtproto.MakeTLInputMediaUploadedPhoto(&mtproto.InputMedia{
			File: mtproto.MakeTLInputFile(&mtproto.InputFile{
				Id_INT64: fileID, Parts: 1, Name: "story.jpg",
			}).To_InputFile(),
		}).To_InputMedia(),
	})
	if err != nil {
		t.Fatal(err)
	}
	media, ok := res.(*mtproto.TLMessageMediaPhoto)
	if !ok || media.GetPhoto_FLAGPHOTO() == nil {
		t.Fatalf("uploadMedia %T", res)
	}
	return media.GetPhoto_FLAGPHOTO()
}

func storyIDIn(t *testing.T, obj mtproto.TLObject) int32 {
	t.Helper()
	for _, u := range updatesOf(obj) {
		if u.GetPredicateName() == mtproto.Predicate_updateStoryID && u.GetId_INT32() != 0 {
			return u.GetId_INT32()
		}
		if id := u.GetStory().GetId(); id != 0 {
			return id
		}
	}
	t.Fatalf("no story id in %T", obj)
	return 0
}
