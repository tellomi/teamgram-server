package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

func TestSendReactionThumbsUp(t *testing.T) {
	a, b, _, bSelf := sharedPair(t)
	c, cancel := contextTimeout(45 * time.Second)
	defer cancel()
	text := fmt.Sprintf("react-%d", time.Now().UnixNano())
	peerForA := inputPeerUser(bSelf.ID, bSelf.AccessHash)
	msgID, err := a.SendText(c, peerForA, text, nil)
	if err != nil {
		t.Fatal(err)
	}
	peerForB := inputPeerUser(aliceSeen.ID, aliceSeen.AccessHash)
	err = waitUntil(c, 400*time.Millisecond, func() (bool, error) {
		msgs, err := b.History(c, peerForB, 30)
		if err != nil {
			return false, err
		}
		return hasText(msgs, text), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.SendReaction(c, peerForB, msgID, "👍")
	if err != nil {
		t.Fatal(err)
	}
	if !reactionIn(res, msgID, "👍") {
		err = waitUntil(c, 400*time.Millisecond, func() (bool, error) {
			for _, side := range []struct {
				c    *Client
				peer *mtproto.InputPeer
			}{
				{a, peerForA},
				{b, peerForB},
			} {
				msgs, err := side.c.History(c, side.peer, 30)
				if err != nil {
					return false, err
				}
				if !hasReaction(msgs, msgID, "👍") {
					return false, nil
				}
			}
			return true, nil
		})
		if err != nil {
			t.Fatalf("👍 did not stay on message %d as MessageReactions: %v", msgID, err)
		}
	}
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
