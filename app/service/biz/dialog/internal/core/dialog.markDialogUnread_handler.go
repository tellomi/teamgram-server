package core

import (
	"context"
	"time"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"
)

// DialogMarkDialogUnread
// dialog.markDialogUnread user_id:long peer_type:int peer_id:long unread_mark:Bool = Bool;
func (c *DialogCore) DialogMarkDialogUnread(in *dialog.TLDialogMarkDialogUnread) (*mtproto.Bool, error) {
	if in.GetUserId() == 0 || in.GetPeerId() == 0 {
		return mtproto.BoolTrue, nil
	}
	unread := in.GetUnreadMark() != nil && mtproto.FromBool(in.GetUnreadMark())
	n, err := c.svcCtx.Dao.SetUnreadMark(c.ctx, in.GetUserId(), in.GetPeerType(), in.GetPeerId(), unread)
	if err != nil {
		c.Logger.Errorf("dialog.markDialogUnread - error: %v", err)
		return nil, err
	}
	keys := []string{dialog.GetDialogCacheKeyByPeer(in.GetUserId(), in.GetPeerType(), in.GetPeerId())}
	if n == 0 && unread {
		_, _, err = c.svcCtx.Dao.DialogsDAO.InsertIgnore(c.ctx, &dataobject.DialogsDO{
			UserId:           in.GetUserId(),
			PeerType:         in.GetPeerType(),
			PeerId:           in.GetPeerId(),
			PeerDialogId:     mtproto.MakePeerDialogId(in.GetPeerType(), in.GetPeerId()),
			UnreadMark:       true,
			DraftMessageData: "null",
			Date2:            time.Now().Unix(),
		})
		if err != nil {
			c.Logger.Errorf("dialog.markDialogUnread - insert: %v", err)
			return nil, err
		}
		if k := dialog.GetCacheKeyByPeerType(in.GetUserId(), in.GetPeerType()); k != "" {
			keys = append(keys, k)
		}
	}
	_, _, err = c.svcCtx.Dao.ExecInvalidate(c.ctx, func(context.Context, *sqlx.DB) (int64, int64, error) {
		return 0, n, nil
	}, keys...)
	if err != nil {
		c.Logger.Errorf("dialog.markDialogUnread - cache: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
