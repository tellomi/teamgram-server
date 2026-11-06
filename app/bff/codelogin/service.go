package codelogin

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/authorization/model"
	"github.com/teamgram/teamgram-server/pkg/verify"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	defaultTTL      = 180 * time.Second
	defaultAttempts = 5
	floodWait       = int32(900)
	smsTimeout      = int32(60)

	purposeSetup    = "setup"
	purposeChange   = "change"
	purposePassport = "passport"
)

// Options wires the existing senders and the phone-code table.
// SendEmail and SendSMS return the code that was actually delivered.
// A nil return code keeps the code this service generated.
type Options struct {
	SendEmail  func(ctx context.Context, to, code string) (string, error)
	SendSMS    func(ctx context.Context, phone, code string) (string, error)
	LookupUser func(ctx context.Context, phone string) (int64, error)
	// IdentityEmail is users.email for a user, or "" for a phone account.
	// IdentityOwner is the user id that owns users.email, or 0.
	// Nil leaves the Redis login email unconstrained (tests).
	IdentityEmail  func(ctx context.Context, userID int64) (string, error)
	IdentityOwner  func(ctx context.Context, email string) (int64, error)
	Phones         PhoneCodes
	Now            func() time.Time
	EmailLen       int
	SMSLen         int
	TTL            time.Duration
	MaxAttempts    int
	NormalizePhone func(string) (string, error)
}

// Service is the P0 login-code state machine.
type Service struct {
	kv       KV
	opt      Options
	memClock func() time.Time
}

// New builds a service. A nil KV keeps records in memory.
func New(kv KV, opt Options) *Service {
	now := opt.Now
	if now == nil {
		now = time.Now
	}
	if kv == nil {
		kv = newMemKV(now)
	}
	if opt.EmailLen <= 0 {
		opt.EmailLen = 6
	}
	if opt.SMSLen <= 0 {
		opt.SMSLen = 6
	}
	if opt.TTL <= 0 {
		opt.TTL = defaultTTL
	}
	if opt.MaxAttempts <= 0 {
		opt.MaxAttempts = defaultAttempts
	}
	opt.Now = now
	return &Service{kv: kv, opt: opt}
}

func (s *Service) now() time.Time {
	if s == nil || s.opt.Now == nil {
		return time.Now()
	}
	return s.opt.Now()
}

type record struct {
	Code     string `json:"code"`
	Email    string `json:"email,omitempty"`
	Phone    string `json:"phone,omitempty"`
	Hash     string `json:"hash,omitempty"`
	UserID   int64  `json:"user_id,omitempty"`
	AuthKey  int64  `json:"auth_key,omitempty"`
	Expires  int64  `json:"expires"`
	Attempts int    `json:"attempts,omitempty"`
	Purpose  string `json:"purpose,omitempty"`
	Reset    bool   `json:"reset,omitempty"`
}

func (s *Service) put(ctx context.Context, key string, rec record) error {
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	sec := int(rec.Expires - s.now().Unix())
	if sec < 1 {
		sec = int(s.opt.TTL.Seconds())
	}
	return s.kv.SetexCtx(ctx, key, string(raw), sec)
}

func (s *Service) get(ctx context.Context, key string) (record, bool, error) {
	raw, err := s.kv.GetCtx(ctx, key)
	if err != nil || raw == "" {
		return record{}, false, err
	}
	var rec record
	if err = json.Unmarshal([]byte(raw), &rec); err != nil {
		return record{}, false, err
	}
	if rec.Expires > 0 && s.now().Unix() > rec.Expires {
		_, _ = s.kv.DelCtx(ctx, key)
		return record{}, false, nil
	}
	return rec, true, nil
}

func (s *Service) ttl() int64 {
	return s.now().Add(s.opt.TTL).Unix()
}

func match(sent, got string) bool {
	if sent == "" || len(sent) != len(got) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(sent), []byte(got)) == 1
}

func (s *Service) freshCode(n int) (string, error) {
	code := verify.RandomDigits(n)
	if code == "" {
		return "", mtproto.ErrInternalServerError
	}
	return code, nil
}

