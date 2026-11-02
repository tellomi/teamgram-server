package speech

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	dfsclient "github.com/teamgram/teamgram-server/app/service/dfs/client"
	dfspb "github.com/teamgram/teamgram-server/app/service/dfs/dfs"

	"github.com/zeromicro/go-zero/core/logx"
)

// MessageStore is the user/chat message box reader.
type MessageStore interface {
	MessageGetUserMessage(ctx context.Context, in *messagepb.TLMessageGetUserMessage) (*mtproto.MessageBox, error)
}

// FileStore is dfs.downloadFile.
type FileStore interface {
	DfsDownloadFile(ctx context.Context, in *dfspb.TLDfsDownloadFile) (*mtproto.Upload_File, error)
}

// New wires storage clients to the handler. recognize may be nil only in tests
// that replace the handler.
func New(messages MessageStore, files FileStore, push Notifier, recognize Recognizer) *Handler {
	return NewHandler(
		func(ctx context.Context, userID int64, peer *mtproto.PeerUtil, msgID int32) (Clip, error) {
			return loadClip(ctx, messages, userID, peer, msgID)
		},
		func(ctx context.Context, clip Clip) ([]byte, error) {
			return fetchClip(ctx, files, clip)
		},
		recognize,
		push,
	)
}

func loadClip(ctx context.Context, messages MessageStore, userID int64, peer *mtproto.PeerUtil, msgID int32) (Clip, error) {
	if peer.PeerType == mtproto.PEER_CHANNEL {
		return loadChannelClip(ctx, userID, peer, msgID)
	}
	if messages == nil {
		return Clip{}, mtproto.ErrInternalServerError
	}
	box, err := messages.MessageGetUserMessage(ctx, &messagepb.TLMessageGetUserMessage{
		UserId: userID,
		Id:     msgID,
	})
	if err != nil {
		logx.WithContext(ctx).Errorf("messages.transcribeAudio load: %v", err)
		return Clip{}, mtproto.ErrMsgIdInvalid
	}
	if box == nil || box.GetMessage() == nil || !sameDialog(box, userID, peer) {
		return Clip{}, mtproto.ErrMsgIdInvalid
	}
	return clipFromMessage(box.GetMessage())
}

func loadChannelClip(ctx context.Context, userID int64, peer *mtproto.PeerUtil, msgID int32) (Clip, error) {
	res, err := channel.Shared().GetMessages(ctx, userID, mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  peer.PeerId,
		AccessHash: peer.AccessHash,
	}).To_InputChannel(), []int32{msgID})
	if err != nil {
		logx.WithContext(ctx).Errorf("messages.transcribeAudio channel: %v", err)
		return Clip{}, err
	}
	if res == nil || len(res.GetMessages()) == 0 {
		return Clip{}, mtproto.ErrMsgIdInvalid
	}
	return clipFromMessage(res.GetMessages()[0])
}

func sameDialog(box *mtproto.MessageBox, userID int64, peer *mtproto.PeerUtil) bool {
	switch peer.PeerType {
	case mtproto.PEER_SELF:
		return (box.GetPeerType() == mtproto.PEER_USER || box.GetPeerType() == mtproto.PEER_SELF) &&
			(box.GetPeerId() == userID || box.GetPeerId() == peer.PeerId)
	case mtproto.PEER_USER:
		return box.GetPeerType() == mtproto.PEER_USER && box.GetPeerId() == peer.PeerId
	case mtproto.PEER_CHAT:
		return box.GetPeerType() == mtproto.PEER_CHAT && box.GetPeerId() == peer.PeerId
	default:
		return false
	}
}

func clipFromMessage(msg *mtproto.Message) (Clip, error) {
	if msg == nil || msg.GetPredicateName() == mtproto.Predicate_messageEmpty {
		return Clip{}, mtproto.ErrMsgIdInvalid
	}
	media := msg.GetMedia()
	if media == nil {
		return Clip{}, mtproto.ErrMediaEmpty
	}
	doc := media.GetDocument()
	if doc == nil || doc.GetId() == 0 {
		return Clip{}, mtproto.ErrMediaEmpty
	}
	var voice, round bool
	var duration float64
	for _, attr := range doc.GetAttributes() {
		if attr.GetVoice() {
			voice = true
		}
		if attr.GetRoundMessage() {
			round = true
		}
		if d := attr.GetDuration_INT32(); d > 0 && float64(d) > duration {
			duration = float64(d)
		}
		if d := attr.GetDuration_FLOAT64(); d > duration {
			duration = d
		}
		if d := attr.GetDuration(); d > duration {
			duration = d
		}
	}
	if media.GetVoice() {
		voice = true
	}
	if !voice && !round {
		return Clip{}, mtproto.ErrMediaEmpty
	}
	name := "voice.ogg"
	if round && !voice {
		name = "round.mp4"
	}
	size := doc.GetSize2_INT64()
	if size == 0 {
		size = doc.GetSize2()
	}
	if size == 0 && doc.GetSize2_INT32() > 0 {
		size = int64(doc.GetSize2_INT32())
	}
	return Clip{
		DocumentID:    doc.GetId(),
		AccessHash:    doc.GetAccessHash(),
		FileReference: doc.GetFileReference(),
		Filename:      name,
		Duration:      duration,
		Size:          size,
	}, nil
}

func fetchClip(ctx context.Context, files FileStore, clip Clip) ([]byte, error) {
	if files == nil {
		return nil, mtproto.ErrInternalServerError
	}
	if clip.Size > MaxBytes {
		return nil, ErrAudioTooLong
	}
	const chunk = 512 * 1024
	buf := make([]byte, 0, 64*1024)
	var offset int64
	for {
		if len(buf) > MaxBytes {
			return nil, ErrAudioTooLong
		}
		file, err := files.DfsDownloadFile(ctx, &dfspb.TLDfsDownloadFile{
			Location: mtproto.MakeTLInputDocumentFileLocation(&mtproto.InputFileLocation{
				Id:            clip.DocumentID,
				AccessHash:    clip.AccessHash,
				FileReference: clip.FileReference,
			}).To_InputFileLocation(),
			Offset: offset,
			Limit:  chunk,
		})
		if err != nil {
			logx.WithContext(ctx).Errorf("messages.transcribeAudio download: %v", err)
			return nil, mtproto.ErrMediaEmpty
		}
		part := []byte(nil)
		if file != nil {
			part = file.GetBytes()
		}
		if len(part) == 0 {
			break
		}
		buf = append(buf, part...)
		offset += int64(len(part))
		if len(part) < chunk || (clip.Size > 0 && offset >= clip.Size) {
			break
		}
	}
	if len(buf) > MaxBytes {
		return nil, ErrAudioTooLong
	}
	return buf, nil
}

// Ensure dfsclient is referenced so a signature drift fails the build next to FileStore.
var _ FileStore = dfsclient.DfsClient(nil)
