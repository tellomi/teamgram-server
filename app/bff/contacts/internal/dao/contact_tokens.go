package dao

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/teamgram/marmota/pkg/stores/kv"
	gokv "github.com/zeromicro/go-zero/core/stores/kv"
)

// Profile links and free-message links.
//
// contacts.exportContactToken returns https://t.me/contact/<token>
// (core.telegram.org/api/links). The link stays valid until expires.
//
// contacts.exportNoPaidMessagesToken returns a link whose free= query
// is the no-paid token. Telegram Desktop (local_url_handlers.cpp) reads
// that parameter as noPaidToken and later calls
// contacts.importNoPaidMessagesToken. temporary tokens last one minute,
// matching the client string "Expire after 1 minute". revoke replaces
// the previous tokens for this user.
const (
	contactShareTTL    = 24 * time.Hour
	noPaidTemporaryTTL = time.Minute
	// Carrier links for a permanent free-message URL have to outlive a
	// normal profile link, because the URL embeds a contact token when
	// the owner has no username.
	noPaidPermanentTTL = 10 * 365 * 24 * time.Hour

	tokenKeyPrefix = "contacts_token_v1:"
)

// CarrierTTL is the lifetime of a contact token used inside a permanent free-message link.
func CarrierTTL() time.Duration { return noPaidPermanentTTL }

// TokenBook stores contact-profile tokens and no-paid-message tokens.
type TokenBook struct {
	kv  tokenKV
	now func() time.Time
}

type tokenKV interface {
	GetCtx(ctx context.Context, key string) (string, error)
	SetCtx(ctx context.Context, key, value string) error
	SetexCtx(ctx context.Context, key, value string, seconds int) error
	DelCtx(ctx context.Context, keys ...string) (int, error)
}

type contactRecord struct {
	Owner   int64 `json:"owner"`
	Expires int32 `json:"expires"`
}

type noPaidRecord struct {
	Owner     int64 `json:"owner"`
	Expires   int32 `json:"expires"`
	Temporary bool  `json:"temporary"`
}

// NewTokenBook uses Redis when KV is configured and an in-process map otherwise.
func NewTokenBook(conf gokv.KvConf) *TokenBook {
	var store tokenKV
	if len(conf) == 0 {
		store = newMemKV()
	} else {
		store = kv.NewStore(conf)
	}
	return &TokenBook{kv: store, now: time.Now}
}

// ExportContact returns the caller's current profile token, minting one
// that lasts contactShareTTL when the previous one is missing or expired.
func (b *TokenBook) ExportContact(ctx context.Context, owner int64) (token string, expires int32, err error) {
	exp := int32(b.now().Add(contactShareTTL).Unix())
	return b.ensureContact(ctx, owner, "share", exp)
}

// EnsureContactCarrier returns a contact token that is still valid at minExpires.
func (b *TokenBook) EnsureContactCarrier(ctx context.Context, owner int64, minExpires int32) (string, error) {
	token, _, err := b.ensureContact(ctx, owner, "carrier", minExpires)
	return token, err
}

func (b *TokenBook) ensureContact(ctx context.Context, owner int64, slot string, minExpires int32) (string, int32, error) {
	indexKey := tokenKeyPrefix + "contact_slot:" + slot + ":" + strconv.FormatInt(owner, 10)
	if raw, err := b.kv.GetCtx(ctx, indexKey); err == nil && raw != "" {
		if rec, ok := b.liveContact(ctx, raw); ok && (slot == "share" || rec.Expires >= minExpires) {
			return raw, rec.Expires, nil
		}
	}
	token, err := newToken()
	if err != nil {
		return "", 0, err
	}
	rec := contactRecord{Owner: owner, Expires: minExpires}
	if err := b.putJSON(ctx, tokenKeyPrefix+"contact:"+token, rec, minExpires); err != nil {
		return "", 0, err
	}
	ttl := secondsUntil(b.now(), minExpires)
	if err := b.set(ctx, indexKey, token, ttl); err != nil {
		return "", 0, err
	}
	return token, minExpires, nil
}

// LookupContact returns the owner of a profile token.
func (b *TokenBook) LookupContact(ctx context.Context, token string) (int64, error) {
	rec, ok := b.liveContact(ctx, token)
	if !ok {
		return 0, errNotFound
	}
	return rec.Owner, nil
}

