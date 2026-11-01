package book

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"strconv"

	"github.com/teamgram/proto/mtproto"
)

// Top peers.
// https://core.telegram.org/method/contacts.getTopPeers
// https://core.telegram.org/method/contacts.resetTopPeerRating
// https://core.telegram.org/method/contacts.toggleTopPeers
//
// Correspondents, bots in private chats, groups, and channels are ranked from
// the user's dialogs (more recently active dialogs rank higher). Phone calls,
// forwards, inline bots, main mini apps, and guest bots have no event stream
// on this server yet: their ratings live in this book and stay empty until
// Bump records one. resetTopPeerRating forces a peer to zero until the next
// Bump. toggleTopPeers makes getTopPeers return contacts.topPeersDisabled.

const (
	CatCorrespondents = "correspondents"
	CatBotsPM         = "bots_pm"
	CatBotsInline     = "bots_inline"
	CatPhoneCalls     = "phone_calls"
	CatForwardUsers   = "forward_users"
	CatForwardChats   = "forward_chats"
	CatGroups         = "groups"
	CatChannels       = "channels"
	CatBotsApp        = "bots_app"
	CatBotsGuest      = "bots_guestchat"

	topKeyPrefix = "contacts_top_v1:"
	// Rating half-life window used when a recorded event is read back.
	// A week of silence roughly halves an event rating.
	topDecaySeconds = 7 * 24 * 3600
)

// Schema constructor ids, used as the hash salt for a category.
// https://core.telegram.org/type/TopPeerCategory
var catCRC = map[string]int64{
	CatCorrespondents: 0x0637b7ed,
	CatBotsPM:         0xab661b5b,
	CatBotsInline:     0x148677e2,
	CatPhoneCalls:     0x1e76a78c,
	CatForwardUsers:   0xa8406ca9,
	CatForwardChats:   0xfbeec0f0,
	CatGroups:         0xbd17a14a,
	CatChannels:       0x161d9628,
	CatBotsApp:        0xfd9e7bec,
	CatBotsGuest:      0x6c24f3dd,
}

// Dialog-backed categories. The rest only move when Bump is called.
var dialogCats = map[string]bool{
	CatCorrespondents: true,
	CatBotsPM:         true,
	CatGroups:         true,
	CatChannels:       true,
}

// Peer is a user, basic group, or channel.
type Peer struct {
	Type int32
	ID   int64
}

func (p Peer) key() string {
	return strconv.FormatInt(int64(p.Type), 10) + ":" + strconv.FormatInt(p.ID, 10)
}

// Hit is one dialog the ranker can score.
type Hit struct {
	Peer      Peer
	Date      int64
	Bot       bool
	Megagroup bool
}

// Rated is one peer inside a category page.
type Rated struct {
	Peer   Peer
	Rating float64
}

// Ranked is one requested category after offset/limit.
type Ranked struct {
	Cat   string
	CRC   int64
	Total int
	Peers []Rated
}

type topRec struct {
	Disabled bool       `json:"disabled,omitempty"`
	Zero     []string   `json:"zero,omitempty"`
	Extra    []extraRow `json:"extra,omitempty"`
}

type extraRow struct {
	Cat     string  `json:"cat"`
	Type    int32   `json:"type"`
	ID      int64   `json:"id"`
	Rating  float64 `json:"rating"`
	Updated int64   `json:"updated"`
}

// KnownCategory reports whether name is a layer-232 topPeerCategory.
func KnownCategory(name string) bool {
	_, ok := catCRC[name]
	return ok
}

// CategoryOf maps a TopPeerCategory predicate to the book name.
func CategoryOf(cat *mtproto.TopPeerCategory) string {
	if cat == nil {
		return ""
	}
	switch cat.GetPredicateName() {
	case mtproto.Predicate_topPeerCategoryCorrespondents:
		return CatCorrespondents
	case mtproto.Predicate_topPeerCategoryBotsPM:
		return CatBotsPM
	case mtproto.Predicate_topPeerCategoryBotsInline:
		return CatBotsInline
	case mtproto.Predicate_topPeerCategoryPhoneCalls:
		return CatPhoneCalls
	case mtproto.Predicate_topPeerCategoryForwardUsers:
		return CatForwardUsers
	case mtproto.Predicate_topPeerCategoryForwardChats:
		return CatForwardChats
	case mtproto.Predicate_topPeerCategoryGroups:
		return CatGroups
	case mtproto.Predicate_topPeerCategoryChannels:
		return CatChannels
	case mtproto.Predicate_topPeerCategoryBotsApp:
		return CatBotsApp
	case mtproto.Predicate_topPeerCategoryBotsGuestChat:
		return CatBotsGuest
	default:
		return ""
	}
}

// SetTopEnabled stores the contacts.toggleTopPeers switch. The default is on.
func (b *Book) SetTopEnabled(ctx context.Context, user int64, enabled bool) error {
	rec := b.loadTop(ctx, user)
	rec.Disabled = !enabled
	return b.saveTop(ctx, user, rec)
}

// TopEnabled reports whether top peers are switched on. Missing state is on.
func (b *Book) TopEnabled(ctx context.Context, user int64) bool {
	return !b.loadTop(ctx, user).Disabled
}

// ResetTop forces one peer's rating in a category to zero.
func (b *Book) ResetTop(ctx context.Context, user int64, cat string, peer Peer) error {
	if !KnownCategory(cat) || peer.ID == 0 {
		return mtproto.ErrPeerIdInvalid
	}
	rec := b.loadTop(ctx, user)
	token := cat + "|" + peer.key()
	if !contains(rec.Zero, token) {
		rec.Zero = append(rec.Zero, token)
	}
	kept := rec.Extra[:0]
	for _, row := range rec.Extra {
		if row.Cat == cat && row.Type == peer.Type && row.ID == peer.ID {
			continue
		}
		kept = append(kept, row)
	}
	rec.Extra = kept
	return b.saveTop(ctx, user, rec)
}

