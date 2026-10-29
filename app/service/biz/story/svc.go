package story

import (
	"context"
	"hash/fnv"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
)

// Hooks connect stories to contacts, usernames, and the update fanout.
// All of them are optional. Tests use MarkContact instead of the contact hooks.
type Hooks struct {
	Push             func(ctx context.Context, userID int64, excludeAuth []int64, updates *mtproto.Updates)
	ContactIDs       func(ctx context.Context, userID int64) ([]int64, error)
	IsContact        func(ctx context.Context, owner, viewer int64) bool
	IsCloseFriend    func(ctx context.Context, owner, viewer int64) bool
	SetUserMax       func(ctx context.Context, userID int64, maxID int32)
	SetStoriesHidden func(ctx context.Context, userID, peerID int64, hidden bool)
	Username         func(ctx context.Context, userID int64) string
	MatchName        func(ctx context.Context, userID int64, q string) bool
}

// Service implements stories for users and channels.
type Service struct {
	store        Store
	mu           sync.Mutex
	hooks        Hooks
	contacts     map[int64]map[int64]bool
	closeFriends map[int64]map[int64]bool
	clock        func() int32
}

func New(store Store) *Service {
	if store == nil {
		store = NewMemoryStore()
	}
	return &Service{
		store:        store,
		contacts:     map[int64]map[int64]bool{},
		closeFriends: map[int64]map[int64]bool{},
	}
}

func (s *Service) SetHooks(h Hooks) {
	s.mu.Lock()
	s.hooks = h
	s.mu.Unlock()
}

func (s *Service) hooksCopy() Hooks {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hooks
}

func (s *Service) hooksEmpty() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hooks.Push == nil && s.hooks.ContactIDs == nil && s.hooks.IsContact == nil
}

// SetClock overrides the clock. Tests use it to expire stories.
func (s *Service) SetClock(now func() int32) {
	s.mu.Lock()
	s.clock = now
	s.mu.Unlock()
}

func (s *Service) now() int32 {
	s.mu.Lock()
	fn := s.clock
	s.mu.Unlock()
	if fn != nil {
		return fn()
	}
	return int32(time.Now().Unix())
}

// MarkContact records a mutual contact. Both users see each other's contact-visible stories.
func (s *Service) MarkContact(a, b int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	addPair(s.contacts, a, b)
	addPair(s.contacts, b, a)
}

// MarkCloseFriend records owner’s close-friend list. The friend is also a contact.
func (s *Service) MarkCloseFriend(owner, friend int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	addPair(s.contacts, owner, friend)
	addPair(s.contacts, friend, owner)
	addPair(s.closeFriends, owner, friend)
}

func addPair(m map[int64]map[int64]bool, a, b int64) {
	if m[a] == nil {
		m[a] = map[int64]bool{}
	}
	m[a][b] = true
}

func (s *Service) isContact(ctx context.Context, owner, viewer int64) bool {
	if owner == viewer {
		return true
	}
	s.mu.Lock()
	local := s.contacts[owner][viewer]
	hook := s.hooks.IsContact
	s.mu.Unlock()
	if local {
		return true
	}
	return hook != nil && hook(ctx, owner, viewer)
}

func (s *Service) isCloseFriend(ctx context.Context, owner, viewer int64) bool {
	s.mu.Lock()
	local := s.closeFriends[owner][viewer]
	hook := s.hooks.IsCloseFriend
	s.mu.Unlock()
	if local {
		return true
	}
	return hook != nil && hook(ctx, owner, viewer)
}

func (s *Service) contactIDs(ctx context.Context, userID int64) []int64 {
	s.mu.Lock()
	var ids []int64
	for id := range s.contacts[userID] {
		ids = append(ids, id)
	}
	hook := s.hooks.ContactIDs
	s.mu.Unlock()
	if hook != nil {
		if extra, err := hook(ctx, userID); err == nil {
			ids = append(ids, extra...)
		}
	}
	return unique64(ids)
}

func (s *Service) canPost(ctx context.Context, actor int64, owner Owner, accessHash int64) error {
	if !owner.ok() {
		return mtproto.ErrPeerIdInvalid
	}
	if owner.Kind == KindUser {
		if actor != owner.ID {
			return mtproto.ErrPeerIdInvalid
		}
		return nil
	}
	_, post, _, _, err := channel.Shared().StoryPerm(ctx, actor, owner.ID, accessHash)
	if err != nil {
		return err
	}
	if !post {
		return mtproto.ErrChatAdminRequired
	}
	return nil
}

