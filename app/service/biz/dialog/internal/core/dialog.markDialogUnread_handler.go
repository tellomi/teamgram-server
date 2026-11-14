package core

import (
	"context"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
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
	// getDialogs caches DialogExt, including unread_mark. A bare UPDATE left
	// that object in place, so clearing the mark still came back as set.
	_, _, err := c.svcCtx.Dao.ExecInvalidate(
		c.ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			err := c.svcCtx.Dao.SetUnreadMark(ctx, in.GetUserId(), in.GetPeerType(), in.GetPeerId(), unread)
			return 0, 0, err
		},
		dialog.GetDialogCacheKeyByPeer(in.GetUserId(), in.GetPeerType(), in.GetPeerId()))
	if err != nil {
		c.Logger.Errorf("dialog.markDialogUnread - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
