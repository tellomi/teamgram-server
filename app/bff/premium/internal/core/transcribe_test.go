package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/premium/internal/speech"
	"github.com/teamgram/teamgram-server/app/bff/premium/internal/svc"
	"github.com/teamgram/teamgram-server/pkg/premium"
	"google.golang.org/protobuf/proto"
)

func TestMessagesTranscribeAudioIgnoresPremium(t *testing.T) {
	t.Setenv(premium.EnvEveryone, "false")
	t.Setenv(premium.EnvFreeFeatures, "")
	t.Setenv(premium.EnvTranscribeEnabled, "true")
	premium.Reload()
	t.Cleanup(func() { premium.Reload() })
	if premium.IsMember(false, 0, 1) || premium.Allows(premium.FeatureVoiceToText, false) {
		t.Fatal("non-member was allowed by the premium policy")
	}
	var calls int
	h := speech.NewHandler(
		func(context.Context, int64, *mtproto.PeerUtil, int32) (speech.Clip, error) {
			return speech.Clip{DocumentID: 11, Filename: "voice.ogg", Duration: 2}, nil
		},
		func(context.Context, speech.Clip) ([]byte, error) {
			return []byte("OggS"), nil
		},
		speech.RecognizerFunc(func(context.Context, []byte, string) (string, error) {
			calls++
			return "收到", nil
		}),
		nil,
	)
	c := withUser(4)
	c.svcCtx = &svc.ServiceContext{Speech: h}
	out, err := c.MessagesTranscribeAudio(&mtproto.TLMessagesTranscribeAudio{
		Peer:  mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 5}).To_InputPeer(),
		MsgId: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.GetPending() || out.GetText() != "收到" || calls != 1 {
		t.Fatalf("pending=%v text=%q calls=%d", out.GetPending(), out.GetText(), calls)
	}
}

// The session resolves messages.transcribeDraft through NewTLObjectByClassID.
// Calling Decode on a hand-built object skips that lookup.
func TestDecodedTranscribeDraftReachesHandler(t *testing.T) {
	audio := []byte("OggS\x00dictation")
	req := &mtproto.TLMessagesTranscribeAudio{}
	req.SetDraftAudio(audio)
	buf := mtproto.NewEncodeBuf(64)
	if err := req.Encode(buf, mtproto.Layer); err != nil {
		t.Fatal(err)
	}
	d := mtproto.NewDecodeBuf(buf.GetBuf())
	obj := d.Object()
	if err := d.GetError(); err != nil {
		t.Fatal(err)
	}
	got, ok := obj.(*mtproto.TLMessagesTranscribeAudio)
	if !ok {
		t.Fatalf("%T", obj)
	}
	var saw []byte
	h := speech.NewHandler(nil, nil, speech.RecognizerFunc(func(_ context.Context, in []byte, _ string) (string, error) {
		saw = append([]byte(nil), in...)
		return "<|zh|>你好", nil
	}), nil)
	c := withUser(7)
	c.svcCtx = &svc.ServiceContext{Speech: h}
	out, err := c.MessagesTranscribeAudio(got)
	if err != nil {
		t.Fatal(err)
	}
	if out.GetText() != "你好" || string(saw) != string(audio) {
		t.Fatalf("text %q saw %q", out.GetText(), saw)
	}
}

func TestEmptyDraftIsMediaEmpty(t *testing.T) {
	req := &mtproto.TLMessagesTranscribeAudio{}
	req.SetDraftAudio(nil)
	buf := mtproto.NewEncodeBuf(32)
	if err := req.Encode(buf, mtproto.Layer); err != nil {
		t.Fatal(err)
	}
	d := mtproto.NewDecodeBuf(buf.GetBuf())
	obj := d.Object()
	if err := d.GetError(); err != nil {
		t.Fatal(err)
	}
	got, ok := obj.(*mtproto.TLMessagesTranscribeAudio)
	if !ok {
		t.Fatalf("%T", obj)
	}
	wire, err := proto.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var forwarded mtproto.TLMessagesTranscribeAudio
	if err := proto.Unmarshal(wire, &forwarded); err != nil {
		t.Fatal(err)
	}
	c := withUser(7)
	c.svcCtx = &svc.ServiceContext{Speech: speech.NewHandler(nil, nil, speech.RecognizerFunc(func(context.Context, []byte, string) (string, error) {
		t.Fatal("empty draft called the recognizer")
		return "", nil
	}), nil)}
	_, err = c.MessagesTranscribeAudio(&forwarded)
	if codeOf(err) != "MEDIA_EMPTY" {
		t.Fatalf("empty draft %v", err)
	}
}
