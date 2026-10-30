package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// MessagesUpdateDialogFiltersOrder
// messages.updateDialogFiltersOrder#c563c1e4 order:Vector<int> = Bool;
// https://core.telegram.org/method/messages.updateDialogFiltersOrder
// https://core.telegram.org/api/folders
//
// The order vector includes 0 for dialogFilterDefault (the All chats folder).
// An empty vector is a no-op success. Unknown positive ids are left untouched
// by storage and still echoed in updateDialogFilterOrder.
func (c *FoldersCore) MessagesUpdateDialogFiltersOrder(in *mtproto.TLMessagesUpdateDialogFiltersOrder) (*mtproto.Bool, error) {
	order := in.GetOrder()
	if len(order) == 0 {
		return mtproto.BoolTrue, nil
	}
	for _, id := range order {
		if id < 0 {
			return nil, mtproto.ErrFilterIdInvalid
		}
	}
	if _, err := c.svcCtx.Dao.DialogClient.DialogUpdateDialogFiltersOrder(c.ctx, &dialog.TLDialogUpdateDialogFiltersOrder{
		UserId: c.userID(),
		Order:  order,
	}); err != nil {
		c.Logger.Errorf("messages.updateDialogFiltersOrder - error: %v", err)
		return nil, err
	}
	c.pushNotMe(mtproto.MakeTLUpdateDialogFilterOrder(&mtproto.Update{
		Order_VECTORINT32: append([]int32(nil), order...),
	}).To_Update())
	return mtproto.BoolTrue, nil
}
