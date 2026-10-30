package mtproto

import "fmt"

// Feed constructors are not in telegram_api_layer232.tl. iOS and macOS
// already send the layer-233 ids. See schema.tl.feed.proto.
const (
	Predicate_feedPosition                              = "feedPosition"
	Predicate_feedItemMessage                           = "feedItemMessage"
	Predicate_feedItemMessageGroup                      = "feedItemMessageGroup"
	Predicate_feedItemInterestCategories                = "feedItemInterestCategories"
	Predicate_feed_feedMessagesNotModified              = "feed_feedMessagesNotModified"
	Predicate_feed_feedMessages                         = "feed_feedMessages"
	Predicate_feed_forYouFeed                           = "feed_forYouFeed"
	Predicate_feed_interestCategory                     = "feed_interestCategory"
	Predicate_feed_interestCategoriesNotModified        = "feed_interestCategoriesNotModified"
	Predicate_feed_interestCategories                   = "feed_interestCategories"
	Predicate_feed_getFeed                              = "feed_getFeed"
	Predicate_feed_getForYou                            = "feed_getForYou"
	Predicate_feed_getInterestCategories                = "feed_getInterestCategories"
	Predicate_feed_readFeed                             = "feed_readFeed"
	Predicate_feed_setInterestCategories                = "feed_setInterestCategories"
	crcFeedPosition                              uint32 = 0x5059dc73
	crcFeedItemMessage                           uint32 = 0x7e73b3a5
	crcFeedItemMessageGroup                      uint32 = 0x652f4c71
	crcFeedItemInterestCategories                uint32 = 0x6be7187b
	crcFeedMessagesNotModified                   uint32 = 0xdb1a3503
	crcFeedMessages                              uint32 = 0xdcf754b9
	crcFeedForYou                                uint32 = 0x52577805
	crcFeedInterestCategory                      uint32 = 0x6c4666e3
	crcFeedInterestCategoriesNotModified         uint32 = 0x79cf2195
	crcFeedInterestCategories                    uint32 = 0xecb62fa0
	crcFeedGetFeed                               uint32 = 0x7e76d7d3
	crcFeedGetForYou                             uint32 = 0x33b8e5eb
	crcFeedGetInterestCategories                 uint32 = 0xba7a1dba
	crcFeedReadFeed                              uint32 = 0xb436c1ff
	crcFeedSetInterestCategories                 uint32 = 0x9f2958dc
)