// Bump records one use of a peer in a category and clears a previous reset.
// Dialog-backed categories reappear at their dialog date. Event categories
// gain one point, decayed from the previous sample.
func (b *Book) Bump(ctx context.Context, user int64, cat string, peer Peer) error {
	if !KnownCategory(cat) || peer.ID == 0 {
		return mtproto.ErrPeerIdInvalid
	}
	rec := b.loadTop(ctx, user)
	token := cat + "|" + peer.key()
	rec.Zero = remove(rec.Zero, token)
	if !dialogCats[cat] {
		now := b.now().Unix()
		found := false
		for i := range rec.Extra {
			row := &rec.Extra[i]
			if row.Cat != cat || row.Type != peer.Type || row.ID != peer.ID {
				continue
			}
			row.Rating = decay(row.Rating, row.Updated, now) + 1
			row.Updated = now
			found = true
			break
		}
		if !found {
			rec.Extra = append(rec.Extra, extraRow{
				Cat: cat, Type: peer.Type, ID: peer.ID, Rating: 1, Updated: now,
			})
		}
	}
	return b.saveTop(ctx, user, rec)
}

// Rank builds the requested categories. offset and limit page each category.
// Empty categories are left out. Disabled users are the caller's problem:
// Rank still returns scores so tests can see them.
func (b *Book) Rank(ctx context.Context, user int64, cats []string, hits []Hit, offset, limit int) []Ranked {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	rec := b.loadTop(ctx, user)
	zero := map[string]bool{}
	for _, z := range rec.Zero {
		zero[z] = true
	}
	now := b.now().Unix()
	var out []Ranked
	for _, cat := range cats {
		if !KnownCategory(cat) {
			continue
		}
		all := scoreCat(cat, hits, rec, zero, now)
		page := Ranked{Cat: cat, CRC: catCRC[cat], Total: len(all)}
		if offset < len(all) {
			end := offset + limit
			if end > len(all) {
				end = len(all)
			}
			page.Peers = all[offset:end]
		}
		if page.Total == 0 {
			continue
		}
		out = append(out, page)
	}
	return out
}

func scoreCat(cat string, hits []Hit, rec topRec, zero map[string]bool, now int64) []Rated {
	var all []Rated
	if dialogCats[cat] {
		for _, hit := range hits {
			if !hitMatches(cat, hit) || hit.Date <= 0 || hit.Peer.ID == 0 {
				continue
			}
			if zero[cat+"|"+hit.Peer.key()] {
				continue
			}
			all = append(all, Rated{Peer: hit.Peer, Rating: float64(hit.Date)})
		}
	}
	for _, row := range rec.Extra {
		if row.Cat != cat || row.ID == 0 {
			continue
		}
		peer := Peer{Type: row.Type, ID: row.ID}
		if zero[cat+"|"+peer.key()] {
			continue
		}
		rating := decay(row.Rating, row.Updated, now)
		if rating < 0.01 {
			continue
		}
		all = append(all, Rated{Peer: peer, Rating: rating})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Rating != all[j].Rating {
			return all[i].Rating > all[j].Rating
		}
		if all[i].Peer.Type != all[j].Peer.Type {
			return all[i].Peer.Type < all[j].Peer.Type
		}
		return all[i].Peer.ID < all[j].Peer.ID
	})
	return all
}

func hitMatches(cat string, hit Hit) bool {
	switch cat {
	case CatCorrespondents:
		return hit.Peer.Type == mtproto.PEER_USER && !hit.Bot
	case CatBotsPM:
		return hit.Peer.Type == mtproto.PEER_USER && hit.Bot
	case CatGroups:
		return hit.Peer.Type == mtproto.PEER_CHAT || (hit.Peer.Type == mtproto.PEER_CHANNEL && hit.Megagroup)
	case CatChannels:
		return hit.Peer.Type == mtproto.PEER_CHANNEL && !hit.Megagroup
	default:
		return false
	}
}

func decay(rating float64, updated, now int64) float64 {
	if rating <= 0 {
		return 0
	}
	if updated <= 0 || now <= updated {
		return rating
	}
	return rating * math.Exp(-float64(now-updated)/topDecaySeconds)
}

// HashRanks is the cache hash clients send back.
// The mix is the Android long hash: hash ^= id>>21; hash ^= id<<35; hash ^= id>>4; hash += id.
func HashRanks(rows []Ranked) int64 {
	var hash int64
	for _, row := range rows {
		hash = mix(hash, row.CRC)
		for _, peer := range row.Peers {
			hash = mix(hash, int64(peer.Peer.Type))
			hash = mix(hash, peer.Peer.ID)
		}
	}
	return hash
}

func mix(hash, id int64) int64 {
	hash ^= id >> 21
	hash ^= id << 35
	hash ^= id >> 4
	return hash + id
}

func (b *Book) loadTop(ctx context.Context, user int64) topRec {
	raw, err := b.kv.GetCtx(ctx, topKeyPrefix+strconv.FormatInt(user, 10))
	if err != nil || raw == "" {
		return topRec{}
	}
	var rec topRec
	if json.Unmarshal([]byte(raw), &rec) != nil {
		return topRec{}
	}
	return rec
}

func (b *Book) saveTop(ctx context.Context, user int64, rec topRec) error {
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return b.kv.SetCtx(ctx, topKeyPrefix+strconv.FormatInt(user, 10), string(raw))
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func remove(ss []string, s string) []string {
	out := ss[:0]
	for _, v := range ss {
		if v != s {
			out = append(out, v)
		}
	}
	return out
}
