package speech

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	dfspb "github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	"github.com/teamgram/teamgram-server/pkg/premium"
	"google.golang.org/grpc/status"
)

func TestTranscribeVoiceCachesAndStripsTags(t *testing.T) {
	t.Setenv(premium.EnvTranscribeEnabled, "true")
	var calls int
	var pushed *mtproto.Update
	h := NewHandler(
		func(context.Context, int64, *mtproto.PeerUtil, int32) (Clip, error) {
			return Clip{DocumentID: 42, Filename: "voice.ogg", Duration: 3}, nil
		},
		func(context.Context, Clip) ([]byte, error) {
			return []byte("OggS"), nil
		},
		RecognizerFunc(func(context.Context, []byte, string) (string, error) {
			calls++
			return "<|zh|><|HAPPY|><|Speech|><|withitn|>你好，世界。", nil
		}),
		notifierFunc(func(_ context.Context, in *sync.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
			pushed = in.GetUpdates().GetUpdates()[0]
			return nil, nil
		}),
	)
	md := &metadata.RpcMetadata{UserId: 5, PermAuthKeyId: 9}
	in := &mtproto.TLMessagesTranscribeAudio{
		Peer:  mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 6}).To_InputPeer(),
		MsgId: 3,
	}
	out, err := h.Transcribe(context.Background(), md, in)
	if err != nil {
		t.Fatal(err)
	}
	if out.GetPending() || out.GetText() != "你好，世界。" || out.GetTranscriptionId() != 42 {
		t.Fatalf("pending=%v text=%q id=%d", out.GetPending(), out.GetText(), out.GetTranscriptionId())
	}
	if out.GetTrialRemainsNum().GetValue() != premium.TranscribeTrialWeekly {
		t.Fatalf("trial remains %v", out.GetTrialRemainsNum())
	}
	if calls != 1 {
		t.Fatalf("asr calls %d", calls)
	}
	if pushed == nil || pushed.GetPending() || pushed.GetText() != "你好，世界。" || pushed.GetMsgId_INT32() != 3 {
		t.Fatalf("update %#v", pushed)
	}
	again, err := h.Transcribe(context.Background(), md, in)
	if err != nil {
		t.Fatal(err)
	}
	if again.GetText() != out.GetText() || again.GetTranscriptionId() != out.GetTranscriptionId() || calls != 1 {
		t.Fatalf("cache missed calls=%d text=%q", calls, again.GetText())
	}
}

func TestTranscribeAllowsNonPremiumAndRejectsPlainText(t *testing.T) {
	t.Setenv(premium.EnvEveryone, "false")
	t.Setenv(premium.EnvTranscribeEnabled, "false")
	premium.Reload()
	t.Cleanup(func() { premium.Reload() })
	if premium.Allows(premium.FeatureVoiceToText, false) {
		t.Fatal("voice_to_text was marked free; this test wants a non-member")
	}
	h := NewHandler(
		func(context.Context, int64, *mtproto.PeerUtil, int32) (Clip, error) {
			return Clip{}, mtproto.ErrMediaEmpty
		},
		func(context.Context, Clip) ([]byte, error) {
			t.Fatal("fetched a text message")
			return nil, nil
		},
		RecognizerFunc(func(context.Context, []byte, string) (string, error) {
			t.Fatal("ran asr")
			return "", nil
		}),
		nil,
	)
	_, err := h.Transcribe(context.Background(), &metadata.RpcMetadata{UserId: 1}, &mtproto.TLMessagesTranscribeAudio{
		Peer:  mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 2}).To_InputPeer(),
		MsgId: 8,
	})
	if status.Convert(err).Message() != "MEDIA_EMPTY" {
		t.Fatalf("err %v", err)
	}
}

func TestTranscribeRejectsLongVoice(t *testing.T) {
	h := NewHandler(
		func(context.Context, int64, *mtproto.PeerUtil, int32) (Clip, error) {
			return Clip{DocumentID: 1, Duration: MaxDuration + 1, Filename: "voice.ogg"}, nil
		},
		func(context.Context, Clip) ([]byte, error) {
			t.Fatal("downloaded a clip over the limit")
			return nil, nil
		},
		RecognizerFunc(func(context.Context, []byte, string) (string, error) {
			t.Fatal("ran asr")
			return "", nil
		}),
		nil,
	)
	_, err := h.Transcribe(context.Background(), &metadata.RpcMetadata{UserId: 1}, &mtproto.TLMessagesTranscribeAudio{
		Peer:  mtproto.MakeTLInputPeerSelf(&mtproto.InputPeer{}).To_InputPeer(),
		MsgId: 4,
	})
	if status.Convert(err).Message() != "AUDIO_TOO_LONG" {
		t.Fatalf("err %v", err)
	}
}

