package service

import (
	"context"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"

	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/pkg/premium"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func (s *Service) MessagesGetPersonalChannelHistory(ctx context.Context, in *mtproto.TLMessagesGetPersonalChannelHistory) (*mtproto.Messages_Messages, error) {
	md := s.md(ctx)
	userID := md.GetUserId()
	if in.GetUserId().GetPredicateName() == mtproto.Predicate_inputUser {
		userID = in.GetUserId().GetUserId()
	}
	return channel.Shared().PersonalHistory(ctx, md.GetUserId(), userID, in.GetLimit(), in.GetMaxId(), in.GetMinId())
}

func (s *Service) ChannelsGetAdminLog(ctx context.Context, in *mtproto.TLChannelsGetAdminLog) (*mtproto.Channels_AdminLogResults, error) {
	var admins []int64
	for _, u := range in.GetAdmins() {
		if u.GetUserId() != 0 {
			admins = append(admins, u.GetUserId())
		}
	}
	return channel.Shared().GetAdminLog(ctx, s.md(ctx).GetUserId(), in.GetChannel(), in.GetQ(), in.GetEventsFilter(), admins, in.GetMaxId(), in.GetMinId(), in.GetLimit())
}

func (s *Service) ChannelsSetStickers(ctx context.Context, in *mtproto.TLChannelsSetStickers) (*mtproto.Bool, error) {
	return channel.Shared().SetStickers(ctx, s.md(ctx).GetUserId(), in.GetChannel(), in.GetStickerset())
}

func (s *Service) ChannelsReadMessageContents(ctx context.Context, in *mtproto.TLChannelsReadMessageContents) (*mtproto.Bool, error) {
	if err := channel.Shared().ReadContents(ctx, s.md(ctx).GetUserId(), in.GetChannel(), in.GetId()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) ChannelsDeleteHistory9BAA9647(ctx context.Context, in *mtproto.TLChannelsDeleteHistory9BAA9647) (*mtproto.Updates, error) {
	md := s.md(ctx)
	pts, count, err := channel.Shared().DeleteHistory(ctx, md.GetUserId(), md.GetPermAuthKeyId(), in.GetChannel(), in.GetMaxId(), in.GetForEveryone())
	if err != nil {
		return nil, err
	}
	return mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateDeleteChannelMessages(&mtproto.Update{
		ChannelId: in.GetChannel().GetChannelId(),
		Pts_INT32: pts,
		PtsCount:  count,
	}).To_Update()), nil
}

func (s *Service) ChannelsDeleteHistoryAF369D42(ctx context.Context, in *mtproto.TLChannelsDeleteHistoryAF369D42) (*mtproto.Bool, error) {
	md := s.md(ctx)
	if _, _, err := channel.Shared().DeleteHistory(ctx, md.GetUserId(), md.GetPermAuthKeyId(), in.GetChannel(), in.GetMaxId(), true); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) ChannelsTogglePreHistoryHidden(ctx context.Context, in *mtproto.TLChannelsTogglePreHistoryHidden) (*mtproto.Updates, error) {
	md := s.md(ctx)
	return channel.Shared().TogglePreHistory(ctx, md.GetUserId(), md.GetPermAuthKeyId(), in.GetChannel(), mtproto.FromBool(in.GetEnabled()))
}

func (s *Service) ChannelsGetGroupsForDiscussion(ctx context.Context, in *mtproto.TLChannelsGetGroupsForDiscussion) (*mtproto.Messages_Chats, error) {
	return channel.Shared().GroupsForDiscussion(ctx, s.md(ctx).GetUserId())
}

