package e2e

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestSendAsTypingPreviewAndForward covers spreadsheet rows 214, 215, 250, 251, and 252.
// Phones 9996610718, 9996610719, and 9996610720.
func TestSendAsTypingPreviewAndForward(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	a, aSelf := dialPhone(t, ctx, cfg, "9996610718", "previewA")
	b, bSelf := dialPhone(t, ctx, cfg, "9996610719", "previewB")
	c, cSelf := dialPhone(t, ctx, cfg, "9996610720", "previewC")

	groupID, groupHash, err := a.CreateMegagroup(ctx, "preview room")
	if err != nil {
		t.Fatal(err)
	}
	group := inputChannel(groupID, groupHash)
	if err := a.InviteToChannel(ctx, group, inputUser(bSelf.ID, bSelf.AccessHash), inputUser(cSelf.ID, cSelf.AccessHash)); err != nil {
		t.Fatal(err)
	}
	newsID, newsHash := createBroadcast(t, ctx, a, "preview news")

	sent, err := a.Invoke(ctx, &mtproto.TLMessagesSendMessage{
		Constructor: mtproto.CRC32_messages_sendMessage_fef48f62,
		Peer:        inputPeerChannel(groupID, groupHash),
		Message:     "as the channel",
		RandomId:    randomID(),
		SendAs:      inputPeerChannel(newsID, newsHash),
	})
	if err != nil {
		t.Fatal(err)
	}
	sentID := messageID(sent)
	if sentID == 0 {
		t.Fatalf("send_as returned %T", sent)
	}
	for _, who := range []*Client{a, b} {
		hist, err := who.History(ctx, inputPeerChannel(groupID, groupHash), 20)
		if err != nil {
			t.Fatal(err)
		}
		var found *mtproto.Message
		for _, m := range hist {
			if m.GetId() == sentID {
				found = m
			}
		}
		if found.GetFromId().GetChannelId() != newsID {
			t.Fatalf("from_id %#v", found.GetFromId())
		}
	}

	before := len(b.Pushed())
	if _, err := a.Invoke(ctx, &mtproto.TLMessagesSetTyping{
		Constructor: mtproto.CRC32_messages_setTyping,
		Peer:        inputPeerChannel(groupID, groupHash),
		TopMsgId:    &wrapperspb.Int32Value{Value: sentID},
		Action:      mtproto.MakeTLSendMessageTypingAction(&mtproto.SendMessageAction{}).To_SendMessageAction(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		return sawChannelTyping(b.Pushed()[before:], groupID, sentID), nil
	}); err != nil {
		t.Fatal(err)
	}

	edited, err := a.Invoke(ctx, &mtproto.TLMessagesEditMessage{
		Constructor: mtproto.CRC32_messages_editMessage_48f71778,
		Peer:        inputPeerChannel(groupID, groupHash),
		Id:          sentID,
		Message:     &wrapperspb.StringValue{Value: "看 https://tell.cc/addstickers/Twemoji"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = edited
	hist, err := b.History(ctx, inputPeerChannel(groupID, groupHash), 20)
	if err != nil {
		t.Fatal(err)
	}
	var card *mtproto.Message
	for _, m := range hist {
		if m.GetId() == sentID {
			card = m
		}
	}
	if card.GetMedia().GetWebpage() == nil {
		t.Fatalf("edited channel message has no webpage: %#v", card.GetMedia())
	}

	photoPeer := inputPeerUser(bSelf.ID, bSelf.AccessHash)
	photoID := sendPlainPhoto(t, ctx, a, photoPeer, "keep this")
	fwd, err := a.Invoke(ctx, &mtproto.TLMessagesForwardMessages{
		Constructor:       mtproto.CRC32_messages_forwardMessages_13704a7c,
		DropMediaCaptions: true,
		FromPeer:          photoPeer,
		Id:                []int32{photoID},
		RandomId:          []int64{randomID()},
		ToPeer:            inputPeerUser(cSelf.ID, cSelf.AccessHash),
	})
	if err != nil {
		t.Fatal(err)
	}
	if messageID(fwd) == 0 {
		t.Fatalf("forward returned %T", fwd)
	}
	cHist, err := c.History(ctx, inputPeerUser(aSelf.ID, aSelf.AccessHash), 10)
	if err != nil {
		t.Fatal(err)
	}
	var copy *mtproto.Message
	for _, m := range cHist {
		if m.GetMedia().GetPhoto_FLAGPHOTO() != nil {
			copy = m
		}
	}
	if copy == nil || copy.GetMessage() != "" {
		t.Fatalf("caption still present: %#v", copy)
	}

	if _, err := a.Invoke(ctx, &mtproto.TLMessagesToggleNoForwards{
		Constructor: mtproto.CRC32_messages_toggleNoForwards_b11eafa2,
		Peer:        inputPeerChannel(groupID, groupHash),
		Enabled:     mtproto.BoolTrue,
	}); err != nil {
		t.Fatal(err)
	}
	_, err = b.Invoke(ctx, &mtproto.TLMessagesForwardMessages{
		Constructor: mtproto.CRC32_messages_forwardMessages_13704a7c,
		FromPeer:    inputPeerChannel(groupID, groupHash),
		Id:          []int32{sentID},
		RandomId:    []int64{randomID()},
		ToPeer:      inputPeerUser(cSelf.ID, cSelf.AccessHash),
	})
	rpc, ok := asRPCError(err)
	if !ok || rpc.Message != "CHAT_FORWARDS_RESTRICTED" {
		t.Fatalf("protected forward: %v", err)
	}
}

func sawChannelTyping(objs []mtproto.TLObject, channelID int64, top int32) bool {
	for _, obj := range objs {
		for _, u := range updatesOf(obj) {
			if u.GetPredicateName() != mtproto.Predicate_updateChannelUserTyping || u.GetChannelId() != channelID {
				continue
			}
			if u.GetTopMsgId_FLAGINT32().GetValue() == top {
				return true
			}
		}
	}
	return false
}

func sendPlainPhoto(t *testing.T, ctx context.Context, c *Client, to *mtproto.InputPeer, caption string) int32 {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 8), uint8(y * 8), 20, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 60}); err != nil {
		t.Fatal(err)
	}
	fileID := uploadBytes(t, c, buf.Bytes())
	res, err := c.Invoke(ctx, &mtproto.TLMessagesSendMedia{
		Constructor: mtproto.CRC32_messages_sendMedia_7852834e,
		Peer:        to,
		Message:     caption,
		Media: mtproto.MakeTLInputMediaUploadedPhoto(&mtproto.InputMedia{
			File: mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: fileID, Parts: 1, Name: "cap.jpg"}).To_InputFile(),
		}).To_InputMedia(),
		RandomId: randomID(),
	})
	if err != nil {
		t.Fatal(err)
	}
	id := messageID(res)
	if id == 0 {
		t.Fatalf("photo send %T", res)
	}
	return id
}