func (s *Service) canEdit(ctx context.Context, actor int64, owner Owner, accessHash int64) error {
	if owner.Kind == KindUser {
		return s.canPost(ctx, actor, owner, accessHash)
	}
	_, _, edit, _, err := channel.Shared().StoryPerm(ctx, actor, owner.ID, accessHash)
	if err != nil {
		return err
	}
	if !edit {
		return mtproto.ErrChatAdminRequired
	}
	return nil
}

func (s *Service) canDelete(ctx context.Context, actor int64, owner Owner, accessHash int64) error {
	if owner.Kind == KindUser {
		return s.canPost(ctx, actor, owner, accessHash)
	}
	_, _, _, del, err := channel.Shared().StoryPerm(ctx, actor, owner.ID, accessHash)
	if err != nil {
		return err
	}
	if !del {
		return mtproto.ErrChatAdminRequired
	}
	return nil
}

func (s *Service) canViewPeer(ctx context.Context, viewer int64, owner Owner, accessHash int64) error {
	if !owner.ok() {
		return mtproto.ErrPeerIdInvalid
	}
	if owner.Kind == KindUser {
		return nil
	}
	view, _, _, _, err := channel.Shared().StoryPerm(ctx, viewer, owner.ID, accessHash)
	if err != nil {
		return err
	}
	if !view {
		return mtproto.ErrChannelPrivate
	}
	return nil
}

func (s *Service) isManager(ctx context.Context, viewer int64, owner Owner) bool {
	if owner.Kind == KindUser {
		return viewer == owner.ID
	}
	_, post, edit, del, err := channel.Shared().StoryPerm(ctx, viewer, owner.ID, 0)
	return err == nil && (post || edit || del)
}

func privacySubject(rec *Record) int64 {
	if rec.Owner.Kind == KindUser {
		return rec.Owner.ID
	}
	return rec.Sender
}

func (s *Service) privacyAllows(ctx context.Context, rec *Record, viewer int64) bool {
	if rec == nil || rec.Item == nil {
		return false
	}
	if rec.Owner.Kind == KindUser && viewer == rec.Owner.ID {
		return true
	}
	if rec.Owner.Kind == KindChannel && s.isManager(ctx, viewer, rec.Owner) {
		return true
	}
	rules := rec.Item.GetPrivacy()
	if len(rules) == 0 {
		return true
	}
	for _, r := range rules {
		if r.GetPredicateName() == mtproto.Predicate_privacyValueDisallowUsers && contains64(r.Users, viewer) {
			return false
		}
	}
	var allowAll, allowContacts, sawAllow bool
	for _, r := range rules {
		switch r.GetPredicateName() {
		case mtproto.Predicate_privacyValueAllowAll:
			allowAll = true
			sawAllow = true
		case mtproto.Predicate_privacyValueAllowContacts:
			allowContacts = true
			sawAllow = true
		case mtproto.Predicate_privacyValueAllowCloseFriends:
			sawAllow = true
			if s.isCloseFriend(ctx, privacySubject(rec), viewer) {
				return true
			}
		case mtproto.Predicate_privacyValueAllowUsers:
			sawAllow = true
			if contains64(r.Users, viewer) {
				return true
			}
		}
	}
	if allowAll {
		return true
	}
	if allowContacts && s.isContact(ctx, privacySubject(rec), viewer) {
		return true
	}
	return !sawAllow
}

func (s *Service) visible(ctx context.Context, viewer int64, rec *Record, includeExpired, pinnedOnly bool) bool {
	if rec == nil || rec.Deleted || rec.Item == nil {
		return false
	}
	if pinnedOnly && !rec.Item.Pinned {
		return false
	}
	if !includeExpired && !rec.active(s.now()) {
		return false
	}
	if !s.privacyAllows(ctx, rec, viewer) {
		return false
	}
	return true
}

func applyPrivacyFlags(item *mtproto.StoryItem) {
	item.Public = false
	item.Contacts = false
	item.CloseFriends = false
	item.SelectedContacts = false
	for _, r := range item.Privacy {
		switch r.GetPredicateName() {
		case mtproto.Predicate_privacyValueAllowAll:
			item.Public = true
		case mtproto.Predicate_privacyValueAllowContacts:
			item.Contacts = true
		case mtproto.Predicate_privacyValueAllowCloseFriends:
			item.CloseFriends = true
		case mtproto.Predicate_privacyValueAllowUsers:
			item.SelectedContacts = true
		}
	}
}

