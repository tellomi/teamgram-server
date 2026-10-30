package catalog

import (
	"strings"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/emojiblock"
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
	s.mu.Lock()
	defer s.mu.Unlock()
	var sets []*mtproto.StickerSet
	var ids []int64
	for _, p := range s.packs {
		if p.system() || p.Emojis != emoji {
			continue
		}
		on, archived, _ := s.installed(userID, p)
		if !on || archived {
			continue
		}
		var keep bool
		ids, keep = appendPackKey(ids, p)
		if !keep {
			continue
		}
		sets = append(sets, p.tl(true, false))
	}
	sum := hash64(ids)
	if hash != 0 && hash == sum {
		return mtproto.MakeTLMessagesAllStickersNotModified(&mtproto.Messages_AllStickers{}).To_Messages_AllStickers()
	}
	if sets == nil {
		sets = []*mtproto.StickerSet{}
	}
	return mtproto.MakeTLMessagesAllStickers(&mtproto.Messages_AllStickers{
		Hash: sum,
		Sets: sets,
	}).To_Messages_AllStickers()
}

func (s *Service) Featured(hash int64, emoji bool) *mtproto.Messages_FeaturedStickers {
	s.mu.Lock()
	defer s.mu.Unlock()
	var sets []*mtproto.StickerSetCovered
	var ids []int64
	for _, p := range s.packs {
		if !p.Featured || p.system() || p.Emojis != emoji {
			continue
		}
		var keep bool
		ids, keep = appendPackKey(ids, p)
		if !keep {
			continue
		}
		sets = append(sets, p.covered(false, false))
	}
	sum := hash64(ids)
	if hash != 0 && hash == sum {
		return mtproto.MakeTLMessagesFeaturedStickersNotModified(&mtproto.Messages_FeaturedStickers{}).To_Messages_FeaturedStickers()
	}
	if sets == nil {
		sets = []*mtproto.StickerSetCovered{}
	}
	return mtproto.MakeTLMessagesFeaturedStickers(&mtproto.Messages_FeaturedStickers{
		Count: int32(len(sets)),
		Hash:  sum,
		Sets:  sets,
	}).To_Messages_FeaturedStickers()
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
	q = strings.ToLower(strings.TrimSpace(q))
	var sets []*mtproto.StickerSetCovered
	var ids []int64
	for _, p := range s.packs {
		if p.system() {
			continue
		}
		if emojiOnly && !p.Emojis {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(p.Title), q) && !strings.Contains(strings.ToLower(p.ShortName), q) {
			continue
		}
		var keep bool
		ids, keep = appendPackKey(ids, p)
		if !keep {
			continue
		}
		sets = append(sets, p.covered(false, false))
	}
	sum := hash64(ids)
	if hash != 0 && hash == sum {
		return mtproto.MakeTLMessagesFoundStickerSetsNotModified(&mtproto.Messages_FoundStickerSets{}).To_Messages_FoundStickerSets()
	}
	if sets == nil {
		sets = []*mtproto.StickerSetCovered{}
	}
	return mtproto.MakeTLMessagesFoundStickerSets(&mtproto.Messages_FoundStickerSets{
		Hash: sum,
		Sets: sets,
	}).To_Messages_FoundStickerSets()
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
		if p.system() {
			continue
		}
		if custom != p.Emojis {
			continue
		}
		for _, item := range p.Items {
			if item.Emoticon != emoticon || seen[item.DocumentID] || emojiblock.Blocked(item.Emoticon) {
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

func (s *Service) SearchDocuments(q string, emoji bool, hash int64) *mtproto.Messages_FoundStickers {
	s.mu.Lock()
	defer s.mu.Unlock()
	q = strings.ToLower(strings.TrimSpace(q))
	var docs []*mtproto.Document
	var ids []int64
	seen := map[int64]bool{}
	for _, p := range s.packs {
		if p.system() || p.Emojis != emoji {
			continue
		}
		for _, item := range p.Items {
			if seen[item.DocumentID] || emojiblock.Blocked(item.Emoticon) {
				continue
			}
			if q != "" && item.Emoticon != q && !hasKeyword(item, q) {
				continue
			}
			seen[item.DocumentID] = true
			docs = append(docs, item.document(p))
			ids = append(ids, item.DocumentID)
		}
	}
	sum := hash64(ids)
	if hash != 0 && hash == sum {
		return mtproto.MakeTLMessagesFoundStickersNotModified(&mtproto.Messages_FoundStickers{}).To_Messages_FoundStickers()
	}
	if docs == nil {
		docs = []*mtproto.Document{}
	}
	return mtproto.MakeTLMessagesFoundStickers(&mtproto.Messages_FoundStickers{
		Hash:     sum,
		Stickers: docs,
	}).To_Messages_FoundStickers()
}

func hasKeyword(item *Item, q string) bool {
	for _, word := range item.Keywords {
		if strings.Contains(strings.ToLower(word), q) {
			return true
		}
	}
	return false
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

func (s *Service) Keywords(lang string, from int32) *mtproto.EmojiKeywordsDifference {
	s.mu.Lock()
	defer s.mu.Unlock()
	version := emojiKeywordVersion()
	diff := &mtproto.EmojiKeywordsDifference{
		LangCode:    lang,
		FromVersion: from,
		Version:     version,
		Keywords:    []*mtproto.EmojiKeyword{},
	}
	if from >= version {
		return mtproto.MakeTLEmojiKeywordsDifference(diff).To_EmojiKeywordsDifference()
	}
	grouped := map[string][]string{}
	var order []string
	for _, link := range s.docs {
		if emojiblock.Blocked(link.item.Emoticon) {
			continue
		}
		for _, word := range link.item.Keywords {
			key := strings.ToLower(word)
			if _, ok := grouped[key]; !ok {
				order = append(order, key)
			}
			if !contains(grouped[key], link.item.Emoticon) {
				grouped[key] = append(grouped[key], link.item.Emoticon)
			}
		}
	}
	for _, key := range order {
		diff.Keywords = append(diff.Keywords, mtproto.MakeTLEmojiKeyword(&mtproto.EmojiKeyword{
			Keyword:   key,
			Emoticons: grouped[key],
		}).To_EmojiKeyword())
	}
	return mtproto.MakeTLEmojiKeywordsDifference(diff).To_EmojiKeywordsDifference()
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

func (s *Service) Languages() *mtproto.Vector_EmojiLanguage {
	return &mtproto.Vector_EmojiLanguage{Datas: []*mtproto.EmojiLanguage{
		mtproto.MakeTLEmojiLanguage(&mtproto.EmojiLanguage{LangCode: "en"}).To_EmojiLanguage(),
	}}
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
