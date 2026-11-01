package book

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/proto"
)

const userKeyPrefix = "passport_user_v1:"

type userBox struct {
	secret   int64
	values   map[string]*mtproto.SecureValue
	files    map[int64]*mtproto.SecureFile
	verified map[string]struct{}
	codes    []phoneCode
	errors   map[int64][]*mtproto.SecureValueError
	grants   []Grant
}

func newUserBox() *userBox {
	return &userBox{
		values:   map[string]*mtproto.SecureValue{},
		files:    map[int64]*mtproto.SecureFile{},
		verified: map[string]struct{}{},
		errors:   map[int64][]*mtproto.SecureValueError{},
	}
}

// Save stores one encrypted secure value.
// https://core.telegram.org/method/account.saveSecureValue
//
// secure_secret_id is the passport secret hash the client derived. Zero is
// SECURE_SECRET_REQUIRED. A different non-zero id replaces the secret and
// drops previously stored values, because this server cannot re-encrypt them.
func (b *Book) Save(ctx context.Context, user, secretID int64, in *mtproto.InputSecureValue) (*mtproto.SecureValue, error) {
	if secretID == 0 {
		return nil, mtproto.ErrSecureSecretRequired
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	box, err := b.ready(ctx, user)
	if err != nil {
		return nil, err
	}
	// A new passport secret drops old ciphertext only after the new value
	// validates. A rejected upload must leave the previous record in place.
	rotating := box.secret != 0 && box.secret != secretID
	src := box
	if rotating {
		src = newUserBox()
		src.verified = box.verified
	}
	value, err := src.convert(in, int32(b.now().Unix()))
	if err != nil {
		return nil, err
	}
	if rotating {
		box.values = map[string]*mtproto.SecureValue{}
		box.files = map[int64]*mtproto.SecureFile{}
		box.errors = map[int64][]*mtproto.SecureValueError{}
		box.grants = nil
	}
	box.secret = secretID
	key, _ := typeKey(value.GetType())
	box.values[key] = value
	box.dropStaleErrors(key, value)
	box.reindex()
	if err = b.persist(ctx, user, box); err != nil {
		return nil, err
	}
	return cloneValue(value), nil
}

// All returns every saved value.
// https://core.telegram.org/method/account.getAllSecureValues
func (b *Book) All(ctx context.Context, user int64) ([]*mtproto.SecureValue, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	box, err := b.ready(ctx, user)
	if err != nil {
		return nil, err
	}
	return box.ordered(nil), nil
}

// Get returns the saved values of the requested types.
// https://core.telegram.org/method/account.getSecureValue
func (b *Book) Get(ctx context.Context, user int64, types []*mtproto.SecureValueType) ([]*mtproto.SecureValue, error) {
	keys, err := keysOf(types)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	box, err := b.ready(ctx, user)
	if err != nil {
		return nil, err
	}
	return box.ordered(keys), nil
}

// Delete removes the saved values of the requested types.
// https://core.telegram.org/method/account.deleteSecureValue
func (b *Book) Delete(ctx context.Context, user int64, types []*mtproto.SecureValueType) error {
	keys, err := keysOf(types)
	if err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	box, err := b.ready(ctx, user)
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return nil
	}
	for key := range keys {
		delete(box.values, key)
		deleteErrorsOfType(box, key)
	}
	box.reindex()
	return b.persist(ctx, user, box)
}

func keysOf(types []*mtproto.SecureValueType) (map[string]bool, error) {
	if len(types) == 0 {
		return nil, nil
	}
	out := make(map[string]bool, len(types))
	for _, t := range types {
		key, err := typeKey(t)
		if err != nil {
			return nil, err
		}
		out[key] = true
	}
	return out, nil
}

func (u *userBox) ordered(filter map[string]bool) []*mtproto.SecureValue {
	out := make([]*mtproto.SecureValue, 0, len(u.values))
	for _, key := range typeOrder {
		if filter != nil && !filter[key] {
			continue
		}
		if v := u.values[key]; v != nil {
			out = append(out, cloneValue(v))
		}
	}
	return out
}

func (u *userBox) reindex() {
	u.files = map[int64]*mtproto.SecureFile{}
	add := func(f *mtproto.SecureFile) {
		if f != nil && f.GetId() != 0 {
			u.files[f.GetId()] = f
		}
	}
	for _, v := range u.values {
		add(v.GetFrontSide())
		add(v.GetReverseSide())
		add(v.GetSelfie())
		for _, f := range v.GetTranslation() {
			add(f)
		}
		for _, f := range v.GetFiles() {
			add(f)
		}
	}
}

