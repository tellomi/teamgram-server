package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/story"
)

// storyShareMedia turns inputMediaStory into messageMediaStory.
// The story the viewer is allowed to see is attached so the chat bubble
// can draw the card without a second round trip. A missing or hidden
// story is STORY_ID_INVALID, the same error stories.getStoriesByID uses
// for an id the caller cannot open.
func (c *MessagesCore) storyShareMedia(media *mtproto.InputMedia) (*mtproto.MessageMedia, error) {
	if c == nil || c.MD == nil || media == nil {
		return nil, mtproto.ErrStoryIdInvalid
	}
	peer := mtproto.FromInputPeer2(c.MD.UserId, media.GetPeer())
	if peer == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	var owner story.Owner
	var hash int64
	switch peer.PeerType {
	case mtproto.PEER_SELF:
		owner = story.Owner{Kind: story.KindUser, ID: c.MD.UserId}
	case mtproto.PEER_USER:
		if peer.PeerId == 0 {
			return nil, mtproto.ErrPeerIdInvalid
		}
		owner = story.Owner{Kind: story.KindUser, ID: peer.PeerId}
		hash = peer.AccessHash
	case mtproto.PEER_CHANNEL:
		if peer.PeerId == 0 {
			return nil, mtproto.ErrPeerIdInvalid
		}
		owner = story.Owner{Kind: story.KindChannel, ID: peer.PeerId}
		hash = peer.AccessHash
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}
	id := media.GetId_INT32()
	if id == 0 {
		return nil, mtproto.ErrStoryIdInvalid
	}
	got, err := story.Shared().GetByID(c.ctx, c.MD.UserId, owner, hash, []int32{id})
	if err != nil {
		return nil, err
	}
	var item *mtproto.StoryItem
	if got != nil && len(got.GetStories()) > 0 {
		item = got.GetStories()[0]
	}
	if item == nil || item.GetPredicateName() == mtproto.Predicate_storyItemDeleted || item.GetId() == 0 {
		return nil, mtproto.ErrStoryIdInvalid
	}
	return mtproto.MakeTLMessageMediaStory(&mtproto.MessageMedia{
		Peer:  owner.Peer(),
		Id:    id,
		Story: item,
	}).To_MessageMedia(), nil
}
