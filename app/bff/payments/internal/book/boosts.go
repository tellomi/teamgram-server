package book

import (
	"sort"
	"strconv"
	"strings"

	"github.com/teamgram/proto/mtproto"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func (b *Book) MyBoosts(self int64) (*mtproto.Premium_MyBoosts, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	rows := []*mtproto.MyBoost{}
	if b.isPremium() {
		acc := b.ensure(self)
		now := b.clock()
		for i := range acc.slots {
			rows = append(rows, myBoost(int32(i), &acc.slots[i], now))
		}
	}
	return mtproto.MakeTLPremiumMyBoosts(&mtproto.Premium_MyBoosts{
		MyBoosts: rows,
		Chats:    []*mtproto.Chat{},
		Users:    []*mtproto.User{},
	}).To_Premium_MyBoosts(), nil
}

func (b *Book) ApplyBoost(self int64, peer *mtproto.InputPeer, slots []int32, hasSlots bool) (*mtproto.Premium_MyBoosts, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	ref, _, err := parsePeer(self, peer)
	if err != nil {
		return nil, err
	}
	if !b.isPremium() {
		return nil, fail(CodePremiumRequired)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	acc := b.ensure(self)
	now := b.clock()
	chosen, err := pickSlots(acc, ref.key(), slots, hasSlots, now)
	if err != nil {
		return nil, err
	}
	for _, idx := range chosen {
		s := &acc.slots[idx]
		if s.peer != ref.key() {
			s.cooldownUntil = now + cooldown
			s.date = now
		}
		s.peer = ref.key()
		s.kind = ref.kind
		s.id = ref.id
		s.expires = now + boostLife
		if s.date == 0 {
			s.date = now
		}
	}
	rows := make([]*mtproto.MyBoost, 0, slotCount)
	for i := range acc.slots {
		rows = append(rows, myBoost(int32(i), &acc.slots[i], now))
	}
	return mtproto.MakeTLPremiumMyBoosts(&mtproto.Premium_MyBoosts{
		MyBoosts: rows,
		Chats:    []*mtproto.Chat{},
		Users:    []*mtproto.User{},
	}).To_Premium_MyBoosts(), nil
}

func (b *Book) BoostsStatus(self int64, peer *mtproto.InputPeer) (*mtproto.Premium_BoostsStatus, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	ref, _, err := parsePeer(self, peer)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.clock()
	n, mine := b.count(ref.key(), self, now)
	if !b.isPremium() {
		mine = nil
	}
	level, current, next, hasNext := levelOf(n)
	st := &mtproto.Premium_BoostsStatus{
		Level:              level,
		CurrentLevelBoosts: current,
		Boosts:             n,
		BoostUrl:           "",
	}
	if hasNext {
		st.NextLevelBoosts = wrapperspb.Int32(next)
	}
	if len(mine) > 0 {
		st.MyBoost = true
		st.MyBoostSlots = mine
	}
	return mtproto.MakeTLPremiumBoostsStatus(st).To_Premium_BoostsStatus(), nil
}

func (b *Book) BoostsList(self int64, in *mtproto.TLPremiumGetBoostsList) (*mtproto.Premium_BoostsList, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	if in == nil {
		return nil, fail(CodePeerInvalid)
	}
	ref, _, err := parsePeer(self, in.GetPeer())
	if err != nil {
		return nil, err
	}
	limit, err := pageLimitOf(in.GetLimit())
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.clock()
	var rows []liveBoost
	if !in.GetGifts() {
		rows = b.live(ref.key(), now)
	}
	return pageBoosts(rows, in.GetOffset(), limit)
}

func (b *Book) UserBoosts(self int64, peer *mtproto.InputPeer, user *mtproto.InputUser) (*mtproto.Premium_BoostsList, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	ref, _, err := parsePeer(self, peer)
	if err != nil {
		return nil, err
	}
	uid, err := parseUser(self, user)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.clock()
	var rows []liveBoost
	for _, row := range b.live(ref.key(), now) {
		if row.user == uid {
			rows = append(rows, row)
		}
	}
	limit := int32(len(rows))
	if limit == 0 {
		limit = pageLimit
	}
	return pageBoosts(rows, "", limit)
}

type liveBoost struct {
	user    int64
	slot    int32
	date    int32
	expires int32
}

func (b *Book) ensure(self int64) *account {
	acc := b.users[self]
	if acc == nil {
		acc = &account{}
		b.users[self] = acc
	}
	now := b.clock()
	for i := range acc.slots {
		s := &acc.slots[i]
		if s.expires != 0 && s.expires <= now {
			*s = slotState{}
		}
		if s.expires == 0 {
			s.date = now
			s.expires = now + boostLife
		}
	}
	return acc
}

func (b *Book) count(key string, self int64, now int32) (int32, []int32) {
	var n int32
	var mine []int32
	for uid, acc := range b.users {
		for i := range acc.slots {
			s := &acc.slots[i]
			if s.peer != key || s.expires <= now {
				continue
			}
			n++
			if uid == self {
				mine = append(mine, int32(i))
			}
		}
	}
	sort.Slice(mine, func(i, j int) bool { return mine[i] < mine[j] })
	return n, mine
}

func (b *Book) live(key string, now int32) []liveBoost {
	var rows []liveBoost
	for uid, acc := range b.users {
		for i := range acc.slots {
			s := &acc.slots[i]
			if s.peer != key || s.expires <= now {
				continue
			}
			rows = append(rows, liveBoost{user: uid, slot: int32(i), date: s.date, expires: s.expires})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].date != rows[j].date {
			return rows[i].date < rows[j].date
		}
		if rows[i].user != rows[j].user {
			return rows[i].user < rows[j].user
		}
		return rows[i].slot < rows[j].slot
	})
	return rows
}

func pickSlots(acc *account, key string, slots []int32, hasSlots bool, now int32) ([]int, error) {
	if !hasSlots || len(slots) == 0 {
		for i := range acc.slots {
			s := &acc.slots[i]
			if s.peer == "" || s.expires <= now {
				return []int{i}, nil
			}
		}
		return nil, fail(CodeBoostsEmpty)
	}
	seen := map[int32]bool{}
	var out []int
	for _, slot := range slots {
		if slot < 0 || slot >= slotCount || seen[slot] {
			if seen[slot] {
				continue
			}
			return nil, fail(CodeSlotInvalid)
		}
		seen[slot] = true
		s := &acc.slots[slot]
		if s.peer != "" && s.peer != key && s.expires > now && s.cooldownUntil > now {
			return nil, fail(CodeBoostCooldown)
		}
		out = append(out, int(slot))
	}
	return out, nil
}

func myBoost(slot int32, s *slotState, now int32) *mtproto.MyBoost {
	mb := &mtproto.MyBoost{
		Slot:    slot,
		Date:    s.date,
		Expires: s.expires,
	}
	if s.peer != "" && s.expires > now {
		mb.Peer = peerOf(s.kind, s.id)
	}
	if s.cooldownUntil > now {
		mb.CooldownUntilDate = wrapperspb.Int32(s.cooldownUntil)
	}
	return mtproto.MakeTLMyBoost(mb).To_MyBoost()
}

func peerOf(kind string, id int64) *mtproto.Peer {
	switch kind {
	case "user":
		return mtproto.MakePeerUser(id)
	case "chat":
		return mtproto.MakePeerChat(id)
	case "channel":
		return mtproto.MakePeerChannel(id)
	default:
		return nil
	}
}

func levelOf(n int32) (level, current, next int32, hasNext bool) {
	if n < 0 {
		n = 0
	}
	level = n
	if level > 100 {
		level = 100
	}
	current = level
	if level < 100 {
		return level, current, level + 1, true
	}
	return level, current, 0, false
}

func pageBoosts(rows []liveBoost, offset string, limit int32) (*mtproto.Premium_BoostsList, error) {
	start := 0
	if strings.TrimSpace(offset) != "" {
		n, err := strconv.Atoi(strings.TrimSpace(offset))
		if err != nil || n < 0 || n > len(rows) {
			return nil, fail(CodeOffsetInvalid)
		}
		start = n
	}
	if limit < 0 {
		limit = 0
	}
	end := start + int(limit)
	if end > len(rows) {
		end = len(rows)
	}
	out := make([]*mtproto.Boost, 0, end-start)
	for _, row := range rows[start:end] {
		out = append(out, mtproto.MakeTLBoost(&mtproto.Boost{
			Id:      "boost-" + itoa(row.user) + "-" + itoa(int64(row.slot)),
			UserId:  wrapperspb.Int64(row.user),
			Date:    row.date,
			Expires: row.expires,
		}).To_Boost())
	}
	list := &mtproto.Premium_BoostsList{
		Count:  int32(len(rows)),
		Boosts: out,
		Users:  []*mtproto.User{},
	}
	if end < len(rows) {
		list.NextOffset = wrapperspb.String(strconv.Itoa(end))
	}
	return mtproto.MakeTLPremiumBoostsList(list).To_Premium_BoostsList(), nil
}
