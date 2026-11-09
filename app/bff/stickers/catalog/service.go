package catalog

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/emojiblock"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Service answers the sticker and emoji RPCs from one in-memory catalog.
type Service struct {
	mu           sync.Mutex
	packs        []*Pack
	byID         map[int64]*Pack
	byShort      map[string]*Pack
	docs         map[int64]*linkedDoc
	users        map[int64]*userState
	reactionUses map[string]int64
	persist      persister
	now          func() int32
}

type linkedDoc struct {
	item *Item
	pack *Pack
}

type userState struct {
	install        map[int64]installRow
	recent         []recentRow
	faved          []favedRow
	reactionRecent []string
	featuredRead   map[int64]bool
	gifs           []gifRow
}

type installRow struct {
	Installed bool
	Archived  bool
	Order     int32
}

type recentRow struct {
	DocumentID int64
	Attached   bool
	Date       int32
}

type favedRow struct {
	DocumentID int64
	Date       int32
}

type persister interface {
	saveInstall(userID, packID int64, row installRow) error
	saveRecent(userID int64, row recentRow, remove bool) error
	saveFaved(userID, documentID int64, date int32, remove bool) error
}

func newService(packs []*Pack) *Service {
	s := &Service{
		byID:         map[int64]*Pack{},
		byShort:      map[string]*Pack{},
		docs:         map[int64]*linkedDoc{},
		users:        map[int64]*userState{},
		reactionUses: map[string]int64{},
		now:          func() int32 { return int32(time.Now().Unix()) },
	}
	s.packs = packs
	for _, p := range packs {
		s.byID[p.ID] = p
		s.byShort[strings.ToLower(p.ShortName)] = p
		for _, item := range p.Items {
			if _, ok := s.docs[item.DocumentID]; !ok {
				s.docs[item.DocumentID] = &linkedDoc{item: item, pack: p}
			}
		}
	}
	return s
}

func (s *Service) state(userID int64) *userState {
	st := s.users[userID]
	if st == nil {
		st = &userState{install: map[int64]installRow{}}
		s.users[userID] = st
	}
	return st
}

func (s *Service) installed(userID int64, p *Pack) (on, archived bool, order int32) {
	if row, ok := s.state(userID).install[p.ID]; ok {
		return row.Installed, row.Archived, row.Order
	}
	if p.InstalledDefault {
		return true, false, 0
	}
	return false, false, 0
}

func (s *Service) AllStickers(userID, hash int64) *mtproto.Messages_AllStickers {
	return s.listStickers(userID, hash, false)
}

func (s *Service) EmojiStickers(userID, hash int64) *mtproto.Messages_AllStickers {
	return s.listStickers(userID, hash, true)
}

func (s *Service) listStickers(userID, hash int64, emoji bool) *mtproto.Messages_AllStickers {
	return s.listInstalled(userID, hash, func(p *Pack) bool {
		return !p.system() && !p.foreignSeed() && p.Emojis == emoji && !p.Masks
	})
}

func (s *Service) listInstalled(userID, hash int64, match func(*Pack) bool) *mtproto.Messages_AllStickers {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listInstalledLocked(userID, hash, match)
}

func (s *Service) listInstalledLocked(userID, hash int64, match func(*Pack) bool) *mtproto.Messages_AllStickers {
	type row struct {
		p     *Pack
		order int32
	}
	var rows []row
	for _, p := range s.packs {
		if !match(p) {
			continue
		}
		on, archived, order := s.installed(userID, p)
		if !on || archived {
			continue
		}
		rows = append(rows, row{p: p, order: order})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].order != rows[j].order {
			return rows[i].order < rows[j].order
		}
		return rows[i].p.ID < rows[j].p.ID
	})
	// Hash the visible pack ids before building StickerSet objects. A client
	// that already has this install list gets allStickersNotModified without
	// walking every document into a TL value. getAllStickers has no offset;
	// the objects themselves are set metadata, not the sticker bytes.
	var kept []row
	var ids []int64
	for _, row := range rows {
		var keep bool
		ids, keep = appendPackKey(ids, row.p)
		if !keep {
			continue
		}
		kept = append(kept, row)
	}
	sum := hash64(ids)
	if hash != 0 && hash == sum {
		return mtproto.MakeTLMessagesAllStickersNotModified(&mtproto.Messages_AllStickers{}).To_Messages_AllStickers()
	}
	sets := make([]*mtproto.StickerSet, 0, len(kept))
	for _, row := range kept {
		sets = append(sets, row.p.tl(true, false))
	}
	return mtproto.MakeTLMessagesAllStickers(&mtproto.Messages_AllStickers{
		Hash: sum,
		Sets: sets,
	}).To_Messages_AllStickers()
}