func TestClipFromRoundVideo(t *testing.T) {
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Media: mtproto.MakeTLMessageMediaDocument(&mtproto.MessageMedia{
			Document: mtproto.MakeTLDocument(&mtproto.Document{
				Id:         9,
				AccessHash: 3,
				MimeType:   "video/mp4",
				Size2:      1000,
				Attributes: []*mtproto.DocumentAttribute{
					mtproto.MakeTLDocumentAttributeVideo(&mtproto.DocumentAttribute{
						RoundMessage:     true,
						Duration_FLOAT64: 12,
					}).To_DocumentAttribute(),
				},
			}).To_Document(),
		}).To_MessageMedia(),
	}).To_Message()
	clip, err := clipFromMessage(msg)
	if err != nil {
		t.Fatal(err)
	}
	if clip.DocumentID != 9 || clip.Filename != "round.mp4" || clip.Duration != 12 {
		t.Fatalf("clip %#v", clip)
	}
	plain := mtproto.MakeTLMessage(&mtproto.Message{
		Message: "hello",
	}).To_Message()
	if _, err := clipFromMessage(plain); status.Convert(err).Message() != "MEDIA_EMPTY" {
		t.Fatalf("plain text err %v", err)
	}
}

func TestHTTPRecognizer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/transcribe" || r.Header.Get("Content-Type") != "application/octet-stream" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"<|en|>hello"}`))
	}))
	defer srv.Close()
	got, err := (&HTTPRecognizer{Endpoint: srv.URL + "/v1/transcribe", Client: srv.Client()}).Transcribe(context.Background(), []byte("abc"), "voice.ogg")
	if err != nil || got != "<|en|>hello" {
		t.Fatalf("got %q err %v", got, err)
	}
	long := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"too_long"}`))
	}))
	defer long.Close()
	_, err = (&HTTPRecognizer{Endpoint: long.URL + "/v1/transcribe", Client: long.Client()}).Transcribe(context.Background(), []byte("abc"), "voice.ogg")
	if !errors.Is(err, ErrAudioTooLong) {
		t.Fatalf("err %v", err)
	}
}

func TestEmptyRecognitionIsNoSpeech(t *testing.T) {
	h := NewHandler(
		func(context.Context, int64, *mtproto.PeerUtil, int32) (Clip, error) {
			return Clip{DocumentID: 1, Filename: "voice.ogg", Duration: 1}, nil
		},
		func(context.Context, Clip) ([]byte, error) { return []byte("OggS"), nil },
		RecognizerFunc(func(context.Context, []byte, string) (string, error) {
			return "<|nospeech|>", nil
		}),
		nil,
	)
	_, err := h.Transcribe(context.Background(), &metadata.RpcMetadata{UserId: 1}, &mtproto.TLMessagesTranscribeAudio{
		Peer:  mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 2}).To_InputPeer(),
		MsgId: 1,
	})
	if status.Convert(err).Message() != "TRANSCRIPTION_FAILED" {
		t.Fatalf("err %v", err)
	}
}

func TestFetchStopsAtShortRead(t *testing.T) {
	files := &seqFiles{parts: [][]byte{bytesRepeat(512 * 1024), []byte("tail")}}
	got, err := fetchClip(context.Background(), files, Clip{DocumentID: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 512*1024+4 || files.calls != 2 {
		t.Fatalf("len %d calls %d", len(got), files.calls)
	}
}

type seqFiles struct {
	parts [][]byte
	calls int
}

func (s *seqFiles) DfsDownloadFile(_ context.Context, in *dfspb.TLDfsDownloadFile) (*mtproto.Upload_File, error) {
	i := s.calls
	s.calls++
	if i >= len(s.parts) {
		return &mtproto.Upload_File{}, nil
	}
	if in.GetOffset() < 0 {
		return nil, mtproto.ErrMediaEmpty
	}
	return &mtproto.Upload_File{Bytes: s.parts[i]}, nil
}

func bytesRepeat(n int) []byte {
	return make([]byte, n)
}

func TestSameDialog(t *testing.T) {
	box := &mtproto.MessageBox{PeerType: mtproto.PEER_USER, PeerId: 7}
	self := &mtproto.PeerUtil{PeerType: mtproto.PEER_SELF, PeerId: 7}
	if !sameDialog(box, 7, self) {
		t.Fatal("saved messages did not match")
	}
	other := &mtproto.PeerUtil{PeerType: mtproto.PEER_USER, PeerId: 8}
	if sameDialog(box, 7, other) {
		t.Fatal("wrong peer matched")
	}
}

type notifierFunc func(context.Context, *sync.TLSyncUpdatesNotMe) (*mtproto.Void, error)

func (f notifierFunc) SyncUpdatesNotMe(ctx context.Context, in *sync.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	return f(ctx, in)
}
