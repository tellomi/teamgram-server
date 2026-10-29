package channel

import (
	"context"
	"sort"
	"sync"

	"github.com/teamgram/proto/mtproto"
)

type memoryStore struct {
	mu       sync.Mutex
	nextID   int64
	channels map[int64]*Channel
	parts    map[int64]map[int64]*Participant
	messages map[int64]map[int32]*StoredMessage
	events   map[int64][]*PtsEvent
	reads    map[int64]map[int64]*ReadState
	features map[int64]*Features
	personal map[int64]int64
}

func NewMemoryStore() Store {
	return &memoryStore{
		nextID:   mtproto.MinNebulaChatChannelID,
		channels: map[int64]*Channel{},
		parts:    map[int64]map[int64]*Participant{},
		messages: map[int64]map[int32]*StoredMessage{},
		events:   map[int64][]*PtsEvent{},
		reads:    map[int64]map[int64]*ReadState{},
		features: map[int64]*Features{},
		personal: map[int64]int64{},
	}
}

func (s *memoryStore) AllocID(ctx context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextID
	s.nextID++
	return id, nil
}

func (s *memoryStore) CreateChannel(ctx context.Context, ch *Channel, creator *Participant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ch.ID == 0 {
		ch.ID = s.nextID
		s.nextID++
	} else if ch.ID >= s.nextID {
		s.nextID = ch.ID + 1
	}
	s.channels[ch.ID] = ch.clone()
	if s.parts[ch.ID] == nil {
		s.parts[ch.ID] = map[int64]*Participant{}
	}
	creator.ChannelID = ch.ID
	s.parts[ch.ID][creator.UserID] = creator.clone()
	if s.messages[ch.ID] == nil {
		s.messages[ch.ID] = map[int32]*StoredMessage{}
	}
	if s.reads[ch.ID] == nil {
		s.reads[ch.ID] = map[int64]*ReadState{}
	}
	return nil
}

func (s *memoryStore) GetChannel(ctx context.Context, id int64) (*Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.channels[id].clone(), nil
}

func (s *memoryStore) SaveChannel(ctx context.Context, ch *Channel) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.channels[ch.ID]
	if cur == nil {
		return mtproto.ErrChannelInvalid
	}
	// Pts and top_message are owned by Commit* so a metadata write cannot roll them back.
	cp := ch.clone()
	cp.Pts = cur.Pts
	cp.TopMessage = cur.TopMessage
	s.channels[ch.ID] = cp
	return nil
}

func (s *memoryStore) FindByUsername(ctx context.Context, username string) (*Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range s.channels {
		if !ch.Deleted && ch.Username != "" && equalFold(ch.Username, username) {
			return ch.clone(), nil
		}
	}
	return nil, nil
}

func (s *memoryStore) FindByMigratedChat(ctx context.Context, chatID int64) (*Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range s.channels {
		if !ch.Deleted && ch.MigratedFromChatID == chatID {
			return ch.clone(), nil
		}
	}
	return nil, nil
}

func (s *memoryStore) ListChannels(ctx context.Context, ids []int64) ([]*Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Channel, 0, len(ids))
	for _, id := range ids {
		if ch := s.channels[id]; ch != nil {
			out = append(out, ch.clone())
		}
	}
	return out, nil
}

