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
	// sendMessage can return an id before this session's get-message read
	// sees the row. That read answers MSG_ID_INVALID until the row is there.
	// Once history shows the text, a text message must be MEDIA_EMPTY.
	ready, cancelReady := context.WithTimeout(ctx, 8*time.Second)
	defer cancelReady()
	var last string
	err = waitUntil(ready, 200*time.Millisecond, func() (bool, error) {
		msgs, herr := a.History(ready, peer, 30)
		if herr != nil {
			return false, herr
		}
		if idOfText(msgs, "not a voice note") != id {
			last = "history missing the text"
			return false, nil
		}
		_, terr := a.Invoke(ready, &mtproto.TLMessagesTranscribeAudio{
			Constructor: mtproto.CRC32_messages_transcribeAudio,
			Peer:        peer,
			MsgId:       id,
		})
		rpc, ok := asRPCError(terr)
		if !ok {
			return false, terr
		}
		last = rpc.Message
		msg := strings.ToUpper(rpc.Message)
		if strings.Contains(msg, "PREMIUM_ACCOUNT_REQUIRED") || strings.Contains(msg, "TRANSCRIPTION_FAILED") {
			return false, terr
		}
		if strings.Contains(msg, "MEDIA_EMPTY") {
			return true, nil
		}
		if strings.Contains(msg, "MSG_ID_INVALID") {
			return false, nil
		}
		return false, terr
	})
	if err != nil {
		t.Fatalf("transcribeAudio last %q: %v", last, err)
	}
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