// featuredPageSize is the first page messages.getFeaturedStickers returns.
// The sticker panel replaces its trending list with that page. Further
// pages are messages.getOldFeaturedStickers. count is the full list.
const featuredPageSize = 100

func (s *Service) Featured(userID, hash int64, emoji bool) *mtproto.Messages_FeaturedStickers {
	return s.featuredPage(userID, hash, emoji, 0, featuredPageSize, true)
}

// OldFeatured answers messages.getOldFeaturedStickers. offset is an index
// into the same featured list, and limit is the page size (default and max
// are featuredPageSize).
func (s *Service) OldFeatured(userID, hash int64, offset, limit int32) *mtproto.Messages_FeaturedStickers {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > featuredPageSize {
		limit = featuredPageSize
	}
	return s.featuredPage(userID, hash, false, offset, limit, false)
}

func (s *Service) featuredPage(userID, hash int64, emoji bool, offset, limit int32, firstPage bool) *mtproto.Messages_FeaturedStickers {
	s.mu.Lock()
	defer s.mu.Unlock()
	type feat struct {
		p      *Pack
		unread bool
	}
	var all []feat
	var ids []int64
	seen := s.state(userID).featuredRead
	for _, p := range s.packs {
		if !p.Featured || p.system() || p.foreignSeed() || p.Emojis != emoji {
			continue
		}
		var keep bool
		ids, keep = appendPackKey(ids, p)
		if !keep {
			continue
		}
		unread := seen == nil || !seen[p.ID]
		if unread {
			ids = append(ids, p.ID)
		}
		all = append(all, feat{p: p, unread: unread})
	}
	total := len(all)
	// The hash covers the whole featured list, including unread, so a new
	// pack or a read changes it. Building covers is deferred until the hash
	// misses, and then only the requested page is encoded.
	sum := hash64(ids)
	if !firstPage {
		var pageIDs []int64
		page := sliceFeat(all, offset, limit)
		for _, row := range page {
			var keep bool
			pageIDs, keep = appendPackKey(pageIDs, row.p)
			if !keep {
				continue
			}
			if row.unread {
				pageIDs = append(pageIDs, row.p.ID)
			}
		}
		sum = hash64(pageIDs)
	}
	if hash != 0 && hash == sum {
		return mtproto.MakeTLMessagesFeaturedStickersNotModified(&mtproto.Messages_FeaturedStickers{
			Count: int32(total),
		}).To_Messages_FeaturedStickers()
	}
	page := sliceFeat(all, offset, limit)
	sets := make([]*mtproto.StickerSetCovered, 0, len(page))
	var unread []int64
	for _, row := range page {
		sets = append(sets, row.p.covered(false, false))
		if row.unread {
			unread = append(unread, row.p.ID)
		}
	}
	if unread == nil {
		unread = []int64{}
	}
	return mtproto.MakeTLMessagesFeaturedStickers(&mtproto.Messages_FeaturedStickers{
		Count:  int32(total),
		Hash:   sum,
		Sets:   sets,
		Unread: unread,
	}).To_Messages_FeaturedStickers()
}

func sliceFeat[T any](all []T, offset, limit int32) []T {
	if offset < 0 {
		offset = 0
	}
	if int(offset) >= len(all) || limit <= 0 {
		return nil
	}
	end := int(offset) + int(limit)
	if end > len(all) {
		end = len(all)
	}
	return all[int(offset):end]
}

func (s *Service) GetSet(userID int64, in *mtproto.InputStickerSet, hash int32) (*mtproto.Messages_StickerSet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.lookup(in)
	if err != nil {
		return nil, err
	}
	sum := int32(hash64(p.docIDs()))
	if hash != 0 && hash == sum {
		return mtproto.MakeTLMessagesStickerSetNotModified(&mtproto.Messages_StickerSet{}).To_Messages_StickerSet(), nil
	}
	on, archived, _ := s.installed(userID, p)
	if p.system() {
		on, archived = true, false
	}
	return p.full(on, archived), nil
}

