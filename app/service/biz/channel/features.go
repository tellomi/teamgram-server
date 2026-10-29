package channel

import (
	"context"
	"encoding/json"

	"github.com/teamgram/proto/mtproto"
)

// Features is the extra channel state that is not on the core row: admin log,
// topics, reactions, join requests, slow mode, discussion link, and pins.
type Features struct {
	SlowmodeSeconds    int32
	HiddenPrehistory   bool
	ParticipantsHidden bool
	JoinRequest        bool
	JoinToSend         bool
	Gigagroup          bool
	LinkedChatID       int64
	StickerSetID       int64
	EmojiSetID         int64
	PinnedMsgID        int32
	AvailableMinID     int32
	FutureCreatorID    int64
	Lat                float64
	Long               float64
	LogSeq             int64
	AdminLog           []AdminEvent
	Topics             []TopicRec
	Reactions          []ReactRec
	JoinRequests       []JoinReq
	LastSend           map[int64]int32
	UserMinID          map[int64]int32
	ReadContents       map[int64][]int32
	MsgTopic           map[int32]int32
	AvailableReactions []string
}

type AdminEvent struct {
	ID     int64
	Date   int32
	UserID int64
	Action []byte
}

type TopicRec struct {
	ID         int32
	Title      string
	IconColor  int32
	IconEmoji  int64
	CreatorID  int64
	Date       int32
	TopMessage int32
	Closed     bool
	Pinned     bool
	Hidden     bool
}

type ReactRec struct {
	MsgID    int32
	UserID   int64
	Emoticon string
	Date     int32
	Big      bool
}

type JoinReq struct {
	UserID int64
	Date   int32
	About  string
}

func (f *Features) clone() *Features {
	if f == nil {
		return &Features{}
	}
	raw, err := json.Marshal(f)
	if err != nil {
		cp := *f
		return &cp
	}
	out := &Features{}
	if err := json.Unmarshal(raw, out); err != nil {
		cp := *f
		return &cp
	}
	return out
}

func (s *memoryStore) GetFeatures(ctx context.Context, channelID int64) (*Features, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.features[channelID].clone(), nil
}

func (s *memoryStore) SaveFeatures(ctx context.Context, channelID int64, f *Features) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.channels[channelID] == nil {
		return mtproto.ErrChannelInvalid
	}
	s.features[channelID] = f.clone()
	return nil
}

func (s *memoryStore) GetPersonalChannel(ctx context.Context, userID int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.personal[userID], nil
}

func (s *memoryStore) SetPersonalChannel(ctx context.Context, userID, channelID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if channelID == 0 {
		delete(s.personal, userID)
		return nil
	}
	s.personal[userID] = channelID
	return nil
}
