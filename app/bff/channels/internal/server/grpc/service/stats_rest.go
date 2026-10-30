package service

import (
	"bytes"
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"github.com/teamgram/teamgram-server/app/service/biz/story"
)

func (StatsService) StatsLoadAsyncGraph(ctx context.Context, in *mtproto.TLStatsLoadAsyncGraph) (*mtproto.StatsGraph, error) {
	self, _ := actor(ctx)
	if self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrGraphInvalidReload
	}
	_ = in.GetX()
	return channel.ReadGraph(in.GetToken())
}

func (StatsService) StatsGetMessageStats(ctx context.Context, in *mtproto.TLStatsGetMessageStats) (*mtproto.Stats_MessageStats, error) {
	self, err := statsUser(ctx)
	if err != nil {
		return nil, err
	}
	return channel.Shared().MessageStats(ctx, self, in.GetChannel(), in.GetMsgId())
}

func (StatsService) StatsGetMessagePublicForwards5F150144(ctx context.Context, in *mtproto.TLStatsGetMessagePublicForwards5F150144) (*mtproto.Stats_PublicForwards, error) {
	self, err := statsUser(ctx)
	if err != nil {
		return nil, err
	}
	return channel.Shared().MessagePublicForwards(ctx, self, in.GetChannel(), in.GetMsgId(), in.GetOffset(), in.GetLimit())
}

func (StatsService) StatsGetMessagePublicForwards5630281B(ctx context.Context, in *mtproto.TLStatsGetMessagePublicForwards5630281B) (*mtproto.Messages_Messages, error) {
	self, err := statsUser(ctx)
	if err != nil {
		return nil, err
	}
	return channel.Shared().MessagePublicForwardsLegacy(ctx, self, in.GetChannel(), in.GetMsgId(), in.GetLimit())
}

func (StatsService) StatsGetStoryStats(ctx context.Context, in *mtproto.TLStatsGetStoryStats) (*mtproto.Stats_StoryStats, error) {
	self, err := statsUser(ctx)
	if err != nil {
		return nil, err
	}
	owner, hash, err := storyOwner(self, in.GetPeer())
	if err != nil {
		return nil, err
	}
	return story.Shared().StoryStats(ctx, self, owner, hash, in.GetId())
}

func (StatsService) StatsGetStoryPublicForwards(ctx context.Context, in *mtproto.TLStatsGetStoryPublicForwards) (*mtproto.Stats_PublicForwards, error) {
	self, err := statsUser(ctx)
	if err != nil {
		return nil, err
	}
	owner, hash, err := storyOwner(self, in.GetPeer())
	if err != nil {
		return nil, err
	}
	return story.Shared().StoryPublicForwards(ctx, self, owner, hash, in.GetId(), in.GetOffset(), in.GetLimit())
}

func (StatsService) StatsGetPollStats(ctx context.Context, in *mtproto.TLStatsGetPollStats) (*mtproto.Stats_PollStats, error) {
	self, err := statsUser(ctx)
	if err != nil {
		return nil, err
	}
	return pollStats(ctx, self, in.GetPeer(), in.GetMsgId())
}

func statsUser(ctx context.Context) (int64, error) {
	self, _ := actor(ctx)
	if self == 0 {
		return 0, mtproto.ErrAuthKeyUnregistered
	}
	return self, nil
}

func storyOwner(self int64, peer *mtproto.InputPeer) (story.Owner, int64, error) {
	if peer == nil {
		return story.Owner{}, 0, mtproto.ErrPeerIdInvalid
	}
	p := mtproto.FromInputPeer2(self, peer)
	switch p.PeerType {
	case mtproto.PEER_SELF:
		return story.Owner{Kind: story.KindUser, ID: self}, 0, nil
	case mtproto.PEER_USER:
		if p.PeerId == 0 {
			return story.Owner{}, 0, mtproto.ErrPeerIdInvalid
		}
		return story.Owner{Kind: story.KindUser, ID: p.PeerId}, p.AccessHash, nil
	case mtproto.PEER_CHANNEL:
		if p.PeerId == 0 {
			return story.Owner{}, 0, mtproto.ErrPeerIdInvalid
		}
		return story.Owner{Kind: story.KindChannel, ID: p.PeerId}, p.AccessHash, nil
	default:
		return story.Owner{}, 0, mtproto.ErrPeerIdInvalid
	}
}

func pollStats(ctx context.Context, self int64, peer *mtproto.InputPeer, msgID int32) (*mtproto.Stats_PollStats, error) {
	if peer == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if msgID <= 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	p := mtproto.FromInputPeer2(self, peer)
	if p.PeerType == mtproto.PEER_SELF {
		p.PeerType = mtproto.PEER_USER
		p.PeerId = self
	}
	if !p.IsUserOrChatOrChannel() || p.PeerId == 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	var poll *mtproto.Poll
	var results *mtproto.PollResults
	if p.IsChannel() {
		page, err := channel.Shared().GetMessages(ctx, self, &mtproto.InputChannel{
			PredicateName: mtproto.Predicate_inputChannel,
			ChannelId:     p.PeerId,
			AccessHash:    p.AccessHash,
		}, []int32{msgID})
		if err != nil {
			return nil, err
		}
		if len(page.GetMessages()) > 0 && page.Messages[0].GetPredicateName() != mtproto.Predicate_messageEmpty {
			media := page.Messages[0].GetMedia()
			if media.GetPredicateName() == mtproto.Predicate_messageMediaPoll {
				poll = media.GetPoll()
				results = media.GetResults()
			}
		}
	}
	if snap, res, err := chatbox.Shared().PollByMessage(ctx, p.PeerType, p.PeerId, msgID); err == nil && snap != nil {
		poll, results = snap, res
	}
	if poll == nil {
		return nil, mtproto.ErrMessageIdInvalid
	}
	xs := make([]int64, 0, len(poll.GetAnswers()))
	ys := make([]float64, 0, len(poll.GetAnswers()))
	for i, ans := range poll.GetAnswers() {
		xs = append(xs, int64(i+1))
		ys = append(ys, float64(voters(results, ans.GetOption())))
	}
	if len(xs) == 0 {
		xs = []int64{1}
		ys = []float64{0}
	}
	return mtproto.MakeTLStatsPollStats(&mtproto.Stats_PollStats{
		VotesGraph: channel.MintGraph("Votes", xs, ys, true),
	}).To_Stats_PollStats(), nil
}

func voters(results *mtproto.PollResults, option []byte) int32 {
	for _, row := range results.GetResults() {
		if bytes.Equal(row.GetOption(), option) {
			if row.GetVoters_FLAGINT32() != nil {
				return row.GetVoters_FLAGINT32().GetValue()
			}
			return row.GetVoters_INT32()
		}
	}
	return 0
}
