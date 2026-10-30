package core

import (
	"github.com/teamgram/proto/mtproto"
)

// MessagesGetDialogFiltersEFD48C89
// messages.getDialogFilters#efd48c89 = messages.DialogFilters;
// https://core.telegram.org/method/messages.getDialogFilters
// https://core.telegram.org/constructor/messages.dialogFilters
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
// Older layer of the same method. tags_enabled is not part of this result.
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
