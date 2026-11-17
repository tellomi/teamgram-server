package password

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/srp"
	"github.com/teamgram/teamgram-server/pkg/verify"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	// AlgoName is passwordKdfAlgoSHA256SHA256PBKDF2HMACSHA512iter100000SHA256ModPow.
	AlgoName         = "passwordKdfAlgoSHA256SHA256PBKDF2HMACSHA512iter100000SHA256ModPow"
	verifierLen      = 256
	secureRandomLen  = 256
	newSalt1Len      = 8
	newSalt2Len      = 16
	clientSaltMin    = 8
	hintMax          = 128
	secureSecretMax  = 1024
	challengeTTL     = 10 * time.Minute
	resetAfter       = 7 * 24 * time.Hour
	floodFree        = 4
	defaultCodeLen   = 6
	defaultTTL       = 180 * time.Second
	defaultResend    = 60 * time.Second
	defaultAttempts  = 5
	emailFlood       = int32(900)
	secureAlgoPBKDF2 = "pbkdf2"
	secureAlgoSHA512 = "sha512"
)

// EmailFunc delivers a recovery-email code through the existing sender.
// The returned string is the code that was actually sent.
type EmailFunc func(ctx context.Context, to, code string) (string, error)

// Options configures storage and the email sender. No new environment variables.
type Options struct {
	DSN         string
	Store       Store
	Now         func() time.Time
	SendEmail   EmailFunc
	CodeLen     int
	TTL         time.Duration
	Resend      time.Duration
	MaxAttempts int
}

// Service is the cloud-password state machine shared by the twofa and authorization RPCs.
type Service struct {
	opt   Options
	store Store
}

var (
	mu      sync.RWMutex
	current = New(Options{})
	// attachedLoginEmail is the verified Redis login email for a phone
	// account. users.email still wins when it is set. Nil leaves the
	// pattern empty for a phone account.
	attachedLoginEmail func(ctx context.Context, userID int64) (string, error)
)

// SetAttachedLoginEmail installs the reader account.getPassword uses when
// users.email is empty. The authorization process sets it to the same Redis
// key auth.sendCode mails. Tests pass nil to clear it.
func SetAttachedLoginEmail(fn func(ctx context.Context, userID int64) (string, error)) {
	mu.Lock()
	attachedLoginEmail = fn
	mu.Unlock()
}

func readAttachedLoginEmail(ctx context.Context, userID int64) (string, error) {
	mu.RLock()
	fn := attachedLoginEmail
	mu.RUnlock()
	if fn == nil || userID == 0 {
		return "", nil
	}
	return fn(ctx, userID)
}

// New builds a service. A nil store uses memory, or MySQL when DSN is set.
func New(opt Options) *Service {
	if opt.CodeLen <= 0 {
		opt.CodeLen = defaultCodeLen
	}
	if opt.TTL <= 0 {
		opt.TTL = defaultTTL
	}
	if opt.Resend <= 0 {
		opt.Resend = defaultResend
	}
	if opt.MaxAttempts <= 0 {
		opt.MaxAttempts = defaultAttempts
	}
	st := opt.Store
	if st == nil {
		st = openStore(opt.DSN)
	}
	return &Service{opt: opt, store: st}
}

