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

// FoldersEditPeerFolders
// folders.editPeerFolders#6847d0ab folder_peers:Vector<InputFolderPeer> = Updates;
func (s *Service) FoldersEditPeerFolders(ctx context.Context, request *mtproto.TLFoldersEditPeerFolders) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("folders.editPeerFolders - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.FoldersEditPeerFolders(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("folders.editPeerFolders - reply: {%s}", r)
	return r, err
}

// ChatlistsExportChatlistInvite
// chatlists.exportChatlistInvite#8472478e chatlist:InputChatlist title:string peers:Vector<InputPeer> = chatlists.ExportedChatlistInvite;
func (s *Service) ChatlistsExportChatlistInvite(ctx context.Context, request *mtproto.TLChatlistsExportChatlistInvite) (*mtproto.Chatlists_ExportedChatlistInvite, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("chatlists.exportChatlistInvite - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.ChatlistsExportChatlistInvite(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("chatlists.exportChatlistInvite - reply: {%s}", r)
	return r, err
}

// ChatlistsDeleteExportedInvite
// chatlists.deleteExportedInvite#719c5c5e chatlist:InputChatlist slug:string = Bool;
func (s *Service) ChatlistsDeleteExportedInvite(ctx context.Context, request *mtproto.TLChatlistsDeleteExportedInvite) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("chatlists.deleteExportedInvite - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.ChatlistsDeleteExportedInvite(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("chatlists.deleteExportedInvite - reply: {%s}", r)
	return r, err
}

// ChatlistsEditExportedInvite
// chatlists.editExportedInvite#653db63d flags:# chatlist:InputChatlist slug:string title:flags.1?string peers:flags.2?Vector<InputPeer> = ExportedChatlistInvite;
func (s *Service) ChatlistsEditExportedInvite(ctx context.Context, request *mtproto.TLChatlistsEditExportedInvite) (*mtproto.ExportedChatlistInvite, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("chatlists.editExportedInvite - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.ChatlistsEditExportedInvite(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("chatlists.editExportedInvite - reply: {%s}", r)
	return r, err
}

// ChatlistsGetExportedInvites
// chatlists.getExportedInvites#ce03da83 chatlist:InputChatlist = chatlists.ExportedInvites;
func (s *Service) ChatlistsGetExportedInvites(ctx context.Context, request *mtproto.TLChatlistsGetExportedInvites) (*mtproto.Chatlists_ExportedInvites, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("chatlists.getExportedInvites - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.ChatlistsGetExportedInvites(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("chatlists.getExportedInvites - reply: {%s}", r)
	return r, err
}

// ChatlistsCheckChatlistInvite
// chatlists.checkChatlistInvite#41c10fff slug:string = chatlists.ChatlistInvite;
func (s *Service) ChatlistsCheckChatlistInvite(ctx context.Context, request *mtproto.TLChatlistsCheckChatlistInvite) (*mtproto.Chatlists_ChatlistInvite, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("chatlists.checkChatlistInvite - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.ChatlistsCheckChatlistInvite(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("chatlists.checkChatlistInvite - reply: {%s}", r)
	return r, err
}

// ChatlistsJoinChatlistInvite
// chatlists.joinChatlistInvite#a6b1e39a slug:string peers:Vector<InputPeer> = Updates;
func (s *Service) ChatlistsJoinChatlistInvite(ctx context.Context, request *mtproto.TLChatlistsJoinChatlistInvite) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("chatlists.joinChatlistInvite - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.ChatlistsJoinChatlistInvite(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("chatlists.joinChatlistInvite - reply: {%s}", r)
	return r, err
}

// ChatlistsGetChatlistUpdates
// chatlists.getChatlistUpdates#89419521 chatlist:InputChatlist = chatlists.ChatlistUpdates;
func (s *Service) ChatlistsGetChatlistUpdates(ctx context.Context, request *mtproto.TLChatlistsGetChatlistUpdates) (*mtproto.Chatlists_ChatlistUpdates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("chatlists.getChatlistUpdates - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.ChatlistsGetChatlistUpdates(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("chatlists.getChatlistUpdates - reply: {%s}", r)
	return r, err
}

// ChatlistsJoinChatlistUpdates
// chatlists.joinChatlistUpdates#e089f8f5 chatlist:InputChatlist peers:Vector<InputPeer> = Updates;
func (s *Service) ChatlistsJoinChatlistUpdates(ctx context.Context, request *mtproto.TLChatlistsJoinChatlistUpdates) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("chatlists.joinChatlistUpdates - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.ChatlistsJoinChatlistUpdates(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("chatlists.joinChatlistUpdates - reply: {%s}", r)
	return r, err
}

// ChatlistsHideChatlistUpdates
// chatlists.hideChatlistUpdates#66e486fb chatlist:InputChatlist = Bool;
func (s *Service) ChatlistsHideChatlistUpdates(ctx context.Context, request *mtproto.TLChatlistsHideChatlistUpdates) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("chatlists.hideChatlistUpdates - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.ChatlistsHideChatlistUpdates(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("chatlists.hideChatlistUpdates - reply: {%s}", r)
	return r, err
}

// ChatlistsGetLeaveChatlistSuggestions
// chatlists.getLeaveChatlistSuggestions#fdbcd714 chatlist:InputChatlist = Vector<Peer>;
func (s *Service) ChatlistsGetLeaveChatlistSuggestions(ctx context.Context, request *mtproto.TLChatlistsGetLeaveChatlistSuggestions) (*mtproto.Vector_Peer, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("chatlists.getLeaveChatlistSuggestions - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.ChatlistsGetLeaveChatlistSuggestions(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("chatlists.getLeaveChatlistSuggestions - reply: {%s}", r)
	return r, err
}

// ChatlistsLeaveChatlist
// chatlists.leaveChatlist#74fae13a chatlist:InputChatlist peers:Vector<InputPeer> = Updates;
func (s *Service) ChatlistsLeaveChatlist(ctx context.Context, request *mtproto.TLChatlistsLeaveChatlist) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("chatlists.leaveChatlist - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.ChatlistsLeaveChatlist(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("chatlists.leaveChatlist - reply: {%s}", r)
	return r, err
}