func init() {
	registerLayer232Extra(Predicate_feedPosition, crcFeedPosition, func(c int32) TLObject {
		return &FeedPosition{PredicateName: Predicate_feedPosition, Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_feedItemMessage, crcFeedItemMessage, func(c int32) TLObject {
		return &FeedItem{PredicateName: Predicate_feedItemMessage, Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_feedItemMessageGroup, crcFeedItemMessageGroup, func(c int32) TLObject {
		return &FeedItem{PredicateName: Predicate_feedItemMessageGroup, Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_feedItemInterestCategories, crcFeedItemInterestCategories, func(c int32) TLObject {
		return &FeedItem{PredicateName: Predicate_feedItemInterestCategories, Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_feed_feedMessagesNotModified, crcFeedMessagesNotModified, func(c int32) TLObject {
		return &Feed_FeedMessages{PredicateName: Predicate_feed_feedMessagesNotModified, Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_feed_feedMessages, crcFeedMessages, func(c int32) TLObject {
		return &Feed_FeedMessages{PredicateName: Predicate_feed_feedMessages, Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_feed_forYouFeed, crcFeedForYou, func(c int32) TLObject {
		return &Feed_ForYouFeed{PredicateName: Predicate_feed_forYouFeed, Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_feed_interestCategory, crcFeedInterestCategory, func(c int32) TLObject {
		return &Feed_InterestCategory{PredicateName: Predicate_feed_interestCategory, Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_feed_interestCategoriesNotModified, crcFeedInterestCategoriesNotModified, func(c int32) TLObject {
		return &Feed_InterestCategories{PredicateName: Predicate_feed_interestCategoriesNotModified, Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_feed_interestCategories, crcFeedInterestCategories, func(c int32) TLObject {
		return &Feed_InterestCategories{PredicateName: Predicate_feed_interestCategories, Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_feed_getFeed, crcFeedGetFeed, func(c int32) TLObject {
		return &TLFeedGetFeed{Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_feed_getForYou, crcFeedGetForYou, func(c int32) TLObject {
		return &TLFeedGetForYou{Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_feed_getInterestCategories, crcFeedGetInterestCategories, func(c int32) TLObject {
		return &TLFeedGetInterestCategories{Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_feed_readFeed, crcFeedReadFeed, func(c int32) TLObject {
		return &TLFeedReadFeed{Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_feed_setInterestCategories, crcFeedSetInterestCategories, func(c int32) TLObject {
		return &TLFeedSetInterestCategories{Constructor: TLConstructor(c)}
	})
}

// MakeFeedPosition builds feedPosition.
func MakeFeedPosition(date, id int32, peer *Peer) *FeedPosition {
	return &FeedPosition{
		PredicateName: Predicate_feedPosition,
		Constructor:   ctorID(crcFeedPosition),
		Date:          date,
		Peer:          peer,
		Id:            id,
	}
}

// MakeFeedMessagesNotModified builds feed.feedMessagesNotModified.
func MakeFeedMessagesNotModified() *Feed_FeedMessages {
	return &Feed_FeedMessages{
		PredicateName: Predicate_feed_feedMessagesNotModified,
		Constructor:   ctorID(crcFeedMessagesNotModified),
	}
}

// MakeFeedMessages builds feed.feedMessages. Nil slices become empty vectors.
func MakeFeedMessages(maxPos, minPos, readPos *FeedPosition, messages []*Message, chats []*Chat, users []*User) *Feed_FeedMessages {
	if messages == nil {
		messages = []*Message{}
	}
	if chats == nil {
		chats = []*Chat{}
	}
	if users == nil {
		users = []*User{}
	}
	return &Feed_FeedMessages{
		PredicateName:   Predicate_feed_feedMessages,
		Constructor:     ctorID(crcFeedMessages),
		MaxPosition:     maxPos,
		MinPosition:     minPos,
		ReadMaxPosition: readPos,
		HasMax:          maxPos != nil,
		HasMin:          minPos != nil,
		HasRead:         readPos != nil,
		Messages:        messages,
		Chats:           chats,
		Users:           users,
	}
}

// MakeFeedForYou builds feed.forYouFeed.
func MakeFeedForYou(items []*FeedItem, chats []*Chat, users []*User) *Feed_ForYouFeed {
	if items == nil {
		items = []*FeedItem{}
	}
	if chats == nil {
		chats = []*Chat{}
	}
	if users == nil {
		users = []*User{}
	}
	return &Feed_ForYouFeed{
		PredicateName: Predicate_feed_forYouFeed,
		Constructor:   ctorID(crcFeedForYou),
		Items:         items,
		Chats:         chats,
		Users:         users,
	}
}

// MakeFeedInterestItem builds feedItemInterestCategories.
func MakeFeedInterestItem() *FeedItem {
	return &FeedItem{
		PredicateName: Predicate_feedItemInterestCategories,
		Constructor:   ctorID(crcFeedItemInterestCategories),
	}
}

// MakeFeedInterestCategoriesNotModified builds feed.interestCategoriesNotModified.
func MakeFeedInterestCategoriesNotModified() *Feed_InterestCategories {
	return &Feed_InterestCategories{
		PredicateName: Predicate_feed_interestCategoriesNotModified,
		Constructor:   ctorID(crcFeedInterestCategoriesNotModified),
	}
}

// MakeFeedInterestCategories builds feed.interestCategories.
func MakeFeedInterestCategories(rows []*Feed_InterestCategory, selected []int32) *Feed_InterestCategories {
	if rows == nil {
		rows = []*Feed_InterestCategory{}
	}
	if selected == nil {
		selected = []int32{}
	}
	return &Feed_InterestCategories{
		PredicateName: Predicate_feed_interestCategories,
		Constructor:   ctorID(crcFeedInterestCategories),
		Categories:    rows,
		Selected:      selected,
	}
}

// MakeFeedInterestCategory builds feed.interestCategory.
func MakeFeedInterestCategory(id int32, title, emoji string) *Feed_InterestCategory {
	return &Feed_InterestCategory{
		PredicateName: Predicate_feed_interestCategory,
		Constructor:   ctorID(crcFeedInterestCategory),
		Id:            id,
		Title:         title,
		Emoji:         emoji,
		HasEmoji:      emoji != "",
	}
}

func (m *FeedPosition) Encode(x *EncodeBuf, layer int32) error {
	if m == nil || m.GetPeer() == nil {
		return fmt.Errorf("feedPosition: missing peer")
	}
	x.UInt(crcFeedPosition)
	x.Int(m.GetDate())
	if err := m.GetPeer().Encode(x, layer); err != nil {
		return err
	}
	x.Int(m.GetId())
	return nil
}

func (m *FeedPosition) Decode(dBuf *DecodeBuf) error {
	if m.Constructor == 0 {
		m.Constructor = TLConstructor(dBuf.Int())
	}
	if uint32(m.Constructor) != crcFeedPosition {
		return fmt.Errorf("feedPosition: constructor 0x%x", uint32(m.Constructor))
	}
	m.PredicateName = Predicate_feedPosition
	m.Date = dBuf.Int()
	peer := &Peer{}
	if err := peer.Decode(dBuf); err != nil {
		return err
	}
	m.Peer = peer
	m.Id = dBuf.Int()
	return dBuf.GetError()
}

func (m *FeedItem) Encode(x *EncodeBuf, layer int32) error {
	if m == nil {
		return fmt.Errorf("feedItem: nil")
	}
	switch m.GetPredicateName() {
	case Predicate_feedItemMessage:
		x.UInt(crcFeedItemMessage)
		if m.GetMessage() == nil {
			return fmt.Errorf("feedItemMessage: missing message")
		}
		return m.GetMessage().Encode(x, layer)
	case Predicate_feedItemMessageGroup:
		x.UInt(crcFeedItemMessageGroup)
		return encodeTLObjects(x, layer, messagesAsObjects(m.GetMessages()))
	case Predicate_feedItemInterestCategories, "":
		x.UInt(crcFeedItemInterestCategories)
		return nil
	default:
		return fmt.Errorf("feedItem: %s", m.GetPredicateName())
	}
}

func (m *FeedItem) Decode(dBuf *DecodeBuf) error {
	if m.Constructor == 0 {
		m.Constructor = TLConstructor(dBuf.Int())
	}
	switch uint32(m.Constructor) {
	case crcFeedItemMessage:
		m.PredicateName = Predicate_feedItemMessage
		msg, err := decodeMessage(dBuf)
		if err != nil {
			return err
		}
		m.Message = msg
	case crcFeedItemMessageGroup:
		m.PredicateName = Predicate_feedItemMessageGroup
		msgs, err := decodeMessages(dBuf)
		if err != nil {
			return err
		}
		m.Messages = msgs
	case crcFeedItemInterestCategories:
		m.PredicateName = Predicate_feedItemInterestCategories
	default:
		return fmt.Errorf("feedItem: constructor 0x%x", uint32(m.Constructor))
	}
	return dBuf.GetError()
}

func (m *Feed_FeedMessages) Encode(x *EncodeBuf, layer int32) error {
	if m == nil {
		m = MakeFeedMessagesNotModified()
	}
	switch m.GetPredicateName() {
	case Predicate_feed_feedMessagesNotModified:
		x.UInt(crcFeedMessagesNotModified)
		return nil
	case Predicate_feed_feedMessages, "":
		x.UInt(crcFeedMessages)
		var flags uint32
		if m.GetHasMax() {
			flags |= 1 << 0
		}
		if m.GetHasMin() {
			flags |= 1 << 1
		}
		if m.GetHasRead() {
			flags |= 1 << 2
		}
		x.UInt(flags)
		if flags&(1<<0) != 0 {
			if err := m.GetMaxPosition().Encode(x, layer); err != nil {
				return err
			}
		}
		if flags&(1<<1) != 0 {
			if err := m.GetMinPosition().Encode(x, layer); err != nil {
				return err
			}
		}
		if flags&(1<<2) != 0 {
			if err := m.GetReadMaxPosition().Encode(x, layer); err != nil {
				return err
			}
		}
		if err := encodeTLObjects(x, layer, messagesAsObjects(m.GetMessages())); err != nil {
			return err
		}
		if err := encodeTLObjects(x, layer, chatsAsObjects(m.GetChats())); err != nil {
			return err
		}
		return encodeTLObjects(x, layer, usersAsObjects(m.GetUsers()))
	default:
		return fmt.Errorf("feed.FeedMessages: %s", m.GetPredicateName())
	}
}

func (m *Feed_FeedMessages) Decode(dBuf *DecodeBuf) error {
	if m.Constructor == 0 {
		m.Constructor = TLConstructor(dBuf.Int())
	}
	switch uint32(m.Constructor) {
	case crcFeedMessagesNotModified:
		m.PredicateName = Predicate_feed_feedMessagesNotModified
		return dBuf.GetError()
	case crcFeedMessages:
		m.PredicateName = Predicate_feed_feedMessages
	default:
		return fmt.Errorf("feed.FeedMessages: constructor 0x%x", uint32(m.Constructor))
	}
	flags := dBuf.UInt()
	if flags&(1<<0) != 0 {
		pos := &FeedPosition{}
		if err := pos.Decode(dBuf); err != nil {
			return err
		}
		m.MaxPosition = pos
		m.HasMax = true
	}
	if flags&(1<<1) != 0 {
		pos := &FeedPosition{}
		if err := pos.Decode(dBuf); err != nil {
			return err
		}
		m.MinPosition = pos
		m.HasMin = true
	}
	if flags&(1<<2) != 0 {
		pos := &FeedPosition{}
		if err := pos.Decode(dBuf); err != nil {
			return err
		}
		m.ReadMaxPosition = pos
		m.HasRead = true
	}
	msgs, err := decodeMessages(dBuf)
	if err != nil {
		return err
	}
	chats, err := decodeChats(dBuf)
	if err != nil {
		return err
	}
	users, err := decodeUsers(dBuf)
	if err != nil {
		return err
	}
	m.Messages, m.Chats, m.Users = msgs, chats, users
	return dBuf.GetError()
}

func (m *Feed_ForYouFeed) Encode(x *EncodeBuf, layer int32) error {
	if m == nil {
		m = MakeFeedForYou(nil, nil, nil)
	}
	x.UInt(crcFeedForYou)
	x.UInt(0)
	items := make([]TLObject, 0, len(m.GetItems()))
	for _, item := range m.GetItems() {
		items = append(items, item)
	}
	if err := encodeTLObjects(x, layer, items); err != nil {
		return err
	}
	if err := encodeTLObjects(x, layer, chatsAsObjects(m.GetChats())); err != nil {
		return err
	}
	return encodeTLObjects(x, layer, usersAsObjects(m.GetUsers()))
}

func (m *Feed_ForYouFeed) Decode(dBuf *DecodeBuf) error {
	if m.Constructor == 0 {
		m.Constructor = TLConstructor(dBuf.Int())
	}
	if uint32(m.Constructor) != crcFeedForYou {
		return fmt.Errorf("feed.forYouFeed: constructor 0x%x", uint32(m.Constructor))
	}
	m.PredicateName = Predicate_feed_forYouFeed
	_ = dBuf.UInt()
	raw, err := decodeObjects(dBuf)
	if err != nil {
		return err
	}
	items := make([]*FeedItem, 0, len(raw))
	for _, obj := range raw {
		item, ok := obj.(*FeedItem)
		if !ok || item == nil {
			return fmt.Errorf("feed.forYouFeed: item")
		}
		items = append(items, item)
	}
	chats, err := decodeChats(dBuf)
	if err != nil {
		return err
	}
	users, err := decodeUsers(dBuf)
	if err != nil {
		return err
	}
	m.Items, m.Chats, m.Users = items, chats, users
	return dBuf.GetError()
}

func (m *Feed_InterestCategory) Encode(x *EncodeBuf, layer int32) error {
	if m == nil {
		return fmt.Errorf("feed.interestCategory: nil")
	}
	_ = layer
	x.UInt(crcFeedInterestCategory)
	var flags uint32
	if m.GetHasEmoji() {
		flags |= 1 << 0
	}
	if m.GetHasEmojiDocument() {
		flags |= 1 << 1
	}
	x.UInt(flags)
	x.Int(m.GetId())
	x.String(m.GetTitle())
	if flags&(1<<0) != 0 {
		x.String(m.GetEmoji())
	}
	if flags&(1<<1) != 0 {
		x.Long(m.GetEmojiDocumentId())
	}
	return nil
}

func (m *Feed_InterestCategory) Decode(dBuf *DecodeBuf) error {
	if m.Constructor == 0 {
		m.Constructor = TLConstructor(dBuf.Int())
	}
	if uint32(m.Constructor) != crcFeedInterestCategory {
		return fmt.Errorf("feed.interestCategory: constructor 0x%x", uint32(m.Constructor))
	}
	m.PredicateName = Predicate_feed_interestCategory
	flags := dBuf.UInt()
	m.Id = dBuf.Int()
	m.Title = dBuf.String()
	if flags&(1<<0) != 0 {
		m.HasEmoji = true
		m.Emoji = dBuf.String()
	}
	if flags&(1<<1) != 0 {
		m.HasEmojiDocument = true
		m.EmojiDocumentId = dBuf.Long()
	}
	return dBuf.GetError()
}

func (m *Feed_InterestCategories) Encode(x *EncodeBuf, layer int32) error {
	if m == nil || m.GetPredicateName() == Predicate_feed_interestCategoriesNotModified {
		x.UInt(crcFeedInterestCategoriesNotModified)
		return nil
	}
	x.UInt(crcFeedInterestCategories)
	rows := make([]TLObject, 0, len(m.GetCategories()))
	for _, row := range m.GetCategories() {
		rows = append(rows, row)
	}
	if err := encodeTLObjects(x, layer, rows); err != nil {
		return err
	}
	x.VectorInt(m.GetSelected())
	return nil
}

func (m *Feed_InterestCategories) Decode(dBuf *DecodeBuf) error {
	if m.Constructor == 0 {
		m.Constructor = TLConstructor(dBuf.Int())
	}
	switch uint32(m.Constructor) {
	case crcFeedInterestCategoriesNotModified:
		m.PredicateName = Predicate_feed_interestCategoriesNotModified
		return dBuf.GetError()
	case crcFeedInterestCategories:
		m.PredicateName = Predicate_feed_interestCategories
	default:
		return fmt.Errorf("feed.InterestCategories: constructor 0x%x", uint32(m.Constructor))
	}
	raw, err := decodeObjects(dBuf)
	if err != nil {
		return err
	}
	rows := make([]*Feed_InterestCategory, 0, len(raw))
	for _, obj := range raw {
		row, ok := obj.(*Feed_InterestCategory)
		if !ok || row == nil {
			return fmt.Errorf("feed.interestCategory")
		}
		rows = append(rows, row)
	}
	m.Categories = rows
	m.Selected = dBuf.VectorInt()
	return dBuf.GetError()
}

func (m *TLFeedGetFeed) Encode(x *EncodeBuf, layer int32) error {
	if m == nil {
		m = &TLFeedGetFeed{}
	}
	x.UInt(crcFeedGetFeed)
	var flags uint32
	if m.GetOffsetToMaxRead() {
		flags |= 1 << 3
	}
	if m.GetOffsetPosition() != nil {
		flags |= 1 << 0
	}
	if m.GetMaxPosition() != nil {
		flags |= 1 << 1
	}
	if m.GetMinPosition() != nil {
		flags |= 1 << 2
	}
	x.UInt(flags)
	x.Int(m.GetFilterId())
	if flags&(1<<0) != 0 {
		if err := m.GetOffsetPosition().Encode(x, layer); err != nil {
			return err
		}
	}
	x.Int(m.GetAddOffset())
	x.Int(m.GetLimit())
	if flags&(1<<1) != 0 {
		if err := m.GetMaxPosition().Encode(x, layer); err != nil {
			return err
		}
	}
	if flags&(1<<2) != 0 {
		if err := m.GetMinPosition().Encode(x, layer); err != nil {
			return err
		}
	}
	x.Long(m.GetHash())
	return nil
}

func (m *TLFeedGetFeed) Decode(dBuf *DecodeBuf) error {
	flags := dBuf.UInt()
	m.OffsetToMaxRead = flags&(1<<3) != 0
	m.FilterId = dBuf.Int()
	if flags&(1<<0) != 0 {
		pos := &FeedPosition{}
		if err := pos.Decode(dBuf); err != nil {
			return err
		}
		m.OffsetPosition = pos
	}
	m.AddOffset = dBuf.Int()
	m.Limit = dBuf.Int()
	if flags&(1<<1) != 0 {
		pos := &FeedPosition{}
		if err := pos.Decode(dBuf); err != nil {
			return err
		}
		m.MaxPosition = pos
	}
	if flags&(1<<2) != 0 {
		pos := &FeedPosition{}
		if err := pos.Decode(dBuf); err != nil {
			return err
		}
		m.MinPosition = pos
	}
	m.Hash = dBuf.Long()
	return dBuf.GetError()
}

func (m *TLFeedGetForYou) Encode(x *EncodeBuf, layer int32) error {
	if m == nil {
		m = &TLFeedGetForYou{}
	}
	_ = layer
	x.UInt(crcFeedGetForYou)
	var flags uint32
	if m.GetHasQuery() {
		flags |= 1 << 0
	}
	x.UInt(flags)
	if flags&(1<<0) != 0 {
		x.String(m.GetQuery())
	}
	x.Int(m.GetLimit())
	return nil
}

func (m *TLFeedGetForYou) Decode(dBuf *DecodeBuf) error {
	flags := dBuf.UInt()
	if flags&(1<<0) != 0 {
		m.HasQuery = true
		m.Query = dBuf.String()
	}
	m.Limit = dBuf.Int()
	return dBuf.GetError()
}

func (m *TLFeedGetInterestCategories) Encode(x *EncodeBuf, layer int32) error {
	if m == nil {
		m = &TLFeedGetInterestCategories{}
	}
	_ = layer
	x.UInt(crcFeedGetInterestCategories)
	x.Long(m.GetHash())
	return nil
}

func (m *TLFeedGetInterestCategories) Decode(dBuf *DecodeBuf) error {
	m.Hash = dBuf.Long()
	return dBuf.GetError()
}

func (m *TLFeedReadFeed) Encode(x *EncodeBuf, layer int32) error {
	if m == nil || m.GetMaxPosition() == nil {
		return fmt.Errorf("feed.readFeed: missing position")
	}
	x.UInt(crcFeedReadFeed)
	x.Int(m.GetFilterId())
	return m.GetMaxPosition().Encode(x, layer)
}

func (m *TLFeedReadFeed) Decode(dBuf *DecodeBuf) error {
	m.FilterId = dBuf.Int()
	pos := &FeedPosition{}
	if err := pos.Decode(dBuf); err != nil {
		return err
	}
	m.MaxPosition = pos
	return dBuf.GetError()
}

func (m *TLFeedSetInterestCategories) Encode(x *EncodeBuf, layer int32) error {
	if m == nil {
		m = &TLFeedSetInterestCategories{}
	}
	_ = layer
	x.UInt(crcFeedSetInterestCategories)
	x.VectorInt(m.GetCategoryIds())
	return nil
}

func (m *TLFeedSetInterestCategories) Decode(dBuf *DecodeBuf) error {
	m.CategoryIds = dBuf.VectorInt()
	return dBuf.GetError()
}

func encodeTLObjects(x *EncodeBuf, layer int32, rows []TLObject) error {
	x.Int(int32(CRC32_vector))
	x.Int(int32(len(rows)))
	for _, row := range rows {
		if row == nil {
			return fmt.Errorf("feed: nil vector element")
		}
		if err := row.Encode(x, layer); err != nil {
			return err
		}
	}
	return nil
}

func decodeObjects(dBuf *DecodeBuf) ([]TLObject, error) {
	c := dBuf.Int()
	if c != int32(CRC32_vector) {
		return nil, fmt.Errorf("feed: expected vector, got 0x%x", uint32(c))
	}
	n := dBuf.Int()
	if n < 0 || n > maxVectorLen {
		return nil, fmt.Errorf("feed: length %d", n)
	}
	out := make([]TLObject, 0, n)
	for i := int32(0); i < n; i++ {
		obj := dBuf.Object()
		if obj == nil {
			return nil, fmt.Errorf("feed: object")
		}
		out = append(out, obj)
	}
	return out, dBuf.GetError()
}

func decodeMessage(dBuf *DecodeBuf) (*Message, error) {
	msg := &Message{}
	if err := msg.Decode(dBuf); err != nil {
		return nil, err
	}
	return msg, dBuf.GetError()
}

func decodeVector(dBuf *DecodeBuf, each func() error) error {
	c := dBuf.Int()
	if c != int32(CRC32_vector) {
		return fmt.Errorf("feed: expected vector, got 0x%x", uint32(c))
	}
	n := dBuf.Int()
	if n < 0 || n > maxVectorLen {
		return fmt.Errorf("feed: length %d", n)
	}
	for i := int32(0); i < n; i++ {
		if err := each(); err != nil {
			return err
		}
	}
	return dBuf.GetError()
}

func decodeMessages(dBuf *DecodeBuf) ([]*Message, error) {
	var out []*Message
	err := decodeVector(dBuf, func() error {
		msg, err := decodeMessage(dBuf)
		if err != nil {
			return err
		}
		out = append(out, msg)
		return nil
	})
	if out == nil {
		out = []*Message{}
	}
	return out, err
}

func decodeChats(dBuf *DecodeBuf) ([]*Chat, error) {
	var out []*Chat
	err := decodeVector(dBuf, func() error {
		chat := &Chat{}
		if err := chat.Decode(dBuf); err != nil {
			return err
		}
		out = append(out, chat)
		return nil
	})
	if out == nil {
		out = []*Chat{}
	}
	return out, err
}

func decodeUsers(dBuf *DecodeBuf) ([]*User, error) {
	var out []*User
	err := decodeVector(dBuf, func() error {
		user := &User{}
		if err := user.Decode(dBuf); err != nil {
			return err
		}
		out = append(out, user)
		return nil
	})
	if out == nil {
		out = []*User{}
	}
	return out, err
}

func messagesAsObjects(rows []*Message) []TLObject {
	out := make([]TLObject, 0, len(rows))
	for _, row := range rows {
		out = append(out, row)
	}
	return out
}

func chatsAsObjects(rows []*Chat) []TLObject {
	out := make([]TLObject, 0, len(rows))
	for _, row := range rows {
		out = append(out, row)
	}
	return out
}

func usersAsObjects(rows []*User) []TLObject {
	out := make([]TLObject, 0, len(rows))
	for _, row := range rows {
		out = append(out, row)
	}
	return out
}
