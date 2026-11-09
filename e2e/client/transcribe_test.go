package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestTranscribeAudioIsNotPremiumOrStub is the client-visible failure where
// every voice note came back as "No speech detected". A text message is not
// audio, so the server must say so. It must not ask for Premium and it must
// not use the old stub error. A zero id is rejected before any lookup.
func TestTranscribeAudioIsNotPremiumOrStub(t *testing.T) {
	a, _, _, bSelf := sharedPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	peer := inputPeerUser(bSelf.ID, bSelf.AccessHash)

	_, err := a.Invoke(ctx, &mtproto.TLMessagesTranscribeAudio{
		Constructor: mtproto.CRC32_messages_transcribeAudio,
		Peer:        peer,
		MsgId:       0,
	})
	assertTranscribeError(t, err, "MSG_ID_INVALID")

	id, err := a.SendText(ctx, peer, "not a voice note", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.Invoke(ctx, &mtproto.TLMessagesTranscribeAudio{
		Constructor: mtproto.CRC32_messages_transcribeAudio,
		Peer:        peer,
		MsgId:       id,
	})
	assertTranscribeError(t, err, "MEDIA_EMPTY")
}

func assertTranscribeError(t *testing.T, err error, want string) {
	t.Helper()
	rpc, ok := asRPCError(err)
	if !ok {
		t.Fatalf("transcribeAudio: %v", err)
	}
	msg := strings.ToUpper(rpc.Message)
	if strings.Contains(msg, "PREMIUM_ACCOUNT_REQUIRED") || strings.Contains(msg, "TRANSCRIPTION_FAILED") {
		t.Fatalf("transcribeAudio returned %s", rpc.Message)
	}
	if !strings.Contains(msg, want) {
		t.Fatalf("transcribeAudio returned %s, want %s", rpc.Message, want)
	}
}

// TestTranscribeDraftReplies is the dictation request iOS sends as
// messages.transcribeDraft. A decode failure used to drop it, so the client
// waited until the voice bubble disappeared. Empty audio is rejected by the
// handler and does not call the recognizer.
func TestTranscribeDraftReplies(t *testing.T) {
	a, _, _, _ := sharedPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req := &mtproto.TLMessagesTranscribeAudio{}
	req.SetDraftAudio(nil)
	_, err := a.Invoke(ctx, req)
	assertTranscribeError(t, err, "MEDIA_EMPTY")
}
