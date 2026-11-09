package enforcement

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

const (
	AdminUserID int64 = 777000
	AIUserID    int64 = 777001

	KindWarning   = "warning"
	KindScoped    = "scoped_restriction"
	KindReadOnly  = "read_only_freeze"
	KindPermanent = "permanent_disabled"
	KindSecurity  = "security_lock"

	ScopeStranger = "stranger_message"
	ScopeDiscover = "discover_post"
	ScopeProfile  = "edit_profile"
	ScopeRebind   = "change_identifier"

	RoleAgent    = "agent"
	RoleSenior   = "senior"
	RoleAppeals  = "appeals"
	RoleAuditor  = "auditor"
	RoleSysadmin = "sysadmin"

	ActionUphold       = "uphold"
	ActionLift         = "lift"
	ActionInsufficient = "insufficient"

	StatusPendingApproval = "pending_approval"
	StatusActive          = "active"
	StatusLifted          = "lifted"
	StatusRejected        = "rejected"

	AppealOpen          = "open"
	AppealUpheld        = "upheld"
	AppealLifted        = "lifted"
	AppealPendingReview = "pending_review"
)

var (
	ErrCreateDisabled = errors.New("enforcement: create disabled")
	ErrRole           = errors.New("enforcement: role refused")
	ErrNotFound       = errors.New("enforcement: not found")
	ErrClosed         = errors.New("enforcement: appeal already decided")
)

// Actor is an operator. The AI user is not an actor and cannot create sanctions.
type Actor struct {
	ID   string
	Role string
}

// Sanction is an append-only case. Later decisions are events, not rewrites.
type Sanction struct {
	ID             string
	UserID         int64
	Kind           string
	Scopes         []string
	ReasonCategory string
	PolicyClause   string
	RuleVersion    string
	EvidenceRef    string
	Initiator      string
	Approver       string
	Start          time.Time
	End            *time.Time
	Permanent      bool
	Status         string
	IdempotencyKey string
}

// Event is a later decision on a case. The original sanction stays.
type Event struct {
	ID         string
	SanctionID string
	Type       string
	Actor      string
	At         time.Time
	Reason     string
}

// Appeal is one user's case. User text is untrusted input.
type Appeal struct {
	ID            string
	SanctionID    string
	UserID        int64
	Text          string
	Status        string
	Action        string
	UserReason    string
	RuleRef       string
	EvidenceRefs  []string
	Model         string
	PolicyVersion string
	Attempts      int
}

// Notice is one outbox row. The idempotency key is case id plus event.
type Notice struct {
	Key      string
	UserID   int64
	From     int64
	Template string
	Body     string
	Status   string
	Attempts int
}

// Audit is append-only. Delete is refused by not exposing a delete method.
type Audit struct {
	At     time.Time
	Actor  string
	Action string
	CaseID string
	Detail string
}

// Login is the result after a verified identifier. Before verification the
// code stays empty.
type Login struct {
	BindOrdinary bool
	WaitPassword bool
	Code         string
	Grant        *Grant
	State        string
	CaseIDs      []string
	EndsAt       *time.Time
}

// Grant is the short-lived disposition credential. It is not an auth key.
type Grant struct {
	Token   string
	UserID  int64
	Expires time.Time
}

// Decision is the shared check for a protected action.
type Decision struct {
	Allowed    bool
	Code       string
	State      string
	CaseIDs    []string
	EndsAt     *time.Time
	FailClosed bool
	Degraded   bool
}

// Sender delivers one 777000 private message. A failure leaves the outbox row.
type Sender interface {
	SendFromAdmin(userID int64, key, body string) error
}

// Judge is @tellomi. Implementations may be a live completer or a test fake.
type Judge interface {
	Decide(in AppealBrief) (Verdict, error)
}

// AppealBrief is the only material the judge receives.
type AppealBrief struct {
	CaseID         string
	UserID         int64
	Kind           string
	RuleVersion    string
	PolicyClause   string
	ReasonCategory string
	EvidenceRef    string
	UserText       string
}

