package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// DialogMarkDialogUnread
// dialog.markDialogUnread user_id:long peer_type:int peer_id:long unread_mark:Bool = Bool;
func (c *DialogCore) DialogMarkDialogUnread(in *dialog.TLDialogMarkDialogUnread) (*mtproto.Bool, error) {
	if in.GetUserId() == 0 || in.GetPeerId() == 0 {
		return mtproto.BoolTrue, nil
	}
	unread := in.GetUnreadMark() != nil && mtproto.FromBool(in.GetUnreadMark())
	if err := c.svcCtx.Dao.SetUnreadMark(c.ctx, in.GetUserId(), in.GetPeerType(), in.GetPeerId(), unread); err != nil {
		c.Logger.Errorf("dialog.markDialogUnread - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