func (s *Service) deliverEmail(ctx context.Context, email, code string) (string, error) {
	if s.opt.SendEmail == nil {
		return "", mtproto.ErrEmailNotSetup
	}
	sent, err := s.opt.SendEmail(ctx, email, code)
	if err != nil {
		return "", err
	}
	if sent == "" {
		sent = code
	}
	return sent, nil
}

func (s *Service) deliverSMS(ctx context.Context, phone, code string) (string, error) {
	if s.opt.SendSMS == nil {
		return "", mtproto.ErrSmsCodeCreateFailed
	}
	sent, err := s.opt.SendSMS(ctx, phone, code)
	if err != nil {
		return "", err
	}
	if sent == "" {
		sent = code
	}
	return sent, nil
}

func normalizeEmail(raw string) (string, error) {
	email := verify.NormalizeEmail(raw)
	if !verify.ValidEmail(email) || len(email) > 255 {
		return "", mtproto.ErrEmailInvalid
	}
	return email, nil
}

func (s *Service) canon(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", mtproto.ErrPhoneNumberInvalid
	}
	if verify.IsEmail(raw) {
		return normalizeEmail(raw)
	}
	if s.opt.NormalizePhone != nil {
		return s.opt.NormalizePhone(raw)
	}
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, raw)
	if len(digits) < 8 || len(digits) > 15 {
		return "", mtproto.ErrPhoneNumberInvalid
	}
	return digits, nil
}

func emailKey(authKey int64, purpose, email string) string {
	return "tellomi:p0:email:" + itoa(authKey) + ":" + purpose + ":" + email
}

func recoveryKey(userID int64) string {
	return "tellomi:p0:recovery:" + itoa(userID)
}

func loginEmailKey(userID int64) string {
	return "tellomi:p0:loginemail:" + itoa(userID)
}

func pendingEmailKey(authKey int64, phone string) string {
	return "tellomi:p0:pendingemail:" + itoa(authKey) + ":" + phone
}

func signInKey(userID int64, code string) string {
	return "tellomi:p0:signincode:" + itoa(userID) + ":" + code
}

func confirmKey(hash string) string {
	return "tellomi:p0:confirm:" + hash
}

func confirmCodeKey(hash string) string {
	return "tellomi:p0:confirmcode:" + hash
}

// SaveLoginEmail remembers a verified login address for userID.
// An email-only account can only store its own users.email here.
// An address that is already another account's users.email is refused.
// The cloud-password recovery address is not this key.
func (s *Service) SaveLoginEmail(ctx context.Context, userID int64, email string) error {
	if s == nil || userID == 0 {
		return nil
	}
	email, err := normalizeEmail(email)
	if err != nil {
		return err
	}
	if err = s.permitLoginEmail(ctx, userID, email); err != nil {
		return err
	}
	return s.kv.SetexCtx(ctx, loginEmailKey(userID), email, 10*365*24*3600)
}

// permitLoginEmail refuses a Redis login address that would diverge from
// users.email or take an address another account already uses as its identity.
// Hooks left nil allow the write (unit tests of the older phone flow).
func (s *Service) permitLoginEmail(ctx context.Context, userID int64, email string) error {
	if s == nil || (s.opt.IdentityEmail == nil && s.opt.IdentityOwner == nil) {
		return nil
	}
	if s.opt.IdentityEmail != nil && userID != 0 {
		mine, err := s.opt.IdentityEmail(ctx, userID)
		if err != nil {
			return err
		}
		mine = verify.NormalizeEmail(strings.TrimSpace(mine))
		if mine != "" && mine != email {
			return mtproto.ErrEmailNotAllowed
		}
	}
	if s.opt.IdentityOwner != nil {
		owner, err := s.opt.IdentityOwner(ctx, email)
		if err != nil {
			return err
		}
		if owner != 0 && owner != userID {
			return mtproto.ErrEmailNotAllowed
		}
	}
	return nil
}

