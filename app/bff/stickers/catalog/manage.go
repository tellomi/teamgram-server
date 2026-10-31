package catalog

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	maxStaticStickers   = 120
	maxAnimatedStickers = 50
	maxEmojiStickers    = 200
	maxSavedGifs        = 200
	maxStickerTitle     = 64
	maxStickerEmoji     = 32
)

var shortNameRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{4,31}$`)

// gifRow is one saved GIF. The bytes stay in the media store; this is the
// catalog the saved-GIF panel lists.
type gifRow struct {
	DocumentID int64
	AccessHash int64
	Date       int32
	MIME       string
	Size       int32
	Width      int32
	Height     int32
	FileName   string
	Video      bool
}

// CreateInput is stickers.createStickerSet.
type CreateInput struct {
	Self      int64
	Owner     *mtproto.InputUser
	Title     string
	ShortName string
	Masks     bool
	Emojis    bool
	TextColor bool
	Animated  bool
	Videos    bool
	Thumb     *mtproto.InputDocument
	Stickers  []*mtproto.InputStickerSetItem
}

func (s *Service) MaskStickers(userID, hash int64) *mtproto.Messages_AllStickers {
	return s.listInstalled(userID, hash, func(p *Pack) bool {
		return p.Masks && !p.system()
	})
}

func (s *Service) ArchivedStickers(userID int64, masks, emojis bool, offsetID int64, limit int32) *mtproto.Messages_ArchivedStickers {
	s.mu.Lock()
	defer s.mu.Unlock()
	packs := s.archivedPacks(userID, masks, emojis)
	page, total := pagePacks(packs, offsetID, limit)
	sets := make([]*mtproto.StickerSetCovered, 0, len(page))
	for _, p := range page {
		sets = append(sets, p.covered(true, true))
	}
	return mtproto.MakeTLMessagesArchivedStickers(&mtproto.Messages_ArchivedStickers{
		Count: int32(total),
		Sets:  sets,
	}).To_Messages_ArchivedStickers()
}

func (s *Service) MyStickers(userID, offsetID int64, limit int32) *mtproto.Messages_MyStickers {
	s.mu.Lock()
	defer s.mu.Unlock()
	var owned []*Pack
	for _, p := range s.packs {
		if p.Owner != userID || p.system() || len(p.visibleItems()) == 0 {
			continue
		}
		owned = append(owned, p)
	}
	sort.Slice(owned, func(i, j int) bool { return owned[i].ID > owned[j].ID })
	page, total := pagePacks(owned, offsetID, limit)
	sets := make([]*mtproto.StickerSetCovered, 0, len(page))
	for _, p := range page {
		on, archived, _ := s.installed(userID, p)
		sets = append(sets, p.covered(on, archived))
	}
	return mtproto.MakeTLMessagesMyStickers(&mtproto.Messages_MyStickers{
		Count: int32(total),
		Sets:  sets,
	}).To_Messages_MyStickers()
}

func (s *Service) AttachedStickers(media *mtproto.InputStickeredMedia) (*mtproto.Vector_StickerSetCovered, error) {
	if media == nil || media.GetPredicateName() == "" {
		return nil, mtproto.ErrMediaEmpty
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := &mtproto.Vector_StickerSetCovered{Datas: []*mtproto.StickerSetCovered{}}
	switch media.GetPredicateName() {
	case mtproto.Predicate_inputStickeredMediaPhoto:
		photo := media.GetId_INPUTPHOTO()
		if photo == nil || photo.GetPredicateName() == mtproto.Predicate_inputPhotoEmpty || photo.GetId() == 0 {
			return nil, mtproto.ErrMediaInvalid
		}
		// Photos are not indexed to a sticker set. An empty vector is the
		// schema result when the picture has no attached stickers.
		return out, nil
	case mtproto.Predicate_inputStickeredMediaDocument:
		doc := media.GetId_INPUTDOCUMENT()
		if doc == nil || doc.GetPredicateName() == mtproto.Predicate_inputDocumentEmpty || doc.GetId() == 0 {
			return nil, mtproto.ErrMediaInvalid
		}
		link := s.docs[doc.GetId()]
		if link == nil || link.pack == nil || link.pack.system() {
			return out, nil
		}
		if doc.GetAccessHash() != 0 && doc.GetAccessHash() != link.item.AccessHash {
			return nil, mtproto.ErrMediaInvalid
		}
		out.Datas = append(out.Datas, link.pack.covered(false, false))
		return out, nil
	default:
		return nil, mtproto.ErrMediaInvalid
	}
}

func (s *Service) Reorder(userID int64, masks, emojis bool, order []int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(order) == 0 {
		return nil
	}
	pos := map[int64]int32{}
	for i, id := range order {
		if _, ok := pos[id]; ok {
			continue
		}
		pos[id] = int32(i)
	}
	var rest int32
	for _, p := range s.packs {
		if !kindMatch(p, masks, emojis) {
			continue
		}
		on, archived, prev := s.installed(userID, p)
		if !on || archived {
			continue
		}
		next, ok := pos[p.ID]
		if !ok {
			next = int32(len(order)) + rest
			rest++
		}
		if next == prev {
			continue
		}
		row := installRow{Installed: true, Archived: false, Order: next}
		if s.persist != nil {
			if err := s.persist.saveInstall(userID, p.ID, row); err != nil {
				return err
			}
		}
		s.state(userID).install[p.ID] = row
	}
	return nil
}

func (s *Service) Toggle(userID int64, uninstall, archive, unarchive bool, sets []*mtproto.InputStickerSet) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(sets) == 0 {
		return mtproto.ErrStickersetInvalid
	}
	row := installRow{Installed: true, Archived: false, Order: s.now()}
	switch {
	case uninstall:
		row = installRow{Installed: false}
	case archive:
		row.Archived = true
	case unarchive:
		row.Archived = false
	}
	for _, in := range sets {
		p, err := s.lookup(in)
		if err != nil {
			return err
		}
		if p.system() {
			continue
		}
		next := row
		if !uninstall && !archive && !unarchive {
			next.Order = s.now()
		}
		if archive {
			if prev, ok := s.state(userID).install[p.ID]; ok && prev.Installed {
				next.Order = prev.Order
			}
		}
		if s.persist != nil {
			if err = s.persist.saveInstall(userID, p.ID, next); err != nil {
				return err
			}
		}
		s.state(userID).install[p.ID] = next
	}
	return nil
}

func (s *Service) ReadFeatured(userID int64, ids []int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.state(userID)
	if st.featuredRead == nil {
		st.featuredRead = map[int64]bool{}
	}
	for _, id := range ids {
		if id == 0 || st.featuredRead[id] {
			continue
		}
		st.featuredRead[id] = true
		if mp, ok := s.mysql(); ok {
			if err := mp.saveFeaturedRead(userID, id); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) SavedGifs(userID, hash int64) *mtproto.Messages_SavedGifs {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows := s.state(userID).gifs
	var docs []*mtproto.Document
	var ids []int64
	for _, row := range rows {
		docs = append(docs, row.document())
		ids = append(ids, row.DocumentID)
	}
	sum := hash64(ids)
	if hash != 0 && hash == sum {
		return mtproto.MakeTLMessagesSavedGifsNotModified(&mtproto.Messages_SavedGifs{}).To_Messages_SavedGifs()
	}
	if docs == nil {
		docs = []*mtproto.Document{}
	}
	return mtproto.MakeTLMessagesSavedGifs(&mtproto.Messages_SavedGifs{
		Hash: sum,
		Gifs: docs,
	}).To_Messages_SavedGifs()
}

func (s *Service) SaveGif(userID int64, doc *mtproto.InputDocument, unsave bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := s.resolveGif(doc)
	if err != nil {
		return err
	}
	st := s.state(userID)
	next := make([]gifRow, 0, len(st.gifs))
	for _, cur := range st.gifs {
		if cur.DocumentID == row.DocumentID {
			continue
		}
		next = append(next, cur)
	}
	if unsave {
		if mp, ok := s.mysql(); ok {
			if err = mp.deleteGif(userID, row.DocumentID); err != nil {
				return err
			}
		}
		st.gifs = next
		return nil
	}
	row.Date = s.now()
	next = append([]gifRow{row}, next...)
	if len(next) > maxSavedGifs {
		dropped := next[maxSavedGifs:]
		next = next[:maxSavedGifs]
		if mp, ok := s.mysql(); ok {
			for _, old := range dropped {
				if err = mp.deleteGif(userID, old.DocumentID); err != nil {
					return err
				}
			}
		}
	}
	if mp, ok := s.mysql(); ok {
		if err = mp.saveGif(userID, next[0]); err != nil {
			return err
		}
	}
	st.gifs = next
	return nil
}

func (s *Service) CheckShortName(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shortNameAvailable(name)
}

func (s *Service) SuggestShortName(title string) *mtproto.Stickers_SuggestedShortName {
	s.mu.Lock()
	defer s.mu.Unlock()
	return mtproto.MakeTLStickersSuggestedShortName(&mtproto.Stickers_SuggestedShortName{
		ShortName: s.uniqueShort(slugTitle(title)),
	}).To_Stickers_SuggestedShortName()
}

func (s *Service) CreateSet(in CreateInput) (*mtproto.Messages_StickerSet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	owner, err := setOwner(in.Self, in.Owner)
	if err != nil {
		return nil, err
	}
	title, err := cleanTitle(in.Title)
	if err != nil {
		return nil, err
	}
	shortName := strings.TrimSpace(in.ShortName)
	if err = s.shortNameAvailable(shortName); err != nil {
		if err == mtproto.ErrShortNameInvalid {
			return nil, mtproto.ErrPackShortNameInvalid
		}
		if err == mtproto.ErrShortNameOccupied {
			return nil, mtproto.ErrPackShortNameOccupied
		}
		return nil, err
	}
	if len(in.Stickers) == 0 {
		return nil, mtproto.ErrStickersEmpty
	}
	items, animated, videos, err := s.collectItems(in.Stickers, in.Emojis)
	if err != nil {
		return nil, err
	}
	p := &Pack{
		ShortName: shortName,
		Title:     title,
		Kind:      KindSticker,
		Masks:     in.Masks,
		Emojis:    in.Emojis,
		TextColor: in.TextColor,
		Animated:  in.Animated || animated,
		Videos:    in.Videos || videos,
		Owner:     owner,
		Items:     items,
	}
	if p.Emojis {
		p.Kind = KindCustomEmoji
	}
	if len(p.Items) > stickerLimit(p) {
		return nil, mtproto.ErrStickersTooMuch
	}
	if in.Thumb != nil && in.Thumb.GetPredicateName() != mtproto.Predicate_inputDocumentEmpty && in.Thumb.GetId() != 0 {
		thumb, err := s.resolveStickerItem(in.Thumb, "", nil, nil)
		if err != nil {
			return nil, err
		}
		if !thumbImage(thumb.MIME) {
			return nil, mtproto.ErrStickerThumbPngNopng
		}
		p.ThumbID = thumb.DocumentID
	}
	p.ID, p.AccessHash = s.allocPackID(owner, shortName)
	if err = s.persistPack(p); err != nil {
		return nil, err
	}
	s.addPack(p)
	install := installRow{Installed: true, Archived: false, Order: s.now()}
	if s.persist != nil {
		if err = s.persist.saveInstall(owner, p.ID, install); err != nil {
			return nil, err
		}
	}
	s.state(owner).install[p.ID] = install
	return p.full(true, false), nil
}

func (s *Service) AddSticker(userID int64, set *mtproto.InputStickerSet, sticker *mtproto.InputStickerSetItem) (*mtproto.Messages_StickerSet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.ownedPack(userID, set)
	if err != nil {
		return nil, err
	}
	if sticker == nil {
		return nil, mtproto.ErrStickerDocumentInvalid
	}
	item, err := s.itemFromInput(sticker, p.Emojis)
	if err != nil {
		return nil, err
	}
	for _, cur := range p.Items {
		if cur.DocumentID == item.DocumentID {
			return nil, mtproto.ErrStickerDocumentInvalid
		}
	}
	if len(p.Items)+1 > stickerLimit(p) {
		return nil, mtproto.ErrStickersTooMuch
	}
	p.Items = append(p.Items, item)
	s.noteMotion(p, item)
	return s.commitPack(userID, p)
}

func (s *Service) RemoveSticker(userID int64, doc *mtproto.InputDocument) (*mtproto.Messages_StickerSet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, idx, err := s.ownedByDocument(userID, doc)
	if err != nil {
		return nil, err
	}
	if len(p.Items) <= 1 {
		return nil, mtproto.ErrStickersEmpty
	}
	p.Items = append(p.Items[:idx], p.Items[idx+1:]...)
	if p.ThumbID == doc.GetId() {
		p.ThumbID = 0
	}
	return s.commitPack(userID, p)
}

// SetThumb implements stickers.setStickerSetThumb.
// Neither thumb nor thumb_document_id clears the thumbnail.
// https://core.telegram.org/method/stickers.setStickerSetThumb
func (s *Service) SetThumb(userID int64, set *mtproto.InputStickerSet, thumb *mtproto.InputDocument, thumbID *wrapperspb.Int64Value) (*mtproto.Messages_StickerSet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.ownedPack(userID, set)
	if err != nil {
		return nil, err
	}
	var id int64
	switch {
	case thumbID != nil:
		id = thumbID.GetValue()
	case thumb != nil && thumb.GetPredicateName() != mtproto.Predicate_inputDocumentEmpty:
		id = thumb.GetId()
	}
	if id == 0 {
		p.ThumbID = 0
		return s.commitPack(userID, p)
	}
	var access int64
	if thumb != nil && thumb.GetId() == id {
		access = thumb.GetAccessHash()
	}
	if !packHasDoc(p, id, access) {
		return nil, mtproto.ErrStickerDocumentInvalid
	}
	p.ThumbID = id
	return s.commitPack(userID, p)
}

// ChangeSticker implements stickers.changeSticker.
// https://core.telegram.org/method/stickers.changeSticker
func (s *Service) ChangeSticker(userID int64, doc *mtproto.InputDocument, emoji *wrapperspb.StringValue, coords *mtproto.MaskCoords, keywords *wrapperspb.StringValue) (*mtproto.Messages_StickerSet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, idx, err := s.ownedByDocument(userID, doc)
	if err != nil {
		return nil, err
	}
	item := p.Items[idx]
	if emoji != nil {
		clean, err := cleanEmoji(emoji.GetValue())
		if err != nil {
			return nil, err
		}
		item.Emoticon = clean
	}
	if coords != nil {
		if coords.GetN() < 0 || coords.GetN() > 3 || coords.GetZoom() < 0 {
			return nil, mtproto.ErrStickerInvalid
		}
		item.MaskCoords = mtproto.MakeTLMaskCoords(&mtproto.MaskCoords{
			N: coords.GetN(), X: coords.GetX(), Y: coords.GetY(), Zoom: coords.GetZoom(),
		}).To_MaskCoords()
	}
	if keywords != nil {
		item.Keywords = splitKeywords(keywords.GetValue())
	}
	return s.commitPack(userID, p)
}

func packHasDoc(p *Pack, id, access int64) bool {
	if p == nil || id == 0 {
		return false
	}
	for _, item := range p.Items {
		if item.DocumentID != id {
			continue
		}
		if access != 0 && access != item.AccessHash {
			return false
		}
		return true
	}
	return false
}

func (s *Service) ChangePosition(userID int64, doc *mtproto.InputDocument, position int32) (*mtproto.Messages_StickerSet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, idx, err := s.ownedByDocument(userID, doc)
	if err != nil {
		return nil, err
	}
	if position < 0 || int(position) >= len(p.Items) {
		return nil, mtproto.ErrStickerInvalid
	}
	if int(position) != idx {
		item := p.Items[idx]
		rest := make([]*Item, 0, len(p.Items)-1)
		rest = append(rest, p.Items[:idx]...)
		rest = append(rest, p.Items[idx+1:]...)
		out := make([]*Item, 0, len(p.Items))
		out = append(out, rest[:position]...)
		out = append(out, item)
		out = append(out, rest[position:]...)
		p.Items = out
	}
	return s.commitPack(userID, p)
}

func (s *Service) ReplaceSticker(userID int64, doc *mtproto.InputDocument, sticker *mtproto.InputStickerSetItem) (*mtproto.Messages_StickerSet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, idx, err := s.ownedByDocument(userID, doc)
	if err != nil {
		return nil, err
	}
	if sticker == nil {
		return nil, mtproto.ErrStickerDocumentInvalid
	}
	item, err := s.itemFromInput(sticker, p.Emojis)
	if err != nil {
		return nil, err
	}
	for i, cur := range p.Items {
		if i != idx && cur.DocumentID == item.DocumentID {
			return nil, mtproto.ErrStickerDocumentInvalid
		}
	}
	p.Items[idx] = item
	if p.ThumbID == doc.GetId() {
		p.ThumbID = item.DocumentID
	}
	s.noteMotion(p, item)
	return s.commitPack(userID, p)
}

func (s *Service) RenameSet(userID int64, set *mtproto.InputStickerSet, title string) (*mtproto.Messages_StickerSet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.ownedPack(userID, set)
	if err != nil {
		return nil, err
	}
	clean, err := cleanTitle(title)
	if err != nil {
		return nil, err
	}
	p.Title = clean
	return s.commitPack(userID, p)
}

func (s *Service) DeleteSet(userID int64, set *mtproto.InputStickerSet) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.ownedPack(userID, set)
	if err != nil {
		return err
	}
	if mp, ok := s.mysql(); ok {
		if err = mp.deleteOwnedPack(p.ID); err != nil {
			return err
		}
	}
	s.removePack(p)
	return nil
}

func (s *Service) archivedPacks(userID int64, masks, emojis bool) []*Pack {
	var out []*Pack
	for _, p := range s.packs {
		if !kindMatch(p, masks, emojis) {
			continue
		}
		on, archived, _ := s.installed(userID, p)
		if !on || !archived || len(p.visibleItems()) == 0 {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

func kindMatch(p *Pack, masks, emojis bool) bool {
	if p == nil || p.system() {
		return false
	}
	if emojis {
		return p.Emojis
	}
	if masks {
		return p.Masks && !p.Emojis
	}
	return !p.Emojis && !p.Masks
}

func pagePacks(packs []*Pack, offsetID int64, limit int32) ([]*Pack, int) {
	limit = clampLimit(limit)
	total := len(packs)
	out := make([]*Pack, 0)
	for _, p := range packs {
		if offsetID != 0 && p.ID >= offsetID {
			continue
		}
		out = append(out, p)
		if int32(len(out)) >= limit {
			break
		}
	}
	return out, total
}

func clampLimit(limit int32) int32 {
	if limit < 1 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func (s *Service) ownedPack(userID int64, in *mtproto.InputStickerSet) (*Pack, error) {
	p, err := s.lookup(in)
	if err != nil {
		return nil, err
	}
	if p.Owner == 0 || p.Owner != userID {
		return nil, mtproto.ErrStickersetInvalid
	}
	return p, nil
}

func (s *Service) ownedByDocument(userID int64, doc *mtproto.InputDocument) (*Pack, int, error) {
	if doc == nil || doc.GetPredicateName() == mtproto.Predicate_inputDocumentEmpty || doc.GetId() == 0 {
		return nil, 0, mtproto.ErrStickerDocumentInvalid
	}
	for _, p := range s.packs {
		if p.Owner != userID {
			continue
		}
		for i, item := range p.Items {
			if item.DocumentID != doc.GetId() {
				continue
			}
			if doc.GetAccessHash() != 0 && doc.GetAccessHash() != item.AccessHash {
				return nil, 0, mtproto.ErrStickerDocumentInvalid
			}
			return p, i, nil
		}
	}
	return nil, 0, mtproto.ErrStickerDocumentInvalid
}

func (s *Service) commitPack(userID int64, p *Pack) (*mtproto.Messages_StickerSet, error) {
	if err := s.persistPack(p); err != nil {
		return nil, err
	}
	s.reindex(p)
	on, archived, _ := s.installed(userID, p)
	return p.full(on, archived), nil
}

func (s *Service) persistPack(p *Pack) error {
	mp, ok := s.mysql()
	if !ok {
		return nil
	}
	return mp.saveOwnedPack(p)
}

func (s *Service) addPack(p *Pack) {
	s.packs = append(s.packs, p)
	s.byID[p.ID] = p
	s.byShort[strings.ToLower(p.ShortName)] = p
	for _, item := range p.Items {
		s.indexItem(p, item)
	}
}

func (s *Service) removePack(p *Pack) {
	delete(s.byID, p.ID)
	if s.byShort[strings.ToLower(p.ShortName)] == p {
		delete(s.byShort, strings.ToLower(p.ShortName))
	}
	next := s.packs[:0]
	for _, q := range s.packs {
		if q != p {
			next = append(next, q)
		}
	}
	s.packs = next
	for id, link := range s.docs {
		if link.pack == p {
			delete(s.docs, id)
			s.relink(id)
		}
	}
}

func (s *Service) reindex(p *Pack) {
	keep := map[int64]bool{}
	for _, item := range p.Items {
		keep[item.DocumentID] = true
	}
	for id, link := range s.docs {
		if link.pack == p && !keep[id] {
			delete(s.docs, id)
			s.relink(id)
		}
	}
	for _, item := range p.Items {
		link := s.docs[item.DocumentID]
		if link == nil {
			s.docs[item.DocumentID] = &linkedDoc{item: item, pack: p}
			continue
		}
		if link.pack == p {
			link.item = item
		}
	}
}

func (s *Service) indexItem(p *Pack, item *Item) {
	if item == nil {
		return
	}
	if _, ok := s.docs[item.DocumentID]; ok {
		return
	}
	s.docs[item.DocumentID] = &linkedDoc{item: item, pack: p}
}

func (s *Service) relink(id int64) {
	if id == 0 || s.docs[id] != nil {
		return
	}
	for _, p := range s.packs {
		for _, item := range p.Items {
			if item.DocumentID == id {
				s.docs[id] = &linkedDoc{item: item, pack: p}
				return
			}
		}
	}
}

func (s *Service) allocPackID(userID int64, shortName string) (int64, int64) {
	for n := 0; n < 16; n++ {
		key := shortName
		if n > 0 {
			key = shortName + "#" + strconv.Itoa(n)
		}
		id := stableID("user-pack", strconv.FormatInt(userID, 10), key)
		if s.byID[id] != nil {
			continue
		}
		return id, stableID("user-pack-access", strconv.FormatInt(userID, 10), key)
	}
	id := stableID("user-pack", strconv.FormatInt(userID, 10), shortName, strconv.FormatInt(int64(s.now()), 10))
	return id, stableID("user-pack-access", strconv.FormatInt(id, 10))
}

func (s *Service) collectItems(stickers []*mtproto.InputStickerSetItem, emoji bool) ([]*Item, bool, bool, error) {
	var items []*Item
	seen := map[int64]bool{}
	var animated, videos bool
	for _, sticker := range stickers {
		item, err := s.itemFromInput(sticker, emoji)
		if err != nil {
			return nil, false, false, err
		}
		if seen[item.DocumentID] {
			return nil, false, false, mtproto.ErrStickerDocumentInvalid
		}
		seen[item.DocumentID] = true
		items = append(items, item)
		if item.Animated {
			animated = true
		}
		if item.Video {
			videos = true
		}
	}
	return items, animated, videos, nil
}

func (s *Service) itemFromInput(sticker *mtproto.InputStickerSetItem, emojiSet bool) (*Item, error) {
	if sticker == nil {
		return nil, mtproto.ErrStickerDocumentInvalid
	}
	emoji, err := cleanEmoji(sticker.GetEmoji())
	if err != nil {
		return nil, err
	}
	var keywords []string
	if sticker.GetKeywords() != nil {
		keywords = splitKeywords(sticker.GetKeywords().GetValue())
	}
	item, err := s.resolveStickerItem(sticker.GetDocument(), emoji, sticker.GetMaskCoords(), keywords)
	if err != nil {
		return nil, err
	}
	item.CustomEmoji = emojiSet || item.CustomEmoji
	return item, nil
}

func (s *Service) resolveStickerItem(doc *mtproto.InputDocument, emoji string, coords *mtproto.MaskCoords, keywords []string) (*Item, error) {
	if doc == nil || doc.GetPredicateName() == mtproto.Predicate_inputDocumentEmpty || doc.GetId() == 0 {
		return nil, mtproto.ErrStickerDocumentInvalid
	}
	if link := s.docs[doc.GetId()]; link != nil && link.item != nil {
		if doc.GetAccessHash() != 0 && doc.GetAccessHash() != link.item.AccessHash {
			return nil, mtproto.ErrStickerDocumentInvalid
		}
		item := cloneItem(link.item)
		if emoji != "" {
			item.Emoticon = emoji
		}
		if coords != nil {
			item.MaskCoords = coords
		}
		if len(keywords) > 0 {
			item.Keywords = keywords
		}
		return item, nil
	}
	if mp, ok := s.mysql(); ok {
		item, found := mp.findDocument(doc.GetId())
		if found {
			if doc.GetAccessHash() != 0 && doc.GetAccessHash() != item.AccessHash {
				return nil, mtproto.ErrStickerDocumentInvalid
			}
			if !stickerMIME(item.MIME) {
				return nil, mtproto.ErrStickerMimeInvalid
			}
			item.Emoticon = emoji
			item.MaskCoords = coords
			item.Keywords = keywords
			return item, nil
		}
	}
	return nil, mtproto.ErrStickerDocumentInvalid
}

func (s *Service) resolveGif(doc *mtproto.InputDocument) (gifRow, error) {
	if doc == nil || doc.GetPredicateName() == mtproto.Predicate_inputDocumentEmpty || doc.GetId() == 0 {
		return gifRow{}, mtproto.ErrGifIdInvalid
	}
	if link := s.docs[doc.GetId()]; link != nil && link.item != nil {
		if doc.GetAccessHash() != 0 && doc.GetAccessHash() != link.item.AccessHash {
			return gifRow{}, mtproto.ErrGifIdInvalid
		}
		if !gifMIME(link.item.MIME) {
			return gifRow{}, mtproto.ErrGifContentTypeInvalid
		}
		return gifFromItem(link.item), nil
	}
	if mp, ok := s.mysql(); ok {
		item, found := mp.findDocument(doc.GetId())
		if found {
			if doc.GetAccessHash() != 0 && doc.GetAccessHash() != item.AccessHash {
				return gifRow{}, mtproto.ErrGifIdInvalid
			}
			if !gifMIME(item.MIME) {
				return gifRow{}, mtproto.ErrGifContentTypeInvalid
			}
			return gifFromItem(item), nil
		}
	}
	return gifRow{}, mtproto.ErrGifIdInvalid
}

func gifFromItem(item *Item) gifRow {
	name := item.FileName
	if name == "" {
		name = "animation.gif"
	}
	w, h := item.Width, item.Height
	if w == 0 {
		w = 512
	}
	if h == 0 {
		h = 512
	}
	return gifRow{
		DocumentID: item.DocumentID,
		AccessHash: item.AccessHash,
		MIME:       item.MIME,
		Size:       item.Size,
		Width:      w,
		Height:     h,
		FileName:   name,
		Video:      item.Video || strings.HasPrefix(item.MIME, "video/"),
	}
}

func (row gifRow) document() *mtproto.Document {
	attrs := []*mtproto.DocumentAttribute{
		mtproto.MakeTLDocumentAttributeImageSize(&mtproto.DocumentAttribute{W: row.Width, H: row.Height}).To_DocumentAttribute(),
		mtproto.MakeTLDocumentAttributeFilename(&mtproto.DocumentAttribute{FileName: row.FileName}).To_DocumentAttribute(),
		mtproto.MakeTLDocumentAttributeAnimated(&mtproto.DocumentAttribute{}).To_DocumentAttribute(),
	}
	if row.Video {
		attrs = append(attrs, mtproto.MakeTLDocumentAttributeVideo(&mtproto.DocumentAttribute{
			W:                 row.Width,
			H:                 row.Height,
			SupportsStreaming: true,
		}).To_DocumentAttribute())
	}
	return mtproto.MakeTLDocument(&mtproto.Document{
		Id:            row.DocumentID,
		AccessHash:    row.AccessHash,
		FileReference: []byte{},
		Date:          row.Date,
		MimeType:      row.MIME,
		Size2_INT32:   row.Size,
		Size2_INT64:   int64(row.Size),
		Size2:         int64(row.Size),
		DcId:          1,
		Attributes:    attrs,
	}).To_Document()
}

func (s *Service) shortNameAvailable(name string) error {
	name = strings.TrimSpace(name)
	if !shortNameRE.MatchString(name) {
		return mtproto.ErrShortNameInvalid
	}
	if s.byShort[strings.ToLower(name)] != nil {
		return mtproto.ErrShortNameOccupied
	}
	return nil
}

func (s *Service) uniqueShort(base string) string {
	if !shortNameRE.MatchString(base) {
		base = "stickers"
	}
	for n := 0; n < 1000; n++ {
		name := base
		if n > 0 {
			suf := strconv.Itoa(n)
			name = base
			if len(name)+len(suf) > 32 {
				name = strings.TrimRight(name[:32-len(suf)], "_")
			}
			name += suf
		}
		if shortNameRE.MatchString(name) && s.byShort[strings.ToLower(name)] == nil {
			return name
		}
	}
	return "stickers"
}

func slugTitle(title string) string {
	var b strings.Builder
	underscore := false
	for _, r := range title {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
			underscore = false
		default:
			if b.Len() > 0 && !underscore {
				b.WriteByte('_')
				underscore = true
			}
		}
	}
	s := strings.Trim(b.String(), "_")
	if s == "" || !isLetter(s[0]) {
		s = "s" + s
	}
	if len(s) < 5 {
		s += "pack"
	}
	if len(s) > 32 {
		s = strings.TrimRight(s[:32], "_")
	}
	if !shortNameRE.MatchString(s) {
		return "stickers"
	}
	return s
}

func isLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func cleanTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	n := utf8.RuneCountInString(title)
	if n < 1 || n > maxStickerTitle {
		return "", mtproto.ErrPackTitleInvalid
	}
	return title, nil
}

func cleanEmoji(emoji string) (string, error) {
	emoji = strings.TrimSpace(emoji)
	n := utf8.RuneCountInString(emoji)
	if n < 1 || n > maxStickerEmoji {
		return "", mtproto.ErrStickerEmojiInvalid
	}
	return emoji, nil
}

func splitKeywords(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == ';'
	})
	var out []string
	seen := map[string]bool{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || seen[strings.ToLower(part)] {
			continue
		}
		seen[strings.ToLower(part)] = true
		out = append(out, part)
	}
	return out
}

func stickerLimit(p *Pack) int {
	if p.Emojis {
		return maxEmojiStickers
	}
	if p.Animated || p.Videos {
		return maxAnimatedStickers
	}
	return maxStaticStickers
}

func stickerMIME(mime string) bool {
	switch strings.ToLower(mime) {
	case "image/webp", "image/png", "application/x-tgsticker", "video/webm":
		return true
	default:
		return false
	}
}

func gifMIME(mime string) bool {
	switch strings.ToLower(mime) {
	case "image/gif", "video/mp4", "video/webm":
		return true
	default:
		return false
	}
}

func thumbImage(mime string) bool {
	switch strings.ToLower(mime) {
	case "image/webp", "image/png":
		return true
	default:
		return false
	}
}

func (s *Service) noteMotion(p *Pack, item *Item) {
	if item.Animated {
		p.Animated = true
	}
	if item.Video {
		p.Videos = true
	}
}

func cloneItem(item *Item) *Item {
	if item == nil {
		return nil
	}
	cp := *item
	if item.Keywords != nil {
		cp.Keywords = append([]string(nil), item.Keywords...)
	}
	if item.MaskCoords != nil {
		coords := *item.MaskCoords
		cp.MaskCoords = &coords
	}
	return &cp
}

func setOwner(self int64, in *mtproto.InputUser) (int64, error) {
	if in == nil || in.GetPredicateName() == mtproto.Predicate_inputUserSelf || in.GetPredicateName() == "" {
		if self <= 0 {
			return 0, mtproto.ErrUserIdInvalid
		}
		return self, nil
	}
	id := mtproto.ToUserIdByInput(self, in)
	if id <= 0 || id != self {
		return 0, mtproto.ErrUserIdInvalid
	}
	return id, nil
}

func (s *Service) mysql() (mysqlPersist, bool) {
	mp, ok := s.persist.(mysqlPersist)
	return mp, ok
}
