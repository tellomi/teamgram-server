package book

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/phonenumber"
)

// Passport phone verification is account.sendVerifyPhoneCode and
// account.verifyPhone. It is not auth.sendCode. The code lives with the
// passport record. Test-range numbers use the dev code from pkg/phonenumber.
// Other numbers are issued a code; SMS delivery is not wired (the login
// sender is commented out and must not be reused from here).

const (
	codeDigits  = 5
	codeTTL     = 3 * time.Minute
	maxAttempts = 5
	maxCodes    = 8
)

type phoneCode struct {
	Phone string `json:"phone"`
	Hash  string `json:"hash"`
	Sum   string `json:"sum"`
	Exp   int64  `json:"exp"`
	Tries int    `json:"tries,omitempty"`
}

// SendPhoneCode records a code for a normalized phone and returns the
// phone_code_hash plus the digits the caller may deliver.
func (b *Book) SendPhoneCode(ctx context.Context, user int64, phone string) (string, string, error) {
	norm, err := phonenumber.CheckAndGetPhoneNumber(phone)
	if err != nil {
		return "", "", mtproto.ErrPhoneNumberInvalid
	}
	code, ok := phonenumber.TestPhoneCode(norm)
	if !ok {
		code, err = randomCode()
		if err != nil {
			return "", "", mtproto.ErrSmsCodeCreateFailed
		}
	}
	hash, err := randomHash()
	if err != nil {
		return "", "", mtproto.ErrSmsCodeCreateFailed
	}
	sum := sha256.Sum256([]byte(code))
	b.mu.Lock()
	defer b.mu.Unlock()
	box, err := b.ready(ctx, user)
	if err != nil {
		return "", "", err
	}
	now := b.now()
	kept := box.codes[:0]
	for _, row := range box.codes {
		if row.Phone == norm || now.Unix() >= row.Exp {
			continue
		}
		kept = append(kept, row)
	}
	box.codes = append(kept, phoneCode{
		Phone: norm,
		Hash:  hash,
		Sum:   hex.EncodeToString(sum[:]),
		Exp:   now.Add(codeTTL).Unix(),
	})
	if len(box.codes) > maxCodes {
		box.codes = box.codes[len(box.codes)-maxCodes:]
	}
	if err = b.persist(ctx, user, box); err != nil {
		return "", "", err
	}
	return hash, code, nil
}

// VerifyPhone checks the code and remembers the phone so a phone secure
// value can be saved.
// https://core.telegram.org/method/account.verifyPhone
func (b *Book) VerifyPhone(ctx context.Context, user int64, phone, hash, code string) error {
	if hash == "" {
		return mtproto.ErrPhoneCodeHashEmpty
	}
	if code == "" {
		return mtproto.ErrPhoneCodeEmpty
	}
	norm, err := phonenumber.CheckAndGetPhoneNumber(phone)
	if err != nil {
		return mtproto.ErrPhoneNumberInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	box, err := b.ready(ctx, user)
	if err != nil {
		return err
	}
	idx := -1
	for i, row := range box.codes {
		if row.Hash == hash {
			idx = i
			break
		}
	}
	if idx < 0 {
		return mtproto.ErrPhoneCodeExpired
	}
	row := box.codes[idx]
	if row.Phone != norm || b.now().Unix() >= row.Exp {
		box.codes = append(box.codes[:idx], box.codes[idx+1:]...)
		_ = b.persist(ctx, user, box)
		return mtproto.ErrPhoneCodeExpired
	}
	sum := sha256.Sum256([]byte(code))
	want, err := hex.DecodeString(row.Sum)
	if err != nil || subtle.ConstantTimeCompare(sum[:], want) != 1 {
		row.Tries++
		if row.Tries >= maxAttempts {
			box.codes = append(box.codes[:idx], box.codes[idx+1:]...)
		} else {
			box.codes[idx] = row
		}
		_ = b.persist(ctx, user, box)
		return mtproto.ErrPhoneCodeInvalid
	}
	box.codes = append(box.codes[:idx], box.codes[idx+1:]...)
	box.verified[norm] = struct{}{}
	return b.persist(ctx, user, box)
}

func randomCode() (string, error) {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	n := binary.BigEndian.Uint32(buf[:]) % 100000
	out := make([]byte, codeDigits)
	for i := codeDigits - 1; i >= 0; i-- {
		out[i] = byte('0' + n%10)
		n /= 10
	}
	return string(out), nil
}

func randomHash() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}
