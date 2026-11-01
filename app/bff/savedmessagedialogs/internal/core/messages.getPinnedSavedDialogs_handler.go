package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// MessagesGetPinnedSavedDialogs
// messages.getPinnedSavedDialogs#d63d94e0 = messages.SavedDialogs;
// https://core.telegram.org/method/messages.getPinnedSavedDialogs
func (c *SavedMessageDialogsCore) MessagesGetPinnedSavedDialogs(in *mtproto.TLMessagesGetPinnedSavedDialogs) (*mtproto.Messages_SavedDialogs, error) {
	if err := c.requireUser(); err != nil {
		return nil, err
	}
	list, err := c.svcCtx.Dao.DialogGetPinnedSavedDialogs(c.ctx, &dialog.TLDialogGetPinnedSavedDialogs{
		UserId: c.MD.UserId,
	})
	if err != nil {
		c.Logger.Errorf("messages.getPinnedSavedDialogs - error: %v", err)
		return nil, err
	}
	var dialogs []*mtproto.SavedDialog
	if list != nil {
		dialogs = list.Dialogs
	}
	return c.packSaved(dialogs, int32(len(dialogs)), true)
}
