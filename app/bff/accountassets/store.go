package accountassets

import (
	"sort"
	"sync"

	"github.com/teamgram/proto/mtproto"
)

// Account themes, wallpapers, ringtones, and connected business bots.
// Catalogs are per user. A hash of 0 always returns the list so a client
// with an empty cache does not stop on themesNotModified.
// https://core.telegram.org/api/themes
// https://core.telegram.org/api/wallpapers
// https://core.telegram.org/api/business

type peerKey struct {
	kind string
	id   int64
}

type userState struct {
	savedThemes   map[int64]struct{}
	installed     map[string]int64
	savedWalls    map[int64]struct{}
	installedWall int64
	savedTones    map[int64]struct{}
	bots          map[int64]*botLink
	botOrder      []int64
	paused        map[peerKey]bool
	disabled      map[peerKey]struct{}
}

type store struct {
	mu        sync.Mutex
	seq       int64
	docs      map[int64]*docRec
	themes    map[int64]*themeRec
	themeSlug map[string]int64
	walls     map[int64]*wallRec
	wallSlug  map[string]int64
	users     map[int64]*userState
}

var (
	gate    sync.Mutex
	current = newStore()
)

func newStore() *store {
	return &store{
		docs:      map[int64]*docRec{},
		themes:    map[int64]*themeRec{},
		themeSlug: map[string]int64{},
		walls:     map[int64]*wallRec{},
		wallSlug:  map[string]int64{},
		users:     map[int64]*userState{},
	}
}

func newUser() *userState {
	return &userState{
		savedThemes: map[int64]struct{}{},
		installed:   map[string]int64{},
		savedWalls:  map[int64]struct{}{},
		savedTones:  map[int64]struct{}{},
		bots:        map[int64]*botLink{},
		paused:      map[peerKey]bool{},
		disabled:    map[peerKey]struct{}{},
	}
}

// Reset clears process state. Tests call it.
func Reset() {
	gate.Lock()
	current = newStore()
	gate.Unlock()
}

func shared() *store {
	gate.Lock()
	s := current
	gate.Unlock()
	return s
}

func (s *store) user(id int64) *userState {
	u := s.users[id]
	if u == nil {
		u = newUser()
		s.users[id] = u
	}
	return u
}

func (s *store) nextID() int64 {
	s.seq++
	return s.seq + 1000
}

func accessOf(id int64) int64 {
	h := uint64(id)*0x9E3779B185EBCA87 + 0xC2B2AE3D27D4EB4F
	out := int64(h & 0x7fffffffffffffff)
	if out == 0 {
		return 1
	}
	return out
}

type idVer struct {
	id  int64
	ver int64
}

func hashPairs(pairs []idVer) int64 {
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].id == pairs[j].id {
			return pairs[i].ver < pairs[j].ver
		}
		return pairs[i].id < pairs[j].id
	})
	var h uint64 = 1469598103934665603
	for _, p := range pairs {
		h = (h ^ uint64(p.id)) * 1099511628211
		h = (h ^ uint64(p.ver)) * 1099511628211
	}
	out := int64(h & 0x7fffffffffffffff)
	if out == 0 {
		return 1
	}
	return out
}

// EmptyHash is the catalog hash when the list has no rows.
var EmptyHash = hashPairs(nil)

func notModified(client, current int64) bool {
	return client != 0 && client == current
}

func themesReply(hash int64, themes []*mtproto.Theme) *mtproto.Account_Themes {
	if themes == nil {
		themes = []*mtproto.Theme{}
	}
	return mtproto.MakeTLAccountThemes(&mtproto.Account_Themes{
		Hash:   hash,
		Themes: themes,
	}).To_Account_Themes()
}

func themesNotModified() *mtproto.Account_Themes {
	return mtproto.MakeTLAccountThemesNotModified(&mtproto.Account_Themes{}).To_Account_Themes()
}
