package channel

import (
	"context"
	"strconv"

	"github.com/teamgram/proto/mtproto"
)

func topicReadKey(userID int64, topicID int32) string {
	return strconv.FormatInt(userID, 10) + ":" + strconv.FormatInt(int64(topicID), 10)
}

// topicCursor is one viewer's forum topic read state.
// Messages the viewer sent are not unread. read_outbox_max_id is the newest
// message that viewer sent in the topic.
func topicCursor(self int64, topicID int32, f *Features, stored []*StoredMessage) (read, unread, outbox int32) {
	if f != nil && f.TopicRead != nil {
		read = f.TopicRead[topicReadKey(self, topicID)]
	}
	for _, m := range stored {
		if m == nil || m.Deleted || !messageInTopic(f, m, topicID) {
			continue
		}
		if m.SenderID == self {
			if m.ID > outbox {
				outbox = m.ID
			}
			continue
		}
		if m.ID > read {
			unread++
		}
	}
	return read, unread, outbox
}

// ReadTopic records messages.readDiscussion for one forum topic.
// A msg_id that is not a topic is TOPIC_ID_INVALID so a comment thread,
// which uses the same RPC, can keep the previous channel read path.
func (s *Service) ReadTopic(ctx context.Context, self, channelID int64, topicID, readMax int32) error {
	if topicID <= 0 {
		return mtproto.ErrTopicIdInvalid
	}
	ch, _, err := s.requireView(ctx, self, &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: channelID})
	if err != nil {
		return err
	}
	f := s.features(ctx, ch.ID)
	if _, ok := findTopic(f, topicID); !ok {
		return mtproto.ErrTopicIdInvalid
	}
	if readMax < 0 {
		readMax = 0
	}
	if f.TopicRead == nil {
		f.TopicRead = map[string]int32{}
	}
	key := topicReadKey(self, topicID)
	if readMax <= f.TopicRead[key] {
		return nil
	}
	f.TopicRead[key] = readMax
	return s.saveFeatures(ctx, ch.ID, f)
}
