package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
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
	return mtproto.MakeTLUser(&mtproto.User{Id: id}).To_User(), nil
}

func actor(ctx context.Context) (int64, int64) {
	md := metadata.RpcMetadataFromIncoming(ctx)
	if md == nil {
		return 0, 0
	}
	return md.GetUserId(), md.GetPermAuthKeyId()
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
	return listTopics(ctx, in.GetPeer(), in.GetQ().GetValue(), in.GetLimit())
}

func (ForumsService) ChannelsGetForumTopics(ctx context.Context, in *mtproto.TLChannelsGetForumTopics) (*mtproto.Messages_ForumTopics, error) {
	self, _ := actor(ctx)
	topics, chat, err := channel.Shared().ListTopics(ctx, self, in.GetChannel().GetChannelId(), in.GetQ().GetValue(), in.GetLimit())
	if err != nil {
		return nil, err
	}
	return topicsResult(topics, chat), nil
}

func (ForumsService) MessagesGetForumTopicsByID(ctx context.Context, in *mtproto.TLMessagesGetForumTopicsByID) (*mtproto.Messages_ForumTopics, error) {
	res, err := listTopics(ctx, in.GetPeer(), "", 100)
	if err != nil {
		return nil, err
	}
	want := map[int32]bool{}
	for _, id := range in.GetTopics() {
		want[id] = true
	}
	var kept []*mtproto.ForumTopic
	for _, t := range res.Topics {
		if want[t.GetId()] {
			kept = append(kept, t)
		}
	}
	res.Topics = kept
	res.Count = int32(len(kept))
	return res, nil
}

func (ForumsService) ChannelsGetForumTopicsByID(ctx context.Context, in *mtproto.TLChannelsGetForumTopicsByID) (*mtproto.Messages_ForumTopics, error) {
	self, _ := actor(ctx)
	topics, chat, err := channel.Shared().ListTopics(ctx, self, in.GetChannel().GetChannelId(), "", 100)
	if err != nil {
		return nil, err
	}
	want := map[int32]bool{}
	for _, id := range in.GetTopics() {
		want[id] = true
	}
	var kept []*mtproto.ForumTopic
	for _, t := range topics {
		if want[t.GetId()] {
			kept = append(kept, t)
		}
	}
	return topicsResult(kept, chat), nil
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
	return editTopic(ctx, channelID(in.GetPeer(), self), in.GetTopicId(), in.GetTitle().GetValue(), in.GetClosed())
}

func (ForumsService) ChannelsEditForumTopic(ctx context.Context, in *mtproto.TLChannelsEditForumTopic) (*mtproto.Updates, error) {
	return editTopic(ctx, in.GetChannel().GetChannelId(), in.GetTopicId(), in.GetTitle().GetValue(), in.GetClosed())
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
	pts, count, err := channel.Shared().DeleteHistory(ctx, self, auth, &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: id}, 0, false)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessagesAffectedHistory(&mtproto.Messages_AffectedHistory{Pts: pts, PtsCount: count}).To_Messages_AffectedHistory(), nil
}

func (ForumsService) ChannelsDeleteTopicHistory(ctx context.Context, in *mtproto.TLChannelsDeleteTopicHistory) (*mtproto.Messages_AffectedHistory, error) {
	self, auth := actor(ctx)
	pts, count, err := channel.Shared().DeleteHistory(ctx, self, auth, in.GetChannel(), 0, false)
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
	for i, topicID := range in.GetOrder() {
		_ = channel.Shared().PinTopic(ctx, self, id, topicID, i == 0)
	}
	return mtproto.MakeUpdatesByUpdates(channelUpdate(id)), nil
}

func (ForumsService) ChannelsReorderPinnedForumTopics(ctx context.Context, in *mtproto.TLChannelsReorderPinnedForumTopics) (*mtproto.Updates, error) {
	self, _ := actor(ctx)
	for i, topicID := range in.GetOrder() {
		_ = channel.Shared().PinTopic(ctx, self, in.GetChannel().GetChannelId(), topicID, true)
		_ = i
	}
	return mtproto.MakeUpdatesByUpdates(channelUpdate(in.GetChannel().GetChannelId())), nil
}

