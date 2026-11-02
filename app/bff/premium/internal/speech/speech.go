// Package speech implements messages.transcribeAudio against the SenseVoice sidecar.
package speech

import (
	"context"
	"fmt"
	"hash/fnv"
	"regexp"
	"strings"
	"sync"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/pkg/premium"

	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	// MaxDuration is transcribe_audio_trial_duration_max. Longer clips are rejected.
	MaxDuration = 300
	// MaxBytes bounds a voice note or video-note download before it is sent to ASR.
	MaxBytes = 48 << 20
)

// ErrAudioTooLong is returned when the clip is over MaxDuration or MaxBytes.
var ErrAudioTooLong = status.Error(mtproto.ErrBadRequest, "AUDIO_TOO_LONG")

// Clip is the document a voice note or round video stores in dfs.
type Clip struct {
	DocumentID    int64
	AccessHash    int64
	FileReference []byte
	Filename      string
	Duration      float64
	Size          int64
}

// Loader reads the caller's message and returns the audio clip.
type Loader func(ctx context.Context, userID int64, peer *mtproto.PeerUtil, msgID int32) (Clip, error)

// Fetcher downloads the document bytes from storage.
type Fetcher func(ctx context.Context, clip Clip) ([]byte, error)

// Recognizer turns audio bytes into text.
type Recognizer interface {
	Transcribe(ctx context.Context, audio []byte, filename string) (string, error)
}

// RecognizerFunc adapts a function to Recognizer.
type RecognizerFunc func(ctx context.Context, audio []byte, filename string) (string, error)

// Transcribe implements Recognizer.
func (f RecognizerFunc) Transcribe(ctx context.Context, audio []byte, filename string) (string, error) {
	return f(ctx, audio, filename)
}

// Notifier pushes updateTranscribedAudio to the caller's other sessions.
type Notifier interface {
	SyncUpdatesNotMe(ctx context.Context, in *syncpb.TLSyncUpdatesNotMe) (*mtproto.Void, error)
}

// Handler is messages.transcribeAudio.
type Handler struct {
	load      Loader
	fetch     Fetcher
	recognize Recognizer
	push      Notifier
	cache     *resultCache
	mu        sync.Mutex
}

// NewHandler builds a handler. load, fetch, and recognize are required.
func NewHandler(load Loader, fetch Fetcher, recognize Recognizer, push Notifier) *Handler {
	return &Handler{
		load:      load,
		fetch:     fetch,
		recognize: recognize,
		push:      push,
		cache:     newResultCache(),
	}
}

var senseTag = regexp.MustCompile(`<\|[^>|]{0,64}\|>`)

// CleanText removes SenseVoice language, emotion, event, and ITN tags.
func CleanText(text string) string {
	return strings.TrimSpace(senseTag.ReplaceAllString(text, ""))
}

// Transcribe runs speech recognition and returns a final messages.transcribedAudio.
// pending is false. Premium is not required. An empty recognition is
// TRANSCRIPTION_FAILED; every other call used to return that unconditionally.
func (h *Handler) Transcribe(ctx context.Context, md *metadata.RpcMetadata, in *mtproto.TLMessagesTranscribeAudio) (*mtproto.Messages_TranscribedAudio, error) {
	if in == nil || in.GetPeer() == nil || in.GetMsgId() == 0 {
		return nil, mtproto.ErrMsgIdInvalid
	}
	if md == nil || md.GetUserId() == 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	if h == nil || h.load == nil || h.fetch == nil || h.recognize == nil {
		return nil, mtproto.ErrInternalServerError
	}
	peer := mtproto.FromInputPeer2(md.GetUserId(), in.GetPeer())
	if peer.PeerType == mtproto.PEER_UNKNOWN || peer.PeerType == mtproto.PEER_EMPTY {
		return nil, mtproto.ErrPeerIdInvalid
	}
	clip, err := h.load(ctx, md.GetUserId(), peer, in.GetMsgId())
	if err != nil {
		return nil, err
	}
	if clip.Duration > MaxDuration {
		return nil, ErrAudioTooLong
	}
	if clip.Size > MaxBytes {
		return nil, ErrAudioTooLong
	}
	keys := cacheKeys(md.GetUserId(), peer, in.GetMsgId(), clip.DocumentID)
	if got, ok := h.cache.get(keys...); ok {
		return h.finish(ctx, md, peer, in.GetMsgId(), got, false), nil
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if got, ok := h.cache.get(keys...); ok {
		return h.finish(ctx, md, peer, in.GetMsgId(), got, false), nil
	}
	audio, err := h.fetch(ctx, clip)
	if err != nil {
		return nil, err
	}
	if len(audio) == 0 {
		return nil, mtproto.ErrMediaEmpty
	}
	if len(audio) > MaxBytes {
		return nil, ErrAudioTooLong
	}
	callCtx := ctx
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, recognizeTimeout)
		defer cancel()
	}
	raw, err := h.recognize.Transcribe(callCtx, audio, clip.Filename)
	if err != nil {
		if status.Convert(err).Message() == "AUDIO_TOO_LONG" {
			return nil, ErrAudioTooLong
		}
		logx.WithContext(ctx).Errorf("messages.transcribeAudio: %v", err)
		return nil, mtproto.ErrTranscriptionFailed
	}
	text := CleanText(raw)
	if text == "" {
		return nil, mtproto.ErrTranscriptionFailed
	}
	got := cachedResult{ID: transcriptionID(clip.DocumentID, md.GetUserId(), peer, in.GetMsgId()), Text: text}
	h.cache.put(got, keys...)
	return h.finish(ctx, md, peer, in.GetMsgId(), got, true), nil
}

