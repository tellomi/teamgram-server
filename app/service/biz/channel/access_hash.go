package channel

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/accesshash"
)

// IssuedAccessHash is the access_hash stored for this channel and returned
// on channel objects. Missing and deleted channels report ok false.
func (s *Service) IssuedAccessHash(ctx context.Context, id int64) (int64, bool, error) {
	if s == nil || s.store == nil || id == 0 {
		return 0, false, nil
	}
	ch, err := s.store.GetChannel(ctx, id)
	if err != nil {
		return 0, false, err
	}
	if ch == nil || ch.Deleted {
		return 0, false, nil
	}
	return ch.AccessHash, true, nil
}

// VisibleMessage is a channel post the viewer is allowed to see.
// A private channel requires membership. A public channel (one with a
// username) is visible without joining. The returned message is what the
// client was shown. The internal sender of an anonymous post is not added.
func (s *Service) VisibleMessage(ctx context.Context, viewer, channelID int64, msgID int32) (*mtproto.Message, bool, error) {
	if s == nil || s.store == nil || viewer == 0 || channelID == 0 || msgID == 0 {
		return nil, false, nil
	}
	ch, err := s.store.GetChannel(ctx, channelID)
	if err != nil {
		return nil, false, err
	}
	if ch == nil || ch.Deleted {
		return nil, false, nil
	}
	p, err := s.store.GetParticipant(ctx, channelID, viewer)
	if err != nil {
		return nil, false, err
	}
	if !ch.Public() && (p == nil || !p.InChannel()) {
		return nil, false, nil
	}
	stored, err := s.store.GetMessage(ctx, channelID, msgID)
	if err != nil {
		return nil, false, err
	}
	if stored == nil || stored.Deleted || len(stored.Data) == 0 {
		return nil, false, nil
	}
	body, err := unmarshalMessage(stored.Data)
	if err != nil {
		return nil, false, err
	}
	return body, true, nil
}

// hashOK applies TELLOMI_ACCESS_HASH_ENFORCE to a plain inputChannel.
// inputChannelFromMessage has no hash of its own. The bff gate checks that
// the message refers to this channel before the handler runs.
func hashOK(in *mtproto.InputChannel, stored int64) bool {
	if accesshash.IsInputChannelFromMessage(in) {
		return true
	}
	return accesshash.AllowProvided(in.GetAccessHash(), stored)
}
