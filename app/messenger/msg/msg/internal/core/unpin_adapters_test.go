package core

import (
	"context"
	"fmt"

	"github.com/teamgram/proto/mtproto"
	inboxhelper "github.com/teamgram/teamgram-server/app/messenger/msg/inbox"
	inboxpb "github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	msgdao "github.com/teamgram/teamgram-server/app/messenger/msg/internal/dao"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	dialoghelper "github.com/teamgram/teamgram-server/app/service/biz/dialog"
	dialogpb "github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	idgenhelper "github.com/teamgram/teamgram-server/app/service/idgen"
	idgenpb "github.com/teamgram/teamgram-server/app/service/idgen/idgen"
)

// errNotOnUnpinPath is returned by interface methods the unpin call does not use.
func errNotOnUnpinPath(name string) error {
	return fmt.Errorf("%s is not on the unpin-all call path", name)
}

type localDialog struct {
	svc *dialoghelper.ServiceContext
}

func (d localDialog) DialogInsertOrUpdateDialog(ctx context.Context, in *dialogpb.TLDialogInsertOrUpdateDialog) (*mtproto.Bool, error) {
	return dialoghelper.NewDialogCore(ctx, d.svc).DialogInsertOrUpdateDialog(in)
}

type localInbox struct {
	dao *msgdao.Dao
}

func (l localInbox) InboxUnpinAllMessages(ctx context.Context, in *inboxpb.TLInboxUnpinAllMessages) (*mtproto.Void, error) {
	return inboxhelper.NewInboxCore(ctx, &inboxhelper.ServiceContext{Dao: l.dao}).InboxUnpinAllMessages(in)
}

type localSync struct {
	notMe []*syncpb.TLSyncUpdatesNotMe
	push  []*syncpb.TLSyncPushUpdates
}

func (s *localSync) SyncUpdatesNotMe(ctx context.Context, in *syncpb.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	s.notMe = append(s.notMe, in)
	return mtproto.EmptyVoid, nil
}

func (s *localSync) SyncPushUpdates(ctx context.Context, in *syncpb.TLSyncPushUpdates) (*mtproto.Void, error) {
	s.push = append(s.push, in)
	return mtproto.EmptyVoid, nil
}

type seqClient struct {
	core *idgenhelper.IdgenCore
}

func (s seqClient) IdgenGetNextNSeqId(ctx context.Context, in *idgenpb.TLIdgenGetNextNSeqId) (*mtproto.Int64, error) {
	return s.core.IdgenGetNextNSeqId(in)
}

func (l localDialog) DialogSaveDraftMessage(ctx context.Context, in *dialogpb.TLDialogSaveDraftMessage) (*mtproto.Bool, error) {
	return nil, errNotOnUnpinPath("DialogSaveDraftMessage")
}

func (l localDialog) DialogClearDraftMessage(ctx context.Context, in *dialogpb.TLDialogClearDraftMessage) (*mtproto.Bool, error) {
	return nil, errNotOnUnpinPath("DialogClearDraftMessage")
}

func (l localDialog) DialogGetAllDrafts(ctx context.Context, in *dialogpb.TLDialogGetAllDrafts) (*dialogpb.Vector_PeerWithDraftMessage, error) {
	return nil, errNotOnUnpinPath("DialogGetAllDrafts")
}

func (l localDialog) DialogClearAllDrafts(ctx context.Context, in *dialogpb.TLDialogClearAllDrafts) (*dialogpb.Vector_PeerWithDraftMessage, error) {
	return nil, errNotOnUnpinPath("DialogClearAllDrafts")
}

func (l localDialog) DialogMarkDialogUnread(ctx context.Context, in *dialogpb.TLDialogMarkDialogUnread) (*mtproto.Bool, error) {
	return nil, errNotOnUnpinPath("DialogMarkDialogUnread")
}

func (l localDialog) DialogToggleDialogPin(ctx context.Context, in *dialogpb.TLDialogToggleDialogPin) (*mtproto.Int32, error) {
	return nil, errNotOnUnpinPath("DialogToggleDialogPin")
}