func (u *userBox) dropStaleErrors(key string, value *mtproto.SecureValue) {
	have := componentHashes(value)
	for bot, list := range u.errors {
		kept := list[:0]
		for _, err := range list {
			t, terr := typeKey(err.GetType())
			if terr != nil || t != key || !errorStillApplies(err, have) {
				if terr == nil && t != key {
					kept = append(kept, err)
				}
				continue
			}
			kept = append(kept, err)
		}
		if len(kept) == 0 {
			delete(u.errors, bot)
			continue
		}
		u.errors[bot] = kept
	}
}

func deleteErrorsOfType(u *userBox, key string) {
	for bot, list := range u.errors {
		kept := list[:0]
		for _, err := range list {
			t, terr := typeKey(err.GetType())
			if terr != nil || t == key {
				continue
			}
			kept = append(kept, err)
		}
		if len(kept) == 0 {
			delete(u.errors, bot)
			continue
		}
		u.errors[bot] = kept
	}
}

func (b *Book) ready(ctx context.Context, user int64) (*userBox, error) {
	if box, ok := b.users[user]; ok {
		return box, nil
	}
	box, err := b.load(ctx, user)
	if err != nil {
		return nil, err
	}
	b.users[user] = box
	return box, nil
}

func (b *Book) load(ctx context.Context, user int64) (*userBox, error) {
	raw, err := b.kv.GetCtx(ctx, userKey(user))
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return newUserBox(), nil
	}
	var snap userSnap
	if err = json.Unmarshal([]byte(raw), &snap); err != nil {
		logx.Errorf("passport: drop unreadable record user=%d: %v", user, err)
		return newUserBox(), nil
	}
	return snap.box(), nil
}

func (b *Book) persist(ctx context.Context, user int64, box *userBox) error {
	raw, err := json.Marshal(snapOf(box))
	if err != nil {
		return err
	}
	return b.kv.SetCtx(ctx, userKey(user), string(raw))
}

func userKey(user int64) string {
	return userKeyPrefix + strconv.FormatInt(user, 10)
}

type userSnap struct {
	Secret   int64               `json:"secret,omitempty"`
	Values   map[string]string   `json:"values,omitempty"`
	Verified []string            `json:"verified,omitempty"`
	Codes    []phoneCode         `json:"codes,omitempty"`
	Errors   map[string][]string `json:"errors,omitempty"`
	Grants   []Grant             `json:"grants,omitempty"`
}

func snapOf(box *userBox) userSnap {
	snap := userSnap{
		Secret: box.secret,
		Values: map[string]string{},
		Errors: map[string][]string{},
		Grants: box.grants,
		Codes:  box.codes,
	}
	for key, value := range box.values {
		if raw, err := proto.Marshal(value); err == nil {
			snap.Values[key] = base64.StdEncoding.EncodeToString(raw)
		}
	}
	for phone := range box.verified {
		snap.Verified = append(snap.Verified, phone)
	}
	for bot, list := range box.errors {
		key := strconv.FormatInt(bot, 10)
		for _, err := range list {
			if raw, mErr := proto.Marshal(err); mErr == nil {
				snap.Errors[key] = append(snap.Errors[key], base64.StdEncoding.EncodeToString(raw))
			}
		}
	}
	return snap
}

func (s userSnap) box() *userBox {
	box := newUserBox()
	box.secret = s.Secret
	box.codes = s.Codes
	box.grants = s.Grants
	for _, phone := range s.Verified {
		box.verified[phone] = struct{}{}
	}
	for key, raw := range s.Values {
		buf, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			continue
		}
		value := &mtproto.SecureValue{}
		if proto.Unmarshal(buf, value) != nil {
			continue
		}
		box.values[key] = value
	}
	for bot, list := range s.Errors {
		id, err := strconv.ParseInt(bot, 10, 64)
		if err != nil {
			continue
		}
		for _, raw := range list {
			buf, err := base64.StdEncoding.DecodeString(raw)
			if err != nil {
				continue
			}
			item := &mtproto.SecureValueError{}
			if proto.Unmarshal(buf, item) != nil {
				continue
			}
			box.errors[id] = append(box.errors[id], item)
		}
	}
	box.reindex()
	return box
}

// clock is overridden by tests.
func (b *Book) at(t time.Time) { b.now = func() time.Time { return t } }
