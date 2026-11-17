// Package profile stores Telegram Business fields.
//
// Hours, location, greeting, away message, intro, and chat links are what
// account.updateBusiness* and account.*BusinessChatLink write. users.getFullUser
// reads the same book so a later profile fetch shows the saved fields.
// When MySQL is configured, UseDSN keeps the book across restarts. The link
// field is brand.LinkBase()+"/m/<slug>". account.resolveBusinessChatLink
// resolves the slug.
package profile

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/brand"
	"github.com/teamgram/teamgram-server/pkg/tz"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	maxPeriods     = 28
	weekMinutes    = 7 * 24 * 60
	maxAddress     = 96
	maxIntroTitle  = 64
	maxIntroAbout  = 70
	maxLinkTitle   = 32
	maxLinkMessage = 4096
	maxEntities    = 100
	maxLinks       = 100
	maxRecipients  = 100
	maxNoActivity  = 366
)

// Card is the business block stored on userFull.
type Card struct {
	WorkHours *mtproto.BusinessWorkHours
	Location  *mtproto.BusinessLocation
	Greeting  *mtproto.BusinessGreetingMessage
	Away      *mtproto.BusinessAwayMessage
	Intro     *mtproto.BusinessIntro
}

type linkRow struct {
	slug     string
	message  string
	entities []*mtproto.MessageEntity
	title    string
	hasTitle bool
	views    int32
}

type peerNote struct {
	LastActivity int32
	GreetingSent int32
	// GreetedPrior is the dialog's previous message time that GreetingSent
	// was decided from. It is not the clock time the shortcut was copied.
	GreetedPrior int32
	AwaySent     int32
	AwayWindow   string
}

type row struct {
	hours       *mtproto.BusinessWorkHours
	location    *mtproto.BusinessLocation
	greeting    *mtproto.BusinessGreetingMessage
	away        *mtproto.BusinessAwayMessage
	intro       *mtproto.BusinessIntro
	links       []*linkRow
	peers       map[int64]*peerNote
	awayVersion int32
}

// Book is the business profile store. Without MySQL it stays in memory.
// gates serializes one user's read-modify-write, including the SQL that
// runs outside mu. mu only covers the in-memory maps.
type Book struct {
	mu    sync.Mutex
	db    docDB
	users map[int64]*row
	slugs map[string]int64
	now   func() time.Time
	gates sync.Map // int64 -> *sync.Mutex
}

// New returns an empty book.
func New() *Book {
	return &Book{
		users: map[int64]*row{},
		slugs: map[string]int64{},
		now:   time.Now,
	}
}

var shared = New()

// Shared is the book the BFF handlers and users.getFullUser share.
func Shared() *Book {
	return shared
}

// ResetShared drops process state. Tests call it so cases do not leak.
func ResetShared() {
	shared = New()
}

func (b *Book) touch(user int64) *row {
	r := b.users[user]
	if r == nil {
		r = &row{}
		b.users[user] = r
	}
	if r.peers == nil {
		r.peers = map[int64]*peerNote{}
	}
	return r
}

// Card returns a copy of the saved business fields. Nil means nothing is set.
// open_now is recomputed from the stored timezone and weekly periods.
// MySQL is read before mu is taken, so a slow query does not freeze the book.
func (b *Book) Card(user int64) *Card {
	if b == nil {
		return nil
	}
	g := b.userGate(user)
	g.Lock()
	defer g.Unlock()
	raw, found, enabled, err := b.fetchUser(user)
	b.mu.Lock()
	defer b.mu.Unlock()
	if err == nil {
		_ = b.applyFetched(user, raw, found, enabled)
	}
	return b.cardLocked(user)
}

func (b *Book) cardLocked(user int64) *Card {
	r := b.users[user]
	if r == nil || !r.any() {
		return nil
	}
	card := &Card{}
	if r.hours != nil {
		hours := cloneHours(r.hours)
		hours.OpenNow = openNow(hours.GetTimezoneId(), hours.GetWeeklyOpen(), b.now())
		card.WorkHours = hours
	}
	card.Location = cloneLoc(r.location)
	card.Greeting = cloneGreeting(r.greeting)
	card.Away = cloneAway(r.away)
	card.Intro = cloneIntro(r.intro)
	return card
}