func (s *Service) lookup(in *mtproto.InputStickerSet) (*Pack, error) {
	if in == nil {
		return nil, mtproto.ErrStickersetInvalid
	}
	var p *Pack
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputStickerSetID:
		p = s.byID[in.GetId()]
		if p != nil && in.GetAccessHash() != 0 && in.GetAccessHash() != p.AccessHash {
			return nil, mtproto.ErrStickersetInvalid
		}
	case mtproto.Predicate_inputStickerSetShortName:
		p = s.byShort[strings.ToLower(in.GetShortName())]
	case mtproto.Predicate_inputStickerSetAnimatedEmoji:
		p = s.byShort[strings.ToLower(ShortAnimatedEmoji)]
	case mtproto.Predicate_inputStickerSetAnimatedEmojiAnimations:
		p = s.byShort[strings.ToLower(ShortAnimatedAnimations)]
	case mtproto.Predicate_inputStickerSetEmojiGenericAnimations:
		p = s.byShort[strings.ToLower(ShortGenericAnimations)]
	case mtproto.Predicate_inputStickerSetPremiumGifts:
		p = s.byShort[strings.ToLower(ShortPremiumGifts)]
	case mtproto.Predicate_inputStickerSetEmojiDefaultStatuses:
		p = s.byShort[strings.ToLower(ShortDefaultStatuses)]
	case mtproto.Predicate_inputStickerSetEmojiDefaultTopicIcons:
		p = s.byShort[strings.ToLower(ShortTopicIcons)]
	case mtproto.Predicate_inputStickerSetEmojiChannelDefaultStatuses:
		p = s.byShort[strings.ToLower(ShortChannelStatuses)]
	case mtproto.Predicate_inputStickerSetDice:
		// Clients request this while rendering 🎲 and the other dice.
		// A dedicated face sheet is not shipped; the animated emoji set
		// answers the RPC so session does not fail it as STICKERSET_INVALID.
		p = s.byShort[strings.ToLower(ShortDice)]
	case mtproto.Predicate_inputStickerSetTonGifts:
		p = s.byShort[strings.ToLower(ShortTonGifts)]
	default:
		return nil, mtproto.ErrStickersetInvalid
	}
	if p == nil || len(p.Items) == 0 {
		return nil, mtproto.ErrStickersetInvalid
	}
	return p, nil
}

func (s *Service) Install(userID int64, in *mtproto.InputStickerSet, archived bool) (*mtproto.Messages_StickerSetInstallResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.lookup(in)
	if err != nil {
		return nil, err
	}
	if p.system() {
		return mtproto.MakeTLMessagesStickerSetInstallResultSuccess(&mtproto.Messages_StickerSetInstallResult{
			Sets: []*mtproto.StickerSetCovered{},
		}).To_Messages_StickerSetInstallResult(), nil
	}
	row := installRow{Installed: true, Archived: archived, Order: s.now()}
	if s.persist != nil {
		if err = s.persist.saveInstall(userID, p.ID, row); err != nil {
			return nil, err
		}
	}
	s.state(userID).install[p.ID] = row
	return mtproto.MakeTLMessagesStickerSetInstallResultSuccess(&mtproto.Messages_StickerSetInstallResult{
		Sets: []*mtproto.StickerSetCovered{},
	}).To_Messages_StickerSetInstallResult(), nil
}

func (s *Service) Uninstall(userID int64, in *mtproto.InputStickerSet) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.lookup(in)
	if err != nil {
		return err
	}
	row := installRow{Installed: false}
	if s.persist != nil {
		if err = s.persist.saveInstall(userID, p.ID, row); err != nil {
			return err
		}
	}
	s.state(userID).install[p.ID] = row
	return nil
}

