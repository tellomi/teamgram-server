package e2e

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

//go:embed testdata/voice.ogg
var voiceOgg []byte

// TestTranscribeReceivedVoice is the owner's report: transcribing a voice note
// the other side sent showed "no words recognized" while the sender's own
// notes worked. The recipient's private copy has its own id and stores
// peer_id = the recipient, and the server rejected it before any download.
//
// CI does not run the SenseVoice sidecar, so the furthest the call can get
// there is the recognizer: TRANSCRIPTION_UNAVAILABLE proves the message and
// its document were found and downloaded. With TELLOMI_E2E_ASR=1 (a stack
// that runs asr) the text must be non-empty.
func TestTranscribeReceivedVoice(t *testing.T) {
	a, b, _, bSelf := sharedPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	caption := fmt.Sprintf("voice-private-%d", time.Now().UnixNano())
	if _, err := a.SendVoice(ctx, inputPeerUser(bSelf.ID, bSelf.AccessHash), voiceOgg, 7, caption); err != nil {
		t.Fatal(err)
	}
	peerForB := inputPeerUser(aliceSeen.ID, aliceSeen.AccessHash)
	own := waitForText(t, ctx, b, peerForB, caption)
	assertTranscribed(t, ctx, b, peerForB, own, "recipient, private")

	t.Run("group", func(t *testing.T) {
		chatID, err := a.CreateChat(ctx, fmt.Sprintf("e2e-voice-%d", time.Now().Unix()), inputUser(bSelf.ID, bSelf.AccessHash))
		if err != nil {
			t.Fatal(err)
		}
		peer := inputPeerChat(chatID)
		caption := fmt.Sprintf("voice-group-%d", time.Now().UnixNano())
		if _, err := a.SendVoice(ctx, peer, voiceOgg, 7, caption); err != nil {
			t.Fatal(err)
		}
		own := waitForText(t, ctx, b, peer, caption)
		assertTranscribed(t, ctx, b, peer, own, "recipient, group")
	})
}

func waitForText(t *testing.T, ctx context.Context, c *Client, peer *mtproto.InputPeer, text string) int32 {
	t.Helper()
	var own int32
	err := waitUntil(ctx, 400*time.Millisecond, func() (bool, error) {
		msgs, err := c.History(ctx, peer, 30)
		if err != nil {
			return false, err
		}
		own = idOfText(msgs, text)
		return own != 0, nil
	})
	if err != nil {
		t.Fatalf("recipient never got %q: %v", text, err)
	}
	return own
}

func assertTranscribed(t *testing.T, ctx context.Context, c *Client, peer *mtproto.InputPeer, msgID int32, who string) {
	t.Helper()
	res, err := c.Invoke(ctx, &mtproto.TLMessagesTranscribeAudio{
		Constructor: mtproto.CRC32_messages_transcribeAudio,
		Peer:        peer,
		MsgId:       msgID,
	})
	if os.Getenv("TELLOMI_E2E_ASR") == "1" {
		if err != nil {
			t.Fatalf("%s: transcribeAudio: %v", who, err)
		}
		got, ok := res.(*mtproto.TLMessagesTranscribedAudio)
		if !ok || strings.TrimSpace(got.GetText()) == "" {
			t.Fatalf("%s: transcribeAudio returned %v", who, res)
		}
		return
	}
	if err == nil {
		return
	}
	rpc, ok := asRPCError(err)
	if !ok {
		t.Fatalf("%s: transcribeAudio: %v", who, err)
	}
	if !strings.Contains(strings.ToUpper(rpc.Message), "TRANSCRIPTION_UNAVAILABLE") {
		t.Fatalf("%s: transcribeAudio returned %s, want text or TRANSCRIPTION_UNAVAILABLE (the message or its audio was not found)", who, rpc.Message)
	}
}