func listTopics(ctx context.Context, peer *mtproto.InputPeer, q string, limit int32) (*mtproto.Messages_ForumTopics, error) {
	self, _ := actor(ctx)
	topics, chat, err := channel.Shared().ListTopics(ctx, self, channelID(peer, self), q, limit)
	if err != nil {
		return nil, err
	}
	return topicsResult(topics, chat), nil
}

func topicsResult(topics []*mtproto.ForumTopic, chat *mtproto.Chat) *mtproto.Messages_ForumTopics {
	if topics == nil {
		topics = []*mtproto.ForumTopic{}
	}
	chats := []*mtproto.Chat{}
	if chat != nil {
		chats = []*mtproto.Chat{chat}
	}
	return mtproto.MakeTLMessagesForumTopics(&mtproto.Messages_ForumTopics{
		Count:    int32(len(topics)),
		Topics:   topics,
		Messages: []*mtproto.Message{},
		Chats:    chats,
		Users:    []*mtproto.User{},
	}).To_Messages_ForumTopics()
}

func editTopic(ctx context.Context, channelID int64, topicID int32, title string, closed *mtproto.Bool) (*mtproto.Updates, error) {
	self, _ := actor(ctx)
	var closedPtr *bool
	if closed != nil {
		v := mtproto.FromBool(closed)
		closedPtr = &v
	}
	if _, err := channel.Shared().EditTopic(ctx, self, channelID, topicID, title, closedPtr); err != nil {
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

// ReactionsService stores emoji reactions on channel messages.
type ReactionsService struct {
	mtproto.UnimplementedRPCReactionsServer
}

func (ReactionsService) MessagesSendReaction(ctx context.Context, in *mtproto.TLMessagesSendReaction) (*mtproto.Updates, error) {
	self, _ := actor(ctx)
	var emos []string
	for _, r := range in.GetReaction_FLAGVECTORREACTION() {
		if r.GetEmoticon() != "" {
			emos = append(emos, r.GetEmoticon())
		}
	}
	return channel.Shared().React(ctx, self, channelID(in.GetPeer(), self), in.GetMsgId(), emos, in.GetBig())
}

func (ReactionsService) MessagesGetMessagesReactions(ctx context.Context, in *mtproto.TLMessagesGetMessagesReactions) (*mtproto.Updates, error) {
	self, _ := actor(ctx)
	return channel.Shared().ReactionsFor(ctx, self, channelID(in.GetPeer(), self), in.GetId())
}

func (ReactionsService) MessagesGetMessageReactionsList(ctx context.Context, in *mtproto.TLMessagesGetMessageReactionsList) (*mtproto.Messages_MessageReactionsList, error) {
	self, _ := actor(ctx)
	up, err := channel.Shared().ReactionsFor(ctx, self, channelID(in.GetPeer(), self), []int32{in.GetId()})
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessagesMessageReactionsList(&mtproto.Messages_MessageReactionsList{
		Count:     int32(len(up.Updates)),
		Reactions: []*mtproto.MessagePeerReaction{},
		Chats:     up.Chats,
		Users:     []*mtproto.User{},
	}).To_Messages_MessageReactionsList(), nil
}

// ThreadsService serves discussion replies for channel posts.
type ThreadsService struct {
	mtproto.UnimplementedRPCMessageThreadsServer
}

func (ThreadsService) MessagesGetReplies(ctx context.Context, in *mtproto.TLMessagesGetReplies) (*mtproto.Messages_Messages, error) {
	self, _ := actor(ctx)
	return channel.Shared().Replies(ctx, self, channelID(in.GetPeer(), self), in.GetMsgId(), in.GetOffsetId(), in.GetLimit())
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