func (r *row) any() bool {
	return r.hours != nil || r.location != nil || r.greeting != nil || r.away != nil || r.intro != nil
}

// SetWorkHours replaces opening hours. A nil value clears them.
func (b *Book) SetWorkHours(user int64, hours *mtproto.BusinessWorkHours) error {
	if hours == nil {
		return b.update(user, func(*[]string) error {
			if r := b.users[user]; r != nil {
				r.hours = nil
			}
			return nil
		})
	}
	if err := validateHours(hours); err != nil {
		return err
	}
	stored := mtproto.MakeTLBusinessWorkHours(&mtproto.BusinessWorkHours{
		TimezoneId: hours.GetTimezoneId(),
		WeeklyOpen: copyPeriods(hours.GetWeeklyOpen()),
	}).To_BusinessWorkHours()
	stored.OpenNow = openNow(stored.GetTimezoneId(), stored.GetWeeklyOpen(), b.now())
	return b.update(user, func(*[]string) error {
		b.touch(user).hours = stored
		return nil
	})
}

// SetLocation replaces the public address. Nil geo and nil address clear it.
func (b *Book) SetLocation(user int64, geo *mtproto.InputGeoPoint, address *wrapperspb.StringValue) error {
	if geo == nil && address == nil {
		return b.update(user, func(*[]string) error {
			if r := b.users[user]; r != nil {
				r.location = nil
			}
			return nil
		})
	}
	loc, err := buildLocation(geo, address)
	if err != nil {
		return err
	}
	return b.update(user, func(*[]string) error {
		b.touch(user).location = loc
		return nil
	})
}

// SetGreeting replaces the greeting. A nil message clears it.
// The caller has already checked that shortcutID names a saved quick reply.
func (b *Book) SetGreeting(self int64, in *mtproto.InputBusinessGreetingMessage) error {
	if in == nil {
		return b.update(self, func(*[]string) error {
			if r := b.users[self]; r != nil {
				r.greeting = nil
			}
			return nil
		})
	}
	rec, err := recipients(self, in.GetRecipients())
	if err != nil {
		return err
	}
	if in.GetShortcutId() <= 0 {
		return mtproto.ErrShortcutInvalid
	}
	if in.GetNoActivityDays() < 0 || in.GetNoActivityDays() > maxNoActivity {
		return bad("NO_ACTIVITY_DAYS_INVALID")
	}
	msg := mtproto.MakeTLBusinessGreetingMessage(&mtproto.BusinessGreetingMessage{
		ShortcutId:     in.GetShortcutId(),
		Recipients:     rec,
		NoActivityDays: in.GetNoActivityDays(),
	}).To_BusinessGreetingMessage()
	return b.update(self, func(*[]string) error {
		b.touch(self).greeting = msg
		return nil
	})
}

// SetAway replaces the away message. A nil message clears it.
func (b *Book) SetAway(self int64, in *mtproto.InputBusinessAwayMessage) error {
	if in == nil {
		return b.update(self, func(*[]string) error {
			if r := b.users[self]; r != nil {
				r.away = nil
				r.awayVersion++
			}
			return nil
		})
	}
	rec, err := recipients(self, in.GetRecipients())
	if err != nil {
		return err
	}
	sched, err := schedule(in.GetSchedule())
	if err != nil {
		return err
	}
	if in.GetShortcutId() <= 0 {
		return mtproto.ErrShortcutInvalid
	}
	msg := mtproto.MakeTLBusinessAwayMessage(&mtproto.BusinessAwayMessage{
		OfflineOnly: in.GetOfflineOnly(),
		ShortcutId:  in.GetShortcutId(),
		Schedule:    sched,
		Recipients:  rec,
	}).To_BusinessAwayMessage()
	return b.update(self, func(*[]string) error {
		row := b.touch(self)
		row.away = msg
		row.awayVersion++
		return nil
	})
}