func (s *memoryStore) ListByUser(ctx context.Context, userID int64) ([]*Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Channel
	for id, parts := range s.parts {
		p := parts[userID]
		if p != nil && p.State == StateNormal {
			if ch := s.channels[id]; ch != nil && !ch.Deleted {
				out = append(out, ch.clone())
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *memoryStore) PutParticipant(ctx context.Context, p *Participant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.parts[p.ChannelID] == nil {
		s.parts[p.ChannelID] = map[int64]*Participant{}
	}
	s.parts[p.ChannelID][p.UserID] = p.clone()
	return nil
}

func (s *memoryStore) GetParticipant(ctx context.Context, channelID, userID int64) (*Participant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.parts[channelID] == nil {
		return nil, nil
	}
	return s.parts[channelID][userID].clone(), nil
}

func (s *memoryStore) ListParticipants(ctx context.Context, channelID int64) ([]*Participant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Participant
	for _, p := range s.parts[channelID] {
		out = append(out, p.clone())
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date == out[j].Date {
			return out[i].UserID < out[j].UserID
		}
		return out[i].Date < out[j].Date
	})
	return out, nil
}

func (s *memoryStore) CountMembers(ctx context.Context, channelID int64) (int32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int32
	for _, p := range s.parts[channelID] {
		if p.State == StateNormal {
			n++
		}
	}
	return n, nil
}

func (s *memoryStore) CommitNew(ctx context.Context, channelID, senderID, randomID int64, date int32, build BuildMessage) (int32, int32, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := s.channels[channelID]
	if ch == nil || ch.Deleted {
		return 0, 0, false, mtproto.ErrChannelInvalid
	}
	if s.messages[channelID] == nil {
		s.messages[channelID] = map[int32]*StoredMessage{}
	}
	if randomID != 0 {
		for _, m := range s.messages[channelID] {
			if m.SenderID == senderID && m.RandomID == randomID && !m.Deleted {
				return m.ID, m.Pts, true, nil
			}
		}
	}
	ch.TopMessage++
	ch.Pts++
	id := ch.TopMessage
	pts := ch.Pts
	payload := build(id, pts)
	msg := &StoredMessage{
		ChannelID: channelID,
		ID:        id,
		SenderID:  senderID,
		Date:      date,
		RandomID:  randomID,
		Pts:       pts,
		Data:      append([]byte(nil), payload...),
	}
	s.messages[channelID][id] = msg
	s.events[channelID] = append(s.events[channelID], &PtsEvent{
		ChannelID: channelID,
		Pts:       pts,
		PtsCount:  1,
		Kind:      KindNew,
		MessageID: id,
		Payload:   append([]byte(nil), payload...),
	})
	return id, pts, false, nil
}

func (s *memoryStore) CommitEdit(ctx context.Context, channelID int64, messageID int32, editDate int32, data []byte) (int32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := s.channels[channelID]
	if ch == nil || ch.Deleted {
		return 0, mtproto.ErrChannelInvalid
	}
	msg := s.messages[channelID][messageID]
	if msg == nil || msg.Deleted {
		return 0, mtproto.ErrMessageIdInvalid
	}
	ch.Pts++
	msg.Data = append([]byte(nil), data...)
	msg.EditDate = editDate
	s.events[channelID] = append(s.events[channelID], &PtsEvent{
		ChannelID: channelID,
		Pts:       ch.Pts,
		PtsCount:  1,
		Kind:      KindEdit,
		MessageID: messageID,
		Payload:   append([]byte(nil), data...),
	})
	return ch.Pts, nil
}

func (s *memoryStore) CommitDelete(ctx context.Context, channelID int64, ids []int32) (int32, int32, []int32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := s.channels[channelID]
	if ch == nil || ch.Deleted {
		return 0, 0, nil, mtproto.ErrChannelInvalid
	}
	var deleted []int32
	for _, id := range ids {
		msg := s.messages[channelID][id]
		if msg == nil || msg.Deleted {
			continue
		}
		msg.Deleted = true
		deleted = append(deleted, id)
	}
	if len(deleted) == 0 {
		return ch.Pts, 0, nil, nil
	}
	ch.Pts += int32(len(deleted))
	s.events[channelID] = append(s.events[channelID], &PtsEvent{
		ChannelID:  channelID,
		Pts:        ch.Pts,
		PtsCount:   int32(len(deleted)),
		Kind:       KindDelete,
		MessageIDs: append([]int32(nil), deleted...),
	})
	return ch.Pts, int32(len(deleted)), deleted, nil
}

func (s *memoryStore) GetMessage(ctx context.Context, channelID int64, id int32) (*StoredMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.messages[channelID] == nil {
		return nil, nil
	}
	return s.messages[channelID][id].clone(), nil
}

func (s *memoryStore) ListMessages(ctx context.Context, channelID int64) ([]*StoredMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*StoredMessage
	for _, m := range s.messages[channelID] {
		out = append(out, m.clone())
	}
	return out, nil
}

func (s *memoryStore) AppendPts(ctx context.Context, channelID int64, kind string, messageID int32, payload []byte) (int32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := s.channels[channelID]
	if ch == nil || ch.Deleted {
		return 0, mtproto.ErrChannelInvalid
	}
	ch.Pts++
	s.events[channelID] = append(s.events[channelID], &PtsEvent{
		ChannelID: channelID,
		Pts:       ch.Pts,
		PtsCount:  1,
		Kind:      kind,
		MessageID: messageID,
		Payload:   append([]byte(nil), payload...),
	})
	return ch.Pts, nil
}

func (s *memoryStore) ListPtsAfter(ctx context.Context, channelID int64, pts int32, limit int) ([]*PtsEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*PtsEvent
	for _, e := range s.events[channelID] {
		if e.Pts > pts {
			out = append(out, e.clone())
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Pts < out[j].Pts })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *memoryStore) GetRead(ctx context.Context, channelID, userID int64) (*ReadState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reads[channelID] == nil {
		return &ReadState{ChannelID: channelID, UserID: userID}, nil
	}
	st := s.reads[channelID][userID]
	if st == nil {
		return &ReadState{ChannelID: channelID, UserID: userID}, nil
	}
	return st.clone(), nil
}

func (s *memoryStore) SaveRead(ctx context.Context, st *ReadState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reads[st.ChannelID] == nil {
		s.reads[st.ChannelID] = map[int64]*ReadState{}
	}
	s.reads[st.ChannelID][st.UserID] = st.clone()
	return nil
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