func (s *Service) identityEmail(ctx context.Context, userID int64) (string, error) {
	if s == nil || s.opt.IdentityEmail == nil || userID == 0 {
		return "", nil
	}
	email, err := s.opt.IdentityEmail(ctx, userID)
	if err != nil {
		return "", err
	}
	email = verify.NormalizeEmail(strings.TrimSpace(email))
	if !verify.ValidEmail(email) {
		return "", nil
	}
	return email, nil
}

// guardSetupEmail keeps an in-progress email registration from attaching a
// second address, and keeps a phone login from claiming another account's
// users.email.
func (s *Service) guardSetupEmail(ctx context.Context, userID int64, canon, email string) error {
	if verify.IsEmail(canon) && email != canon {
		return mtproto.ErrEmailNotAllowed
	}
	if err := s.permitLoginEmail(ctx, userID, email); err != nil {
		return err
	}
	if userID != 0 || s.opt.LookupUser == nil || (s.opt.IdentityEmail == nil && s.opt.IdentityOwner == nil) {
		return nil
	}
	id, err := s.opt.LookupUser(ctx, canon)
	if err != nil || id == 0 {
		return err
	}
	return s.permitLoginEmail(ctx, id, email)
}

// LoginEmail returns the Redis login-email copy, or "" when none is set.
// The cloud-password recovery address lives in user_passwords, not here.
func (s *Service) LoginEmail(ctx context.Context, userID int64) (string, error) {
	if s == nil || userID == 0 {
		return "", nil
	}
	return s.kv.GetCtx(ctx, loginEmailKey(userID))
}

// RememberSignInCode indexes a login code so account.invalidateSignInCodes can drop it.
func (s *Service) RememberSignInCode(ctx context.Context, userID, authKey int64, phone, code, hash string) error {
	if s == nil || userID == 0 || code == "" {
		return nil
	}
	code = stripDashes(code)
	val := itoa(authKey) + "\t" + phone + "\t" + hash
	return s.kv.SetexCtx(ctx, signInKey(userID, code), val, int(s.opt.TTL.Seconds()))
}

func stripDashes(code string) string {
	return strings.Map(func(r rune) rune {
		if r == '-' || r == ' ' {
			return -1
		}
		return r
	}, code)
}

func newHash() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func smsSent(hash, code string) *mtproto.Auth_SentCode {
	return mtproto.MakeTLAuthSentCode(&mtproto.Auth_SentCode{
		Type: mtproto.MakeTLAuthSentCodeTypeSms(&mtproto.Auth_SentCodeType{
			Length: int32(len(code)),
		}).To_Auth_SentCodeType(),
		PhoneCodeHash: hash,
		NextType:      mtproto.MakeTLAuthCodeTypeSms(nil).To_Auth_CodeType(),
		Timeout:       &wrapperspb.Int32Value{Value: smsTimeout},
	}).To_Auth_SentCode()
}

func emailSent(hash, email, code string) *mtproto.Auth_SentCode {
	return mtproto.MakeTLAuthSentCode(&mtproto.Auth_SentCode{
		Type: mtproto.MakeTLAuthSentCodeTypeEmailCode(&mtproto.Auth_SentCodeType{
			EmailPattern: verify.MaskEmail(email),
			Length:       int32(len(code)),
		}).To_Auth_SentCodeType(),
		PhoneCodeHash: hash,
		Timeout:       &wrapperspb.Int32Value{Value: smsTimeout},
	}).To_Auth_SentCode()
}

func sentEmail(email, code string) *mtproto.Account_SentEmailCode {
	return mtproto.MakeTLAccountSentEmailCode(&mtproto.Account_SentEmailCode{
		EmailPattern: verify.MaskEmail(email),
		Length:       int32(len(code)),
	}).To_Account_SentEmailCode()
}

