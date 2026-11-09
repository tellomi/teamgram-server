package core

import (
	"github.com/teamgram/proto/mtproto"
)

// MessagesGetDialogFiltersEFD48C89
// messages.getDialogFilters#efd48c89 = messages.DialogFilters;
// https://core.telegram.org/method/messages.getDialogFilters
// https://core.telegram.org/constructor/messages.dialogFilters
//
// The vector always contains dialogFilterDefault. tags_enabled is the stored
// folder-tags flag (flags.0 on messages.dialogFilters#2ad93719) and is not
// changed by including that default entry.
//
// Already-open iOS sessions keep the previous filter list until they refetch.
// They do that on every cold start, so a force-quit and relaunch after deploy
// is enough; no reinstall. This process does not push updateDialogFilters to
// every online session on startup. It has no session list, each replica would
// fire, and every iOS client would call getDialogFilters at once.
// updateDialogFilters (no fields) remains the official refetch signal and is
// still sent when folder tags actually change.
func (c *FoldersCore) MessagesGetDialogFiltersEFD48C89(in *mtproto.TLMessagesGetDialogFiltersEFD48C89) (*mtproto.Messages_DialogFilters, error) {
	filters, tags, err := c.loadFilters()
	if err != nil {
		c.Logger.Errorf("messages.getDialogFilters - error: %v", err)
		return nil, err
	}
	return messagesDialogFilters(filters, tags), nil
}

// MessagesGetDialogFiltersF19ED96D
// messages.getDialogFilters#f19ed96d = Vector<DialogFilter>;
// Older layer of the same method. The vector still includes
// dialogFilterDefault. tags_enabled is not part of this result.
func (c *FoldersCore) MessagesGetDialogFiltersF19ED96D(in *mtproto.TLMessagesGetDialogFiltersF19ED96D) (*mtproto.Vector_DialogFilter, error) {
	filters, _, err := c.loadFilters()
	if err != nil {
		c.Logger.Errorf("messages.getDialogFilters - error: %v", err)
		return nil, err
	}
	if filters == nil {
		filters = []*mtproto.DialogFilter{}
	}
	return &mtproto.Vector_DialogFilter{Datas: filters}, nil
}