// Verdict is the structured model output. Extra actions are ignored.
type Verdict struct {
	Action        string
	UserReason    string
	RuleRef       string
	EvidenceRefs  []string
	Model         string
	PolicyVersion string
}

type snapshot struct {
	restricts    bool
	readOnly     bool
	permanent    bool
	securityLock bool
	scopes       map[string]bool
	code         string
	ends         *time.Time
	cases        []string
}

// Service is safe for concurrent use. Two instances may share a Store.
type Service struct {
	mu           sync.Mutex
	create       bool
	scopedRebind bool
	dualAfter    time.Duration
	grantSecret  []byte
	grantTTL     time.Duration
	now          func() time.Time
	sender       Sender
	judge        Judge
	revoker      func(userID int64) error
	revoked      map[int64]int
	db           *sql.DB
	tx           *sql.Tx
	reviews      *reviewBook
	auditFrom    int
	eventFrom    int

	rows        map[string]*Sanction
	order       []string
	byUser      map[int64][]string
	idem        map[string]string
	events      []Event
	appeals     map[string]*Appeal
	byCase      map[string]string
	notices     map[string]*Notice
	noticeOrder []string
	audits      []Audit
	bind        map[string]int64
	idBlock     map[string]bool
	locks       map[int64]bool
	grants      map[string]*Grant
	claims      map[string]*Grant
	loginProof  map[int64]string
	subjects    map[string]int64
	known       map[int64]snapshot
	failList    bool
	failGet     bool
}

// Config is process configuration. Zero values do not invent sanction tiers.
type Config struct {
	CreateEnabled      bool
	ScopedBlocksRebind bool
	// DualApprovalAfter is optional. Zero means only permanent cases need a
	// second senior. A 30-day threshold is not assumed.
	DualApprovalAfter time.Duration
	GrantSecret       []byte
	GrantTTL          time.Duration
	Now               func() time.Time
	Sender            Sender
	Judge             Judge
}

func New(cfg Config) *Service {
	ttl := cfg.GrantTTL
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	secret := cfg.GrantSecret
	if len(secret) == 0 {
		secret = []byte("enforcement-grant-unset")
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		create:       cfg.CreateEnabled,
		scopedRebind: cfg.ScopedBlocksRebind,
		dualAfter:    cfg.DualApprovalAfter,
		grantSecret:  append([]byte(nil), secret...),
		grantTTL:     ttl,
		now:          now,
		sender:       cfg.Sender,
		judge:        cfg.Judge,
		revoked:      map[int64]int{},
		rows:         map[string]*Sanction{},
		byUser:       map[int64][]string{},
		idem:         map[string]string{},
		appeals:      map[string]*Appeal{},
		byCase:       map[string]string{},
		notices:      map[string]*Notice{},
		bind:         map[string]int64{},
		idBlock:      map[string]bool{},
		locks:        map[int64]bool{},
		grants:       map[string]*Grant{},
		claims:       map[string]*Grant{},
		known:        map[int64]snapshot{},
		reviews:      newReviewBook(nil),
	}
}

func (s *Service) SetSender(sender Sender) {
	s.mu.Lock()
	s.sender = sender
	s.mu.Unlock()
}

func (s *Service) SetJudge(j Judge) {
	s.mu.Lock()
	s.judge = j
	s.mu.Unlock()
}

func (s *Service) SetRevoker(fn func(userID int64) error) {
	s.mu.Lock()
	s.revoker = fn
	s.mu.Unlock()
}

func (s *Service) SetCreateEnabled(on bool) {
	s.mu.Lock()
	s.create = on
	s.mu.Unlock()
}

// FailList makes the next user-list read fail. Rows remain.
func (s *Service) FailList(on bool) {
	s.mu.Lock()
	s.failList = on
	s.mu.Unlock()
}

func (s *Service) clock() time.Time {
	if s.now == nil {
		return time.Now().UTC()
	}
	return s.now().UTC()
}

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func bindKey(kind, value string) string { return kind + "\x00" + value }
