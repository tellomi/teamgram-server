package catalog

import (
	"hash/fnv"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/logx"
)

const maxRecentReactions = 50

type reactionStore interface {
	saveReactionUse(emoticon string, count int64) error
	saveReactionRecent(userID int64, emoticon string, date int32) error
	clearReactionRecent(userID int64) error
}

// defaultReactionOrder is the available reaction list in catalog order.
// Top reactions fall back to this when nothing has been used yet.
func defaultReactionOrder() []string {
	var out []string
	seen := map[string]bool{}
	for _, def := range emojiCatalog {
		if def.Reaction == "" {
			continue
		}
		emo := emojiFromCode(def.Code)
		if emo == "" || seen[emo] {
			continue
		}
		seen[emo] = true
		out = append(out, emo)
	}
	return out
}

// TopReactions returns available reactions, most used first.
// Equal counts keep the default catalog order. hash 0 always returns the list.
func (s *Service) TopReactions(limit int32, hash int64) *mtproto.Messages_Reactions {
	s.mu.Lock()
	defer s.mu.Unlock()
	order := defaultReactionOrder()
	index := make(map[string]int, len(order))
	for i, emo := range order {
		index[emo] = i
	}
	for i := 1; i < len(order); i++ {
		j := i
		for j > 0 {
			left, right := order[j-1], order[j]
			cl, cr := s.reactionUses[left], s.reactionUses[right]
			if cl > cr || (cl == cr && index[left] <= index[right]) {
				break
			}
			order[j-1], order[j] = order[j], order[j-1]
			j--
		}
	}
	return reactionPage(order, limit, hash)
}

// RecentReactions returns the emojis this user has sent, newest first.
func (s *Service) RecentReactions(userID int64, limit int32, hash int64) *mtproto.Messages_Reactions {
	s.mu.Lock()
	defer s.mu.Unlock()
	var emos []string
	if st := s.users[userID]; st != nil {
		emos = append(emos, st.reactionRecent...)
	}
	return reactionPage(emos, limit, hash)
}

// NoteReaction records emojis the user just sent and bumps the global counts.
func (s *Service) NoteReaction(userID int64, emoticons []string) {
	if userID == 0 || len(emoticons) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reactionUses == nil {
		s.reactionUses = map[string]int64{}
	}
	st := s.state(userID)
	date := s.now()
	var store reactionStore
	if s.persist != nil {
		store, _ = s.persist.(reactionStore)
	}
	for _, emo := range emoticons {
		if emo == "" {
			continue
		}
		s.reactionUses[emo]++
		if store != nil {
			if err := store.saveReactionUse(emo, s.reactionUses[emo]); err != nil {
				logx.Errorf("reaction count %s: %v", emo, err)
			}
		}
		next := make([]string, 0, len(st.reactionRecent)+1)
		next = append(next, emo)
		for _, have := range st.reactionRecent {
			if have != emo {
				next = append(next, have)
			}
		}
		st.reactionRecent = next
		if len(st.reactionRecent) > maxRecentReactions {
			st.reactionRecent = st.reactionRecent[:maxRecentReactions]
		}
		if store != nil {
			if err := store.saveReactionRecent(userID, emo, date); err != nil {
				logx.Errorf("reaction recent %d %s: %v", userID, emo, err)
			}
		}
	}
}

// ClearRecentReactions drops the user's recent reaction list.
func (s *Service) ClearRecentReactions(userID int64) {
	if userID == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if st := s.users[userID]; st != nil {
		st.reactionRecent = nil
	}
	if store, ok := s.persist.(reactionStore); ok {
		if err := store.clearReactionRecent(userID); err != nil {
			logx.Errorf("reaction recent clear %d: %v", userID, err)
		}
	}
}

func reactionPage(emos []string, limit int32, hash int64) *mtproto.Messages_Reactions {
	if limit > 0 && int(limit) < len(emos) {
		emos = emos[:limit]
	}
	sum := reactionHash(emos)
	if hash != 0 && hash == sum {
		return mtproto.MakeTLMessagesReactionsNotModified(&mtproto.Messages_Reactions{}).To_Messages_Reactions()
	}
	out := make([]*mtproto.Reaction, 0, len(emos))
	for _, emo := range emos {
		out = append(out, mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: emo}).To_Reaction())
	}
	return mtproto.MakeTLMessagesReactions(&mtproto.Messages_Reactions{
		Hash:      sum,
		Reactions: out,
	}).To_Messages_Reactions()
}

func reactionHash(emos []string) int64 {
	h := fnv.New64a()
	for _, emo := range emos {
		_, _ = h.Write([]byte(emo))
		_, _ = h.Write([]byte{0})
	}
	out := int64(h.Sum64() & 0x7fffffffffffffff)
	if out == 0 {
		return 1
	}
	return out
}
