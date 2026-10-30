package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/folders/internal/core"
)

// MessagesGetDialogFiltersEFD48C89
// messages.getDialogFilters#efd48c89 = messages.DialogFilters;
func (s *Service) MessagesGetDialogFiltersEFD48C89(ctx context.Context, request *mtproto.TLMessagesGetDialogFiltersEFD48C89) (*mtproto.Messages_DialogFilters, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("messages.getDialogFilters - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.MessagesGetDialogFiltersEFD48C89(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("messages.getDialogFilters - reply: {%s}", r)
	return r, err
}

// MessagesGetDialogFiltersF19ED96D
// messages.getDialogFilters#f19ed96d = Vector<DialogFilter>;
func (s *Service) MessagesGetDialogFiltersF19ED96D(ctx context.Context, request *mtproto.TLMessagesGetDialogFiltersF19ED96D) (*mtproto.Vector_DialogFilter, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("messages.getDialogFilters - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.MessagesGetDialogFiltersF19ED96D(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("messages.getDialogFilters - reply: {%s}", r)
	return r, err
}

// MessagesGetSuggestedDialogFilters
// messages.getSuggestedDialogFilters#a29cd42c = Vector<DialogFilterSuggested>;
func (s *Service) MessagesGetSuggestedDialogFilters(ctx context.Context, request *mtproto.TLMessagesGetSuggestedDialogFilters) (*mtproto.Vector_DialogFilterSuggested, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("messages.getSuggestedDialogFilters - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.MessagesGetSuggestedDialogFilters(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("messages.getSuggestedDialogFilters - reply: {%s}", r)
	return r, err
}

// MessagesUpdateDialogFilter
// messages.updateDialogFilter#1ad4a04a flags:# id:int filter:flags.0?DialogFilter = Bool;
func (s *Service) MessagesUpdateDialogFilter(ctx context.Context, request *mtproto.TLMessagesUpdateDialogFilter) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("messages.updateDialogFilter - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.MessagesUpdateDialogFilter(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("messages.updateDialogFilter - reply: {%s}", r)
	return r, err
}

// MessagesUpdateDialogFiltersOrder
// messages.updateDialogFiltersOrder#c563c1e4 order:Vector<int> = Bool;
func (s *Service) MessagesUpdateDialogFiltersOrder(ctx context.Context, request *mtproto.TLMessagesUpdateDialogFiltersOrder) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("messages.updateDialogFiltersOrder - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.MessagesUpdateDialogFiltersOrder(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("messages.updateDialogFiltersOrder - reply: {%s}", r)
	return r, err
}

// MessagesToggleDialogFilterTags
// messages.toggleDialogFilterTags#fd2dda49 enabled:Bool = Bool;
func (s *Service) MessagesToggleDialogFilterTags(ctx context.Context, request *mtproto.TLMessagesToggleDialogFilterTags) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("messages.toggleDialogFilterTags - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.MessagesToggleDialogFilterTags(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("messages.toggleDialogFilterTags - reply: {%s}", r)
	return r, err
}