func (h *Handler) finish(ctx context.Context, md *metadata.RpcMetadata, peer *mtproto.PeerUtil, msgID int32, got cachedResult, notify bool) *mtproto.Messages_TranscribedAudio {
	if notify {
		h.notify(ctx, md, peer, msgID, got)
	}
	weekly := int32(0)
	if premium.TranscribeEnabled() {
		weekly = int32(premium.TranscribeTrialWeekly)
	}
	return mtproto.MakeTLMessagesTranscribedAudio(&mtproto.Messages_TranscribedAudio{
		Pending:               false,
		TranscriptionId:       got.ID,
		Text:                  got.Text,
		TrialRemainsNum:       wrapperspb.Int32(weekly),
		TrialRemainsUntilDate: wrapperspb.Int32(0),
	}).To_Messages_TranscribedAudio()
}

func (h *Handler) notify(ctx context.Context, md *metadata.RpcMetadata, peer *mtproto.PeerUtil, msgID int32, got cachedResult) {
	if h.push == nil || md == nil {
		return
	}
	_, err := h.push.SyncUpdatesNotMe(ctx, &syncpb.TLSyncUpdatesNotMe{
		UserId:        md.GetUserId(),
		PermAuthKeyId: md.GetPermAuthKeyId(),
		Updates: mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateTranscribedAudio(&mtproto.Update{
			Pending:         false,
			Peer_PEER:       updatePeer(md.GetUserId(), peer),
			MsgId_INT32:     msgID,
			TranscriptionId: got.ID,
			Text:            got.Text,
		}).To_Update()),
	})
	if err != nil {
		logx.WithContext(ctx).Errorf("messages.transcribeAudio push: %v", err)
	}
}

func updatePeer(userID int64, peer *mtproto.PeerUtil) *mtproto.Peer {
	if peer == nil || peer.PeerType == mtproto.PEER_SELF {
		return mtproto.MakePeerUser(userID)
	}
	return peer.ToPeer()
}

func transcriptionID(docID, userID int64, peer *mtproto.PeerUtil, msgID int32) int64 {
	if docID > 0 {
		return docID
	}
	h := fnv.New64a()
	fmt.Fprintf(h, "%d:%d:%d:%d", userID, peer.PeerType, peer.PeerId, msgID)
	id := int64(h.Sum64() & 0x7fffffffffffffff)
	if id == 0 {
		return 1
	}
	return id
}

func cacheKeys(userID int64, peer *mtproto.PeerUtil, msgID int32, docID int64) []string {
	keys := []string{fmt.Sprintf("msg:%d:%d:%d:%d", userID, peer.PeerType, peer.PeerId, msgID)}
	if docID > 0 {
		keys = append(keys, fmt.Sprintf("doc:%d", docID))
	}
	return keys
}

type cachedResult struct {
	ID   int64
	Text string
}

type resultCache struct {
	mu sync.Mutex
	m  map[string]cachedResult
}

func newResultCache() *resultCache {
	return &resultCache{m: map[string]cachedResult{}}
}

func (c *resultCache) get(keys ...string) (cachedResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, key := range keys {
		if got, ok := c.m[key]; ok {
			return got, true
		}
	}
	return cachedResult{}, false
}

func (c *resultCache) put(got cachedResult, keys ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, key := range keys {
		c.m[key] = got
	}
}