func (s *Service) phone(ctx context.Context, authKey int64, phone, hash string) (*PhoneCode, error) {
	if hash == "" {
		return nil, mtproto.ErrPhoneCodeHashEmpty
	}
	if s.opt.Phones == nil {
		return nil, mtproto.ErrPhoneCodeExpired
	}
	pc, err := s.opt.Phones.Get(ctx, authKey, phone, hash)
	if err != nil {
		return nil, err
	}
	if pc == nil {
		return nil, mtproto.ErrPhoneCodeExpired
	}
	if pc.Expires > 0 && s.now().Unix() > pc.Expires {
		return nil, mtproto.ErrPhoneCodeExpired
	}
	return pc, nil
}

func purposeOf(p *mtproto.EmailVerifyPurpose) (kind, phone, hash string, err error) {
	if p == nil {
		return "", "", "", mtproto.ErrPhoneCodeEmpty
	}
	name := p.GetPredicateName()
	ctor := p.GetConstructor()
	switch {
	case name == mtproto.Predicate_emailVerifyPurposeLoginSetup || ctor == mtproto.CRC32_emailVerifyPurposeLoginSetup:
		phone = strings.TrimSpace(p.GetPhoneNumber())
		hash = p.GetPhoneCodeHash()
		if phone == "" {
			return "", "", "", mtproto.ErrPhoneNumberInvalid
		}
		if hash == "" {
			return "", "", "", mtproto.ErrPhoneCodeEmpty
		}
		return purposeSetup, phone, hash, nil
	case name == mtproto.Predicate_emailVerifyPurposeLoginChange || ctor == mtproto.CRC32_emailVerifyPurposeLoginChange:
		return purposeChange, "", "", nil
	case name == mtproto.Predicate_emailVerifyPurposePassport || ctor == mtproto.CRC32_emailVerifyPurposePassport:
		return purposePassport, "", "", nil
	default:
		return "", "", "", mtproto.ErrEmailInvalid
	}
}

func verificationCode(v *mtproto.EmailVerification) (string, error) {
	if v == nil {
		return "", mtproto.ErrCodeInvalid
	}
	name := v.GetPredicateName()
	ctor := v.GetConstructor()
	switch {
	case name == mtproto.Predicate_emailVerificationCode || ctor == mtproto.CRC32_emailVerificationCode:
		if v.GetCode() == "" {
			return "", mtproto.ErrCodeInvalid
		}
		return v.GetCode(), nil
	case name == mtproto.Predicate_emailVerificationGoogle || ctor == mtproto.CRC32_emailVerificationGoogle:
		return "", mtproto.ErrEmailInvalid
	case name == mtproto.Predicate_emailVerificationApple || ctor == mtproto.CRC32_emailVerificationApple:
		return "", mtproto.ErrEmailInvalid
	default:
		if v.GetCode() != "" && name == "" && ctor == 0 {
			return v.GetCode(), nil
		}
		return "", mtproto.ErrEmailInvalid
	}
}

