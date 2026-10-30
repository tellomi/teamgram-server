package channel

import (
	"context"
	"strconv"

	"github.com/teamgram/proto/mtproto"
)

// MessageStats is stats.getMessageStats for a channel an admin can see.
func (s *Service) MessageStats(ctx context.Context, self int64, in *mtproto.InputChannel, msgID int32) (*mtproto.Stats_MessageStats, error) {
	if _, err := s.requireStatsAdmin(ctx, self, in); err != nil {
		return nil, err
	}
	msg, err := s.loadLiveMessage(ctx, in, msgID)
	if err != nil {
		return nil, err
	}
	views := float64(0)
	if msg.GetViews() != nil {
		views = float64(msg.GetViews().GetValue())
	}
	when := int64(msg.GetDate())
	if when <= 0 {
		when = 1
	}
	rx, ry := reactionSeries(msg)
	return mtproto.MakeTLStatsMessageStats(&mtproto.Stats_MessageStats{
		ViewsGraph:              MintGraph("Views", []int64{when}, []float64{views}, false),
		ReactionsByEmotionGraph: MintGraph("Reactions", rx, ry, true),
	}).To_Stats_MessageStats(), nil
}

// MessagePublicForwards is stats.getMessagePublicForwards. This process does
// not store the forwarded copies, so the vector is empty and count is the
// forwards counter on the message.
func (s *Service) MessagePublicForwards(ctx context.Context, self int64, in *mtproto.InputChannel, msgID int32, offset string, limit int32) (*mtproto.Stats_PublicForwards, error) {
	if _, err := s.requireStatsAdmin(ctx, self, in); err != nil {
		return nil, err
	}
	msg, err := s.loadLiveMessage(ctx, in, msgID)
	if err != nil {
		return nil, err
	}
	if _, err := forwardsPage(offset, limit); err != nil {
		return nil, err
	}
	count := int32(0)
	if msg.GetForwards() != nil {
		count = msg.GetForwards().GetValue()
	}
	return mtproto.MakeTLStatsPublicForwards(&mtproto.Stats_PublicForwards{
		Count:    count,
		Forwards: []*mtproto.PublicForward{},
		Chats:    []*mtproto.Chat{},
		Users:    []*mtproto.User{},
	}).To_Stats_PublicForwards(), nil
}

// MessagePublicForwardsLegacy is the pre-layer constructor that returns
// messages.Messages instead of stats.PublicForwards.
func (s *Service) MessagePublicForwardsLegacy(ctx context.Context, self int64, in *mtproto.InputChannel, msgID, limit int32) (*mtproto.Messages_Messages, error) {
	if limit < 0 || limit > 100 {
		return nil, mtproto.ErrLimitInvalid
	}
	ch, err := s.requireStatsAdmin(ctx, self, in)
	if err != nil {
		return nil, err
	}
	msg, err := s.loadLiveMessage(ctx, in, msgID)
	if err != nil {
		return nil, err
	}
	count := int32(0)
	if msg.GetForwards() != nil {
		count = msg.GetForwards().GetValue()
	}
	return mtproto.MakeTLMessagesMessagesSlice(&mtproto.Messages_Messages{
		Count:    count,
		Messages: []*mtproto.Message{},
		Chats:    []*mtproto.Chat{s.ChatFor(ctx, self, ch)},
		Users:    []*mtproto.User{},
	}).To_Messages_Messages(), nil
}

func (s *Service) requireStatsAdmin(ctx context.Context, self int64, in *mtproto.InputChannel) (*Channel, error) {
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if !p.IsAdmin() {
		return nil, mtproto.ErrChatAdminRequired
	}
	return ch, nil
}

func (s *Service) loadLiveMessage(ctx context.Context, in *mtproto.InputChannel, msgID int32) (*mtproto.Message, error) {
	if msgID <= 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	ch, err := s.lookup(ctx, in)
	if err != nil {
		return nil, err
	}
	return s.LoadMessage(ctx, ch.ID, msgID)
}

func reactionSeries(msg *mtproto.Message) ([]int64, []float64) {
	results := msg.GetReactions().GetResults()
	if len(results) == 0 {
		return []int64{1}, []float64{0}
	}
	xs := make([]int64, len(results))
	ys := make([]float64, len(results))
	for i, row := range results {
		xs[i] = int64(i + 1)
		ys[i] = float64(row.GetCount())
	}
	return xs, ys
}

func forwardsPage(offset string, limit int32) (int, error) {
	if limit < 0 || limit > 100 {
		return 0, mtproto.ErrLimitInvalid
	}
	if offset == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(offset)
	if err != nil || n < 0 {
		return 0, mtproto.ErrOffsetInvalid
	}
	return n, nil
}