// SetIntro replaces the intro. A nil intro clears it.
// A sticker document is refused: this process does not resolve documents,
// and a placeholder Document would fail the client's file download.
func (b *Book) SetIntro(user int64, in *mtproto.InputBusinessIntro) error {
	if in == nil {
		return b.update(user, func(*[]string) error {
			if r := b.users[user]; r != nil {
				r.intro = nil
			}
			return nil
		})
	}
	if stickerSet(in.GetSticker()) {
		return bad("STICKER_DOCUMENT_INVALID")
	}
	title := in.GetTitle()
	about := in.GetDescription()
	if mtproto.UTF16Len(title) > maxIntroTitle {
		return mtproto.ErrTitleInvalid
	}
	if mtproto.UTF16Len(about) > maxIntroAbout {
		return mtproto.ErrAboutTooLong
	}
	intro := mtproto.MakeTLBusinessIntro(&mtproto.BusinessIntro{
		Title:       title,
		Description: about,
	}).To_BusinessIntro()
	return b.update(user, func(*[]string) error {
		b.touch(user).intro = intro
		return nil
	})
}

// CreateLink appends a chat link and returns the businessChatLink the client stores.
func (b *Book) CreateLink(user int64, in *mtproto.InputBusinessChatLink) (*mtproto.BusinessChatLink, error) {
	row, err := parseLink(in)
	if err != nil {
		return nil, err
	}
	slug, err := newSlug()
	if err != nil {
		return nil, err
	}
	row.slug = slug
	err = b.update(user, func(*[]string) error {
		r := b.touch(user)
		if len(r.links) >= maxLinks {
			return bad("CHATLINKS_TOO_MUCH")
		}
		r.links = append(r.links, row)
		b.slugs[slug] = user
		return nil
	})
	if err != nil {
		return nil, err
	}
	return linkTL(row), nil
}

// EditLink replaces the message and title of an existing slug.
func (b *Book) EditLink(user int64, slug string, in *mtproto.InputBusinessChatLink) (*mtproto.BusinessChatLink, error) {
	parsed, err := parseLink(in)
	if err != nil {
		return nil, err
	}
	var item *linkRow
	err = b.update(user, func(*[]string) error {
		r := b.users[user]
		item = findLink(r, slug)
		if item == nil || b.slugs[slug] != user {
			return bad("SLUG_INVALID")
		}
		item.message = parsed.message
		item.entities = parsed.entities
		item.title = parsed.title
		item.hasTitle = parsed.hasTitle
		return nil
	})
	if err != nil {
		return nil, err
	}
	return linkTL(item), nil
}

// DeleteLink removes a slug owned by user.
func (b *Book) DeleteLink(user int64, slug string) error {
	if slug == "" {
		return bad("SLUG_INVALID")
	}
	return b.update(user, func(drop *[]string) error {
		r := b.users[user]
		if r == nil || (b.slugs[slug] != user && !linkOwned(r, slug)) {
			return bad("SLUG_INVALID")
		}
		next := r.links[:0]
		found := false
		for _, item := range r.links {
			if item.slug == slug {
				found = true
				continue
			}
			next = append(next, item)
		}
		if !found {
			return bad("SLUG_INVALID")
		}
		r.links = next
		delete(b.slugs, slug)
		*drop = append(*drop, slug)
		return nil
	})
}

// Links is account.businessChatLinks for user. Chats and users stay empty:
// a chat link does not reference other peers.
func (b *Book) Links(user int64) *mtproto.Account_BusinessChatLinks {
	links := []*mtproto.BusinessChatLink{}
	if b != nil {
		g := b.userGate(user)
		g.Lock()
		raw, found, enabled, err := b.fetchUser(user)
		b.mu.Lock()
		if err == nil {
			_ = b.applyFetched(user, raw, found, enabled)
		}
		if r := b.users[user]; r != nil {
			for _, item := range r.links {
				links = append(links, linkTL(item))
			}
		}
		b.mu.Unlock()
		g.Unlock()
	}
	return mtproto.MakeTLAccountBusinessChatLinks(&mtproto.Account_BusinessChatLinks{
		Links: links,
		Chats: []*mtproto.Chat{},
		Users: []*mtproto.User{},
	}).To_Account_BusinessChatLinks()
}