func (l localDialog) DialogGetDialogUnreadMarkList(ctx context.Context, in *dialogpb.TLDialogGetDialogUnreadMarkList) (*dialogpb.Vector_DialogPeer, error) {
	return nil, errNotOnUnpinPath("DialogGetDialogUnreadMarkList")
}

func (l localDialog) DialogGetDialogsByOffsetDate(ctx context.Context, in *dialogpb.TLDialogGetDialogsByOffsetDate) (*dialogpb.Vector_DialogExt, error) {
	return nil, errNotOnUnpinPath("DialogGetDialogsByOffsetDate")
}

func (l localDialog) DialogGetDialogs(ctx context.Context, in *dialogpb.TLDialogGetDialogs) (*dialogpb.Vector_DialogExt, error) {
	return nil, errNotOnUnpinPath("DialogGetDialogs")
}

func (l localDialog) DialogGetDialogsByIdList(ctx context.Context, in *dialogpb.TLDialogGetDialogsByIdList) (*dialogpb.Vector_DialogExt, error) {
	return nil, errNotOnUnpinPath("DialogGetDialogsByIdList")
}

func (l localDialog) DialogGetDialogsCount(ctx context.Context, in *dialogpb.TLDialogGetDialogsCount) (*mtproto.Int32, error) {
	return nil, errNotOnUnpinPath("DialogGetDialogsCount")
}

func (l localDialog) DialogGetPinnedDialogs(ctx context.Context, in *dialogpb.TLDialogGetPinnedDialogs) (*dialogpb.Vector_DialogExt, error) {
	return nil, errNotOnUnpinPath("DialogGetPinnedDialogs")
}

func (l localDialog) DialogReorderPinnedDialogs(ctx context.Context, in *dialogpb.TLDialogReorderPinnedDialogs) (*mtproto.Bool, error) {
	return nil, errNotOnUnpinPath("DialogReorderPinnedDialogs")
}

func (l localDialog) DialogGetDialogById(ctx context.Context, in *dialogpb.TLDialogGetDialogById) (*dialogpb.DialogExt, error) {
	return nil, errNotOnUnpinPath("DialogGetDialogById")
}

func (l localDialog) DialogGetTopMessage(ctx context.Context, in *dialogpb.TLDialogGetTopMessage) (*mtproto.Int32, error) {
	return nil, errNotOnUnpinPath("DialogGetTopMessage")
}

func (l localDialog) DialogDeleteDialog(ctx context.Context, in *dialogpb.TLDialogDeleteDialog) (*mtproto.Bool, error) {
	return nil, errNotOnUnpinPath("DialogDeleteDialog")
}

func (l localDialog) DialogGetUserPinnedMessage(ctx context.Context, in *dialogpb.TLDialogGetUserPinnedMessage) (*mtproto.Int32, error) {
	return nil, errNotOnUnpinPath("DialogGetUserPinnedMessage")
}

func (l localDialog) DialogUpdateUserPinnedMessage(ctx context.Context, in *dialogpb.TLDialogUpdateUserPinnedMessage) (*mtproto.Bool, error) {
	return nil, errNotOnUnpinPath("DialogUpdateUserPinnedMessage")
}

func (l localDialog) DialogInsertOrUpdateDialogFilter(ctx context.Context, in *dialogpb.TLDialogInsertOrUpdateDialogFilter) (*mtproto.Bool, error) {
	return nil, errNotOnUnpinPath("DialogInsertOrUpdateDialogFilter")
}

func (l localDialog) DialogDeleteDialogFilter(ctx context.Context, in *dialogpb.TLDialogDeleteDialogFilter) (*mtproto.Bool, error) {
	return nil, errNotOnUnpinPath("DialogDeleteDialogFilter")
}

func (l localDialog) DialogUpdateDialogFiltersOrder(ctx context.Context, in *dialogpb.TLDialogUpdateDialogFiltersOrder) (*mtproto.Bool, error) {
	return nil, errNotOnUnpinPath("DialogUpdateDialogFiltersOrder")
}

