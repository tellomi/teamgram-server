package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/savedmessagedialogs/saved"
)

// MessagesGetSavedDialogsByID
// messages.getSavedDialogsByID#6f6f9c96 flags:# parent_peer:flags.1?InputPeer ids:Vector<InputPeer> = messages.SavedDialogs;
// https://core.telegram.org/method/messages.getSavedDialogsByID
//
// Returns the saved dialogs named in ids. Peers that have no saved dialog are
// omitted. parent_peer asks for monoforum topics.
func (c *SavedMessageDialogsCore) MessagesGetSavedDialogsByID(in *mtproto.TLMessagesGetSavedDialogsByID) (*mtproto.Messages_SavedDialogs, error) {
	if err := c.requireUser(); err != nil {
		return nil, err
	}
	if saved.ParentSpecified(in.GetParentPeer()) {
		if err := c.requireMonoforum(in.GetParentPeer()); err != nil {
			c.Logger.Errorf("messages.getSavedDialogsByID - error: %v", err)
			return nil, err
		}
		return emptySaved(true, 0), nil
	}
	var dialogs []*mtproto.SavedDialog
	for i, id := range in.GetIds() {
		if i >= saved.ByIDCap {
			break
		}
		peer, err := c.savedPeer(id)
		if err != nil {
			continue
		}
		row, err := c.lookupSaved(peer)
		if err != nil {
			c.Logger.Errorf("messages.getSavedDialogsByID - error: %v", err)
			return nil, err
		}
		if row != nil {
			dialogs = append(dialogs, row)
		}
	}
	return c.packSaved(dialogs, int32(len(dialogs)), true)
}