func (b *TokenBook) liveContact(ctx context.Context, token string) (contactRecord, bool) {
	var rec contactRecord
	raw, err := b.kv.GetCtx(ctx, tokenKeyPrefix+"contact:"+token)
	if err != nil || raw == "" || json.Unmarshal([]byte(raw), &rec) != nil || rec.Owner == 0 {
		return contactRecord{}, false
	}
	if rec.Expires != 0 && rec.Expires <= int32(b.now().Unix()) {
		return contactRecord{}, false
	}
	return rec, true
}

// NoPaidExport is the stored free-message token.
type NoPaidExport struct {
	Token     string
	Expires   int32
	Temporary bool
}

// ExportNoPaid mints or reuses a free-message token for owner.
// revoke drops every previous token for that user first.
func (b *TokenBook) ExportNoPaid(ctx context.Context, owner int64, revoke, temporary bool) (NoPaidExport, error) {
	if revoke {
		b.revokeNoPaid(ctx, owner)
	} else if cur, ok := b.currentNoPaid(ctx, owner, temporary); ok {
		return cur, nil
	}
	token, err := newToken()
	if err != nil {
		return NoPaidExport{}, err
	}
	var expires int32
	if temporary {
		expires = int32(b.now().Add(noPaidTemporaryTTL).Unix())
	}
	rec := noPaidRecord{Owner: owner, Expires: expires, Temporary: temporary}
	var storeUntil int32
	if temporary {
		storeUntil = expires
	} else {
		storeUntil = int32(b.now().Add(noPaidPermanentTTL).Unix())
	}
	if err := b.putJSON(ctx, tokenKeyPrefix+"nopaid:"+token, rec, storeUntil); err != nil {
		return NoPaidExport{}, err
	}
	if err := b.addNoPaidIndex(ctx, owner, token); err != nil {
		return NoPaidExport{}, err
	}
	return NoPaidExport{Token: token, Expires: expires, Temporary: temporary}, nil
}

// LookupNoPaid returns the owner when token is live and belongs to wantOwner.
// wantOwner 0 skips the owner check.
func (b *TokenBook) LookupNoPaid(ctx context.Context, token string, wantOwner int64) (int64, error) {
	var rec noPaidRecord
	raw, err := b.kv.GetCtx(ctx, tokenKeyPrefix+"nopaid:"+token)
	if err != nil || raw == "" || json.Unmarshal([]byte(raw), &rec) != nil || rec.Owner == 0 {
		return 0, errNotFound
	}
	if rec.Expires != 0 && rec.Expires <= int32(b.now().Unix()) {
		return 0, errNotFound
	}
	if wantOwner != 0 && rec.Owner != wantOwner {
		return 0, errNotFound
	}
	return rec.Owner, nil
}

// GrantNoPaid records that sender may message owner without a paid-message fee.
func (b *TokenBook) GrantNoPaid(ctx context.Context, owner, sender int64) error {
	key := exceptionKey(owner, sender)
	return b.kv.SetCtx(ctx, key, "1")
}

// HasNoPaid reports whether sender was granted a free-message exception by owner.
func (b *TokenBook) HasNoPaid(ctx context.Context, owner, sender int64) bool {
	raw, err := b.kv.GetCtx(ctx, exceptionKey(owner, sender))
	return err == nil && raw == "1"
}

func exceptionKey(owner, sender int64) string {
	return tokenKeyPrefix + "exc:" + strconv.FormatInt(owner, 10) + ":" + strconv.FormatInt(sender, 10)
}

func (b *TokenBook) currentNoPaid(ctx context.Context, owner int64, temporary bool) (NoPaidExport, bool) {
	for _, token := range b.noPaidIndex(ctx, owner) {
		var rec noPaidRecord
		raw, err := b.kv.GetCtx(ctx, tokenKeyPrefix+"nopaid:"+token)
		if err != nil || raw == "" || json.Unmarshal([]byte(raw), &rec) != nil {
			continue
		}
		if rec.Temporary != temporary {
			continue
		}
		if rec.Expires != 0 && rec.Expires <= int32(b.now().Unix()) {
			continue
		}
		return NoPaidExport{Token: token, Expires: rec.Expires, Temporary: rec.Temporary}, true
	}
	return NoPaidExport{}, false
}

func (b *TokenBook) revokeNoPaid(ctx context.Context, owner int64) {
	for _, token := range b.noPaidIndex(ctx, owner) {
		_, _ = b.kv.DelCtx(ctx, tokenKeyPrefix+"nopaid:"+token)
	}
	_, _ = b.kv.DelCtx(ctx, b.noPaidIndexKey(owner))
}

