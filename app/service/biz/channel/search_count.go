package channel

import (
	"context"

	"github.com/teamgram/proto/mtproto"
)

// CountByFilter counts channel posts that messages.getSearchCounters would
// report for one filter. It uses the same match as messages.search, and it
// does not stop at the search page size.
func (s *Service) CountByFilter(ctx context.Context, self, channelID int64, filter *mtproto.MessagesFilter) (int32, error) {
	ch, _, err := s.requireView(ctx, self, &mtproto.InputChannel{
		PredicateName: mtproto.Predicate_inputChannel,
		ChannelId:     channelID,
	})
	if err != nil {
		return 0, err
	}
	all, err := s.store.ListMessages(ctx, ch.ID)
	if err != nil {
		return 0, err
	}
	feat := s.features(ctx, ch.ID)
	viewer, _ := s.store.GetParticipant(ctx, ch.ID, self)
	var n int32
	for _, stored := range all {
		if stored == nil || stored.Deleted {
			continue
		}
		if prehistoryBlocks(feat, viewer, stored) {
			continue
		}
		body, err := unmarshalMessage(stored.Data)
		if err != nil || body == nil {
			continue
		}
		if channelFilterMatches(body, filter, feat.PinnedMsgID == stored.ID) {
			n++
		}
	}
	return n, nil
}
