package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestSendReactionThumbsUp reacts from the recipient's side and checks both
// sides. In a private chat each user numbers messages in their own box, so the
// same message has a different id for sender and recipient. A note to Saved
// Messages first moves the sender's counter ahead so the two ids never match
// by luck; the server has to map the reaction onto each side's own id.
func TestSendReactionThumbsUp(t *testing.T) {
	a, b, _, bSelf := sharedPair(t)
	c, cancel := contextTimeout(45 * time.Second)
	defer cancel()
	if _, err := a.SendText(c, inputPeerSelf(), fmt.Sprintf("react-skew-%d", time.Now().UnixNano()), nil); err != nil {
		t.Fatal(err)
	}
	text := fmt.Sprintf("react-%d", time.Now().UnixNano())
	peerForA := inputPeerUser(bSelf.ID, bSelf.AccessHash)
	aID, err := a.SendText(c, peerForA, text, nil)
	if err != nil {
		t.Fatal(err)
	}
	peerForB := inputPeerUser(aliceSeen.ID, aliceSeen.AccessHash)
	var bID int32
	err = waitUntil(c, 400*time.Millisecond, func() (bool, error) {
		msgs, err := b.History(c, peerForB, 30)
		if err != nil {
			return false, err
		}
		bID = idOfText(msgs, text)
		return bID != 0, nil
	})
	if err != nil {
		t.Fatalf("recipient never got %q: %v", text, err)
	}
	t.Logf("sender id %d, recipient id %d", aID, bID)
	res, err := b.SendReaction(c, peerForB, bID, "👍")
	if err != nil {
		t.Fatal(err)
	}
	if !reactionIn(res, bID, "👍") && !updateReactionOn(res, bID, "👍") {
		t.Fatalf("messages.sendReaction did not return 👍 on %d: %T", bID, res)
	}
	for _, side := range []struct {
		name string
		c    *Client
		peer *mtproto.InputPeer
		id   int32
	}{
		{"sender", a, peerForA, aID},
		{"recipient", b, peerForB, bID},
	} {
		msgs, err := side.c.History(c, side.peer, 30)
		if err != nil {
			t.Fatal(err)
		}
		if !hasReaction(msgs, side.id, "👍") {
			t.Fatalf("%s history has no 👍 on its message %d", side.name, side.id)
		}
	}
}

func idOfText(msgs []*mtproto.Message, text string) int32 {
	for _, m := range msgs {
		if m != nil && m.GetMessage() == text {
			return m.GetId()
		}
	}
	return 0
}

func updateReactionOn(obj mtproto.TLObject, msgID int32, emoticon string) bool {
	var ups []*mtproto.Update
	switch v := obj.(type) {
	case *mtproto.TLUpdates:
		ups = v.GetUpdates()
	case *mtproto.TLUpdatesCombined:
		ups = v.GetUpdates()
	case *mtproto.TLUpdateShort:
		ups = []*mtproto.Update{v.GetUpdate()}
	}
	for _, u := range ups {
		if u.GetMsgId_INT32() != msgID {
			continue
		}
		m := mtproto.MakeTLMessage(&mtproto.Message{Id: msgID, Reactions: u.GetReactions_MESSAGEREACTIONS()}).To_Message()
		if hasReaction([]*mtproto.Message{m}, msgID, emoticon) {
			return true
		}
	}
	return false
}

func TestAvailableReactionsDownloadable(t *testing.T) {
	a, _, _, _ := sharedPair(t)
	c, cancel := contextTimeout(90 * time.Second)
	defer cancel()
	var reactions []*mtproto.AvailableReaction
	err := waitUntil(c, time.Second, func() (bool, error) {
		res, err := a.Invoke(c, &mtproto.TLMessagesGetAvailableReactions{
			Constructor: mtproto.CRC32_messages_getAvailableReactions,
		})
		if err != nil {
			return false, err
		}
		switch v := res.(type) {
		case *mtproto.TLMessagesAvailableReactions:
			reactions = v.GetReactions()
			return len(reactions) > 0, nil
		case *mtproto.TLMessagesAvailableReactionsNotModified:
			return false, nil
		default:
			return false, fmt.Errorf("messages.getAvailableReactions returned %T", res)
		}
	})
	if err != nil || len(reactions) == 0 {
		t.Fatalf("messages.getAvailableReactions is empty: %v", err)
	}
	var doc *mtproto.Document
	for _, r := range reactions {
		if r == nil {
			continue
		}
		if r.GetStaticIcon() != nil && r.GetStaticIcon().GetId() != 0 {
			doc = r.GetStaticIcon()
			break
		}
	}
	if doc == nil {
		t.Fatal("available reactions have no downloadable static_icon document")
	}
	fileCtx, fileCancel := contextTimeout(10 * time.Second)
	defer fileCancel()
	res, err := a.Invoke(fileCtx, getFile(&documentRef{
		id:   doc.GetId(),
		hash: doc.GetAccessHash(),
		ref:  doc.GetFileReference(),
	}))
	if err != nil {
		t.Fatalf("upload.getFile for reaction %d: %v", doc.GetId(), err)
	}
	if len(bytesOf(res)) == 0 {
		t.Fatalf("upload.getFile returned %T with no bytes", res)
	}
}

type documentRef struct {
	id, hash int64
	ref      []byte
}

func getFile(doc *documentRef) mtproto.TLObject {
	return &mtproto.TLUploadGetFile{
		Constructor:  mtproto.CRC32_upload_getFile_be5335be,
		CdnSupported: true,
		Location: mtproto.MakeTLInputDocumentFileLocation(&mtproto.InputFileLocation{
			Id:            doc.id,
			AccessHash:    doc.hash,
			FileReference: doc.ref,
			ThumbSize:     "",
		}).To_InputFileLocation(),
		Offset_INT64: 0,
		Limit:        128 * 1024,
	}
}

func bytesOf(obj mtproto.TLObject) []byte {
	if f, ok := obj.(*mtproto.TLUploadFile); ok {
		return f.GetBytes()
	}
	return nil
}