// Resolve is account.resolveBusinessChatLink. Each successful resolve counts a view.
func (b *Book) Resolve(slug string) (*mtproto.Account_ResolvedBusinessChatLinks, error) {
	if slug == "" {
		return nil, bad("SLUG_INVALID")
	}
	b.mu.Lock()
	user, ok := b.slugs[slug]
	b.mu.Unlock()
	if !ok {
		owner, found, err := b.readSlug(slug)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, bad("SLUG_INVALID")
		}
		user = owner
	}
	var item *linkRow
	err := b.update(user, func(*[]string) error {
		item = findLink(b.users[user], slug)
		if item == nil {
			return bad("SLUG_INVALID")
		}
		item.views++
		b.slugs[slug] = user
		return nil
	})
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLAccountResolvedBusinessChatLinks(&mtproto.Account_ResolvedBusinessChatLinks{
		Peer:     mtproto.MakePeerUser(user),
		Message:  item.message,
		Entities: cloneEntities(item.entities),
		Chats:    []*mtproto.Chat{},
		Users:    []*mtproto.User{},
	}).To_Account_ResolvedBusinessChatLinks(), nil
}

func linkOwned(r *row, slug string) bool {
	return findLink(r, slug) != nil
}

func findLink(r *row, slug string) *linkRow {
	if r == nil {
		return nil
	}
	for _, item := range r.links {
		if item.slug == slug {
			return item
		}
	}
	return nil
}

func parseLink(in *mtproto.InputBusinessChatLink) (*linkRow, error) {
	if in == nil {
		return nil, bad("LINK_INVALID")
	}
	msg := in.GetMessage()
	if mtproto.UTF16Len(msg) > maxLinkMessage {
		return nil, mtproto.ErrMessageTooLong
	}
	if err := checkEntities(msg, in.GetEntities()); err != nil {
		return nil, err
	}
	title := ""
	hasTitle := in.GetTitle() != nil
	if hasTitle {
		title = in.GetTitle().GetValue()
		if mtproto.UTF16Len(title) > maxLinkTitle || title == "" {
			return nil, mtproto.ErrTitleInvalid
		}
	}
	return &linkRow{
		message:  msg,
		entities: mtproto.SanitizeMessageEntities(msg, in.GetEntities()),
		title:    title,
		hasTitle: hasTitle,
	}, nil
}

func linkTL(item *linkRow) *mtproto.BusinessChatLink {
	var title *wrapperspb.StringValue
	if item.hasTitle {
		title = wrapperspb.String(item.title)
	}
	return mtproto.MakeTLBusinessChatLink(&mtproto.BusinessChatLink{
		Link:     brand.LinkBase() + "/m/" + item.slug,
		Message:  item.message,
		Entities: cloneEntities(item.entities),
		Title:    title,
		Views:    item.views,
	}).To_BusinessChatLink()
}

func newSlug() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", mtproto.ErrInternalServerError
	}
	return hex.EncodeToString(buf[:]), nil
}

func validateHours(hours *mtproto.BusinessWorkHours) error {
	id := hours.GetTimezoneId()
	if id == "" || len(id) > 64 {
		return bad("TIMEZONE_INVALID")
	}
	if _, err := tz.Load(id); err != nil {
		return bad("TIMEZONE_INVALID")
	}
	periods := hours.GetWeeklyOpen()
	if len(periods) == 0 {
		return bad("BUSINESS_WORK_HOURS_EMPTY")
	}
	if len(periods) > maxPeriods {
		return bad("BUSINESS_WORK_HOURS_PERIOD_INVALID")
	}
	type span struct{ start, end int32 }
	spans := make([]span, 0, len(periods))
	for _, p := range periods {
		if p == nil {
			return bad("BUSINESS_WORK_HOURS_PERIOD_INVALID")
		}
		if p.GetStartMinute() < 0 || p.GetEndMinute() > weekMinutes || p.GetStartMinute() >= p.GetEndMinute() {
			return bad("BUSINESS_WORK_HOURS_PERIOD_INVALID")
		}
		spans = append(spans, span{p.GetStartMinute(), p.GetEndMinute()})
	}
	for i := 1; i < len(spans); i++ {
		j := i
		for j > 0 && spans[j].start < spans[j-1].start {
			spans[j], spans[j-1] = spans[j-1], spans[j]
			j--
		}
	}
	for i := 1; i < len(spans); i++ {
		if spans[i].start < spans[i-1].end {
			return bad("BUSINESS_WORK_HOURS_PERIOD_INVALID")
		}
	}
	return nil
}