func (l localDialog) DialogGetDialogFilters(ctx context.Context, in *dialogpb.TLDialogGetDialogFilters) (*dialogpb.Vector_DialogFilterExt, error) {
	return nil, errNotOnUnpinPath("DialogGetDialogFilters")
}

func (l localDialog) DialogGetDialogFolder(ctx context.Context, in *dialogpb.TLDialogGetDialogFolder) (*dialogpb.Vector_DialogExt, error) {
	return nil, errNotOnUnpinPath("DialogGetDialogFolder")
}

func (l localDialog) DialogEditPeerFolders(ctx context.Context, in *dialogpb.TLDialogEditPeerFolders) (*dialogpb.Vector_DialogPinnedExt, error) {
	return nil, errNotOnUnpinPath("DialogEditPeerFolders")
}

func (l localDialog) DialogGetChannelMessageReadParticipants(ctx context.Context, in *dialogpb.TLDialogGetChannelMessageReadParticipants) (*dialogpb.Vector_Long, error) {
	return nil, errNotOnUnpinPath("DialogGetChannelMessageReadParticipants")
}

func (l localDialog) DialogSetChatTheme(ctx context.Context, in *dialogpb.TLDialogSetChatTheme) (*mtproto.Bool, error) {
	return nil, errNotOnUnpinPath("DialogSetChatTheme")
}

func (l localDialog) DialogSetHistoryTTL(ctx context.Context, in *dialogpb.TLDialogSetHistoryTTL) (*mtproto.Bool, error) {
	return nil, errNotOnUnpinPath("DialogSetHistoryTTL")
}

func (l localDialog) DialogGetMyDialogsData(ctx context.Context, in *dialogpb.TLDialogGetMyDialogsData) (*dialogpb.DialogsData, error) {
	return nil, errNotOnUnpinPath("DialogGetMyDialogsData")
}

func (l localDialog) DialogGetSavedDialogs(ctx context.Context, in *dialogpb.TLDialogGetSavedDialogs) (*dialogpb.SavedDialogList, error) {
	return nil, errNotOnUnpinPath("DialogGetSavedDialogs")
}

func (l localDialog) DialogGetPinnedSavedDialogs(ctx context.Context, in *dialogpb.TLDialogGetPinnedSavedDialogs) (*dialogpb.SavedDialogList, error) {
	return nil, errNotOnUnpinPath("DialogGetPinnedSavedDialogs")
}

func (l localDialog) DialogToggleSavedDialogPin(ctx context.Context, in *dialogpb.TLDialogToggleSavedDialogPin) (*mtproto.Bool, error) {
	return nil, errNotOnUnpinPath("DialogToggleSavedDialogPin")
}

func (l localDialog) DialogReorderPinnedSavedDialogs(ctx context.Context, in *dialogpb.TLDialogReorderPinnedSavedDialogs) (*mtproto.Bool, error) {
	return nil, errNotOnUnpinPath("DialogReorderPinnedSavedDialogs")
}

func (l localDialog) DialogGetDialogFilter(ctx context.Context, in *dialogpb.TLDialogGetDialogFilter) (*dialogpb.DialogFilterExt, error) {
	return nil, errNotOnUnpinPath("DialogGetDialogFilter")
}

func (l localDialog) DialogGetDialogFilterBySlug(ctx context.Context, in *dialogpb.TLDialogGetDialogFilterBySlug) (*dialogpb.DialogFilterExt, error) {
	return nil, errNotOnUnpinPath("DialogGetDialogFilterBySlug")
}

func (l localDialog) DialogCreateDialogFilter(ctx context.Context, in *dialogpb.TLDialogCreateDialogFilter) (*dialogpb.DialogFilterExt, error) {
	return nil, errNotOnUnpinPath("DialogCreateDialogFilter")
}

func (l localDialog) DialogUpdateUnreadCount(ctx context.Context, in *dialogpb.TLDialogUpdateUnreadCount) (*mtproto.Bool, error) {
	return nil, errNotOnUnpinPath("DialogUpdateUnreadCount")
}

