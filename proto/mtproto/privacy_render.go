package mtproto

import (
	"context"
	"sync"
)

// A privacy render cache lives on the request context. Member lists fill it
// once, then every user on that page reads the same answer. A chat-participant
// rule must not open a database query per user.

type privacyCacheKey struct{}

type privacyCache struct {
	mu sync.Mutex
	// user -> chat -> member. A present chat id means membership is known.
	members map[int64]map[int64]bool
	// owner -> the owner blocked the viewer. blockReady means the set was loaded.
	blockReady bool
	blocked    map[int64]bool
}

func newPrivacyCache() *privacyCache {
	return &privacyCache{
		members: map[int64]map[int64]bool{},
		blocked: map[int64]bool{},
	}
}

// WithPrivacyCache returns ctx carrying a request-level privacy cache.
// A context that already has one is returned as-is.
func WithPrivacyCache(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if privacyFrom(ctx) != nil {
		return ctx
	}
	return context.WithValue(ctx, privacyCacheKey{}, newPrivacyCache())
}

func privacyFrom(ctx context.Context) *privacyCache {
	if ctx == nil {
		return nil
	}
	c, _ := ctx.Value(privacyCacheKey{}).(*privacyCache)
	return c
}

// NoteMembership records that userID's membership in every queried chat is known.
// members is the subset they belong to.
func NoteMembership(ctx context.Context, userID int64, queried, members []int64) {
	c := privacyFrom(ctx)
	if c == nil || userID == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	set := c.members[userID]
	if set == nil {
		set = map[int64]bool{}
		c.members[userID] = set
	}
	in := map[int64]struct{}{}
	for _, id := range members {
		in[id] = struct{}{}
	}
	for _, id := range queried {
		_, ok := in[id]
		set[id] = ok
	}
}

// Membership reports whether userID is in any of chats.
// ok is false when any chat has not been recorded for this request.
func Membership(ctx context.Context, userID int64, chats []int64) (member bool, ok bool) {
	c := privacyFrom(ctx)
	if c == nil || userID == 0 || len(chats) == 0 {
		return false, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	set := c.members[userID]
	if set == nil {
		return false, false
	}
	for _, id := range chats {
		in, known := set[id]
		if !known {
			return false, false
		}
		if in {
			member = true
		}
	}
	return member, true
}

// NoteBlocked records which profile owners blocked the viewer.
// Every id in owners was checked.
func NoteBlocked(ctx context.Context, owners, blocked []int64) {
	c := privacyFrom(ctx)
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	in := map[int64]struct{}{}
	for _, id := range blocked {
		in[id] = struct{}{}
	}
	for _, id := range owners {
		_, ok := in[id]
		c.blocked[id] = ok
	}
	c.blockReady = true
}

// OwnerBlockedViewer reports whether the profile owner blocked the viewer.
// ok is false when this request did not load block rows.
func OwnerBlockedViewer(ctx context.Context, owner int64) (blocked bool, ok bool) {
	c := privacyFrom(ctx)
	if c == nil || owner == 0 {
		return false, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.blockReady {
		return false, false
	}
	return c.blocked[owner], true
}

// PrefetchPrivacy loads chat membership and block rows for one viewer and a
// page of users. pkg/privacymember installs it. Nil means those facts stay unknown.
var PrefetchPrivacy func(ctx context.Context, viewer int64, owners, chats []int64)

// PreparePrivacyRender attaches a request cache and fills it for users.
// One call covers the whole page.
func PreparePrivacyRender(ctx context.Context, viewer int64, users []*ImmutableUser) context.Context {
	ctx = WithPrivacyCache(ctx)
	if PrefetchPrivacy == nil || viewer == 0 {
		return ctx
	}
	owners, chats := privacyRenderKeys(viewer, users)
	if len(owners) == 0 && len(chats) == 0 {
		return ctx
	}
	PrefetchPrivacy(ctx, viewer, owners, chats)
	return ctx
}

func privacyRenderKeys(viewer int64, users []*ImmutableUser) (owners, chats []int64) {
	seenOwner := map[int64]struct{}{}
	seenChat := map[int64]struct{}{}
	for _, u := range users {
		if u == nil || u.Id() == 0 || u.Id() == viewer {
			continue
		}
		if _, ok := seenOwner[u.Id()]; !ok {
			seenOwner[u.Id()] = struct{}{}
			owners = append(owners, u.Id())
		}
		for _, key := range u.KeysPrivacyRules {
			for _, rule := range key.GetRules() {
				if rule == nil {
					continue
				}
				switch rule.PredicateName {
				case Predicate_privacyValueAllowChatParticipants, Predicate_privacyValueDisallowChatParticipants:
				default:
					continue
				}
				for _, id := range rule.Chats {
					if id == 0 {
						continue
					}
					if _, ok := seenChat[id]; ok {
						continue
					}
					seenChat[id] = struct{}{}
					chats = append(chats, id)
				}
			}
		}
	}
	return owners, chats
}
