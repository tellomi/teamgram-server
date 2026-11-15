package e2e

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestShareStoryIntoChat is ISSUE-S-STORY-SHARE-001 (表格行 119).
// inputMediaStory is stored as messageMediaStory, not messageMediaUnsupported.
// Phones 9996610645 and 9996610646, code 11111, layer 232.
func TestShareStoryIntoChat(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(3 * time.Minute)
	defer cancel()
	a, aSelf := dialPhone(t, ctx, cfg, "9996610645", "StoryA")
	b, _ := dialPhone(t, ctx, cfg, "9996610646", "StoryB")
	bID, bHash, err := a.ImportPhone(ctx, "9996610646", "StoryB")
	if err != nil {
		t.Fatal(err)
	}
	storyID, callID, access := startLive(t, ctx, a, Self{ID: bID, AccessHash: bHash})
	t.Cleanup(func() {
		c, cancel := contextTimeout(20 * time.Second)
		defer cancel()
		_, _ = a.Invoke(c, &mtproto.TLPhoneDiscardGroupCall{
			Constructor: mtproto.CRC32_phone_discardGroupCall,
			Call:        mtproto.MakeTLInputGroupCall(&mtproto.InputGroupCall{Id: callID, AccessHash: access}).To_InputGroupCall(),
		})
	})
	sent, err := a.Invoke(ctx, &mtproto.TLMessagesSendMedia{
		Constructor: mtproto.CRC32_messages_sendMedia_7852834e,
		Peer:        inputPeerUser(bID, bHash),
		Media: mtproto.MakeTLInputMediaStory(&mtproto.InputMedia{
			Peer:     inputPeerSelf(),
			Id_INT32: storyID,
		}).To_InputMedia(),
		Message:  "story-card",
		RandomId: randomID(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !storyMedia(allMessages(sent), storyID, aSelf.ID) {
		t.Fatalf("sender result is not messageMediaStory: %T", sent)
	}
	peerForB := inputPeerUser(aSelf.ID, aSelf.AccessHash)
	err = waitUntil(ctx, 400*time.Millisecond, func() (bool, error) {
		msgs, err := b.History(ctx, peerForB, 20)
		if err != nil {
			return false, err
		}
		return storyMedia(msgs, storyID, aSelf.ID), nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func storyMedia(msgs []*mtproto.Message, storyID int32, userID int64) bool {
	for _, m := range msgs {
		media := m.GetMedia()
		if media.GetPredicateName() != mtproto.Predicate_messageMediaStory {
			continue
		}
		if media.GetId() == storyID && media.GetPeer().GetUserId() == userID {
			return true
		}
	}
	return false
}