func (l localDialog) DialogToggleDialogFilterTags(ctx context.Context, in *dialogpb.TLDialogToggleDialogFilterTags) (*mtproto.Bool, error) {
	return nil, errNotOnUnpinPath("DialogToggleDialogFilterTags")
}

func (l localDialog) DialogGetDialogFilterTags(ctx context.Context, in *dialogpb.TLDialogGetDialogFilterTags) (*mtproto.Bool, error) {
	return nil, errNotOnUnpinPath("DialogGetDialogFilterTags")
}

func (l localDialog) DialogSetChatWallpaper(ctx context.Context, in *dialogpb.TLDialogSetChatWallpaper) (*mtproto.Bool, error) {
	return nil, errNotOnUnpinPath("DialogSetChatWallpaper")
}

func (l localInbox) InboxEditUserMessageToInbox(ctx context.Context, in *inboxpb.TLInboxEditUserMessageToInbox) (*mtproto.Void, error) {
	return nil, errNotOnUnpinPath("InboxEditUserMessageToInbox")
}

func (l localInbox) InboxEditChatMessageToInbox(ctx context.Context, in *inboxpb.TLInboxEditChatMessageToInbox) (*mtproto.Void, error) {
	return nil, errNotOnUnpinPath("InboxEditChatMessageToInbox")
}

func (l localInbox) InboxDeleteMessagesToInbox(ctx context.Context, in *inboxpb.TLInboxDeleteMessagesToInbox) (*mtproto.Void, error) {
	return nil, errNotOnUnpinPath("InboxDeleteMessagesToInbox")
}

func (l localInbox) InboxDeleteUserHistoryToInbox(ctx context.Context, in *inboxpb.TLInboxDeleteUserHistoryToInbox) (*mtproto.Void, error) {
	return nil, errNotOnUnpinPath("InboxDeleteUserHistoryToInbox")
}

func (l localInbox) InboxDeleteChatHistoryToInbox(ctx context.Context, in *inboxpb.TLInboxDeleteChatHistoryToInbox) (*mtproto.Void, error) {
	return nil, errNotOnUnpinPath("InboxDeleteChatHistoryToInbox")
}

func (l localInbox) InboxReadUserMediaUnreadToInbox(ctx context.Context, in *inboxpb.TLInboxReadUserMediaUnreadToInbox) (*mtproto.Void, error) {
	return nil, errNotOnUnpinPath("InboxReadUserMediaUnreadToInbox")
}

func (l localInbox) InboxReadChatMediaUnreadToInbox(ctx context.Context, in *inboxpb.TLInboxReadChatMediaUnreadToInbox) (*mtproto.Void, error) {
	return nil, errNotOnUnpinPath("InboxReadChatMediaUnreadToInbox")
}

func (l localInbox) InboxUpdateHistoryReaded(ctx context.Context, in *inboxpb.TLInboxUpdateHistoryReaded) (*mtproto.Void, error) {
	return nil, errNotOnUnpinPath("InboxUpdateHistoryReaded")
}

func (l localInbox) InboxUpdatePinnedMessage(ctx context.Context, in *inboxpb.TLInboxUpdatePinnedMessage) (*mtproto.Void, error) {
	return nil, errNotOnUnpinPath("InboxUpdatePinnedMessage")
}

func (l localInbox) InboxSendUserMessageToInboxV2(ctx context.Context, in *inboxpb.TLInboxSendUserMessageToInboxV2) (*mtproto.Void, error) {
	return nil, errNotOnUnpinPath("InboxSendUserMessageToInboxV2")
}

func (l localInbox) InboxEditMessageToInboxV2(ctx context.Context, in *inboxpb.TLInboxEditMessageToInboxV2) (*mtproto.Void, error) {
	return nil, errNotOnUnpinPath("InboxEditMessageToInboxV2")
}

