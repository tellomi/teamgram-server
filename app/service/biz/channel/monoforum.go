package channel

import (
	"context"
	"strconv"

	"github.com/teamgram/proto/mtproto"
)

// SetMonoforum marks a channel as a monoforum (direct messages). Nothing in
// this process creates one unless a caller sets the flag.
func (s *Service) SetMonoforum(ctx context.Context, channelID int64, on bool) error {
	if _, err := s.reload(ctx, channelID); err != nil {
		return err
	}
	f := s.features(ctx, channelID)
	f.Monoforum = on
	return s.saveFeatures(ctx, channelID, f)
}

func (s *Service) monoforumAdmin(ctx context.Context, self int64, in *mtproto.InputChannel) (*Channel, *Participant, error) {
	ch, err := s.lookup(ctx, in)
	if err != nil {
		return nil, nil, err
	}
	if !s.features(ctx, ch.ID).Monoforum {
		return nil, nil, mtproto.ErrChannelInvalid
	}
	p, err := s.store.GetParticipant(ctx, ch.ID, self)
	if err != nil {
		return nil, nil, err
	}
	if p == nil || !p.IsAdmin() || p.IsBot {
		return nil, nil, mtproto.ErrChatAdminRequired
	}
	return ch, p, nil
}

// RequireMonoforumAdmin reports PARENT_PEER_INVALID when the peer is not a
// monoforum and CHAT_ADMIN_REQUIRED when the user is not a non-bot admin.
func (s *Service) RequireMonoforumAdmin(ctx context.Context, self int64, in *mtproto.InputChannel) error {
	_, _, err := s.monoforumAdmin(ctx, self, in)
	if err == mtproto.ErrChannelInvalid {
		return mtproto.ErrParentPeerInvalid
	}
	return err
}

// MonoforumAuthor is channels.getMessageAuthor. The caller must be a non-bot
// admin of a monoforum. The result is the stored sender of that message.
// https://core.telegram.org/method/channels.getMessageAuthor
func (s *Service) MonoforumAuthor(ctx context.Context, self int64, in *mtproto.InputChannel, msgID int32) (int64, error) {
	if msgID <= 0 {
		return 0, mtproto.ErrMsgIdInvalid
	}
	ch, _, err := s.monoforumAdmin(ctx, self, in)
	if err != nil {
		return 0, err
	}
	stored, err := s.store.GetMessage(ctx, ch.ID, msgID)
	if err != nil {
		return 0, err
	}
	if stored == nil || stored.Deleted || stored.SenderID == 0 {
		return 0, mtproto.ErrMsgIdInvalid
	}
	return stored.SenderID, nil
}

func savedReadKey(userID int64, topicType int32, topicID int64) string {
	return strconv.FormatInt(userID, 10) + ":" + strconv.FormatInt(int64(topicType), 10) + ":" + strconv.FormatInt(topicID, 10)
}

// ReadMonoforum is messages.readSavedHistory. parent is the monoforum.
// maxID marks that topic read through this message id and does not move backward.
// https://core.telegram.org/method/messages.readSavedHistory
func (s *Service) ReadMonoforum(ctx context.Context, self int64, parent *mtproto.InputChannel, topicType int32, topicID int64, maxID int32) error {
	if maxID < 0 {
		return mtproto.ErrMsgIdInvalid
	}
	if topicID == 0 && topicType != mtproto.PEER_SELF {
		return mtproto.ErrPeerIdInvalid
	}
	ch, _, err := s.monoforumAdmin(ctx, self, parent)
	if err != nil {
		if err == mtproto.ErrChannelInvalid {
			return mtproto.ErrParentPeerInvalid
		}
		return err
	}
	f := s.features(ctx, ch.ID)
	if f.SavedRead == nil {
		f.SavedRead = map[string]int32{}
	}
	key := savedReadKey(self, topicType, topicID)
	if maxID > f.SavedRead[key] {
		f.SavedRead[key] = maxID
		if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
			return err
		}
	}
	return nil
}

// MonoforumRead is the stored read cursor, or 0.
func (s *Service) MonoforumRead(ctx context.Context, self, channelID int64, topicType int32, topicID int64) int32 {
	f := s.features(ctx, channelID)
	if f.SavedRead == nil {
		return 0
	}
	return f.SavedRead[savedReadKey(self, topicType, topicID)]
}
