package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/savedmessagedialogs/saved"
)

// MessagesGetSavedDialogs
// messages.getSavedDialogs#1e91fc99 flags:# exclude_pinned:flags.0?true parent_peer:flags.1?InputPeer offset_date:int offset_id:int offset_peer:InputPeer limit:int hash:long = messages.SavedDialogs;
// https://core.telegram.org/method/messages.getSavedDialogs
//
// Returns the saved-dialog list, or the topic list of a monoforum when
// parent_peer is set. Pagination is by top_message id (offset_id). A matching
// hash returns messages.savedDialogsNotModified. A short page is
// messages.savedDialogs; a full page is messages.savedDialogsSlice.
func (c *SavedMessageDialogsCore) MessagesGetSavedDialogs(in *mtproto.TLMessagesGetSavedDialogs) (*mtproto.Messages_SavedDialogs, error) {
	if err := c.requireUser(); err != nil {
		return nil, err
	}
	if saved.ParentSpecified(in.GetParentPeer()) {
		if err := c.requireMonoforum(in.GetParentPeer()); err != nil {
			c.Logger.Errorf("messages.getSavedDialogs - error: %v", err)
			return nil, err
		}
		return emptySaved(true, 0), nil
	}
	if in.GetHash() != 0 {
		rows, count, incomplete, err := c.allSaved(in.GetExcludePinned())
		if err != nil {
			c.Logger.Errorf("messages.getSavedDialogs - error: %v", err)
			return nil, err
		}
		if !incomplete && saved.NotModified(in.GetHash(), rows) {
			return mtproto.MakeTLMessagesSavedDialogsNotModified(&mtproto.Messages_SavedDialogs{
				Count: count,
			}).To_Messages_SavedDialogs(), nil
		}
	}
	limit := saved.NormalizeLimit(in.GetLimit(), saved.PageCap)
	list, err := c.querySaved(in.GetExcludePinned(), in.GetOffsetId(), limit)
	if err != nil {
		c.Logger.Errorf("messages.getSavedDialogs - error: %v", err)
		return nil, err
	}
	if list == nil {
		return emptySaved(true, 0), nil
	}
	return c.packSaved(list.Dialogs, list.Count, saved.FullList(int32(len(list.Dialogs)), limit))
}