func (b *TokenBook) noPaidIndexKey(owner int64) string {
	return tokenKeyPrefix + "nopaid_idx:" + strconv.FormatInt(owner, 10)
}

func (b *TokenBook) noPaidIndex(ctx context.Context, owner int64) []string {
	raw, err := b.kv.GetCtx(ctx, b.noPaidIndexKey(owner))
	if err != nil || raw == "" {
		return nil
	}
	var tokens []string
	if json.Unmarshal([]byte(raw), &tokens) != nil {
		return nil
	}
	return tokens
}

func (b *TokenBook) addNoPaidIndex(ctx context.Context, owner int64, token string) error {
	tokens := append(b.noPaidIndex(ctx, owner), token)
	raw, err := json.Marshal(tokens)
	if err != nil {
		return err
	}
	// The index outlives every token it names. Each token key has its own TTL,
	// and revoke deletes the index outright.
	return b.kv.SetCtx(ctx, b.noPaidIndexKey(owner), string(raw))
}

func (b *TokenBook) putJSON(ctx context.Context, key string, v interface{}, expires int32) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return b.set(ctx, key, string(raw), secondsUntil(b.now(), expires))
}

func (b *TokenBook) set(ctx context.Context, key, value string, ttl int) error {
	if ttl > 0 {
		return b.kv.SetexCtx(ctx, key, value, ttl)
	}
	return b.kv.SetCtx(ctx, key, value)
}

func secondsUntil(now time.Time, expires int32) int {
	sec := int(time.Unix(int64(expires), 0).Sub(now).Seconds())
	if sec < 1 {
		return 1
	}
	return sec
}

func newToken() (string, error) {
	var buf [18]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf[:]), nil
}

var errNotFound = errToken("not found")

type errToken string

func (e errToken) Error() string { return string(e) }

// NormalizeContactToken accepts the raw token or a t.me / tg contact link.
func NormalizeContactToken(raw string) string {
	raw = strings.TrimSpace(raw)
	for _, p := range []string{
		"https://t.me/contact/",
		"http://t.me/contact/",
		"https://telegram.me/contact/",
		"tg://contact?token=",
	} {
		if strings.HasPrefix(strings.ToLower(raw), strings.ToLower(p)) {
			raw = raw[len(p):]
			break
		}
	}
	if i := strings.IndexAny(raw, "?&#"); i >= 0 {
		raw = raw[:i]
	}
	return raw
}

// NormalizeNoPaidToken accepts the raw token or a link that carries free=.
func NormalizeNoPaidToken(raw string) string {
	raw = strings.TrimSpace(raw)
	if i := strings.Index(raw, "free="); i >= 0 {
		raw = raw[i+len("free="):]
	}
	if i := strings.IndexAny(raw, "&?#"); i >= 0 {
		raw = raw[:i]
	}
	return raw
}

// ContactLink is the https profile link from core.telegram.org/api/links.
func ContactLink(token string) string {
	return "https://t.me/contact/" + token
}

// NoPaidLink is the link Desktop turns into tg://contact?token=&free= or
// tg://resolve?domain=&free=.
func NoPaidLink(username, contactToken, freeToken string) string {
	if usernameOK(username) {
		return "https://t.me/" + username + "?free=" + freeToken
	}
	return "https://t.me/contact/" + contactToken + "?free=" + freeToken
}

func usernameOK(name string) bool {
	if name == "" || len(name) > 32 {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
		default:
			return false
		}
	}
	return true
}

type memKV struct {
	mu sync.Mutex
	m  map[string]memVal
}

type memVal struct {
	v      string
	expire time.Time
}

func newMemKV() *memKV {
	return &memKV{m: map[string]memVal{}}
}

func (m *memKV) GetCtx(_ context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ent, ok := m.m[key]
	if !ok {
		return "", nil
	}
	if !ent.expire.IsZero() && time.Now().After(ent.expire) {
		delete(m.m, key)
		return "", nil
	}
	return ent.v, nil
}

func (m *memKV) SetCtx(_ context.Context, key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[key] = memVal{v: value}
	return nil
}

func (m *memKV) SetexCtx(_ context.Context, key, value string, seconds int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[key] = memVal{v: value, expire: time.Now().Add(time.Duration(seconds) * time.Second)}
	return nil
}

func (m *memKV) DelCtx(_ context.Context, keys ...string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, key := range keys {
		if _, ok := m.m[key]; ok {
			delete(m.m, key)
			n++
		}
	}
	return n, nil
}
