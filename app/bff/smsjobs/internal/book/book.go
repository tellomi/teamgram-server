// Package book stores Peer-to-Peer Login Program membership.
//
// The methods are documented at:
//
//	https://core.telegram.org/method/smsjobs.isEligibleToJoin
//	https://core.telegram.org/method/smsjobs.join
//	https://core.telegram.org/method/smsjobs.leave
//	https://core.telegram.org/method/smsjobs.updateSettings
//	https://core.telegram.org/method/smsjobs.getStatus
//	https://core.telegram.org/method/smsjobs.getSmsJob
//	https://core.telegram.org/method/smsjobs.finishJob
//
// Tellomi does not dispatch SMS login jobs. The process-wide book is never
// offered, so clients receive the documented NOT_ELIGIBLE, NOT_JOINED, and
// SMSJOB_ID_INVALID errors and no phone number is ever returned. offer is
// for tests of the membership ledger.
package book

import (
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"

	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

var (
	// isEligibleToJoin uses 403. join uses 400. Both texts are NOT_ELIGIBLE.
	errNotEligible     = status.Error(mtproto.ErrForbidden, "NOT_ELIGIBLE")
	errJoinNotEligible = status.Error(mtproto.ErrBadRequest, "NOT_ELIGIBLE")
	errNotJoined       = status.Error(mtproto.ErrBadRequest, "NOT_JOINED")
	errJobInvalid      = status.Error(mtproto.ErrBadRequest, "SMSJOB_ID_INVALID")
)

type member struct {
	allowIntl     bool
	recentSent    int32
	recentSince   int32
	recentRemains int32
	totalSent     int32
	totalSince    int32
	lastGift      string
}

type job struct {
	owner int64
	phone string
	text  string
}

// Book is one program ledger.
type Book struct {
	mu      sync.Mutex
	offered bool
	terms   string
	monthly int32
	members map[int64]*member
	jobs    map[string]*job
}

var (
	once   sync.Once
	shared *Book
)

// Shared is the process-wide ledger. It is not offered.
func Shared() *Book {
	once.Do(func() { shared = New() })
	return shared
}

// New returns a closed ledger.
func New() *Book {
	return &Book{
		members: map[int64]*member{},
		jobs:    map[string]*job{},
	}
}

// offer opens the program on this book. Shared is never offered.
func (b *Book) offer(terms string, monthly int32) {
	if monthly < 0 {
		monthly = 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.offered = true
	b.terms = terms
	b.monthly = monthly
}

// assign stores a job for a joined user. The RPC service does not call this.
func (b *Book) assign(user int64, id, phone, text string) error {
	if user == 0 {
		return mtproto.ErrAuthKeyUnregistered
	}
	if id == "" {
		return errJobInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.offered {
		return errJobInvalid
	}
	if _, ok := b.members[user]; !ok {
		return errNotJoined
	}
	if existing := b.jobs[id]; existing != nil && existing.owner != user {
		return errJobInvalid
	}
	b.jobs[id] = &job{owner: user, phone: phone, text: text}
	return nil
}

// Eligible implements smsjobs.isEligibleToJoin.
func (b *Book) Eligible(user int64) (*mtproto.Smsjobs_EligibilityToJoin, error) {
	if user == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.offered {
		return nil, errNotEligible
	}
	if _, joined := b.members[user]; joined {
		// The subscribe sheet is for people who have not joined.
		// getStatus is the joined view. The documented refusal is NOT_ELIGIBLE.
		return nil, errNotEligible
	}
	return mtproto.MakeTLSmsjobsEligibleToJoin(&mtproto.Smsjobs_EligibilityToJoin{
		TermsUrl:       b.terms,
		MonthlySentSms: b.monthly,
	}).To_Smsjobs_EligibilityToJoin(), nil
}

// Join implements smsjobs.join.
func (b *Book) Join(user int64) (*mtproto.Bool, error) {
	if user == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.offered {
		return nil, errJoinNotEligible
	}
	if _, ok := b.members[user]; ok {
		return mtproto.BoolTrue, nil
	}
	now := int32(time.Now().Unix())
	b.members[user] = &member{
		recentSince:   now,
		recentRemains: b.monthly,
		totalSince:    now,
	}
	return mtproto.BoolTrue, nil
}

// Leave implements smsjobs.leave.
func (b *Book) Leave(user int64) (*mtproto.Bool, error) {
	if user == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.offered {
		return nil, errNotJoined
	}
	if _, ok := b.members[user]; !ok {
		return nil, errNotJoined
	}
	delete(b.members, user)
	for id, j := range b.jobs {
		if j.owner == user {
			delete(b.jobs, id)
		}
	}
	return mtproto.BoolTrue, nil
}

// UpdateSettings implements smsjobs.updateSettings.
func (b *Book) UpdateSettings(user int64, allowInternational bool) (*mtproto.Bool, error) {
	if user == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	m := b.member(user)
	if m == nil {
		return nil, errNotJoined
	}
	m.allowIntl = allowInternational
	return mtproto.BoolTrue, nil
}

// Status implements smsjobs.getStatus.
func (b *Book) Status(user int64) (*mtproto.Smsjobs_Status, error) {
	if user == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	m := b.member(user)
	if m == nil {
		return nil, errNotJoined
	}
	row := mtproto.MakeTLSmsjobsStatus(&mtproto.Smsjobs_Status{
		AllowInternational: m.allowIntl,
		RecentSent:         m.recentSent,
		RecentSince:        m.recentSince,
		RecentRemains:      m.recentRemains,
		TotalSent:          m.totalSent,
		TotalSince:         m.totalSince,
		TermsUrl:           b.terms,
	})
	if m.lastGift != "" {
		row.SetLastGiftSlug(&wrapperspb.StringValue{Value: m.lastGift})
	}
	return row.To_Smsjobs_Status(), nil
}

// Job implements smsjobs.getSmsJob.
func (b *Book) Job(user int64, id string) (*mtproto.SmsJob, error) {
	if user == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if id == "" {
		return nil, errJobInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	j := b.ownJob(user, id)
	if j == nil {
		return nil, errJobInvalid
	}
	return mtproto.MakeTLSmsJob(&mtproto.SmsJob{
		JobId:       id,
		PhoneNumber: j.phone,
		Text:        j.text,
	}).To_SmsJob(), nil
}

// Finish implements smsjobs.finishJob. A set error flag completes the job
// without counting it as sent.
func (b *Book) Finish(user int64, id string, failed bool) (*mtproto.Bool, error) {
	if user == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if id == "" {
		return nil, errJobInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	j := b.ownJob(user, id)
	if j == nil {
		return nil, errJobInvalid
	}
	delete(b.jobs, id)
	if !failed {
		m := b.members[user]
		if m != nil {
			m.recentSent++
			m.totalSent++
			if m.recentRemains > 0 {
				m.recentRemains--
			}
		}
	}
	return mtproto.BoolTrue, nil
}

func (b *Book) member(user int64) *member {
	if !b.offered {
		return nil
	}
	return b.members[user]
}

func (b *Book) ownJob(user int64, id string) *job {
	if !b.offered {
		return nil
	}
	j := b.jobs[id]
	if j == nil || j.owner != user {
		return nil
	}
	return j
}
