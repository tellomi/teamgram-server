package channel

import (
	"context"

	"github.com/teamgram/proto/mtproto"
)

// StoryAttach is the story ring and profile block shown on a channel.
// The story service installs LookupStories; channel objects stay unchanged
// when it is nil.
type StoryAttach struct {
	MaxID   int32
	Live    bool
	Hidden  bool
	Pinned  bool
	Stories *mtproto.PeerStories
}

// LookupStories is set by the story service.
var LookupStories func(ctx context.Context, viewer, channelID int64) StoryAttach

// StoryPerm reports whether userID may view or manage channelID's stories.
// A non-zero accessHash that does not match the channel is rejected.
func (s *Service) StoryPerm(ctx context.Context, userID, channelID, accessHash int64) (view, post, edit, del bool, err error) {
	if s == nil || s.store == nil {
		return false, false, false, false, mtproto.ErrChannelInvalid
	}
	ch, err := s.store.GetChannel(ctx, channelID)
	if err != nil {
		return false, false, false, false, err
	}
	if ch == nil || ch.Deleted {
		return false, false, false, false, mtproto.ErrChannelInvalid
	}
	if accessHash != 0 && accessHash != ch.AccessHash {
		return false, false, false, false, mtproto.ErrChannelInvalid
	}
	p, err := s.store.GetParticipant(ctx, channelID, userID)
	if err != nil {
		return false, false, false, false, err
	}
	member := p != nil && p.InChannel()
	if !member && !ch.Public() {
		return false, false, false, false, mtproto.ErrChannelPrivate
	}
	view = true
	if p.IsCreator() {
		return true, true, true, true, nil
	}
	if p != nil && p.IsAdmin() {
		rights := p.adminRights()
		post = rights.CanPostStories()
		edit = rights.CanEditStories()
		del = rights.CanDeleteStories()
	}
	return view, post, edit, del, nil
}

// MemberIDs lists users currently in the channel.
func (s *Service) MemberIDs(ctx context.Context, channelID int64) ([]int64, error) {
	if s == nil || s.store == nil {
		return nil, nil
	}
	parts, err := s.store.ListParticipants(ctx, channelID)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(parts))
	for _, p := range parts {
		if p.InChannel() {
			ids = append(ids, p.UserID)
		}
	}
	return ids, nil
}

// JoinedChannelIDs lists channels the user is currently in.
func (s *Service) JoinedChannelIDs(ctx context.Context, userID int64) ([]int64, error) {
	if s == nil || s.store == nil {
		return nil, nil
	}
	list, err := s.store.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(list))
	for _, ch := range list {
		ids = append(ids, ch.ID)
	}
	return ids, nil
}