func (s *Service) SearchSets(q string, hash int64, emojiOnly bool) *mtproto.Messages_FoundStickerSets {
	s.mu.Lock()
	defer s.mu.Unlock()
	q = foldText(q)
	var matched []*Pack
	var ids []int64
	for _, p := range s.packs {
		if p.system() || p.foreignSeed() {
			continue
		}
		if p.Emojis != emojiOnly {
			continue
		}
		if q != "" && !strings.Contains(foldText(p.Title), q) && !strings.Contains(foldText(p.ShortName), q) && !packMatchesQuery(p, q) {
			continue
		}
		var keep bool
		ids, keep = appendPackKey(ids, p)
		if !keep {
			continue
		}
		matched = append(matched, p)
	}
	sum := hash64(ids)
	if hash != 0 && hash == sum {
		return mtproto.MakeTLMessagesFoundStickerSetsNotModified(&mtproto.Messages_FoundStickerSets{}).To_Messages_FoundStickerSets()
	}
	sets := make([]*mtproto.StickerSetCovered, 0, len(matched))
	for _, p := range matched {
		sets = append(sets, p.covered(false, false))
	}
	return mtproto.MakeTLMessagesFoundStickerSets(&mtproto.Messages_FoundStickerSets{
		Hash: sum,
		Sets: sets,
	}).To_Messages_FoundStickerSets()
}

// packMatchesQuery is true when a sticker in the pack carries the query as
// an emoticon or as a keyword. The sticker panel's pack search sends the
// typed text here, including Chinese words and a single emoji.
func packMatchesQuery(p *Pack, q string) bool {
	if p == nil || q == "" {
		return false
	}
	langs := searchLangs(nil, q)
	for _, item := range p.visibleItems() {
		if item == nil {
			continue
		}
		if emojiEqual(item.Emoticon, q) || item.keywordHit(q, langs) {
			return true
		}
	}
	return false
}

func (s *Service) StickersByEmoji(emoticon string, hash int64, custom bool) *mtproto.Messages_Stickers {
	s.mu.Lock()
	defer s.mu.Unlock()
	var docs []*mtproto.Document
	var ids []int64
	seen := map[int64]bool{}
	if emojiblock.Blocked(emoticon) {
		return stickersResult(nil, nil, hash)
	}
	for _, p := range s.packs {
		if p.system() || p.foreignSeed() {
			continue
		}
		if custom != p.Emojis {
			continue
		}
		for _, item := range p.Items {
			if !emojiEqual(item.Emoticon, emoticon) || seen[item.DocumentID] || emojiblock.Blocked(item.Emoticon) {
				continue
			}
			seen[item.DocumentID] = true
			docs = append(docs, item.document(p))
			ids = append(ids, item.DocumentID)
		}
	}
	return stickersResult(docs, ids, hash)
}

func stickersResult(docs []*mtproto.Document, ids []int64, hash int64) *mtproto.Messages_Stickers {
	sum := hash64(ids)
	if hash != 0 && hash == sum {
		return mtproto.MakeTLMessagesStickersNotModified(&mtproto.Messages_Stickers{}).To_Messages_Stickers()
	}
	if docs == nil {
		docs = []*mtproto.Document{}
	}
	return mtproto.MakeTLMessagesStickers(&mtproto.Messages_Stickers{Hash: sum, Stickers: docs}).To_Messages_Stickers()
}

// searchPageSize is the default and maximum page for messages.searchStickers.
// Android sends limit 100 and follows next_offset.
const searchPageSize = 100

// SearchDocuments is the unpaged keyword search used by in-process tests.
func (s *Service) SearchDocuments(q string, emoji bool, hash int64) *mtproto.Messages_FoundStickers {
	return s.SearchStickers(q, "", nil, emoji, 0, 0, hash)
}

// SearchStickers answers messages.searchStickers.
// q is the text typed in the sticker panel. emoticon is the concatenation
// of emoji that the client's local keyword list already matched. langCodes
// are the keyboard languages (Android sends one, often zh-CN or zh-Hans).
// limit 0 returns every match and omits next_offset; a positive limit pages.
func (s *Service) SearchStickers(q, emoticon string, langCodes []string, emoji bool, offset, limit int32, hash int64) *mtproto.Messages_FoundStickers {
	s.mu.Lock()
	defer s.mu.Unlock()
	q = strings.TrimSpace(q)
	langs := searchLangs(langCodes, q)
	type hit struct {
		item *Item
		pack *Pack
	}
	var hits []hit
	seen := map[int64]bool{}
	if q == "" && strings.TrimSpace(emoticon) == "" {
		return stickersFound(nil, nil, hash, 0, false)
	}
	for _, p := range s.packs {
		if p.system() || p.foreignSeed() || p.Emojis != emoji {
			continue
		}
		for _, item := range p.Items {
			if seen[item.DocumentID] || emojiblock.Blocked(item.Emoticon) {
				continue
			}
			if !stickerMatches(item, q, emoticon, langs) {
				continue
			}
			seen[item.DocumentID] = true
			hits = append(hits, hit{item: item, pack: p})
		}
	}
	more := false
	next := int32(0)
	if limit > 0 {
		if offset < 0 {
			offset = 0
		}
		if int(offset) >= len(hits) {
			return stickersFound(nil, nil, hash, 0, false)
		}
		end := int(offset) + int(limit)
		if end < len(hits) {
			more = true
			next = int32(end)
		} else {
			end = len(hits)
		}
		hits = hits[int(offset):end]
	}
	docs := make([]*mtproto.Document, 0, len(hits))
	ids := make([]int64, 0, len(hits))
	for _, h := range hits {
		docs = append(docs, h.item.document(h.pack))
		ids = append(ids, h.item.DocumentID)
	}
	return stickersFound(docs, ids, hash, next, more)
}

