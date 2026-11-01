package book

import (
	"context"
	"crypto/sha256"
	"strings"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
)

// Grant is one accepted passport authorization. The encrypted credentials
// stay in the record. No messageActionSecureValuesSentMe is pushed: this
// process has no bot-update path for passport.
type Grant struct {
	BotID     int64    `json:"bot_id"`
	Scope     string   `json:"scope"`
	PublicKey string   `json:"public_key"`
	Types     []string `json:"types,omitempty"`
}

// Form is the account.authorizationForm payload without the bot user.
// The handler attaches the bot and its privacy policy URL.
type Form struct {
	Required []*mtproto.SecureRequiredType
	Values   []*mtproto.SecureValue
	Errors   []*mtproto.SecureValueError
}

// AuthorizationForm parses the scope and returns saved values and errors
// for that bot.
// https://core.telegram.org/method/account.getAuthorizationForm
func (b *Book) AuthorizationForm(ctx context.Context, user, bot int64, scope, publicKey string) (*Form, error) {
	required, err := checkRequest(scope, publicKey)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	box, err := b.ready(ctx, user)
	if err != nil {
		return nil, err
	}
	want := scopeTypes(required)
	values := make([]*mtproto.SecureValue, 0, len(want))
	for _, key := range typeOrder {
		if want[key] {
			if v := box.values[key]; v != nil {
				values = append(values, cloneValue(v))
			}
		}
	}
	var errs []*mtproto.SecureValueError
	for _, item := range box.errors[bot] {
		key, terr := typeKey(item.GetType())
		if terr != nil || !want[key] {
			continue
		}
		errs = append(errs, cloneError(item))
	}
	return &Form{Required: required, Values: values, Errors: errs}, nil
}

// Accept records a grant when the submitted hashes match the saved values
// and the credentials hash is SHA-256 of the credentials data.
// https://core.telegram.org/method/account.acceptAuthorization
// https://core.telegram.org/passport#decrypting-data
func (b *Book) Accept(ctx context.Context, user, bot int64, scope, publicKey string, hashes []*mtproto.SecureValueHash, creds *mtproto.SecureCredentialsEncrypted) error {
	required, err := checkRequest(scope, publicKey)
	if err != nil {
		return err
	}
	if err = checkCredentials(creds); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	box, err := b.ready(ctx, user)
	if err != nil {
		return err
	}
	got := map[string]*mtproto.SecureValue{}
	for _, hash := range hashes {
		key, terr := typeKey(hash.GetType())
		if terr != nil || len(hash.GetHash()) != hashLen {
			return mtproto.ErrDataInvalid
		}
		value := box.values[key]
		if value == nil || !sameHash(value.GetHash(), hash.GetHash()) {
			return mtproto.ErrDataInvalid
		}
		if unresolved(box.errors[bot], key, value) {
			return mtproto.ErrDataInvalid
		}
		got[key] = value
	}
	if err = covers(required, got); err != nil {
		return err
	}
	types := make([]string, 0, len(got))
	for _, key := range typeOrder {
		if got[key] != nil {
			types = append(types, key)
		}
	}
	box.grants = append(box.grants, Grant{
		BotID:     bot,
		Scope:     scope,
		PublicKey: strings.TrimSpace(publicKey),
		Types:     types,
	})
	if len(box.grants) > 20 {
		box.grants = box.grants[len(box.grants)-20:]
	}
	return b.persist(ctx, user, box)
}

// Grants returns accepted authorizations for tests and diagnostics.
func (b *Book) Grants(ctx context.Context, user int64) ([]Grant, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	box, err := b.ready(ctx, user)
	if err != nil {
		return nil, err
	}
	out := make([]Grant, len(box.grants))
	copy(out, box.grants)
	return out, nil
}