// Default is the process-wide service. Configure replaces it at startup.
func Default() *Service {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// ConfigureFromEnv installs the process-wide service.
// Email codes use the existing sender (TELLOMI_EMAIL_PROVIDER, Resend in production).
// DSN is the teamgram database. TEAMGRAM_MYSQL_DSN overrides it. Neither is new.
func ConfigureFromEnv(dsn string) {
	cfg := verify.Load()
	sender := verify.NewEmailSender(cfg.Email)
	Configure(Options{
		DSN:         dsn,
		CodeLen:     cfg.Email.CodeLength,
		TTL:         time.Duration(cfg.EmailLimits.TTLSeconds) * time.Second,
		Resend:      time.Duration(cfg.EmailLimits.ResendSeconds) * time.Second,
		MaxAttempts: cfg.EmailLimits.MaxAttempts,
		SendEmail: func(ctx context.Context, to, code string) (string, error) {
			if !cfg.Email.Enabled() {
				return "", mtproto.ErrEmailNotSetup
			}
			if lim := verify.SharedEmail(); lim != nil {
				if err := lim.AllowSend(ctx, "email", to, ""); err != nil {
					return "", verify.EmailQuotaError(err)
				}
			}
			if err := sender.SendCode(ctx, to, code); err != nil {
				if errors.Is(err, verify.ErrEmailDisabled) {
					return "", mtproto.ErrEmailNotSetup
				}
				return "", mtproto.ErrSmsCodeCreateFailed
			}
			return code, nil
		},
	})
}

// Configure installs the process-wide service. DSN empty keeps memory.
// TEAMGRAM_MYSQL_DSN, when set, overrides DSN. Email uses the existing sender.
func Configure(opt Options) {
	mu.Lock()
	current = New(opt)
	mu.Unlock()
}

// Swap replaces the process-wide service and returns the previous one.
func Swap(svc *Service) *Service {
	mu.Lock()
	prev := current
	if svc == nil {
		svc = New(Options{})
	}
	current = svc
	mu.Unlock()
	return prev
}

func (s *Service) now() time.Time {
	if s != nil && s.opt.Now != nil {
		return s.opt.Now()
	}
	return time.Now()
}

func (s *Service) load(ctx context.Context, userID int64) (Record, error) {
	if s == nil || s.store == nil || userID == 0 {
		return Record{UserID: userID}, nil
	}
	return s.store.Load(ctx, userID)
}

// HasPassword reports whether userID has a stored verifier.
func HasPassword(ctx context.Context, userID int64) (bool, error) {
	rec, err := Default().load(ctx, userID)
	if err != nil {
		return false, err
	}
	return rec.HasPassword, nil
}

// NoteLogin remembers that this auth key still has to pass the cloud password.
// auth.signIn binds the key only after this row exists, so a restarted session
// stays in the password step instead of becoming a normal login.
func NoteLogin(ctx context.Context, authKey, userID int64) error {
	if authKey == 0 || userID == 0 {
		return nil
	}
	return Default().store.PutLogin(ctx, authKey, userID, Default().now().Unix())
}

// PendingUser is the user id waiting on this auth key, or 0.
func PendingUser(ctx context.Context, authKey int64) (int64, error) {
	if authKey == 0 || Default().store == nil {
		return 0, nil
	}
	id, ok, err := Default().store.LoginUser(ctx, authKey)
	if err != nil || !ok {
		return 0, err
	}
	return id, nil
}

// ClearLogin drops the password step for one auth key after a successful check.
func ClearLogin(ctx context.Context, authKey int64) error {
	if authKey == 0 || Default().store == nil {
		return nil
	}
	return Default().store.DeleteLogin(ctx, authKey)
}

// RecoveryEmail is the confirmed 2FA recovery address.
// hasPassword is false when the account has no cloud password.
func RecoveryEmail(ctx context.Context, userID int64) (email string, hasPassword bool, err error) {
	rec, err := Default().load(ctx, userID)
	if err != nil {
		return "", false, err
	}
	return rec.Email, rec.HasPassword, nil
}

// AccountPassword is account.getPassword.
func AccountPassword(ctx context.Context, userID, authKey int64) (*mtproto.Account_Password, error) {
	return Default().accountPassword(ctx, userID, authKey)
}

func (s *Service) accountPassword(ctx context.Context, userID, authKey int64) (*mtproto.Account_Password, error) {
	if userID == 0 {
		return noPassword(s.freshSalts()), nil
	}
	rec, err := s.load(ctx, userID)
	if err != nil {
		return nil, err
	}
	salts := s.freshSalts()
	rec.UserID = userID
	rec.NewSalt1 = salts.salt1
	rec.NewSalt2 = salts.salt2
	rec.NewSecureSalt = salts.secure
	if err = s.store.Save(ctx, rec); err != nil {
		return nil, err
	}
	pw := noPassword(salts)
	if rec.EmailUnconfirmed != "" {
		pw.EmailUnconfirmedPattern = &wrapperspb.StringValue{Value: verify.MaskEmail(rec.EmailUnconfirmed)}
	}
	if rec.PendingReset > 0 {
		pw.PendingResetDate = &wrapperspb.Int32Value{Value: int32(rec.PendingReset)}
	}
	// users.email is the login identity of an email-only account. A phone
	// account keeps its login email in Redis. Clients treat an empty
	// login_email_pattern as "ask the user to bind an email".
	// The recovery address stays on email_unconfirmed_pattern / has_recovery.
	if email, emailErr := s.identityEmail(ctx, userID); emailErr != nil {
		// A failed read must not look like a phone account with no login email.
		// Clients treat a successful empty login_email_pattern as "not bound".
		return nil, mtproto.ErrInternalServerError
	} else if email != "" {
		pw.LoginEmailPattern = &wrapperspb.StringValue{Value: verify.MaskEmail(email)}
	} else if attached, aerr := readAttachedLoginEmail(ctx, userID); aerr != nil {
		return nil, mtproto.ErrInternalServerError
	} else if attached != "" {
		pw.LoginEmailPattern = &wrapperspb.StringValue{Value: verify.MaskEmail(attached)}
	}
	if !rec.HasPassword {
		return pw, nil
	}
	ch, err := s.issue(ctx, rec, authKey)
	if err != nil {
		return nil, err
	}
	pw.HasPassword = true
	pw.HasRecovery = rec.Email != ""
	pw.HasSecureValues = len(rec.SecureSecret) > 0
	pw.CurrentAlgo = modPowAlgo(rec.Salt1, rec.Salt2)
	pw.Srp_B = append([]byte(nil), ch.Pub...)
	pw.SrpId = &wrapperspb.Int64Value{Value: ch.ID}
	if rec.Hint != "" {
		pw.Hint = &wrapperspb.StringValue{Value: rec.Hint}
	}
	return pw, nil
}

type salts struct {
	salt1  []byte
	salt2  []byte
	secure []byte
}

func (s *Service) freshSalts() salts {
	return salts{
		salt1:  random(newSalt1Len),
		salt2:  random(newSalt2Len),
		secure: random(newSalt1Len),
	}
}

func (s *Service) issue(ctx context.Context, rec Record, authKey int64) (Challenge, error) {
	got, err := srp.NewChallenge(rec.Verifier)
	if err != nil {
		return Challenge{}, mtproto.ErrInternalServerError
	}
	id, err := randomID()
	if err != nil {
		return Challenge{}, err
	}
	ch := Challenge{
		ID:        id,
		UserID:    rec.UserID,
		AuthKeyID: authKey,
		Priv:      got.BPriv,
		Pub:       got.BPub,
		Version:   rec.Version,
		Expires:   s.now().Add(challengeTTL).Unix(),
	}
	if err = s.store.PutChallenge(ctx, ch); err != nil {
		return Challenge{}, err
	}
	return ch, nil
}

func noPassword(s salts) *mtproto.Account_Password {
	return mtproto.MakeTLAccountPassword(&mtproto.Account_Password{
		NewAlgo:       modPowAlgo(s.salt1, s.salt2),
		NewSecureAlgo: securePBKDF2(s.secure),
		SecureRandom:  random(secureRandomLen),
	}).To_Account_Password()
}

// Update applies account.updatePasswordSettings.
// A positive code length means the password was stored and the RPC must
// return EMAIL_UNCONFIRMED_<len> so the client can confirm the recovery address.
func Update(ctx context.Context, userID, authKey int64, proof *mtproto.InputCheckPasswordSRP, settings *mtproto.Account_PasswordInputSettings) (int32, error) {
	return Default().update(ctx, userID, authKey, proof, settings)
}

func (s *Service) update(ctx context.Context, userID, authKey int64, proof *mtproto.InputCheckPasswordSRP, settings *mtproto.Account_PasswordInputSettings) (int32, error) {
	rec, err := s.load(ctx, userID)
	if err != nil {
		return 0, err
	}
	if rec.HasPassword {
		if err = s.verify(ctx, rec, authKey, proof); err != nil {
			return 0, err
		}
		rec, err = s.load(ctx, userID)
		if err != nil {
			return 0, err
		}
	} else if err = acceptEmpty(proof); err != nil {
		return 0, err
	}
	if userID == 0 {
		if err = s.validateShape(settings, true); err != nil {
			return 0, err
		}
		return 0, mtproto.ErrNewSaltInvalid
	}
	return s.apply(ctx, rec, settings)
}

// Settings is account.getPasswordSettings.
func Settings(ctx context.Context, userID, authKey int64, proof *mtproto.InputCheckPasswordSRP) (*mtproto.Account_PasswordSettings, error) {
	return Default().settings(ctx, userID, authKey, proof)
}

func (s *Service) settings(ctx context.Context, userID, authKey int64, proof *mtproto.InputCheckPasswordSRP) (*mtproto.Account_PasswordSettings, error) {
	rec, err := s.load(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !rec.HasPassword {
		if err = acceptEmpty(proof); err != nil {
			return nil, mtproto.ErrPasswordHashInvalid
		}
		return emptySettings(), nil
	}
	if err = s.verify(ctx, rec, authKey, proof); err != nil {
		return nil, err
	}
	rec, err = s.load(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := emptySettings()
	if rec.Email != "" {
		out.Email = &wrapperspb.StringValue{Value: rec.Email}
	}
	if len(rec.SecureSecret) > 0 {
		out.SecureSettings = secureSettings(rec)
	}
	return out, nil
}

// Verify checks an SRP proof for auth.checkPassword and account.getTmpPassword.
func Verify(ctx context.Context, userID, authKey int64, proof *mtproto.InputCheckPasswordSRP) error {
	rec, err := Default().load(ctx, userID)
	if err != nil {
		return err
	}
	if !rec.HasPassword {
		return mtproto.ErrPasswordMissing
	}
	return Default().verify(ctx, rec, authKey, proof)
}

// Info is whether userID has a cloud password, when that verifier was stored,
// and the hint. setAt is zero when the row has no password_set_at yet.
func Info(ctx context.Context, userID int64) (has bool, setAt int64, hint string, err error) {
	rec, err := Default().load(ctx, userID)
	if err != nil {
		return false, 0, "", err
	}
	return rec.HasPassword, rec.PasswordSetAt, rec.Hint, nil
}

// CheckPlaintext checks a password typed on the web deletion form.
// The proof is built here so the browser does not implement SRP. The
// plaintext is not written to the log. auth key 0 matches the challenge
// issued below; the stored verifier is what actually accepts the password.
func CheckPlaintext(ctx context.Context, userID int64, plain string) error {
	if userID == 0 || plain == "" {
		return mtproto.ErrPasswordHashInvalid
	}
	pw, err := AccountPassword(ctx, userID, 0)
	if err != nil {
		return err
	}
	if !pw.GetHasPassword() || pw.GetCurrentAlgo() == nil || len(pw.GetSrp_B()) == 0 || pw.GetSrpId() == nil {
		return mtproto.ErrPasswordMissing
	}
	a := make([]byte, srp.PadSize)
	if _, err = rand.Read(a); err != nil {
		return mtproto.ErrInternalServerError
	}
	A, M1, err := srp.ClientProof([]byte(plain), pw.GetCurrentAlgo().GetSalt1(), pw.GetCurrentAlgo().GetSalt2(), a, pw.GetSrp_B())
	if err != nil {
		return mtproto.ErrPasswordHashInvalid
	}
	proof := mtproto.MakeTLInputCheckPasswordSRP(&mtproto.InputCheckPasswordSRP{
		SrpId: pw.GetSrpId().GetValue(),
		A:     A,
		M1:    M1,
	}).To_InputCheckPasswordSRP()
	return Verify(ctx, userID, 0, proof)
}

func (s *Service) verify(ctx context.Context, rec Record, _ int64, proof *mtproto.InputCheckPasswordSRP) error {
	if wait := s.floodWait(rec); wait > 0 {
		return mtproto.NewErrFloodWaitX(wait)
	}
	if predicate(proof) != mtproto.Predicate_inputCheckPasswordSRP {
		return mtproto.ErrPasswordHashInvalid
	}
	ch, ok, err := s.store.TakeChallenge(ctx, proof.GetSrpId())
	if err != nil {
		return err
	}
	if !ok || ch.UserID != rec.UserID || ch.Expires < s.now().Unix() {
		return mtproto.ErrSrpIdInvalid
	}
	// The caller's auth key is not compared to the challenge. iOS
	// verifyPassword does not retry SRP_ID_INVALID, and the first phone
	// login can fetch account.getPassword on the temp key then call
	// auth.checkPassword on the permanent key. srp_id is single-use and
	// M1 still has to prove the password.
	if !rec.HasPassword || ch.Version != rec.Version {
		return mtproto.ErrSrpPasswordChanged
	}
	if !srp.Verify(rec.Verifier, rec.Salt1, rec.Salt2, ch.Priv, ch.Pub, proof.GetA(), proof.GetM1()) {
		return s.noteFailure(ctx, rec)
	}
	if rec.FailCount != 0 || rec.FloodUntil != 0 {
		rec.FailCount = 0
		rec.FloodUntil = 0
		if err = s.store.Save(ctx, rec); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) noteFailure(ctx context.Context, rec Record) error {
	rec.FailCount++
	if rec.FailCount > floodFree {
		rec.FloodUntil = s.now().Unix() + int64(floodSeconds(rec.FailCount))
	}
	if err := s.store.Save(ctx, rec); err != nil {
		return err
	}
	return mtproto.ErrPasswordHashInvalid
}

func (s *Service) floodWait(rec Record) int32 {
	if rec.FailCount <= floodFree || rec.FloodUntil == 0 {
		return 0
	}
	wait := rec.FloodUntil - s.now().Unix()
	if wait <= 0 {
		return 0
	}
	if wait > 86400 {
		wait = 86400
	}
	return int32(wait)
}

func floodSeconds(fails int) int32 {
	shift := fails - floodFree - 1
	if shift < 0 {
		shift = 0
	}
	if shift > 11 {
		shift = 11
	}
	sec := 30 << shift
	if sec > 86400 {
		return 86400
	}
	return int32(sec)
}

func (s *Service) apply(ctx context.Context, rec Record, settings *mtproto.Account_PasswordInputSettings) (int32, error) {
	if err := s.validateShape(settings, !rec.HasPassword); err != nil {
		return 0, err
	}
	// Android and iOS clear 2FA with passwordKdfAlgoUnknown and an empty
	// hash. Hint and email may be present and empty. That is not a new verifier.
	if removesPassword(settings) {
		if !rec.HasPassword {
			return 0, mtproto.ErrNewSaltInvalid
		}
		s.clearPassword(&rec)
		if err := s.store.Save(ctx, rec); err != nil {
			return 0, err
		}
		_ = s.store.DeleteUserLogins(ctx, rec.UserID)
		return 0, nil
	}
	newHash := settings != nil && (settings.GetNewAlgo() != nil || len(settings.GetNewPasswordHash()) > 0)
	if !rec.HasPassword && !newHash {
		return 0, mtproto.ErrNewSettingsEmpty
	}
	if newHash {
		if err := s.checkNewPassword(rec, settings); err != nil {
			return 0, err
		}
	}
	if settings != nil && settings.GetNewSecureSettings() != nil {
		sec := settings.GetNewSecureSettings()
		if len(rec.NewSecureSalt) == 0 || !bytes.Equal(sec.GetSecureAlgo().GetSalt(), rec.NewSecureSalt) {
			return 0, mtproto.ErrNewSettingsInvalid
		}
	}
	var (
		code       string
		codeLen    int32
		email      string
		changeMail bool
	)
	if settings != nil && settings.GetEmail() != nil {
		email = verify.NormalizeEmail(settings.GetEmail().GetValue())
		if email != "" && !verify.ValidEmail(email) {
			return 0, mtproto.ErrEmailInvalid
		}
		if email != rec.Email {
			changeMail = true
		}
	}
	if changeMail && email != "" {
		sent, err := s.sendCode(ctx, email)
		if err != nil {
			return 0, err
		}
		code = sent
		codeLen = int32(len(sent))
	}
	if newHash {
		rec.HasPassword = true
		rec.Algo = AlgoName
		rec.Salt1 = append([]byte(nil), settings.GetNewAlgo().GetSalt1()...)
		rec.Salt2 = append([]byte(nil), settings.GetNewAlgo().GetSalt2()...)
		rec.Verifier = append([]byte(nil), settings.GetNewPasswordHash()...)
		rec.Version++
		rec.FailCount = 0
		rec.FloodUntil = 0
		rec.PendingReset = 0
		rec.PasswordSetAt = s.now().Unix()
		if settings.GetHint() != nil {
			rec.Hint = settings.GetHint().GetValue()
		} else {
			rec.Hint = ""
		}
		_ = s.store.DeleteUserLogins(ctx, rec.UserID)
	} else if settings != nil && settings.GetHint() != nil {
		rec.Hint = settings.GetHint().GetValue()
	}
	if settings != nil && settings.GetNewSecureSettings() != nil {
		sec := settings.GetNewSecureSettings()
		rec.SecureAlgo = secureAlgoName(sec.GetSecureAlgo())
		rec.SecureSalt = append([]byte(nil), sec.GetSecureAlgo().GetSalt()...)
		rec.SecureSecret = append([]byte(nil), sec.GetSecureSecret()...)
		rec.SecureSecretID = sec.GetSecureSecretId()
	}
	if !newHash && rec.HasPassword && !changeMail && (settings == nil || (settings.GetHint() == nil && settings.GetNewSecureSettings() == nil && settings.GetEmail() == nil)) {
		s.clearPassword(&rec)
		_ = s.store.DeleteUserLogins(ctx, rec.UserID)
	}
	if changeMail {
		if email == "" {
			rec.Email = ""
			rec.EmailUnconfirmed = ""
			clearCode(&rec)
		} else {
			rec.EmailUnconfirmed = email
			rec.EmailCode = code
			rec.EmailCodeExpires = s.now().Add(s.opt.TTL).Unix()
			rec.EmailCodeSentAt = s.now().Unix()
			rec.EmailCodeAttempts = 0
		}
	}
	if err := s.store.Save(ctx, rec); err != nil {
		return 0, err
	}
	return codeLen, nil
}

func (s *Service) clearPassword(rec *Record) {
	rec.HasPassword = false
	rec.Algo = ""
	rec.Salt1 = nil
	rec.Salt2 = nil
	rec.Verifier = nil
	rec.Hint = ""
	rec.Email = ""
	rec.EmailUnconfirmed = ""
	clearCode(rec)
	rec.SecureAlgo = ""
	rec.SecureSalt = nil
	rec.SecureSecret = nil
	rec.SecureSecretID = 0
	rec.PendingReset = 0
	rec.PasswordSetAt = 0
	rec.FailCount = 0
	rec.FloodUntil = 0
	rec.Version++
}

func clearCode(rec *Record) {
	rec.EmailCode = ""
	rec.EmailCodeExpires = 0
	rec.EmailCodeSentAt = 0
	rec.EmailCodeAttempts = 0
}

func (s *Service) validateShape(settings *mtproto.Account_PasswordInputSettings, requirePassword bool) error {
	if settings == nil {
		if requirePassword {
			return mtproto.ErrNewSettingsEmpty
		}
		return nil
	}
	algo := settings.GetNewAlgo()
	hash := settings.GetNewPasswordHash()
	if removesPassword(settings) {
		// Checked above. An unknown algorithm with an empty hash is remove.
	} else if algo != nil || len(hash) > 0 {
		if !modPow(algo) || len(algo.GetSalt1()) == 0 || len(algo.GetSalt2()) == 0 || algo.GetG() != srp.Generator || !bytes.Equal(algo.GetP(), srp.Prime) {
			return mtproto.ErrNewSaltInvalid
		}
		if len(hash) != verifierLen || allZero(hash) {
			return mtproto.ErrNewSettingsInvalid
		}
	}
	if hint := settings.GetHint(); hint != nil && len(hint.GetValue()) > hintMax {
		return mtproto.ErrNewSettingsInvalid
	}
	if email := settings.GetEmail(); email != nil {
		raw := email.GetValue()
		if raw != "" && (!verify.ValidEmail(raw) || len(verify.NormalizeEmail(raw)) > 255) {
			return mtproto.ErrEmailInvalid
		}
	}
	if sec := settings.GetNewSecureSettings(); sec != nil {
		name := secureAlgoName(sec.GetSecureAlgo())
		if name == "" || len(sec.GetSecureAlgo().GetSalt()) == 0 || len(sec.GetSecureSecret()) == 0 || len(sec.GetSecureSecret()) > secureSecretMax {
			return mtproto.ErrNewSettingsInvalid
		}
	}
	return nil
}

func (s *Service) checkNewPassword(rec Record, settings *mtproto.Account_PasswordInputSettings) error {
	algo := settings.GetNewAlgo()
	if len(rec.NewSalt1) == 0 || len(algo.GetSalt1()) < len(rec.NewSalt1)+clientSaltMin || !bytes.HasPrefix(algo.GetSalt1(), rec.NewSalt1) {
		return mtproto.ErrNewSaltInvalid
	}
	if len(algo.GetSalt1()) > 128 || len(algo.GetSalt2()) > 64 || !bytes.Equal(algo.GetSalt2(), rec.NewSalt2) {
		return mtproto.ErrNewSaltInvalid
	}
	if sec := settings.GetNewSecureSettings(); sec != nil && !bytes.Equal(sec.GetSecureAlgo().GetSalt(), rec.NewSecureSalt) {
		return mtproto.ErrNewSettingsInvalid
	}
	return nil
}

func (s *Service) sendCode(ctx context.Context, email string) (string, error) {
	if s.opt.SendEmail == nil {
		return "", mtproto.ErrEmailNotSetup
	}
	code := verify.RandomDigits(s.opt.CodeLen)
	sent, err := s.opt.SendEmail(ctx, email, code)
	if err != nil {
		return "", err
	}
	if sent == "" {
		sent = code
	}
	return sent, nil
}

// ConfirmEmail is account.confirmPasswordEmail.
func ConfirmEmail(ctx context.Context, userID int64, code string) error {
	return Default().confirmEmail(ctx, userID, code)
}

func (s *Service) confirmEmail(ctx context.Context, userID int64, code string) error {
	rec, err := s.load(ctx, userID)
	if err != nil {
		return err
	}
	if rec.EmailUnconfirmed == "" || rec.EmailCode == "" || rec.EmailCodeExpires < s.now().Unix() {
		return mtproto.ErrEmailHashExpired
	}
	if rec.EmailCodeAttempts >= s.opt.MaxAttempts {
		return mtproto.NewErrFloodWaitX(emailFlood)
	}
	if subtle.ConstantTimeCompare([]byte(rec.EmailCode), []byte(trim(code))) != 1 || len(rec.EmailCode) != len(trim(code)) {
		rec.EmailCodeAttempts++
		if err = s.store.Save(ctx, rec); err != nil {
			return err
		}
		if rec.EmailCodeAttempts >= s.opt.MaxAttempts {
			return mtproto.NewErrFloodWaitX(emailFlood)
		}
		return mtproto.ErrCodeInvalid
	}
	rec.Email = rec.EmailUnconfirmed
	rec.EmailUnconfirmed = ""
	clearCode(&rec)
	return s.store.Save(ctx, rec)
}

// ResendEmail is account.resendPasswordEmail.
func ResendEmail(ctx context.Context, userID int64) error {
	return Default().resendEmail(ctx, userID)
}

func (s *Service) resendEmail(ctx context.Context, userID int64) error {
	rec, err := s.load(ctx, userID)
	if err != nil {
		return err
	}
	if rec.EmailUnconfirmed == "" {
		return mtproto.ErrEmailHashExpired
	}
	if wait := rec.EmailCodeSentAt + int64(s.opt.Resend/time.Second) - s.now().Unix(); wait > 0 {
		return mtproto.NewErrFloodWaitX(int32(wait))
	}
	sent, err := s.sendCode(ctx, rec.EmailUnconfirmed)
	if err != nil {
		return err
	}
	rec.EmailCode = sent
	rec.EmailCodeExpires = s.now().Add(s.opt.TTL).Unix()
	rec.EmailCodeSentAt = s.now().Unix()
	rec.EmailCodeAttempts = 0
	return s.store.Save(ctx, rec)
}

// CancelEmail is account.cancelPasswordEmail.
func CancelEmail(ctx context.Context, userID int64) error {
	return Default().cancelEmail(ctx, userID)
}

func (s *Service) cancelEmail(ctx context.Context, userID int64) error {
	rec, err := s.load(ctx, userID)
	if err != nil {
		return err
	}
	if rec.EmailUnconfirmed == "" {
		return mtproto.ErrEmailHashExpired
	}
	rec.EmailUnconfirmed = ""
	clearCode(&rec)
	return s.store.Save(ctx, rec)
}

// DeclineReset is account.declinePasswordReset.
func DeclineReset(ctx context.Context, userID int64) error {
	return Default().declineReset(ctx, userID)
}

func (s *Service) declineReset(ctx context.Context, userID int64) error {
	rec, err := s.load(ctx, userID)
	if err != nil {
		return err
	}
	if rec.PendingReset == 0 {
		return mtproto.ErrResetRequestMissing
	}
	rec.PendingReset = 0
	return s.store.Save(ctx, rec)
}

// Reset is account.resetPassword. The first call schedules a 7-day removal.
// A later call after that date removes the verifier.
func Reset(ctx context.Context, userID int64) (*mtproto.Account_ResetPasswordResult, error) {
	return Default().reset(ctx, userID)
}

func (s *Service) reset(ctx context.Context, userID int64) (*mtproto.Account_ResetPasswordResult, error) {
	rec, err := s.load(ctx, userID)
	if err != nil {
		return nil, err
	}
	now := s.now().Unix()
	if !rec.HasPassword {
		return resetOK(), nil
	}
	if rec.PendingReset > now {
		return resetWait(rec.PendingReset), nil
	}
	if rec.PendingReset > 0 && rec.PendingReset <= now {
		s.clearPassword(&rec)
		if err = s.store.Save(ctx, rec); err != nil {
			return nil, err
		}
		_ = s.store.DeleteUserLogins(ctx, userID)
		return resetOK(), nil
	}
	rec.PendingReset = now + int64(resetAfter/time.Second)
	if err = s.store.Save(ctx, rec); err != nil {
		return nil, err
	}
	return resetWait(rec.PendingReset), nil
}

func resetOK() *mtproto.Account_ResetPasswordResult {
	return mtproto.MakeTLAccountResetPasswordOk(&mtproto.Account_ResetPasswordResult{}).To_Account_ResetPasswordResult()
}

func resetWait(until int64) *mtproto.Account_ResetPasswordResult {
	return mtproto.MakeTLAccountResetPasswordRequestedWait(&mtproto.Account_ResetPasswordResult{
		UntilDate: int32(until),
	}).To_Account_ResetPasswordResult()
}

// CheckRecoverySettings rejects a malformed auth.recoverPassword new_settings
// before the recovery code is consumed.
func CheckRecoverySettings(ctx context.Context, userID int64, settings *mtproto.Account_PasswordInputSettings) error {
	if settings == nil {
		return nil
	}
	s := Default()
	rec, err := s.load(ctx, userID)
	if err != nil {
		return err
	}
	if err = s.validateShape(settings, false); err != nil {
		if err == mtproto.ErrNewSettingsEmpty {
			return nil
		}
		return err
	}
	if settings.GetNewAlgo() != nil || len(settings.GetNewPasswordHash()) > 0 {
		return s.checkNewPassword(rec, settings)
	}
	return nil
}

// CommitRecovery stores a new verifier or recovery email after the code is consumed.
func CommitRecovery(ctx context.Context, userID int64, settings *mtproto.Account_PasswordInputSettings) error {
	if settings == nil || userID == 0 {
		return nil
	}
	s := Default()
	rec, err := s.load(ctx, userID)
	if err != nil {
		return err
	}
	rec.UserID = userID
	changed := false
	if settings.GetNewAlgo() != nil && len(settings.GetNewPasswordHash()) == verifierLen {
		if err = s.checkNewPassword(rec, settings); err != nil {
			return err
		}
		rec.HasPassword = true
		rec.Algo = AlgoName
		rec.Salt1 = append([]byte(nil), settings.GetNewAlgo().GetSalt1()...)
		rec.Salt2 = append([]byte(nil), settings.GetNewAlgo().GetSalt2()...)
		rec.Verifier = append([]byte(nil), settings.GetNewPasswordHash()...)
		rec.Version++
		rec.FailCount = 0
		rec.FloodUntil = 0
		rec.PendingReset = 0
		rec.PasswordSetAt = s.now().Unix()
		if settings.GetHint() != nil {
			rec.Hint = settings.GetHint().GetValue()
		}
		changed = true
	}
	if email := settings.GetEmail(); email != nil {
		addr := verify.NormalizeEmail(email.GetValue())
		if addr != "" {
			rec.Email = addr
			rec.EmailUnconfirmed = ""
			clearCode(&rec)
			changed = true
		}
	}
	if !changed || (!rec.Exists && !rec.HasPassword) {
		return nil
	}
	return s.store.Save(ctx, rec)
}

func acceptEmpty(password *mtproto.InputCheckPasswordSRP) error {
	switch predicate(password) {
	case mtproto.Predicate_inputCheckPasswordEmpty:
		return nil
	case mtproto.Predicate_inputCheckPasswordSRP:
		return mtproto.ErrSrpIdInvalid
	default:
		return mtproto.ErrPasswordHashInvalid
	}
}

func predicate(password *mtproto.InputCheckPasswordSRP) string {
	if password == nil {
		return ""
	}
	if password.PredicateName != "" {
		return password.PredicateName
	}
	switch password.Constructor {
	case mtproto.CRC32_inputCheckPasswordEmpty:
		return mtproto.Predicate_inputCheckPasswordEmpty
	case mtproto.CRC32_inputCheckPasswordSRP:
		return mtproto.Predicate_inputCheckPasswordSRP
	default:
		return ""
	}
}

func modPow(algo *mtproto.PasswordKdfAlgo) bool {
	if algo == nil {
		return false
	}
	if algo.PredicateName == mtproto.Predicate_passwordKdfAlgoModPow {
		return true
	}
	return algo.Constructor == mtproto.CRC32_passwordKdfAlgoModPow
}

// removesPassword is the client shape for turning 2FA off: passwordKdfAlgoUnknown
// plus an empty new_password_hash. Empty hint and email flags do not matter.
func removesPassword(settings *mtproto.Account_PasswordInputSettings) bool {
	if settings == nil || !unknownAlgo(settings.GetNewAlgo()) {
		return false
	}
	return len(settings.GetNewPasswordHash()) == 0
}

func unknownAlgo(algo *mtproto.PasswordKdfAlgo) bool {
	if algo == nil {
		return false
	}
	if algo.PredicateName == mtproto.Predicate_passwordKdfAlgoUnknown {
		return true
	}
	return algo.Constructor == mtproto.CRC32_passwordKdfAlgoUnknown
}

func modPowAlgo(salt1, salt2 []byte) *mtproto.PasswordKdfAlgo {
	return mtproto.MakeTLPasswordKdfAlgoModPow(&mtproto.PasswordKdfAlgo{
		Salt1: append([]byte(nil), salt1...),
		Salt2: append([]byte(nil), salt2...),
		G:     srp.Generator,
		P:     append([]byte(nil), srp.Prime...),
	}).To_PasswordKdfAlgo()
}

func securePBKDF2(salt []byte) *mtproto.SecurePasswordKdfAlgo {
	return mtproto.MakeTLSecurePasswordKdfAlgoPBKDF2(&mtproto.SecurePasswordKdfAlgo{
		Salt: append([]byte(nil), salt...),
	}).To_SecurePasswordKdfAlgo()
}

func secureAlgoName(algo *mtproto.SecurePasswordKdfAlgo) string {
	if algo == nil {
		return ""
	}
	switch algo.PredicateName {
	case mtproto.Predicate_securePasswordKdfAlgoPBKDF2:
		return secureAlgoPBKDF2
	case mtproto.Predicate_securePasswordKdfAlgoSHA512:
		return secureAlgoSHA512
	}
	switch algo.Constructor {
	case mtproto.CRC32_securePasswordKdfAlgoPBKDF2:
		return secureAlgoPBKDF2
	case mtproto.CRC32_securePasswordKdfAlgoSHA512:
		return secureAlgoSHA512
	default:
		return ""
	}
}

func secureSettings(rec Record) *mtproto.SecureSecretSettings {
	var algo *mtproto.SecurePasswordKdfAlgo
	if rec.SecureAlgo == secureAlgoSHA512 {
		algo = mtproto.MakeTLSecurePasswordKdfAlgoSHA512(&mtproto.SecurePasswordKdfAlgo{
			Salt: append([]byte(nil), rec.SecureSalt...),
		}).To_SecurePasswordKdfAlgo()
	} else {
		algo = securePBKDF2(rec.SecureSalt)
	}
	return mtproto.MakeTLSecureSecretSettings(&mtproto.SecureSecretSettings{
		SecureAlgo:     algo,
		SecureSecret:   append([]byte(nil), rec.SecureSecret...),
		SecureSecretId: rec.SecureSecretID,
	}).To_SecureSecretSettings()
}

func emptySettings() *mtproto.Account_PasswordSettings {
	return mtproto.MakeTLAccountPasswordSettings(&mtproto.Account_PasswordSettings{}).To_Account_PasswordSettings()
}

func allZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}

func trim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\n' || s[0] == '\t') {
		s = s[1:]
	}
	return s
}

func random(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

func randomID() (int64, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, err
	}
	id := int64(binary.BigEndian.Uint64(b[:]) & 0x7fffffffffffffff)
	if id == 0 {
		id = 1
	}
	return id, nil
}