func stickerMatches(item *Item, q, emoticon string, langs []string) bool {
	if q != "" && (emojiEqual(item.Emoticon, q) || item.keywordHit(q, langs)) {
		return true
	}
	if emoticon != "" && emojiListed(item.Emoticon, emoticon) {
		return true
	}
	return false
}

func stickersFound(docs []*mtproto.Document, ids []int64, hash int64, next int32, more bool) *mtproto.Messages_FoundStickers {
	sum := hash64(ids)
	if hash != 0 && hash == sum {
		out := &mtproto.Messages_FoundStickers{}
		if more {
			out.NextOffset = wrapperspb.Int32(next)
		}
		return mtproto.MakeTLMessagesFoundStickersNotModified(out).To_Messages_FoundStickers()
	}
	if docs == nil {
		docs = []*mtproto.Document{}
	}
	out := &mtproto.Messages_FoundStickers{Hash: sum, Stickers: docs}
	if more {
		out.NextOffset = wrapperspb.Int32(next)
	}
	return mtproto.MakeTLMessagesFoundStickers(out).To_Messages_FoundStickers()
}

func (s *Service) Recent(userID, hash int64, attached bool) *mtproto.Messages_RecentStickers {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recentLocked(userID, hash, attached)
}

func (s *Service) recentLocked(userID, hash int64, attached bool) *mtproto.Messages_RecentStickers {
	var docs []*mtproto.Document
	var packs []*mtproto.StickerPack
	var dates []int32
	var ids []int64
	for _, row := range s.state(userID).recent {
		if row.Attached != attached {
			continue
		}
		link := s.docs[row.DocumentID]
		if link == nil || emojiblock.Blocked(link.item.Emoticon) {
			continue
		}
		docs = append(docs, link.item.document(link.pack))
		packs = append(packs, mtproto.MakeTLStickerPack(&mtproto.StickerPack{
			Emoticon:  link.item.Emoticon,
			Documents: []int64{link.item.DocumentID},
		}).To_StickerPack())
		dates = append(dates, row.Date)
		ids = append(ids, row.DocumentID)
	}
	sum := hash64(ids)
	if hash != 0 && hash == sum {
		return mtproto.MakeTLMessagesRecentStickersNotModified(&mtproto.Messages_RecentStickers{}).To_Messages_RecentStickers()
	}
	if docs == nil {
		docs = []*mtproto.Document{}
		packs = []*mtproto.StickerPack{}
		dates = []int32{}
	}
	return mtproto.MakeTLMessagesRecentStickers(&mtproto.Messages_RecentStickers{
		Hash:     sum,
		Packs:    packs,
		Stickers: docs,
		Dates:    dates,
	}).To_Messages_RecentStickers()
}

func (s *Service) SaveRecent(userID, documentID int64, attached, unsave bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.docs[documentID] == nil {
		return mtproto.ErrStickerIdInvalid
	}
	st := s.state(userID)
	next := st.recent[:0]
	for _, row := range st.recent {
		if row.DocumentID == documentID && row.Attached == attached {
			continue
		}
		next = append(next, row)
	}
	row := recentRow{DocumentID: documentID, Attached: attached, Date: s.now()}
	if !unsave {
		next = append([]recentRow{row}, next...)
		if len(next) > 50 {
			next = next[:50]
		}
	}
	if s.persist != nil {
		if err := s.persist.saveRecent(userID, row, unsave); err != nil {
			return err
		}
	}
	st.recent = next
	return nil
}

