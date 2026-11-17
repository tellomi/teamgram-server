package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// DialogGetUserPinnedMessage
// dialog.getUserPinnedMessage user_id:long peer_type:int peer_id:long = Int32;
//
// The value is dialogs.pinned_msg_id. Zero means this dialog has no pin.
// users.getFullUser, messages.getFullChat, and channels.getFullChannel
// copy a positive value onto pinned_msg_id.
func (c *DialogCore) DialogGetUserPinnedMessage(in *dialog.TLDialogGetUserPinnedMessage) (*mtproto.Int32, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.DialogsDAO == nil || in.GetUserId() == 0 || in.GetPeerId() == 0 {
		return &mtproto.Int32{}, nil
	}
	do, err := c.svcCtx.Dao.DialogsDAO.SelectDialog(c.ctx, in.GetUserId(), in.GetPeerType(), in.GetPeerId())
	if err != nil {
		c.Logger.Errorf("dialog.getUserPinnedMessage - error: %v", err)
		return nil, err
	}
	id := int32(0)
	if do != nil {
		id = do.PinnedMsgId
	}
	return &mtproto.Int32{V: id}, nil
}
