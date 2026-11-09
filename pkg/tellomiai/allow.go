package tellomiai

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"
)

var errNoDirectory = errors.New("tellomi ai: username lookup is not configured")

const (
	// EnvAllowUsernames is a comma-separated username list, without @.
	// Comparison is case-insensitive. "*" allows every user. Empty uses
	// DefaultAllowUsernames.
	EnvAllowUsernames = "TELLOMI_AI_ALLOW_USERNAMES"
	// EnvAllowUserIDs is an optional comma-separated list of numeric user ids.
	// Those ids are allowed without a username lookup.
	EnvAllowUserIDs = "TELLOMI_AI_ALLOW_USER_IDS"

	// DefaultAllowUsernames is the list used when TELLOMI_AI_ALLOW_USERNAMES is unset or blank.
	DefaultAllowUsernames = "erel,lfnuebfci,pikaqiu"

	// AllowCacheFor is how long a successful username resolution is reused.
	AllowCacheFor = time.Minute
	// allowErrorCacheFor is shorter so a directory blip does not stick.
	allowErrorCacheFor = 15 * time.Second
)

// UsernameDirectory resolves usernames (no @) to user ids.
// The existing user service lookup is wired in app/bff/bots.
// Names that are not users are omitted. A transport failure returns an error.
type UsernameDirectory interface {
	Users(ctx context.Context, names []string) (map[string]int64, error)
}

// RealNameChecker is the future replacement for the username allowlist.
// Verified reports whether userID has completed real-name verification.
//
// SetRealNameChecker installs one. While the installed checker is nil, the
// allowlist decides. A non-nil checker replaces the allowlist: a verified
// user is allowed, and an unverified user receives the not-allowed sentence.
// The checker is not consulted for any other decision.
type RealNameChecker interface {
	Verified(ctx context.Context, userID int64) (bool, error)
}

var (
	realNameMu sync.RWMutex
	realName   RealNameChecker
)

// SetRealNameChecker installs the real-name gate that replaces the allowlist.
// Pass nil to use the allowlist again. It is safe to call before Start.
func SetRealNameChecker(c RealNameChecker) {
	realNameMu.Lock()
	realName = c
	realNameMu.Unlock()
}

func currentRealName() RealNameChecker {
	realNameMu.RLock()
	defer realNameMu.RUnlock()
	return realName
}

// Allow is the set of users who may call the model.
// A nil *Allow does not restrict. Start installs one from the environment.
type Allow struct {
	everyone bool
	names    []string
	ids      map[int64]struct{}
	dir      UsernameDirectory
	ttl      time.Duration
	now      func() time.Time

	mu       sync.Mutex
	resolved map[int64]struct{}
	fail     error
	until    time.Time
}

// AllowFromEnv reads TELLOMI_AI_ALLOW_USERNAMES and TELLOMI_AI_ALLOW_USER_IDS.
// A blank username list uses DefaultAllowUsernames. "*" allows everyone and
// does not call dir.
func AllowFromEnv(dir UsernameDirectory) *Allow {
	names := getenv(EnvAllowUsernames)
	if names == "" {
		names = DefaultAllowUsernames
	}
	return ParseAllow(names, getenv(EnvAllowUserIDs), dir)
}

// ParseAllow builds a list from an explicit username string and id string.
// Unlike AllowFromEnv, a blank username string does not insert the default
// names. "*" allows everyone.
func ParseAllow(names, ids string, dir UsernameDirectory) *Allow {
	everyone, list := parseAllowNames(names)
	return &Allow{
		everyone: everyone,
		names:    list,
		ids:      parseAllowIDs(ids),
		dir:      dir,
		ttl:      AllowCacheFor,
	}
}

// Allowed reports whether userID may use the assistant.
// A directory failure is returned so the caller can send the busy sentence
// instead of treating a lookup outage as "not allowed".
func (a *Allow) Allowed(ctx context.Context, userID int64) (bool, error) {
	if a == nil || a.everyone {
		return true, nil
	}
	if userID <= 0 {
		return false, nil
	}
	if _, ok := a.ids[userID]; ok {
		return true, nil
	}
	if len(a.names) == 0 {
		return false, nil
	}
	ids, err := a.userIDs(ctx)
	if err != nil {
		return false, err
	}
	_, ok := ids[userID]
	return ok, nil
}

func (a *Allow) userIDs(ctx context.Context) (map[int64]struct{}, error) {
	now := time.Now()
	if a.now != nil {
		now = a.now()
	}
	a.mu.Lock()
	if now.Before(a.until) {
		ids, err := a.resolved, a.fail
		a.mu.Unlock()
		return ids, err
	}
	a.mu.Unlock()

	if a.dir == nil {
		return nil, errNoDirectory
	}
	got, err := a.dir.Users(ctx, append([]string(nil), a.names...))
	ids := map[int64]struct{}{}
	if err == nil {
		for _, id := range got {
			if id > 0 {
				ids[id] = struct{}{}
			}
		}
	}
	ttl := a.ttl
	if ttl <= 0 {
		ttl = AllowCacheFor
	}
	if err != nil {
		ttl = allowErrorCacheFor
		if a.ttl > 0 && a.ttl < ttl {
			ttl = a.ttl
		}
	}
	a.mu.Lock()
	a.resolved = ids
	a.fail = err
	a.until = now.Add(ttl)
	a.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func parseAllowNames(raw string) (everyone bool, names []string) {
	raw = strings.TrimSpace(raw)
	if raw == "*" {
		return true, nil
	}
	seen := map[string]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		name := strings.TrimPrefix(strings.TrimSpace(part), "@")
		name = strings.ToLower(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return false, names
}

func parseAllowIDs(raw string) map[int64]struct{} {
	out := map[int64]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.ParseInt(part, 10, 64)
		if err != nil || n <= 0 {
			continue
		}
		out[n] = struct{}{}
	}
	return out
}