func allowedUsers(rules []*mtproto.PrivacyRule) []int64 {
	var ids []int64
	for _, r := range rules {
		if r.GetPredicateName() == mtproto.Predicate_privacyValueAllowUsers {
			ids = append(ids, r.Users...)
		}
	}
	return ids
}

func normalizePeriod(p int32) (int32, error) {
	if p == 0 {
		return defaultPeriod, nil
	}
	switch p {
	case 6 * 3600, 12 * 3600, 24 * 3600, 48 * 3600:
		return p, nil
	default:
		return 0, mtproto.ErrStoryPeriodInvalid
	}
}

func (s *Service) activeMax(ctx context.Context, owner Owner) (max int32, live bool) {
	recs, _ := s.store.ListByOwner(ctx, owner)
	now := s.now()
	for _, r := range recs {
		if r.active(now) && r.id() >= max {
			max = r.id()
			live = r.Item.GetLive()
		}
	}
	return max, live
}

func (s *Service) syncUserMax(ctx context.Context, owner Owner) {
	if owner.Kind != KindUser {
		return
	}
	s.mu.Lock()
	fn := s.hooks.SetUserMax
	s.mu.Unlock()
	if fn == nil {
		return
	}
	max, _ := s.activeMax(ctx, owner)
	fn(ctx, owner.ID, max)
}

func (s *Service) audience(ctx context.Context, actor int64, rec *Record) []int64 {
	set := map[int64]bool{actor: true}
	if rec.Owner.Kind == KindChannel {
		ids, _ := channel.Shared().MemberIDs(ctx, rec.Owner.ID)
		for _, id := range ids {
			if s.privacyAllows(ctx, rec, id) {
				set[id] = true
			}
		}
	} else {
		for _, id := range s.contactIDs(ctx, rec.Owner.ID) {
			if s.privacyAllows(ctx, rec, id) {
				set[id] = true
			}
		}
		for _, id := range allowedUsers(rec.Item.GetPrivacy()) {
			if s.privacyAllows(ctx, rec, id) {
				set[id] = true
			}
		}
	}
	out := make([]int64, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (s *Service) push(ctx context.Context, userID int64, exclude []int64, up *mtproto.Updates) {
	s.mu.Lock()
	fn := s.hooks.Push
	s.mu.Unlock()
	if fn == nil || up == nil {
		return
	}
	fn(ctx, userID, exclude, up)
}

func (s *Service) fanout(ctx context.Context, actor, auth int64, rec *Record) {
	for _, uid := range s.audience(ctx, actor, rec) {
		up := s.storyUpdates(ctx, uid, rec, uid == actor)
		var exclude []int64
		if uid == actor && auth != 0 {
			exclude = []int64{auth}
		}
		s.push(ctx, uid, exclude, up)
	}
}

func contains64(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func unique64(ids []int64) []int64 {
	seen := map[int64]bool{}
	var out []int64
	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func contains32(ids []int32, id int32) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func clampLimit(v, def, max int32) int32 {
	if v <= 0 {
		return def
	}
	if v > max {
		return max
	}
	return v
}

func linkHost() string {
	if h := strings.TrimRight(os.Getenv("TEAMGRAM_TME_HOST"), "/"); h != "" {
		return h
	}
	return "https://t.me"
}

func hashState(hidden bool, parts []string) string {
	h := fnv.New64a()
	if hidden {
		_, _ = h.Write([]byte{1})
	} else {
		_, _ = h.Write([]byte{0})
	}
	for _, p := range parts {
		_, _ = h.Write([]byte(p))
		_, _ = h.Write([]byte{0})
	}
	prefix := "s:"
	if hidden {
		prefix = "h:"
	}
	return prefix + strconv.FormatUint(h.Sum64(), 16)
}

func pageAfter(ids []int32, offset, limit int32) ([]int32, int32) {
	if offset > 0 {
		cut := 0
		for cut < len(ids) && ids[cut] != offset {
			cut++
		}
		if cut < len(ids) && ids[cut] == offset {
			ids = ids[cut+1:]
		}
	}
	total := int32(len(ids))
	if limit > 0 && int(limit) < len(ids) {
		ids = ids[:limit]
	}
	return ids, total
}

func protoCloneItem(item *mtproto.StoryItem) *mtproto.StoryItem {
	if item == nil {
		return nil
	}
	return proto.Clone(item).(*mtproto.StoryItem)
}

func optionalInt(v int32) *wrapperspb.Int32Value {
	if v == 0 {
		return nil
	}
	return &wrapperspb.Int32Value{Value: v}
}
