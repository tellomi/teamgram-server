package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// MessagesUpdateDialogFilter
// messages.updateDialogFilter#1ad4a04a flags:# id:int filter:flags.0?DialogFilter = Bool;
// https://core.telegram.org/method/messages.updateDialogFilter
// https://core.telegram.org/api/folders
//
// filter omitted deletes the folder. id 0 is the default "All chats" folder
// (dialogFilterDefault) and is not stored.
func (c *FoldersCore) MessagesUpdateDialogFilter(in *mtproto.TLMessagesUpdateDialogFilter) (*mtproto.Bool, error) {
	if in.GetId() <= 0 {
		return nil, mtproto.ErrFilterIdInvalid
	}
	filter := in.GetFilter()
	if filter == nil {
		if _, err := c.svcCtx.Dao.DialogClient.DialogDeleteDialogFilter(c.ctx, &dialog.TLDialogDeleteDialogFilter{
			UserId: c.userID(),
			Id:     in.GetId(),
		}); err != nil {
			c.Logger.Errorf("messages.updateDialogFilter - delete error: %v", err)
			return nil, err
		}
		c.pushNotMe(mtproto.MakeTLUpdateDialogFilter(&mtproto.Update{
			Id_INT32: in.GetId(),
		}).To_Update())
		return mtproto.BoolTrue, nil
	}

	if err := validateDialogFilter(in.GetId(), filter); err != nil {
		return nil, err
	}
	applyColorPolicy(filter, c.findFilter(in.GetId()), c.allowFolderColor())
	if filter.GetEmoticon() != nil && filter.GetEmoticon().GetValue() == "" {
		filter.Emoticon = nil
	}
	filter.HasMyInvites = false
	sealed := prepareStoredFilter(filter)

	if _, err := c.svcCtx.Dao.DialogClient.DialogInsertOrUpdateDialogFilter(c.ctx, &dialog.TLDialogInsertOrUpdateDialogFilter{
		UserId:       c.userID(),
		Id:           in.GetId(),
		DialogFilter: sealed,
	}); err != nil {
		c.Logger.Errorf("messages.updateDialogFilter - error: %v", err)
		return nil, err
	}
	c.pushNotMe(mtproto.MakeTLUpdateDialogFilter(&mtproto.Update{
		Id_INT32: in.GetId(),
		Filter:   sealed,
	}).To_Update())
	return mtproto.BoolTrue, nil
}