func (l localInbox) InboxReadInboxHistory(ctx context.Context, in *inboxpb.TLInboxReadInboxHistory) (*mtproto.Void, error) {
	return nil, errNotOnUnpinPath("InboxReadInboxHistory")
}

func (l localInbox) InboxReadOutboxHistory(ctx context.Context, in *inboxpb.TLInboxReadOutboxHistory) (*mtproto.Void, error) {
	return nil, errNotOnUnpinPath("InboxReadOutboxHistory")
}

func (l localInbox) InboxReadMediaUnreadToInboxV2(ctx context.Context, in *inboxpb.TLInboxReadMediaUnreadToInboxV2) (*mtproto.Void, error) {
	return nil, errNotOnUnpinPath("InboxReadMediaUnreadToInboxV2")
}

func (l localInbox) InboxUpdatePinnedMessageV2(ctx context.Context, in *inboxpb.TLInboxUpdatePinnedMessageV2) (*mtproto.Void, error) {
	return nil, errNotOnUnpinPath("InboxUpdatePinnedMessageV2")
}

func (s *localSync) SyncUpdatesMe(ctx context.Context, in *syncpb.TLSyncUpdatesMe) (*mtproto.Void, error) {
	return nil, errNotOnUnpinPath("SyncUpdatesMe")
}

func (s *localSync) SyncPushUpdatesIfNot(ctx context.Context, in *syncpb.TLSyncPushUpdatesIfNot) (*mtproto.Void, error) {
	return nil, errNotOnUnpinPath("SyncPushUpdatesIfNot")
}

func (s *localSync) SyncPushBotUpdates(ctx context.Context, in *syncpb.TLSyncPushBotUpdates) (*mtproto.Void, error) {
	return nil, errNotOnUnpinPath("SyncPushBotUpdates")
}

func (s *localSync) SyncPushRpcResult(ctx context.Context, in *syncpb.TLSyncPushRpcResult) (*mtproto.Void, error) {
	return nil, errNotOnUnpinPath("SyncPushRpcResult")
}

func (s *localSync) SyncBroadcastUpdates(ctx context.Context, in *syncpb.TLSyncBroadcastUpdates) (*mtproto.Void, error) {
	return nil, errNotOnUnpinPath("SyncBroadcastUpdates")
}

func (s seqClient) IdgenNextId(ctx context.Context, in *idgenpb.TLIdgenNextId) (*mtproto.Int64, error) {
	return nil, errNotOnUnpinPath("IdgenNextId")
}

func (s seqClient) IdgenNextIds(ctx context.Context, in *idgenpb.TLIdgenNextIds) (*idgenpb.Vector_Long, error) {
	return nil, errNotOnUnpinPath("IdgenNextIds")
}

func (s seqClient) IdgenGetCurrentSeqId(ctx context.Context, in *idgenpb.TLIdgenGetCurrentSeqId) (*mtproto.Int64, error) {
	return nil, errNotOnUnpinPath("IdgenGetCurrentSeqId")
}

func (s seqClient) IdgenSetCurrentSeqId(ctx context.Context, in *idgenpb.TLIdgenSetCurrentSeqId) (*mtproto.Bool, error) {
	return nil, errNotOnUnpinPath("IdgenSetCurrentSeqId")
}

func (s seqClient) IdgenGetNextSeqId(ctx context.Context, in *idgenpb.TLIdgenGetNextSeqId) (*mtproto.Int64, error) {
	return nil, errNotOnUnpinPath("IdgenGetNextSeqId")
}

func (s seqClient) IdgenGetNextIdValList(ctx context.Context, in *idgenpb.TLIdgenGetNextIdValList) (*idgenpb.Vector_IdVal, error) {
	return nil, errNotOnUnpinPath("IdgenGetNextIdValList")
}

func (s seqClient) IdgenGetCurrentSeqIdList(ctx context.Context, in *idgenpb.TLIdgenGetCurrentSeqIdList) (*idgenpb.Vector_IdVal, error) {
	return nil, errNotOnUnpinPath("IdgenGetCurrentSeqIdList")
}
