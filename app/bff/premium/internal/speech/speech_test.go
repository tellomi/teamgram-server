package speech

import (
	"context"
	"encoding/binary"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	dfspb "github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	"github.com/teamgram/teamgram-server/pkg/premium"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
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
	if status.Convert(err).Message() != "MSG_VOICE_TOO_LONG" {
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

type boxStore struct{ box *mtproto.MessageBox }

func (s boxStore) MessageGetUserMessage(_ context.Context, in *messagepb.TLMessageGetUserMessage) (*mtproto.MessageBox, error) {
	if s.box == nil || s.box.GetMessageId() != in.GetId() {
		return nil, mtproto.ErrMsgIdInvalid
	}
	return s.box, nil
}

func voiceMessage(id int32) *mtproto.Message {
	return mtproto.MakeTLMessage(&mtproto.Message{
		Id: id,
		Media: mtproto.MakeTLMessageMediaDocument(&mtproto.MessageMedia{
			Voice: true,
			Document: mtproto.MakeTLDocument(&mtproto.Document{
				Id:         99,
				AccessHash: 5,
				Attributes: []*mtproto.DocumentAttribute{
					mtproto.MakeTLDocumentAttributeAudio(&mtproto.DocumentAttribute{Voice: true, Duration_INT32: 3}).To_DocumentAttribute(),
				},
			}).To_Document(),
		}).To_MessageMedia(),
	}).To_Message()
}

// TestLoadClipIncomingPrivateVoice is the owner's report: a voice note the
// other side sent could not be transcribed. Prod rows for the recipient store
// peer_id = the recipient itself and sender = the partner (messages
// user_id=136907717 box 2756: sender 136907723, peer_id 136907717).
func TestLoadClipIncomingPrivateVoice(t *testing.T) {
	const me, them = int64(136907717), int64(136907723)
	partner := mtproto.MakePeerUtil(mtproto.PEER_USER, them)
	rows := map[string]*mtproto.MessageBox{
		"incoming":        {MessageId: 2756, PeerType: mtproto.PEER_USER, PeerId: me, SenderUserId: them, DialogId1: me, DialogId2: them, Message: voiceMessage(2756)},
		"incoming legacy": {MessageId: 2756, PeerType: mtproto.PEER_USER, PeerId: me, SenderUserId: them, Message: voiceMessage(2756)},
		"outgoing":        {MessageId: 2756, PeerType: mtproto.PEER_USER, PeerId: them, SenderUserId: me, DialogId1: me, DialogId2: them, Message: voiceMessage(2756)},
	}
	for name, box := range rows {
		clip, err := loadClip(context.Background(), boxStore{box}, me, partner, 2756)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if clip.DocumentID != 99 || clip.AccessHash != 5 || clip.Filename != "voice.ogg" {
			t.Fatalf("%s: clip %+v", name, clip)
		}
	}
	other := &mtproto.MessageBox{MessageId: 2756, PeerType: mtproto.PEER_USER, PeerId: me, SenderUserId: 42, DialogId1: 42, DialogId2: me, Message: voiceMessage(2756)}
	if _, err := loadClip(context.Background(), boxStore{other}, me, partner, 2756); err != mtproto.ErrMsgIdInvalid {
		t.Fatalf("message from another chat: %v", err)
	}
}

func TestLoadClipGroupVoice(t *testing.T) {
	const me, them, chat = int64(1), int64(2), int64(120037)
	box := &mtproto.MessageBox{MessageId: 153, PeerType: mtproto.PEER_CHAT, PeerId: chat, SenderUserId: them, Message: voiceMessage(153)}
	if _, err := loadClip(context.Background(), boxStore{box}, me, mtproto.MakePeerUtil(mtproto.PEER_CHAT, chat), 153); err != nil {
		t.Fatal(err)
	}
	if _, err := loadClip(context.Background(), boxStore{box}, me, mtproto.MakePeerUtil(mtproto.PEER_CHAT, chat+1), 153); err != mtproto.ErrMsgIdInvalid {
		t.Fatalf("other group: %v", err)
	}
}

// A recognizer outage is not "no speech".
func TestRecognizerErrorIsUnavailable(t *testing.T) {
	h := NewHandler(
		func(context.Context, int64, *mtproto.PeerUtil, int32) (Clip, error) {
			return Clip{DocumentID: 1, Filename: "voice.ogg", Duration: 1}, nil
		},
		func(context.Context, Clip) ([]byte, error) { return []byte("OggS"), nil },
		RecognizerFunc(func(context.Context, []byte, string) (string, error) {
			return "", errors.New("dial tcp: connection refused")
		}),
		nil,
	)
	_, err := h.Transcribe(context.Background(), &metadata.RpcMetadata{UserId: 1}, &mtproto.TLMessagesTranscribeAudio{
		Peer:  mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 2}).To_InputPeer(),
		MsgId: 1,
	})
	if status.Convert(err).Message() != "TRANSCRIPTION_UNAVAILABLE" {
		t.Fatalf("err %v", err)
	}
}