func copyPeriods(in []*mtproto.BusinessWeeklyOpen) []*mtproto.BusinessWeeklyOpen {
	out := make([]*mtproto.BusinessWeeklyOpen, 0, len(in))
	for _, p := range in {
		out = append(out, mtproto.MakeTLBusinessWeeklyOpen(&mtproto.BusinessWeeklyOpen{
			StartMinute: p.GetStartMinute(),
			EndMinute:   p.GetEndMinute(),
		}).To_BusinessWeeklyOpen())
	}
	return out
}

func openNow(zoneID string, periods []*mtproto.BusinessWeeklyOpen, now time.Time) bool {
	loc, err := tz.Load(zoneID)
	if err != nil {
		return false
	}
	local := now.In(loc)
	weekday := int(local.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	minute := int32((weekday-1)*24*60 + local.Hour()*60 + local.Minute())
	for _, p := range periods {
		if p != nil && minute >= p.GetStartMinute() && minute < p.GetEndMinute() {
			return true
		}
	}
	return false
}

func buildLocation(geo *mtproto.InputGeoPoint, address *wrapperspb.StringValue) (*mtproto.BusinessLocation, error) {
	var point *mtproto.GeoPoint
	if geo != nil && geo.GetPredicateName() != mtproto.Predicate_inputGeoPointEmpty {
		if geo.GetPredicateName() != mtproto.Predicate_inputGeoPoint {
			return nil, mtproto.ErrGeoPointInvalid
		}
		if geo.GetLat() < -90 || geo.GetLat() > 90 || geo.GetLong() < -180 || geo.GetLong() > 180 {
			return nil, mtproto.ErrGeoPointInvalid
		}
		point = mtproto.MakeTLGeoPoint(&mtproto.GeoPoint{
			Lat:  geo.GetLat(),
			Long: geo.GetLong(),
		}).To_GeoPoint()
	}
	addr := ""
	if address != nil {
		addr = address.GetValue()
	}
	if mtproto.UTF16Len(addr) > maxAddress {
		return nil, bad("BUSINESS_ADDRESS_INVALID")
	}
	if point == nil && addr == "" {
		return nil, nil
	}
	return mtproto.MakeTLBusinessLocation(&mtproto.BusinessLocation{
		GeoPoint: point,
		Address:  addr,
	}).To_BusinessLocation(), nil
}

func recipients(self int64, in *mtproto.InputBusinessRecipients) (*mtproto.BusinessRecipients, error) {
	if in == nil {
		return nil, mtproto.ErrBusinessRecipientsEmpty
	}
	ids := make([]int64, 0, len(in.GetUsers()))
	seen := map[int64]bool{}
	for _, user := range in.GetUsers() {
		id, err := inputUser(self, user)
		if err != nil {
			return nil, err
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) > maxRecipients {
		return nil, mtproto.ErrUsersTooMuch
	}
	any := in.GetExistingChats() || in.GetNewChats() || in.GetContacts() || in.GetNonContacts() || len(ids) > 0
	if !any || (in.GetExcludeSelected() && len(ids) == 0 && !in.GetExistingChats() && !in.GetNewChats() && !in.GetContacts() && !in.GetNonContacts()) {
		return nil, mtproto.ErrBusinessRecipientsEmpty
	}
	if ids == nil {
		ids = []int64{}
	}
	return mtproto.MakeTLBusinessRecipients(&mtproto.BusinessRecipients{
		ExistingChats:   in.GetExistingChats(),
		NewChats:        in.GetNewChats(),
		Contacts:        in.GetContacts(),
		NonContacts:     in.GetNonContacts(),
		ExcludeSelected: in.GetExcludeSelected(),
		Users:           ids,
	}).To_BusinessRecipients(), nil
}

func schedule(in *mtproto.BusinessAwayMessageSchedule) (*mtproto.BusinessAwayMessageSchedule, error) {
	if in == nil {
		return nil, bad("SCHEDULE_INVALID")
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_businessAwayMessageScheduleAlways:
		return mtproto.MakeTLBusinessAwayMessageScheduleAlways(&mtproto.BusinessAwayMessageSchedule{}).To_BusinessAwayMessageSchedule(), nil
	case mtproto.Predicate_businessAwayMessageScheduleOutsideWorkHours:
		return mtproto.MakeTLBusinessAwayMessageScheduleOutsideWorkHours(&mtproto.BusinessAwayMessageSchedule{}).To_BusinessAwayMessageSchedule(), nil
	case mtproto.Predicate_businessAwayMessageScheduleCustom:
		if in.GetStartDate() <= 0 || in.GetEndDate() <= in.GetStartDate() {
			return nil, mtproto.ErrScheduleDateInvalid
		}
		return mtproto.MakeTLBusinessAwayMessageScheduleCustom(&mtproto.BusinessAwayMessageSchedule{
			StartDate: in.GetStartDate(),
			EndDate:   in.GetEndDate(),
		}).To_BusinessAwayMessageSchedule(), nil
	default:
		return nil, bad("SCHEDULE_INVALID")
	}
}

func stickerSet(doc *mtproto.InputDocument) bool {
	if doc == nil {
		return false
	}
	return doc.GetPredicateName() != mtproto.Predicate_inputDocumentEmpty
}

func inputUser(self int64, user *mtproto.InputUser) (int64, error) {
	if user == nil {
		return 0, mtproto.ErrUserIdInvalid
	}
	p := mtproto.FromInputUser(self, user)
	switch p.PeerType {
	case mtproto.PEER_SELF:
		if self == 0 {
			return 0, mtproto.ErrUserIdInvalid
		}
		return self, nil
	case mtproto.PEER_USER:
		if p.PeerId == 0 {
			return 0, mtproto.ErrUserIdInvalid
		}
		return p.PeerId, nil
	default:
		return 0, mtproto.ErrUserIdInvalid
	}
}

func checkEntities(text string, entities []*mtproto.MessageEntity) error {
	if len(entities) > maxEntities {
		return mtproto.ErrEntitiesTooLong
	}
	n := mtproto.UTF16Len(text)
	for _, e := range entities {
		if e == nil || e.GetLength() <= 0 || e.GetOffset() < 0 || int64(e.GetOffset())+int64(e.GetLength()) > int64(n) {
			return mtproto.ErrEntityBoundsInvalid
		}
	}
	return nil
}

func bad(code string) error {
	return status.Error(mtproto.ErrBadRequest, code)
}

func cloneHours(in *mtproto.BusinessWorkHours) *mtproto.BusinessWorkHours {
	if in == nil {
		return nil
	}
	return proto.Clone(in).(*mtproto.BusinessWorkHours)
}

func cloneLoc(in *mtproto.BusinessLocation) *mtproto.BusinessLocation {
	if in == nil {
		return nil
	}
	return proto.Clone(in).(*mtproto.BusinessLocation)
}

func cloneGreeting(in *mtproto.BusinessGreetingMessage) *mtproto.BusinessGreetingMessage {
	if in == nil {
		return nil
	}
	return proto.Clone(in).(*mtproto.BusinessGreetingMessage)
}

func cloneAway(in *mtproto.BusinessAwayMessage) *mtproto.BusinessAwayMessage {
	if in == nil {
		return nil
	}
	return proto.Clone(in).(*mtproto.BusinessAwayMessage)
}

func cloneIntro(in *mtproto.BusinessIntro) *mtproto.BusinessIntro {
	if in == nil {
		return nil
	}
	return proto.Clone(in).(*mtproto.BusinessIntro)
}

func cloneEntities(in []*mtproto.MessageEntity) []*mtproto.MessageEntity {
	if len(in) == 0 {
		return nil
	}
	out := make([]*mtproto.MessageEntity, len(in))
	for i, e := range in {
		if e != nil {
			out[i] = proto.Clone(e).(*mtproto.MessageEntity)
		}
	}
	return out
}