func (s *Service) ChannelsSetDiscussionGroup(ctx context.Context, in *mtproto.TLChannelsSetDiscussionGroup) (*mtproto.Bool, error) {
	if err := channel.Shared().SetDiscussionGroup(ctx, s.md(ctx).GetUserId(), in.GetBroadcast(), in.GetGroup()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) ChannelsEditLocation(ctx context.Context, in *mtproto.TLChannelsEditLocation) (*mtproto.Bool, error) {
	pt := in.GetGeoPoint()
	if err := channel.Shared().EditLocation(ctx, s.md(ctx).GetUserId(), in.GetChannel(), pt.GetLat(), pt.GetLong(), in.GetAddress()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) ChannelsToggleSlowMode(ctx context.Context, in *mtproto.TLChannelsToggleSlowMode) (*mtproto.Updates, error) {
	md := s.md(ctx)
	return channel.Shared().ToggleSlowMode(ctx, md.GetUserId(), md.GetPermAuthKeyId(), in.GetChannel(), in.GetSeconds())
}

func (s *Service) ChannelsGetInactiveChannels(ctx context.Context, in *mtproto.TLChannelsGetInactiveChannels) (*mtproto.Messages_InactiveChats, error) {
	return channel.Shared().InactiveChannels(ctx, s.md(ctx).GetUserId())
}

func (s *Service) ChannelsDeleteParticipantHistory(ctx context.Context, in *mtproto.TLChannelsDeleteParticipantHistory) (*mtproto.Messages_AffectedHistory, error) {
	md := s.md(ctx)
	peer := mtproto.FromInputPeer2(md.GetUserId(), in.GetParticipant())
	pts, count, offset, err := channel.Shared().DeleteParticipantHistory(ctx, md.GetUserId(), md.GetPermAuthKeyId(), in.GetChannel(), peer.PeerId)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessagesAffectedHistory(&mtproto.Messages_AffectedHistory{
		Pts: pts, PtsCount: count, Offset: offset,
	}).To_Messages_AffectedHistory(), nil
}

func (s *Service) ChannelsToggleParticipantsHidden(ctx context.Context, in *mtproto.TLChannelsToggleParticipantsHidden) (*mtproto.Updates, error) {
	md := s.md(ctx)
	return channel.Shared().ToggleParticipantsHidden(ctx, md.GetUserId(), md.GetPermAuthKeyId(), in.GetChannel(), mtproto.FromBool(in.GetEnabled()))
}

func (s *Service) ChannelsEditCreator(ctx context.Context, in *mtproto.TLChannelsEditCreator) (*mtproto.Updates, error) {
	md := s.md(ctx)
	userID := in.GetUserId().GetUserId()
	return channel.Shared().EditCreator(ctx, md.GetUserId(), md.GetPermAuthKeyId(), in.GetChannel(), userID, in.GetPassword())
}

func (s *Service) ChannelsGetFutureCreatorAfterLeave(ctx context.Context, in *mtproto.TLChannelsGetFutureCreatorAfterLeave) (*mtproto.User, error) {
	md := s.md(ctx)
	id, err := channel.Shared().FutureCreator(ctx, md.GetUserId(), in.GetChannel())
	if err != nil {
		return nil, err
	}
	users := s.users(ctx, md.GetUserId(), id)
	if len(users) > 0 {
		return users[0], nil
	}
	return mtproto.MakeTLUser(&mtproto.User{
		Id:      id,
		Premium: premium.IsMember(false, 0, time.Now().Unix()),
	}).To_User(), nil
}

func actor(ctx context.Context) (int64, int64) {
	md := metadata.RpcMetadataFromIncoming(ctx)
	if md == nil {
		return 0, 0
	}
	return md.GetUserId(), md.GetPermAuthKeyId()
}

func langCode(ctx context.Context) string {
	md := metadata.RpcMetadataFromIncoming(ctx)
	if md == nil {
		return ""
	}
	return md.GetLangCode()
}

func channelID(peer *mtproto.InputPeer, self int64) int64 {
	p := mtproto.FromInputPeer2(self, peer)
	return p.PeerId
}

// ForumsService implements forum topic RPCs for channels that have the forum flag.
type ForumsService struct {
	mtproto.UnimplementedRPCForumsServer
}

func (ForumsService) MessagesGetForumTopics(ctx context.Context, in *mtproto.TLMessagesGetForumTopics) (*mtproto.Messages_ForumTopics, error) {
	return listTopics(ctx, in.GetPeer(), in.GetQ().GetValue(), in.GetLimit(), in.GetOffsetDate(), in.GetOffsetId(), in.GetOffsetTopic())
}

func (ForumsService) ChannelsGetForumTopics(ctx context.Context, in *mtproto.TLChannelsGetForumTopics) (*mtproto.Messages_ForumTopics, error) {
	self, _ := actor(ctx)
	id := in.GetChannel().GetChannelId()
	topics, chat, err := channel.Shared().ListTopics(ctx, self, id, in.GetQ().GetValue(), in.GetLimit(), in.GetOffsetDate(), in.GetOffsetId(), in.GetOffsetTopic())
	if err != nil {
		return nil, err
	}
	return topicsResult(ctx, id, topics, chat), nil
}

func (ForumsService) MessagesGetForumTopicsByID(ctx context.Context, in *mtproto.TLMessagesGetForumTopicsByID) (*mtproto.Messages_ForumTopics, error) {
	res, err := listTopics(ctx, in.GetPeer(), "", 100, 0, 0, 0)
	if err != nil {
		return nil, err
	}
	res.Topics = topicsByID(res.Topics, in.GetTopics())
	res.Count = int32(len(res.Topics))
	return res, nil
}

func (ForumsService) ChannelsGetForumTopicsByID(ctx context.Context, in *mtproto.TLChannelsGetForumTopicsByID) (*mtproto.Messages_ForumTopics, error) {
	self, _ := actor(ctx)
	topics, chat, err := channel.Shared().ListTopics(ctx, self, in.GetChannel().GetChannelId(), "", 100, 0, 0, 0)
	if err != nil {
		return nil, err
	}
	return topicsResult(ctx, in.GetChannel().GetChannelId(), topicsByID(topics, in.GetTopics()), chat), nil
}

func topicsByID(topics []*mtproto.ForumTopic, ids []int32) []*mtproto.ForumTopic {
	byID := map[int32]*mtproto.ForumTopic{}
	for _, t := range topics {
		byID[t.GetId()] = t
	}
	out := make([]*mtproto.ForumTopic, 0, len(ids))
	for _, id := range ids {
		if t, ok := byID[id]; ok {
			out = append(out, t)
			continue
		}
		out = append(out, mtproto.MakeTLForumTopicDeleted(&mtproto.ForumTopic{Id: id}).To_ForumTopic())
	}
	return out
}

func (ForumsService) MessagesCreateForumTopic(ctx context.Context, in *mtproto.TLMessagesCreateForumTopic) (*mtproto.Updates, error) {
	self, auth := actor(ctx)
	up, _, err := channel.Shared().CreateTopic(ctx, self, auth, channelID(in.GetPeer(), self), in.GetTitle(), in.GetIconColor().GetValue(), in.GetIconEmojiId().GetValue())
	return up, err
}

func (ForumsService) ChannelsCreateForumTopic(ctx context.Context, in *mtproto.TLChannelsCreateForumTopic) (*mtproto.Updates, error) {
	self, auth := actor(ctx)
	up, _, err := channel.Shared().CreateTopic(ctx, self, auth, in.GetChannel().GetChannelId(), in.GetTitle(), in.GetIconColor().GetValue(), in.GetIconEmojiId().GetValue())
	return up, err
}

func (ForumsService) MessagesEditForumTopic(ctx context.Context, in *mtproto.TLMessagesEditForumTopic) (*mtproto.Updates, error) {
	self, _ := actor(ctx)
	return editTopic(ctx, channelID(in.GetPeer(), self), in.GetTopicId(), in.GetTitle().GetValue(), in.GetClosed(), in.GetIconEmojiId(), in.GetHidden())
}

func (ForumsService) ChannelsEditForumTopic(ctx context.Context, in *mtproto.TLChannelsEditForumTopic) (*mtproto.Updates, error) {
	return editTopic(ctx, in.GetChannel().GetChannelId(), in.GetTopicId(), in.GetTitle().GetValue(), in.GetClosed(), in.GetIconEmojiId(), in.GetHidden())
}

func (ForumsService) MessagesUpdatePinnedForumTopic(ctx context.Context, in *mtproto.TLMessagesUpdatePinnedForumTopic) (*mtproto.Updates, error) {
	self, _ := actor(ctx)
	return pinTopic(ctx, channelID(in.GetPeer(), self), in.GetTopicId(), mtproto.FromBool(in.GetPinned()))
}

func (ForumsService) ChannelsUpdatePinnedForumTopic(ctx context.Context, in *mtproto.TLChannelsUpdatePinnedForumTopic) (*mtproto.Updates, error) {
	return pinTopic(ctx, in.GetChannel().GetChannelId(), in.GetTopicId(), mtproto.FromBool(in.GetPinned()))
}

func (ForumsService) MessagesDeleteTopicHistory(ctx context.Context, in *mtproto.TLMessagesDeleteTopicHistory) (*mtproto.Messages_AffectedHistory, error) {
	self, auth := actor(ctx)
	id := channelID(in.GetPeer(), self)
	pts, count, err := channel.Shared().DeleteTopicHistory(ctx, self, auth, id, in.GetTopMsgId())
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessagesAffectedHistory(&mtproto.Messages_AffectedHistory{Pts: pts, PtsCount: count}).To_Messages_AffectedHistory(), nil
}

func (ForumsService) ChannelsDeleteTopicHistory(ctx context.Context, in *mtproto.TLChannelsDeleteTopicHistory) (*mtproto.Messages_AffectedHistory, error) {
	self, auth := actor(ctx)
	pts, count, err := channel.Shared().DeleteTopicHistory(ctx, self, auth, in.GetChannel().GetChannelId(), in.GetTopMsgId())
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessagesAffectedHistory(&mtproto.Messages_AffectedHistory{Pts: pts, PtsCount: count}).To_Messages_AffectedHistory(), nil
}

func (ForumsService) ChannelsToggleForum(ctx context.Context, in *mtproto.TLChannelsToggleForum) (*mtproto.Updates, error) {
	self, auth := actor(ctx)
	_ = auth
	return channel.Shared().ToggleForum(ctx, self, in.GetChannel(), mtproto.FromBool(in.GetEnabled()))
}

func (ForumsService) MessagesReorderPinnedForumTopics(ctx context.Context, in *mtproto.TLMessagesReorderPinnedForumTopics) (*mtproto.Updates, error) {
	self, _ := actor(ctx)
	id := channelID(in.GetPeer(), self)
	if err := channel.Shared().ReorderPinnedTopics(ctx, self, id, in.GetOrder(), in.GetForce()); err != nil {
		return nil, err
	}
	return mtproto.MakeUpdatesByUpdates(channelUpdate(id)), nil
}

func (ForumsService) ChannelsReorderPinnedForumTopics(ctx context.Context, in *mtproto.TLChannelsReorderPinnedForumTopics) (*mtproto.Updates, error) {
	self, _ := actor(ctx)
	id := in.GetChannel().GetChannelId()
	if err := channel.Shared().ReorderPinnedTopics(ctx, self, id, in.GetOrder(), in.GetForce()); err != nil {
		return nil, err
	}
	return mtproto.MakeUpdatesByUpdates(channelUpdate(id)), nil
}

func listTopics(ctx context.Context, peer *mtproto.InputPeer, q string, limit, offsetDate, offsetID, offsetTopic int32) (*mtproto.Messages_ForumTopics, error) {
	self, _ := actor(ctx)
	id := channelID(peer, self)
	topics, chat, err := channel.Shared().ListTopics(ctx, self, id, q, limit, offsetDate, offsetID, offsetTopic)
	if err != nil {
		return nil, err
	}
	return topicsResult(ctx, id, topics, chat), nil
}

func topicsResult(ctx context.Context, channelID int64, topics []*mtproto.ForumTopic, chat *mtproto.Chat) *mtproto.Messages_ForumTopics {
	if topics == nil {
		topics = []*mtproto.ForumTopic{}
	}
	chats := []*mtproto.Chat{}
	if chat != nil {
		chats = []*mtproto.Chat{chat}
	}
	var msgs []*mtproto.Message
	for _, t := range topics {
		if t.GetTopMessage() == 0 {
			continue
		}
		m, err := channel.Shared().LoadMessage(ctx, channelID, t.GetTopMessage())
		if err != nil || m == nil {
			continue
		}
		msgs = append(msgs, m)
	}
	if msgs == nil {
		msgs = []*mtproto.Message{}
	}
	return mtproto.MakeTLMessagesForumTopics(&mtproto.Messages_ForumTopics{
		Count:    int32(len(topics)),
		Topics:   topics,
		Messages: msgs,
		Chats:    chats,
		Users:    []*mtproto.User{},
	}).To_Messages_ForumTopics()
}

func editTopic(ctx context.Context, channelID int64, topicID int32, title string, closed *mtproto.Bool, icon *wrapperspb.Int64Value, hidden *mtproto.Bool) (*mtproto.Updates, error) {
	self, _ := actor(ctx)
	var closedPtr *bool
	if closed != nil {
		v := mtproto.FromBool(closed)
		closedPtr = &v
	}
	var hiddenPtr *bool
	if hidden != nil {
		v := mtproto.FromBool(hidden)
		hiddenPtr = &v
	}
	var iconPtr *int64
	if icon != nil {
		v := icon.GetValue()
		iconPtr = &v
	}
	if _, err := channel.Shared().EditTopic(ctx, self, channelID, topicID, title, closedPtr, iconPtr, hiddenPtr); err != nil {
		return nil, err
	}
	return mtproto.MakeUpdatesByUpdates(channelUpdate(channelID)), nil
}

func pinTopic(ctx context.Context, channelID int64, topicID int32, pinned bool) (*mtproto.Updates, error) {
	self, _ := actor(ctx)
	if err := channel.Shared().PinTopic(ctx, self, channelID, topicID, pinned); err != nil {
		return nil, err
	}
	return mtproto.MakeUpdatesByUpdates(channelUpdate(channelID)), nil
}

func channelUpdate(id int64) *mtproto.Update {
	return mtproto.MakeTLUpdateChannel(&mtproto.Update{ChannelId: id}).To_Update()
}

// ReactionsService stores emoji reactions on messages.
type ReactionsService struct {
	mtproto.UnimplementedRPCReactionsServer
	Users    reactionUserAPI
	Messages reactionMessageAPI
	// Push delivers updates to other users. Nil in tests and when sync is not configured.
	Push reactionPusher
	// Dialogs updates dialog.unread_reactions_count. Nil in tests.
	Dialogs reactionDialogs
	// Chats lists basic-group members for updateMessageReactions. Nil in tests.
	Chats interface {
		ChatGetChatParticipantIdList(ctx context.Context, in *chatpb.TLChatGetChatParticipantIdList) (*chatpb.Vector_Long, error)
	}
}

// reactionPusher sends updates to one user's sessions.
type reactionPusher interface {
	SyncPushUpdates(ctx context.Context, in *sync.TLSyncPushUpdates) (*mtproto.Void, error)
	SyncUpdatesNotMe(ctx context.Context, in *sync.TLSyncUpdatesNotMe) (*mtproto.Void, error)
}

// reactionDialogs adjusts the chat-list reaction badge.
type reactionDialogs interface {
	DialogUpdateUnreadCount(ctx context.Context, in *dialog.TLDialogUpdateUnreadCount) (*mtproto.Bool, error)
}

func (ReactionsService) MessagesGetAvailableReactions(ctx context.Context, in *mtproto.TLMessagesGetAvailableReactions) (*mtproto.Messages_AvailableReactions, error) {
	return catalog.Must().Reactions(in.GetHash()), nil
}

func (s ReactionsService) MessagesSendReaction(ctx context.Context, in *mtproto.TLMessagesSendReaction) (*mtproto.Updates, error) {
	self, _ := actor(ctx)
	peer := mtproto.FromInputPeer2(self, in.GetPeer())
	var emos []string
	var docs []int64
	for _, r := range in.GetReaction_FLAGVECTORREACTION() {
		emo, doc, err := chatbox.ParseReaction(r)
		if err != nil {
			return nil, err
		}
		if emo != "" {
			emos = append(emos, emo)
		}
		if doc != 0 {
			docs = append(docs, doc)
		}
	}
	if peer.IsChannel() {
		if len(docs) > 0 {
			return nil, mtproto.ErrReactionInvalid
		}
		up, err := channel.Shared().React(ctx, self, peer.PeerId, in.GetMsgId(), emos, in.GetBig())
		if err != nil {
			return nil, err
		}
		catalog.Must().NoteReaction(self, emos)
		return up, nil
	}
	before := authorReactionUnread(ctx, self, peer, in.GetMsgId())
	groupBefore := s.groupReactionUnread(ctx, self, peer, in.GetMsgId())
	up, err := chatbox.Shared().React(ctx, self, peer, in.GetMsgId(), emos, docs, in.GetBig())
	if err != nil {
		return nil, err
	}
	catalog.Must().NoteReaction(self, emos)
	after := authorReactionUnread(ctx, self, peer, in.GetMsgId())
	s.pushPeerReaction(ctx, self, peer, in.GetMsgId())
	s.pushOwnOtherSessions(ctx, self, up)
	if peer != nil && peer.IsChat() {
		s.noteGroupReactionUnread(ctx, peer, in.GetMsgId(), groupBefore)
	} else {
		s.addUnreadReactions(ctx, reactionAuthor(ctx, self, peer, in.GetMsgId()), mtproto.PEER_USER, self, unreadDelta(before, after))
	}
	return up, nil
}

// groupReactionUnread snapshots which other members already have an unread
// reaction on this message. The snapshot has to be taken before React.
func (s ReactionsService) groupReactionUnread(ctx context.Context, self int64, peer *mtproto.PeerUtil, msgID int32) map[int64]bool {
	out := map[int64]bool{}
	if s.Chats == nil || peer == nil || !peer.IsChat() {
		return out
	}
	list, err := s.Chats.ChatGetChatParticipantIdList(ctx, &chatpb.TLChatGetChatParticipantIdList{ChatId: peer.PeerId})
	if err != nil || list == nil {
		return out
	}
	for _, id := range list.GetDatas() {
		if id == 0 || id == self {
			continue
		}
		out[id] = chatbox.Shared().MessageHasUnreadReaction(ctx, id, peer, msgID)
	}
	return out
}

func (s ReactionsService) noteGroupReactionUnread(ctx context.Context, peer *mtproto.PeerUtil, msgID int32, before map[int64]bool) {
	if peer == nil {
		return
	}
	for id, was := range before {
		now := chatbox.Shared().MessageHasUnreadReaction(ctx, id, peer, msgID)
		s.addUnreadReactions(ctx, id, peer.PeerType, peer.PeerId, unreadDelta(was, now))
	}
}

func unreadDelta(before, after bool) int32 {
	switch {
	case !before && after:
		return 1
	case before && !after:
		return -1
	default:
		return 0
	}
}

func authorReactionUnread(ctx context.Context, reactor int64, peer *mtproto.PeerUtil, msgID int32) bool {
	_, up := chatbox.Shared().PeerReactionUpdate(ctx, reactor, peer, msgID)
	if up == nil {
		return false
	}
	for _, row := range up.GetReactions_MESSAGEREACTIONS().GetRecentReactions() {
		if row.GetUnread() {
			return true
		}
	}
	return false
}

func reactionAuthor(ctx context.Context, reactor int64, peer *mtproto.PeerUtil, msgID int32) int64 {
	to, _ := chatbox.Shared().PeerReactionUpdate(ctx, reactor, peer, msgID)
	return to
}

func (ReactionsService) MessagesGetTopReactions(ctx context.Context, in *mtproto.TLMessagesGetTopReactions) (*mtproto.Messages_Reactions, error) {
	return catalog.Must().TopReactions(in.GetLimit(), in.GetHash()), nil
}

func (ReactionsService) MessagesGetRecentReactions(ctx context.Context, in *mtproto.TLMessagesGetRecentReactions) (*mtproto.Messages_Reactions, error) {
	self, _ := actor(ctx)
	return catalog.Must().RecentReactions(self, in.GetLimit(), in.GetHash()), nil
}

func (ReactionsService) MessagesClearRecentReactions(ctx context.Context, in *mtproto.TLMessagesClearRecentReactions) (*mtproto.Bool, error) {
	self, _ := actor(ctx)
	catalog.Must().ClearRecentReactions(self)
	return mtproto.BoolTrue, nil
}

func (s ReactionsService) MessagesGetMessagesReactions(ctx context.Context, in *mtproto.TLMessagesGetMessagesReactions) (*mtproto.Updates, error) {
	return s.messagesReactions(ctx, in)
}

func (s ReactionsService) MessagesGetMessageReactionsList(ctx context.Context, in *mtproto.TLMessagesGetMessageReactionsList) (*mtproto.Messages_MessageReactionsList, error) {
	return s.messageReactionsList(ctx, in)
}

// ThreadsService serves discussion replies for channel posts and
// contacts.blockFromReplies (the @replies inbox lives on this RPC service).
type ThreadsService struct {
	mtproto.UnimplementedRPCMessageThreadsServer
	Users    repliesUsers
	Messages repliesMessages
	Outbox   repliesOutbox
	Sync     repliesSync
}

func (ThreadsService) MessagesGetReplies(ctx context.Context, in *mtproto.TLMessagesGetReplies) (*mtproto.Messages_Messages, error) {
	self, _ := actor(ctx)
	return channel.Shared().Replies(ctx, self, channelID(in.GetPeer(), self), in.GetMsgId(), channel.HistoryQuery{
		OffsetID:   in.GetOffsetId(),
		OffsetDate: in.GetOffsetDate(),
		AddOffset:  in.GetAddOffset(),
		Limit:      in.GetLimit(),
		MaxID:      in.GetMaxId(),
		MinID:      in.GetMinId(),
	})
}

func (ThreadsService) MessagesGetDiscussionMessage(ctx context.Context, in *mtproto.TLMessagesGetDiscussionMessage) (*mtproto.Messages_DiscussionMessage, error) {
	self, _ := actor(ctx)
	return channel.Shared().DiscussionMessage(ctx, self, channelID(in.GetPeer(), self), in.GetMsgId())
}

func (ThreadsService) MessagesReadDiscussion(ctx context.Context, in *mtproto.TLMessagesReadDiscussion) (*mtproto.Bool, error) {
	self, _ := actor(ctx)
	peer := mtproto.FromInputPeer2(self, in.GetPeer())
	if err := channel.Shared().ReadHistory(ctx, self, &mtproto.InputChannel{
		PredicateName: mtproto.Predicate_inputChannel,
		ChannelId:     peer.PeerId,
	}, in.GetReadMaxId()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

// StatsService reports follower and message counts for channels the caller admins.
type StatsService struct {
	mtproto.UnimplementedRPCStatisticsServer
}

func (StatsService) StatsGetBroadcastStats(ctx context.Context, in *mtproto.TLStatsGetBroadcastStats) (*mtproto.Stats_BroadcastStats, error) {
	self, _ := actor(ctx)
	return channel.Shared().BroadcastStats(ctx, self, in.GetChannel())
}

func (StatsService) StatsGetMegagroupStats(ctx context.Context, in *mtproto.TLStatsGetMegagroupStats) (*mtproto.Stats_MegagroupStats, error) {
	self, _ := actor(ctx)
	return channel.Shared().MegagroupStats(ctx, self, in.GetChannel())
}

// pushPeerReaction tells the other side of a private chat that reactions on
// the message changed, using that user's own message id.
func (s ReactionsService) pushPeerReaction(ctx context.Context, self int64, peer *mtproto.PeerUtil, msgID int32) {
	if s.Push == nil {
		return
	}
	if peer != nil && peer.IsChat() {
		s.pushGroupReactions(ctx, self, peer, msgID)
		return
	}
	to, up := chatbox.Shared().PeerReactionUpdate(ctx, self, peer, msgID)
	if to == 0 || up == nil {
		return
	}
	_, _ = s.Push.SyncPushUpdates(ctx, &sync.TLSyncPushUpdates{
		UserId:  to,
		Updates: mtproto.MakeUpdatesByUpdates(up),
	})
}

func (s ReactionsService) pushGroupReactions(ctx context.Context, self int64, peer *mtproto.PeerUtil, msgID int32) {
	if s.Chats == nil || peer == nil {
		return
	}
	list, err := s.Chats.ChatGetChatParticipantIdList(ctx, &chatpb.TLChatGetChatParticipantIdList{ChatId: peer.PeerId})
	if err != nil || list == nil {
		return
	}
	for _, id := range list.GetDatas() {
		if id == 0 || id == self {
			continue
		}
		up := chatbox.Shared().GroupReactionUpdateFor(ctx, id, peer, msgID)
		if up == nil {
			continue
		}
		_, _ = s.Push.SyncPushUpdates(ctx, &sync.TLSyncPushUpdates{
			UserId:  id,
			Updates: mtproto.MakeUpdatesByUpdates(up),
		})
	}
}

func (s ReactionsService) pushOwnOtherSessions(ctx context.Context, self int64, up *mtproto.Updates) {
	if s.Push == nil || self == 0 || up == nil {
		return
	}
	_, authKey := actor(ctx)
	if authKey == 0 {
		return
	}
	_, _ = s.Push.SyncUpdatesNotMe(ctx, &sync.TLSyncUpdatesNotMe{
		UserId:        self,
		PermAuthKeyId: authKey,
		Updates:       up,
	})
}

func (s ReactionsService) addUnreadReactions(ctx context.Context, userID int64, peerType int32, peerID int64, delta int32) {
	if s.Dialogs == nil || userID == 0 || peerID == 0 || delta == 0 {
		return
	}
	_, _ = s.Dialogs.DialogUpdateUnreadCount(ctx, &dialog.TLDialogUpdateUnreadCount{
		UserId:               userID,
		PeerType:             peerType,
		PeerId:               peerID,
		UnreadReactionsCount: wrapperspb.Int32(delta),
	})
}