func TestTranscribeRoundVideoReturnsFinalText(t *testing.T) {
	t.Setenv(premium.EnvTranscribeEnabled, "true")
	h := NewHandler(
		func(context.Context, int64, *mtproto.PeerUtil, int32) (Clip, error) {
			return Clip{DocumentID: 9, Filename: "round.mp4", Duration: 4, Size: 8}, nil
		},
		func(context.Context, Clip) ([]byte, error) { return []byte("fake-mp4"), nil },
		RecognizerFunc(func(_ context.Context, audio []byte, name string) (string, error) {
			if name != "round.mp4" || string(audio) != "fake-mp4" {
				t.Fatalf("recognizer got %s %q", name, audio)
			}
			return "你好", nil
		}),
		nil,
	)
	out, err := h.Transcribe(context.Background(), &metadata.RpcMetadata{UserId: 7}, &mtproto.TLMessagesTranscribeAudio{
		Peer:  mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 8}).To_InputPeer(),
		MsgId: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.GetPending() || out.GetText() != "你好" || out.GetTranscriptionId() == 0 {
		t.Fatalf("pending=%v id=%d text=%q", out.GetPending(), out.GetTranscriptionId(), out.GetText())
	}
}

func TestTranscribedAudioTrialFlagBits(t *testing.T) {
	obj := mtproto.MakeTLMessagesTranscribedAudio(&mtproto.Messages_TranscribedAudio{
		Pending:               false,
		TranscriptionId:       9,
		Text:                  "hi",
		TrialRemainsNum:       wrapperspb.Int32(3),
		TrialRemainsUntilDate: wrapperspb.Int32(0),
	})
	buf := mtproto.NewEncodeBuf(64)
	if err := obj.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	raw := buf.GetBuf()
	if len(raw) < 8 || binary.LittleEndian.Uint32(raw[:4]) != 0xcfb9d957 {
		t.Fatalf("constructor %x", raw)
	}
	flags := binary.LittleEndian.Uint32(raw[4:8])
	if flags != (1<<1)|(1<<2) {
		t.Fatalf("flags %d, want trial count on bit 1 and until date on bit 2", flags)
	}
	back := mtproto.NewDecodeBuf(raw)
	decoded := back.Object()
	if err := back.GetError(); err != nil {
		t.Fatal(err)
	}
	got, ok := decoded.(*mtproto.TLMessagesTranscribedAudio)
	if !ok || got.GetPending() || got.GetText() != "hi" || got.GetTrialRemainsNum().GetValue() != 3 || got.GetTrialRemainsUntilDate().GetValue() != 0 {
		t.Fatalf("decoded %T %#v", decoded, got)
	}
}

func TestDraftTranscribeDoesNotNotifyOrKeepAudio(t *testing.T) {
	var pushed bool
	var saw []byte
	h := NewHandler(nil, nil, RecognizerFunc(func(_ context.Context, audio []byte, _ string) (string, error) {
		saw = append([]byte(nil), audio...)
		return "<|zh|>未发送的话", nil
	}), notifierFunc(func(context.Context, *sync.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
		pushed = true
		return nil, nil
	}))
	in := &mtproto.TLMessagesTranscribeAudio{}
	in.SetDraftAudio([]byte("OggS-draft"))
	buf := mtproto.NewEncodeBuf(64)
	if err := in.Encode(buf, mtproto.Layer); err != nil {
		t.Fatal(err)
	}
	raw := buf.GetBuf()
	if binary.LittleEndian.Uint32(raw[:4]) != uint32(mtproto.DraftTranscribeConstructor) {
		t.Fatalf("constructor %x", raw[:4])
	}
	decoded := &mtproto.TLMessagesTranscribeAudio{Constructor: mtproto.DraftTranscribeConstructor}
	if err := decoded.Decode(mtproto.NewDecodeBuf(raw[4:])); err != nil {
		t.Fatal(err)
	}
	wire, err := proto.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	var forwarded mtproto.TLMessagesTranscribeAudio
	if err := proto.Unmarshal(wire, &forwarded); err != nil {
		t.Fatal(err)
	}
	audio := forwarded.DraftAudio()
	if string(audio) != "OggS-draft" {
		t.Fatalf("audio %q", audio)
	}
	out, err := h.Draft(context.Background(), &metadata.RpcMetadata{UserId: 7}, audio)
	forwarded.ClearDraftAudio()
	if err != nil {
		t.Fatal(err)
	}
	if pushed {
		t.Fatal("draft notified another session")
	}
	if out.GetText() != "未发送的话" || out.GetPending() {
		t.Fatalf("%+v", out)
	}
	if string(saw) != "OggS-draft" {
		t.Fatalf("recognizer saw %q", saw)
	}
	if len(forwarded.DraftAudio()) != 0 {
		t.Fatal("audio stayed on the request")
	}
	if _, err := h.Draft(context.Background(), nil, []byte("abc")); status.Convert(err).Message() != "USER_ID_INVALID" {
		t.Fatalf("auth %v", err)
	}
}
