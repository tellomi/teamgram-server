package e2e

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestSendMediaRandomIDIsIdempotent resends messages.sendMedia with the same
// random_id, including two calls in flight. Official clients do this when the
// first RPC is still writing the photo. Both calls must return the same
// message id and the sender's box must contain that random_id once.
func TestSendMediaRandomIDIsIdempotent(t *testing.T) {
	cfg := requireE2E(t)
	a, self := loginNamed(t, cfg, ProfileIOS, "media_retry_a", "RetryA")
	b, _ := loginNamed(t, cfg, ProfileAndroid, "media_retry_b", "RetryB")
	ph, err := cfg.Phone("media_retry_b")
	if err != nil {
		t.Fatal(err)
	}
	cx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	id, hash, err := a.ImportPhone(cx, ph.Number, "RetryB")
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	_ = b
	peer := inputPeerUser(id, hash)
	jpeg := testJPEG(t, 48, 32, 9)

	t.Run("sequential", func(t *testing.T) {
		fileID := uploadBytes(t, a, jpeg)
		rid := randomID()
		id1 := sendPhotoRandom(t, a, peer, fileID, rid)
		id2 := sendPhotoRandom(t, a, peer, fileID, rid)
		if id1 == 0 || id1 != id2 {
			t.Fatalf("resend ids %d and %d", id1, id2)
		}
		assertOneRandomID(t, self.ID, rid, id1)
	})

	t.Run("concurrent", func(t *testing.T) {
		fileID := uploadBytes(t, a, jpeg)
		rid := randomID()
		var wg sync.WaitGroup
		ids := make([]int32, 2)
		errs := make([]error, 2)
		wg.Add(2)
		for i := 0; i < 2; i++ {
			i := i
			go func() {
				defer wg.Done()
				ids[i], errs[i] = sendPhotoRandomResult(a, peer, fileID, rid)
			}()
		}
		wg.Wait()
		var got int32
		for i, err := range errs {
			if err != nil {
				t.Logf("call %d: %v", i, err)
				continue
			}
			if got == 0 {
				got = ids[i]
			} else if ids[i] != got {
				t.Fatalf("concurrent sendMedia ids %d and %d", got, ids[i])
			}
		}
		if got == 0 {
			t.Fatal("both concurrent sends failed")
		}
		assertOneRandomID(t, self.ID, rid, got)
	})
}

func sendPhotoRandom(t *testing.T, c *Client, to *mtproto.InputPeer, fileID, randomID int64) int32 {
	t.Helper()
	id, err := sendPhotoRandomResult(c, to, fileID, randomID)
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("sendMedia returned no message id")
	}
	return id
}

func sendPhotoRandomResult(c *Client, to *mtproto.InputPeer, fileID, randomID int64) (int32, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := c.Invoke(ctx, &mtproto.TLMessagesSendMedia{
		Constructor: mtproto.CRC32_messages_sendMedia_7852834e,
		Peer:        to,
		Media: mtproto.MakeTLInputMediaUploadedPhoto(&mtproto.InputMedia{
			File: mtproto.MakeTLInputFile(&mtproto.InputFile{
				Id_INT64: fileID,
				Parts:    1,
				Name:     "retry.jpg",
			}).To_InputFile(),
		}).To_InputMedia(),
		RandomId: randomID,
	})
	if err != nil {
		return 0, err
	}
	id := messageID(res)
	if id == 0 {
		return 0, fmt.Errorf("sendMedia returned %T without a message id", res)
	}
	return id, nil
}

func assertOneRandomID(t *testing.T, userID, randomID int64, msgID int32) {
	t.Helper()
	n := stringsTrim(mustSQL(t, fmt.Sprintf(
		`SELECT COUNT(*) FROM messages WHERE user_id = %d AND random_id = %d AND deleted = 0`,
		userID, randomID)))
	if n != "1" {
		t.Fatalf("sender rows for random_id %d: %s, want 1", randomID, n)
	}
	box := stringsTrim(mustSQL(t, fmt.Sprintf(
		`SELECT user_message_box_id FROM messages WHERE user_id = %d AND random_id = %d AND deleted = 0`,
		userID, randomID)))
	if box != fmt.Sprint(msgID) {
		t.Fatalf("stored box %s, rpc id %d", box, msgID)
	}
}

func stringsTrim(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r' || s[len(s)-1] == ' ') {
		s = s[:len(s)-1]
	}
	return s
}