func (s *Service) ClearRecent(userID int64, attached bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.state(userID)
	var keep []recentRow
	for _, row := range st.recent {
		if row.Attached == attached {
			if s.persist != nil {
				if err := s.persist.saveRecent(userID, row, true); err != nil {
					return err
				}
			}
			continue
		}
		keep = append(keep, row)
	}
	st.recent = keep
	return nil
}

func (s *Service) Faved(userID, hash int64) *mtproto.Messages_FavedStickers {
	s.mu.Lock()
	defer s.mu.Unlock()
	var docs []*mtproto.Document
	var packs []*mtproto.StickerPack
	var ids []int64
	for _, row := range s.state(userID).faved {
		link := s.docs[row.DocumentID]
		if link == nil || emojiblock.Blocked(link.item.Emoticon) {
			continue
		}
		docs = append(docs, link.item.document(link.pack))
		packs = append(packs, mtproto.MakeTLStickerPack(&mtproto.StickerPack{
			Emoticon:  link.item.Emoticon,
			Documents: []int64{link.item.DocumentID},
		}).To_StickerPack())
		ids = append(ids, row.DocumentID)
	}
	sum := hash64(ids)
	if hash != 0 && hash == sum {
		return mtproto.MakeTLMessagesFavedStickersNotModified(&mtproto.Messages_FavedStickers{}).To_Messages_FavedStickers()
	}
	if docs == nil {
		docs = []*mtproto.Document{}
		packs = []*mtproto.StickerPack{}
	}
	return mtproto.MakeTLMessagesFavedStickers(&mtproto.Messages_FavedStickers{
		Hash:     sum,
		Packs:    packs,
		Stickers: docs,
	}).To_Messages_FavedStickers()
}

func (s *Service) Fave(userID, documentID int64, unfave bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.docs[documentID] == nil {
		return mtproto.ErrStickerIdInvalid
	}
	st := s.state(userID)
	var next []favedRow
	for _, row := range st.faved {
		if row.DocumentID != documentID {
			next = append(next, row)
		}
	}
	date := s.now()
	if !unfave {
		next = append([]favedRow{{DocumentID: documentID, Date: date}}, next...)
		if len(next) > 200 {
			next = next[:200]
		}
	}
	if s.persist != nil {
		if err := s.persist.saveFaved(userID, documentID, date, unfave); err != nil {
			return err
		}
	}
	st.faved = next
	return nil
}

func (s *Service) Documents(ids []int64) *mtproto.Vector_Document {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := &mtproto.Vector_Document{Datas: []*mtproto.Document{}}
	for _, id := range ids {
		link := s.docs[id]
		if link == nil || emojiblock.Blocked(link.item.Emoticon) {
			out.Datas = append(out.Datas, mtproto.MakeTLDocumentEmpty(&mtproto.Document{Id: id}).To_Document())
			continue
		}
		out.Datas = append(out.Datas, link.item.document(link.pack))
	}
	return out
}

func (s *Service) docByFamily(emoticon, family string) *linkedDoc {
	for _, link := range s.docs {
		if link.item.Emoticon != emoticon {
			continue
		}
		if strings.HasPrefix(link.item.AssetPath, "assets/"+family+"/") {
			return link
		}
	}
	return nil
}

func (s *Service) ThumbLocation(in *mtproto.InputStickerSet) (*mtproto.InputFileLocation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.lookup(in)
	if err != nil {
		return nil, err
	}
	items := p.visibleItems()
	if len(items) == 0 {
		return nil, mtproto.ErrStickersetInvalid
	}
	item := items[0]
	if p.ThumbID != 0 {
		for _, it := range items {
			if it.DocumentID == p.ThumbID {
				item = it
				break
			}
		}
	}
	return mtproto.MakeTLInputDocumentFileLocation(&mtproto.InputFileLocation{
		Id:         item.DocumentID,
		AccessHash: item.AccessHash,
	}).To_InputFileLocation(), nil
}

func (s *Service) CustomEmojiIDs(emoticon string) []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if emojiblock.Blocked(emoticon) {
		return nil
	}
	var ids []int64
	seen := map[int64]bool{}
	for _, link := range s.docs {
		if !link.item.CustomEmoji || link.item.Emoticon != emoticon || seen[link.item.DocumentID] || emojiblock.Blocked(link.item.Emoticon) {
			continue
		}
		seen[link.item.DocumentID] = true
		ids = append(ids, link.item.DocumentID)
	}
	return ids
}
