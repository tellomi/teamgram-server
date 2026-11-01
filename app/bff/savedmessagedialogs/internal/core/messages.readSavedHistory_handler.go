package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
)

// MessagesReadSavedHistory
// messages.readSavedHistory#ba4a3b5b parent_peer:InputPeer peer:InputPeer max_id:int = Bool;
// https://core.telegram.org/method/messages.readSavedHistory
//
// Marks a monoforum topic read through max_id. parent_peer is the monoforum.
// The cursor does not move backward.
func (c *SavedMessageDialogsCore) MessagesReadSavedHistory(in *mtproto.TLMessagesReadSavedHistory) (*mtproto.Bool, error) {
	if err := c.requireUser(); err != nil {
		return nil, err
	}
	if in.GetParentPeer() == nil {
		return nil, mtproto.ErrParentPeerInvalid
	}
	parent := mtproto.FromInputPeer2(c.MD.UserId, in.GetParentPeer())
	if parent == nil || parent.PeerType != mtproto.PEER_CHANNEL || parent.PeerId == 0 {
		return nil, mtproto.ErrParentPeerInvalid
	}
	topic, err := c.savedPeer(in.GetPeer())
	if err != nil {
		c.Logger.Errorf("messages.readSavedHistory - error: %v", err)
		return nil, err
	}
	if err := channel.Shared().ReadMonoforum(c.ctx, c.MD.UserId, inputChannel(parent), topic.PeerType, topic.PeerId, in.GetMaxId()); err != nil {
		c.Logger.Errorf("messages.readSavedHistory - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