// SendVerifyEmailCode sends a numeric code with the configured email sender.
// https://core.telegram.org/method/account.sendVerifyEmailCode
func (s *Service) SendVerifyEmailCode(ctx context.Context, authKey, userID int64, in *mtproto.TLAccountSendVerifyEmailCode) (*mtproto.Account_SentEmailCode, error) {
	if in == nil {
		return nil, mtproto.ErrEmailInvalid
	}
	email, err := normalizeEmail(in.Email)
	if err != nil {
		return nil, err
	}
	kind, phone, hash, err := purposeOf(in.Purpose)
	if err != nil {
		return nil, err
	}
	switch kind {
	case purposePassport:
		return nil, mtproto.ErrEmailNotAllowed
	case purposeSetup:
		canon, err := s.canon(phone)
		if err != nil {
			return nil, err
		}
		if _, err = s.phone(ctx, authKey, canon, hash); err != nil {
			if err == mtproto.ErrPhoneCodeHashEmpty || err == mtproto.ErrPhoneNumberInvalid {
				return nil, err
			}
			return nil, mtproto.ErrPhoneHashExpired
		}
		if err = s.guardSetupEmail(ctx, userID, canon, email); err != nil {
			return nil, err
		}
		phone = canon
	case purposeChange:
		if userID == 0 {
			return nil, mtproto.ErrAuthKeyUnregistered
		}
		if err = s.permitLoginEmail(ctx, userID, email); err != nil {
			return nil, err
		}
		have, err := s.LoginEmail(ctx, userID)
		if err != nil {
			return nil, err
		}
		mine, err := s.identityEmail(ctx, userID)
		if err != nil {
			return nil, err
		}
		// An email-only account already has users.email. Confirming that
		// same address does not need a previous Redis row.
		if have == "" && mine == "" {
			return nil, mtproto.ErrEmailNotSetup
		}
	}
	code, err := s.freshCode(s.opt.EmailLen)
	if err != nil {
		return nil, err
	}
	sent, err := s.deliverEmail(ctx, email, code)
	if err != nil {
		return nil, err
	}
	rec := record{
		Code:    sent,
		Email:   email,
		Phone:   phone,
		Hash:    hash,
		UserID:  userID,
		AuthKey: authKey,
		Expires: s.ttl(),
		Purpose: kind,
	}
	if err = s.put(ctx, emailKey(authKey, kind, email), rec); err != nil {
		return nil, err
	}
	if kind == purposeSetup {
		_ = s.kv.SetexCtx(ctx, pendingEmailKey(authKey, phone), email, int(s.opt.TTL.Seconds()))
	}
	if kind == purposeChange {
		s.noteChangeTarget(ctx, authKey, email)
	}
	return sentEmail(email, sent), nil
}

// VerifyEmail checks the code from SendVerifyEmailCode.
// Login setup returns account.emailVerifiedLogin and the next auth.sentCode.
// Login change returns account.emailVerified and stores the address.
// https://core.telegram.org/method/account.verifyEmail
func (s *Service) VerifyEmail(ctx context.Context, authKey, userID int64, in *mtproto.TLAccountVerifyEmail32DA4CF) (*mtproto.Account_EmailVerified, error) {
	if in == nil {
		return nil, mtproto.ErrCodeInvalid
	}
	kind, phone, hash, err := purposeOf(in.Purpose)
	if err != nil {
		return nil, err
	}
	if kind == purposePassport {
		return nil, mtproto.ErrEmailNotAllowed
	}
	code, err := verificationCode(in.Verification)
	if err != nil {
		return nil, err
	}
	var email string
	switch kind {
	case purposeSetup:
		canon, err := s.canon(phone)
		if err != nil {
			return nil, err
		}
		phone = canon
		email, err = s.kv.GetCtx(ctx, pendingEmailKey(authKey, phone))
		if err != nil {
			return nil, err
		}
		if email == "" {
			return nil, mtproto.ErrEmailVerifyExpired
		}
		if _, err = s.phone(ctx, authKey, phone, hash); err != nil {
			return nil, mtproto.ErrPhoneCodeExpired
		}
	case purposeChange:
		if userID == 0 {
			return nil, mtproto.ErrAuthKeyUnregistered
		}
		// The address being confirmed is not on the purpose. The latest
		// send for this auth key is the one whose code we accept. Clients
		// resend the same purpose they used for sendVerifyEmailCode, and
		// the code itself selects the record.
		email, err = s.findChangeEmail(ctx, authKey, code)
		if err != nil {
			return nil, err
		}
	}
	rec, key, err := s.takeEmailCode(ctx, authKey, kind, email, code)
	if err != nil {
		return nil, err
	}
	if kind == purposeChange {
		if err = s.SaveLoginEmail(ctx, userID, rec.Email); err != nil {
			return nil, err
		}
		_, _ = s.kv.DelCtx(ctx, key)
		return mtproto.MakeTLAccountEmailVerified(&mtproto.Account_EmailVerified{
			Email: rec.Email,
		}).To_Account_EmailVerified(), nil
	}
	pc, err := s.phone(ctx, authKey, phone, hash)
	if err != nil {
		return nil, mtproto.ErrPhoneCodeExpired
	}
	if err = s.guardSetupEmail(ctx, userID, phone, rec.Email); err != nil {
		return nil, err
	}
	next, err := s.issueNextLoginCode(ctx, pc, rec.Email)
	if err != nil {
		return nil, err
	}
	uid := userID
	if uid == 0 {
		uid = pc.UserID
	}
	if uid == 0 && s.opt.LookupUser != nil {
		uid, _ = s.opt.LookupUser(ctx, phone)
	}
	if uid != 0 {
		if err = s.SaveLoginEmail(ctx, uid, rec.Email); err != nil {
			return nil, err
		}
	}
	_, _ = s.kv.DelCtx(ctx, key)
	_, _ = s.kv.DelCtx(ctx, pendingEmailKey(authKey, phone))
	return mtproto.MakeTLAccountEmailVerifiedLogin(&mtproto.Account_EmailVerified{
		Email:    rec.Email,
		SentCode: next,
	}).To_Account_EmailVerified(), nil
}