// SetErrors replaces the errors one bot has filed against a user.
// https://core.telegram.org/method/users.setSecureValueErrors
func (b *Book) SetErrors(ctx context.Context, user, bot int64, errs []*mtproto.SecureValueError) error {
	if len(errs) > 20 {
		return mtproto.ErrInputRequestInvalid
	}
	clean := make([]*mtproto.SecureValueError, 0, len(errs))
	for _, item := range errs {
		if err := validateError(item); err != nil {
			return err
		}
		clean = append(clean, cloneError(item))
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	box, err := b.ready(ctx, user)
	if err != nil {
		return err
	}
	if len(clean) == 0 {
		delete(box.errors, bot)
	} else {
		box.errors[bot] = clean
	}
	return b.persist(ctx, user, box)
}

func checkRequest(scope, publicKey string) ([]*mtproto.SecureRequiredType, error) {
	publicKey = strings.TrimSpace(publicKey)
	if publicKey == "" || len(publicKey) < 32 || len(publicKey) > 8192 || !strings.Contains(publicKey, "BEGIN") {
		return nil, mtproto.ErrPublicKeyRequired
	}
	return parseScope(scope)
}

func checkCredentials(creds *mtproto.SecureCredentialsEncrypted) error {
	if creds == nil || len(creds.GetData()) == 0 || len(creds.GetData()) > maxSecureData {
		return mtproto.ErrDataInvalid
	}
	if len(creds.GetHash()) != hashLen || len(creds.GetSecret()) != hashLen {
		return mtproto.ErrDataInvalid
	}
	sum := sha256.Sum256(creds.GetData())
	if !sameHash(sum[:], creds.GetHash()) {
		return mtproto.ErrDataInvalid
	}
	return nil
}

func covers(required []*mtproto.SecureRequiredType, got map[string]*mtproto.SecureValue) error {
	for _, req := range required {
		if req.GetPredicateName() == mtproto.Predicate_secureRequiredTypeOneOf {
			ok := false
			for _, inner := range req.GetTypes() {
				key, err := typeKey(inner.GetType())
				if err != nil {
					return mtproto.ErrDataInvalid
				}
				if flagsMet(inner, got[key]) {
					ok = true
					break
				}
			}
			if !ok {
				return mtproto.ErrDataInvalid
			}
			continue
		}
		key, err := typeKey(req.GetType())
		if err != nil || !flagsMet(req, got[key]) {
			return mtproto.ErrDataInvalid
		}
	}
	return nil
}

func flagsMet(req *mtproto.SecureRequiredType, value *mtproto.SecureValue) bool {
	if value == nil {
		return false
	}
	if req.GetSelfieRequired() && value.GetSelfie() == nil {
		return false
	}
	if req.GetTranslationRequired() && len(value.GetTranslation()) == 0 {
		return false
	}
	return true
}

func unresolved(errs []*mtproto.SecureValueError, key string, value *mtproto.SecureValue) bool {
	have := componentHashes(value)
	for _, err := range errs {
		t, terr := typeKey(err.GetType())
		if terr == nil && t == key && errorStillApplies(err, have) {
			return true
		}
	}
	return false
}

func validateError(item *mtproto.SecureValueError) error {
	if item == nil {
		return mtproto.ErrDataInvalid
	}
	if _, err := typeKey(item.GetType()); err != nil {
		return err
	}
	text := strings.TrimSpace(item.GetText())
	if text == "" || utf8.RuneCountInString(text) > 256 {
		return mtproto.ErrInputRequestInvalid
	}
	switch item.GetPredicateName() {
	case mtproto.Predicate_secureValueErrorData:
		if len(item.GetDataHash()) != hashLen {
			return mtproto.ErrDataHashSizeInvalid
		}
		field := item.GetField()
		if field == "" || len(field) > 64 || !fieldToken(field) {
			return mtproto.ErrInputRequestInvalid
		}
	case mtproto.Predicate_secureValueErrorFrontSide,
		mtproto.Predicate_secureValueErrorReverseSide,
		mtproto.Predicate_secureValueErrorSelfie,
		mtproto.Predicate_secureValueErrorFile,
		mtproto.Predicate_secureValueErrorTranslationFile:
		if len(item.GetFileHash_BYTES()) != hashLen {
			return mtproto.ErrHashSizeInvalid
		}
	case mtproto.Predicate_secureValueErrorFiles, mtproto.Predicate_secureValueErrorTranslationFiles:
		hashes := item.GetFileHash_VECTORBYTES()
		if len(hashes) == 0 {
			return mtproto.ErrHashSizeInvalid
		}
		for _, hash := range hashes {
			if len(hash) != hashLen {
				return mtproto.ErrHashSizeInvalid
			}
		}
	case mtproto.Predicate_secureValueError:
		if len(item.GetHash()) != hashLen {
			return mtproto.ErrHashSizeInvalid
		}
	default:
		return mtproto.ErrDataInvalid
	}
	return nil
}

func errorStillApplies(item *mtproto.SecureValueError, have map[string]struct{}) bool {
	var refs [][]byte
	switch item.GetPredicateName() {
	case mtproto.Predicate_secureValueErrorData:
		refs = [][]byte{item.GetDataHash()}
	case mtproto.Predicate_secureValueErrorFrontSide,
		mtproto.Predicate_secureValueErrorReverseSide,
		mtproto.Predicate_secureValueErrorSelfie,
		mtproto.Predicate_secureValueErrorFile,
		mtproto.Predicate_secureValueErrorTranslationFile:
		refs = [][]byte{item.GetFileHash_BYTES()}
	case mtproto.Predicate_secureValueErrorFiles, mtproto.Predicate_secureValueErrorTranslationFiles:
		refs = item.GetFileHash_VECTORBYTES()
	case mtproto.Predicate_secureValueError:
		refs = [][]byte{item.GetHash()}
	default:
		return false
	}
	if len(refs) == 0 {
		return false
	}
	for _, ref := range refs {
		if _, ok := have[string(ref)]; !ok {
			return false
		}
	}
	return true
}

func fieldToken(s string) bool {
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}