// findChangeEmail scans nothing: the send stored the address on the
// recovery of the auth key under a stable "change" slot as well as the
// per-address key. We keep the latest change target in pendingEmailKey
// with phone "change".
func (s *Service) findChangeEmail(ctx context.Context, authKey int64, code string) (string, error) {
	email, err := s.kv.GetCtx(ctx, pendingEmailKey(authKey, "change"))
	if err != nil {
		return "", err
	}
	if email == "" {
		return "", mtproto.ErrEmailVerifyExpired
	}
	_ = code
	return email, nil
}

func (s *Service) takeEmailCode(ctx context.Context, authKey int64, kind, email, code string) (record, string, error) {
	key := emailKey(authKey, kind, email)
	rec, ok, err := s.get(ctx, key)
	if err != nil {
		return record{}, "", err
	}
	if !ok {
		return record{}, "", mtproto.ErrEmailVerifyExpired
	}
	if rec.Attempts >= s.opt.MaxAttempts {
		return record{}, "", mtproto.NewErrFloodWaitX(floodWait)
	}
	if !match(rec.Code, code) {
		rec.Attempts++
		_ = s.put(ctx, key, rec)
		return record{}, "", mtproto.ErrCodeInvalid
	}
	return rec, key, nil
}

func (s *Service) issueNextLoginCode(ctx context.Context, pc *PhoneCode, email string) (*mtproto.Auth_SentCode, error) {
	if verify.IsEmail(pc.Phone) {
		code, err := s.freshCode(s.opt.EmailLen)
		if err != nil {
			return nil, err
		}
		sent, err := s.deliverEmail(ctx, pc.Phone, code)
		if err != nil {
			return nil, err
		}
		pc.Code = sent
		pc.Extra = sent
		pc.SentType = model.SentCodeTypeEmailCode
		pc.EmailPattern = verify.MaskEmail(pc.Phone)
		pc.Expires = s.ttl()
		if err = s.opt.Phones.Save(ctx, pc); err != nil {
			return nil, err
		}
		return emailSent(pc.Hash, pc.Phone, sent), nil
	}
	code, err := s.freshCode(s.opt.SMSLen)
	if err != nil {
		return nil, err
	}
	sent, err := s.deliverSMS(ctx, pc.Phone, code)
	if err != nil {
		return nil, err
	}
	pc.Code = sent
	pc.Extra = sent
	pc.SentType = model.SentCodeTypeSms
	pc.EmailPattern = ""
	pc.Expires = s.ttl()
	if err = s.opt.Phones.Save(ctx, pc); err != nil {
		return nil, err
	}
	_ = email
	if pc.UserID != 0 {
		_ = s.RememberSignInCode(ctx, pc.UserID, pc.AuthKeyID, pc.Phone, sent, pc.Hash)
	}
	return smsSent(pc.Hash, sent), nil
}

// noteChangeTarget records which address a login-change code was sent to.
func (s *Service) noteChangeTarget(ctx context.Context, authKey int64, email string) {
	_ = s.kv.SetexCtx(ctx, pendingEmailKey(authKey, "change"), email, int(s.opt.TTL.Seconds()))
}
