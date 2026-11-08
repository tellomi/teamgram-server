package enforcement

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

// Create writes a case only when creation is enabled. Permanent cases, and
// timed cases longer than a configured dual-approval window, stay pending
// until a different senior approves. The initiator cannot approve.
func (s *Service) Create(actor Actor, in Sanction) (Sanction, error) {
	row, err := s.createLocked(actor, in)
	s.deliver()
	return row, err
}

func (s *Service) createLocked(actor Actor, in Sanction) (row Sanction, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err = s.begin(); err != nil {
		return Sanction{}, err
	}
	defer func() { err = s.finish(err) }()
	if !s.create {
		return Sanction{}, ErrCreateDisabled
	}
	if actor.Role == RoleAuditor || actor.Role == RoleSysadmin || actor.Role == RoleAppeals || actor.ID == "" {
		s.auditLocked(actor.ID, "create_refused", "", actor.Role)
		return Sanction{}, ErrRole
	}
	if actor.Role != RoleAgent && actor.Role != RoleSenior {
		s.auditLocked(actor.ID, "create_refused", "", actor.Role)
		return Sanction{}, ErrRole
	}
	if in.IdempotencyKey != "" {
		if id, ok := s.idem[in.IdempotencyKey]; ok {
			return *s.rows[id], nil
		}
	}
	if in.UserID == 0 || in.Kind == "" {
		return Sanction{}, ErrNotFound
	}
	in.ID = newID()
	in.Initiator = actor.ID
	if in.Start.IsZero() {
		in.Start = s.clock()
	}
	in.Start = in.Start.UTC()
	if in.End != nil {
		t := in.End.UTC()
		in.End = &t
	}
	needsSecond := in.Permanent || in.Kind == KindPermanent
	if !needsSecond && s.dualAfter > 0 && in.End != nil && in.End.Sub(in.Start) >= s.dualAfter {
		needsSecond = true
	}
	if needsSecond {
		in.Status = StatusPendingApproval
		in.Permanent = in.Permanent || in.Kind == KindPermanent
		if in.Kind == KindPermanent {
			in.Permanent = true
			in.End = nil
		}
	} else {
		in.Status = StatusActive
	}
	row = in
	s.rows[row.ID] = &row
	s.order = append(s.order, row.ID)
	s.byUser[row.UserID] = append(s.byUser[row.UserID], row.ID)
	if row.IdempotencyKey != "" {
		s.idem[row.IdempotencyKey] = row.ID
	}
	s.auditLocked(actor.ID, "create", row.ID, row.Kind)
	if row.Status == StatusActive {
		s.onActiveLocked(&row)
	}
	return row, nil
}

// Approve activates a pending case. The approver must be a different senior.
func (s *Service) Approve(actor Actor, id string) (Sanction, error) {
	row, err := s.approveLocked(actor, id)
	s.deliver()
	return row, err
}

func (s *Service) approveLocked(actor Actor, id string) (row Sanction, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err = s.begin(); err != nil {
		return Sanction{}, err
	}
	defer func() { err = s.finish(err) }()
	got := s.rows[id]
	if got == nil {
		return Sanction{}, ErrNotFound
	}
	row = *got
	if actor.Role != RoleSenior || actor.ID == "" || actor.ID == row.Initiator {
		s.auditLocked(actor.ID, "approve_refused", id, actor.Role)
		return Sanction{}, ErrRole
	}
	if row.Status != StatusPendingApproval {
		return row, nil
	}
	row.Status = StatusActive
	row.Approver = actor.ID
	got.Status = StatusActive
	got.Approver = actor.ID
	s.events = append(s.events, Event{ID: newID(), SanctionID: id, Type: "approve", Actor: actor.ID, At: s.clock()})
	s.auditLocked(actor.ID, "approve", id, "")
	s.onActiveLocked(got)
	return row, nil
}

// Revoke appends a human correction. It does not delete the case.
func (s *Service) Revoke(actor Actor, id, reason string) (Sanction, error) {
	row, err := s.revokeLocked(actor, id, reason)
	s.deliver()
	return row, err
}

func (s *Service) revokeLocked(actor Actor, id, reason string) (out Sanction, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err = s.begin(); err != nil {
		return Sanction{}, err
	}
	defer func() { err = s.finish(err) }()
	row := s.rows[id]
	if row == nil {
		return Sanction{}, ErrNotFound
	}
	if actor.ID == "" || actor.ID == row.Initiator || (row.Approver != "" && actor.ID == row.Approver) {
		s.auditLocked(actor.ID, "revoke_refused", id, "same person")
		return Sanction{}, ErrRole
	}
	if actor.Role != RoleSenior && actor.Role != RoleAppeals {
		s.auditLocked(actor.ID, "revoke_refused", id, actor.Role)
		return Sanction{}, ErrRole
	}
	if row.Status == StatusLifted {
		return *row, nil
	}
	row.Status = StatusLifted
	s.events = append(s.events, Event{ID: newID(), SanctionID: id, Type: "revoke", Actor: actor.ID, At: s.clock(), Reason: reason})
	s.auditLocked(actor.ID, "revoke", id, reason)
	s.queueNoticeLocked(row, "T-05", "revoke")
	s.refreshKnownLocked(row.UserID)
	return *row, nil
}

// BindIdentifier records a verified phone or email. Comparison is exact.
func (s *Service) BindIdentifier(userID int64, kind, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.begin()
	defer func() { _ = s.finish(nil) }()
	if userID == 0 || value == "" {
		return
	}
	s.bind[bindKey(kind, value)] = userID
}

// RestrictIdentifier blocks one identifier and does not change the account.
func (s *Service) RestrictIdentifier(actor Actor, kind, value string) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err = s.begin(); err != nil {
		return err
	}
	defer func() { err = s.finish(err) }()
	if !s.create {
		return ErrCreateDisabled
	}
	if actor.Role != RoleAgent && actor.Role != RoleSenior {
		return ErrRole
	}
	s.idBlock[bindKey(kind, value)] = true
	s.auditLocked(actor.ID, "identifier_restrict", "", kind)
	return nil
}

// Lock is a security lock. It writes no sanction and no penalty notice.
func (s *Service) Lock(userID int64) {
	s.mu.Lock()
	_ = s.begin()
	s.locks[userID] = true
	s.auditLocked("security", "security_lock", "", "")
	_ = s.finish(nil)
	s.mu.Unlock()
}

func (s *Service) Unlock(userID int64) {
	s.mu.Lock()
	_ = s.begin()
	delete(s.locks, userID)
	s.auditLocked("security", "security_unlock", "", "")
	_ = s.finish(nil)
	s.mu.Unlock()
}

func (s *Service) onActiveLocked(row *Sanction) {
	s.queueNoticeLocked(row, templateFor(row.Kind), "active")
	s.refreshKnownLocked(row.UserID)
	if row.Permanent || row.Kind == KindPermanent {
		if s.revoker != nil {
			if _, ok := s.revoked[row.UserID]; !ok {
				if err := s.revoker(row.UserID); err == nil {
					s.revoked[row.UserID] = 1
				}
			}
		} else {
			s.revoked[row.UserID] = 1
		}
	}
}

func templateFor(kind string) string {
	switch kind {
	case KindReadOnly:
		return "T-02"
	case KindPermanent:
		return "T-03"
	default:
		return "T-01"
	}
}

// Stage is the login decision. passwordSatisfied false returns no sanction detail.
func (s *Service) Stage(userID int64, identifier, kind string, passwordSatisfied bool, now time.Time) Login {
	out := s.stageLocked(userID, identifier, kind, passwordSatisfied, now)
	s.deliver()
	return out
}

func (s *Service) stageLocked(userID int64, identifier, kind string, passwordSatisfied bool, now time.Time) Login {
	if s == nil {
		return Login{BindOrdinary: true}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.begin(); err != nil {
		if snap, ok := s.known[userID]; ok && snap.permanent {
			return Login{Code: "ACCOUNT_PERMANENT_DISABLED"}
		}
		return Login{BindOrdinary: true}
	}
	defer func() { _ = s.finish(nil) }()
	if !passwordSatisfied {
		return Login{WaitPassword: true}
	}
	if identifier != "" && s.idBlock[bindKey(kind, identifier)] {
		return Login{Code: "IDENTIFIER_RESTRICTED"}
	}
	if s.locks[userID] {
		return Login{Code: "ACCOUNT_SECURITY_LOCK"}
	}
	s.expireLocked(userID, now)
	snap := s.snapshotLocked(userID, now)
	s.known[userID] = snap
	if !snap.restricts {
		return Login{BindOrdinary: true, State: "normal"}
	}
	out := Login{State: snap.code, CaseIDs: append([]string(nil), snap.cases...), EndsAt: snap.ends}
	if snap.permanent {
		g := s.issueLocked(userID, now)
		out.Grant = &g
		out.Code = "ACCOUNT_PERMANENT_DISABLED"
		return out
	}
	out.BindOrdinary = true
	if snap.readOnly {
		out.Code = "ACCOUNT_READ_ONLY"
	}
	return out
}

// SignupInstead reports that this identifier already belongs to a user.
func (s *Service) SignupInstead(identifier, kind string, now time.Time) (Login, bool) {
	if s == nil {
		return Login{}, false
	}
	s.mu.Lock()
	userID := s.bind[bindKey(kind, identifier)]
	s.mu.Unlock()
	if userID == 0 {
		return Login{}, false
	}
	return s.Stage(userID, identifier, kind, true, now), true
}

func (s *Service) issueLocked(userID int64, now time.Time) Grant {
	mac := hmac.New(sha256.New, s.grantSecret)
	raw := newID()
	_, _ = mac.Write([]byte(raw))
	token := hex.EncodeToString(mac.Sum(nil)) + "." + raw
	g := &Grant{Token: token, UserID: userID, Expires: now.Add(s.grantTTL)}
	s.grants[token] = g
	return *g
}

// UseGrant checks a disposition credential. It never becomes an ordinary session.
func (s *Service) UseGrant(token string, now time.Time) (Grant, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.begin()
	defer func() { _ = s.finish(nil) }()
	g := s.grants[token]
	if g == nil || !now.Before(g.Expires) {
		return Grant{}, false
	}
	return *g, true
}

// Decide is the shared action check.
func (s *Service) Decide(userID int64, action string, stranger bool, now time.Time) Decision {
	out := s.decideLocked(userID, action, stranger, now)
	s.deliver()
	return out
}

func (s *Service) decideLocked(userID int64, action string, stranger bool, now time.Time) Decision {
	if s == nil {
		return Decision{Allowed: true}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.begin(); err != nil {
		if snap, ok := s.known[userID]; ok && snap.restricts {
			return s.decideFrom(snap, action, stranger, true)
		}
		return Decision{Allowed: true, Degraded: true}
	}
	defer func() { _ = s.finish(nil) }()
	if s.failList {
		if snap, ok := s.known[userID]; ok && snap.restricts {
			return s.decideFrom(snap, action, stranger, true)
		}
		return Decision{Allowed: true, Degraded: true}
	}
	s.expireLocked(userID, now)
	snap := s.snapshotLocked(userID, now)
	s.known[userID] = snap
	return s.decideFrom(snap, action, stranger, false)
}

func (s *Service) decideFrom(snap snapshot, action string, stranger, failed bool) Decision {
	d := Decision{Allowed: true, State: "normal", FailClosed: failed, CaseIDs: append([]string(nil), snap.cases...), EndsAt: snap.ends}
	if !snap.restricts {
		return d
	}
	d.State = snap.code
	deny := false
	switch action {
	case "appeal", "read_notice", "logout", "identity":
		deny = false
	case "read_chat":
		deny = snap.permanent
	case "send_message":
		if snap.permanent || snap.readOnly {
			deny = true
		} else if stranger && snap.scopes[ScopeStranger] {
			deny = true
		}
	case "discover_post":
		deny = snap.permanent || snap.readOnly || snap.scopes[ScopeDiscover]
	case "discover_credential":
		deny = snap.permanent
	case "edit_profile":
		deny = snap.permanent || snap.readOnly || snap.scopes[ScopeProfile]
	case "change_identifier":
		deny = snap.permanent || snap.readOnly || (s.scopedRebind && len(snap.scopes) > 0)
	default:
		deny = snap.permanent
	}
	if deny {
		d.Allowed = false
		switch {
		case snap.permanent:
			d.Code = "ACCOUNT_PERMANENT_DISABLED"
		case snap.readOnly:
			d.Code = "ACCOUNT_READ_ONLY"
		default:
			d.Code = "ACCOUNT_SCOPED_RESTRICTION"
		}
	}
	return d
}

func (s *Service) expireLocked(userID int64, now time.Time) {
	now = now.UTC()
	changed := false
	for _, id := range s.byUser[userID] {
		row := s.rows[id]
		if row == nil || row.Status != StatusActive || row.Permanent || row.End == nil {
			continue
		}
		if !now.Before(row.End.UTC()) {
			row.Status = StatusLifted
			s.events = append(s.events, Event{ID: newID(), SanctionID: id, Type: "expire", Actor: "clock", At: now})
			changed = true
		}
	}
	if !changed {
		return
	}
	snap := s.snapshotLocked(userID, now)
	s.known[userID] = snap
	if snap.restricts {
		return
	}
	// One recovery notice for the latest expired case, not one per retry.
	for i := len(s.byUser[userID]) - 1; i >= 0; i-- {
		row := s.rows[s.byUser[userID][i]]
		if row == nil {
			continue
		}
		s.queueNoticeLocked(row, "T-04", "expire")
		break
	}
}

func (s *Service) snapshotLocked(userID int64, now time.Time) snapshot {
	snap := snapshot{scopes: map[string]bool{}}
	now = now.UTC()
	for _, id := range s.byUser[userID] {
		row := s.rows[id]
		if row == nil || row.Status != StatusActive {
			continue
		}
		if row.End != nil && !row.Permanent && now.After(row.End.UTC()) {
			continue
		}
		snap.restricts = true
		snap.cases = append(snap.cases, row.ID)
		if row.End != nil && (snap.ends == nil || row.End.Before(*snap.ends)) {
			t := row.End.UTC()
			snap.ends = &t
		}
		switch row.Kind {
		case KindPermanent:
			snap.permanent = true
			snap.code = KindPermanent
		case KindReadOnly:
			if !snap.permanent {
				snap.readOnly = true
				snap.code = KindReadOnly
			}
		case KindScoped:
			for _, sc := range row.Scopes {
				snap.scopes[sc] = true
			}
			if snap.code == "" {
				snap.code = KindScoped
			}
		}
	}
	if snap.permanent {
		snap.ends = nil
		snap.code = KindPermanent
	}
	return snap
}

func (s *Service) refreshKnownLocked(userID int64) {
	s.known[userID] = s.snapshotLocked(userID, s.clock())
}

// SubmitAppeal opens or returns the one open appeal for a case.
func (s *Service) SubmitAppeal(userID int64, sanctionID, text string) (Appeal, error) {
	out, err := s.submitAppeal(userID, sanctionID, text)
	s.deliver()
	return out, err
}

func (s *Service) submitAppeal(userID int64, sanctionID, text string) (Appeal, error) {
	s.mu.Lock()
	_ = s.begin()
	row := s.rows[sanctionID]
	if row == nil || row.UserID != userID {
		_ = s.finish(nil)
		s.mu.Unlock()
		return Appeal{}, ErrNotFound
	}
	if id, ok := s.byCase[sanctionID]; ok {
		existing := *s.appeals[id]
		_ = s.finish(nil)
		s.mu.Unlock()
		return existing, nil
	}
	a := &Appeal{
		ID: newID(), SanctionID: sanctionID, UserID: userID,
		Text: text, Status: AppealOpen,
	}
	s.appeals[a.ID] = a
	s.byCase[sanctionID] = a.ID
	s.auditLocked("user", "appeal_submit", sanctionID, "")
	s.queueNoticeLocked(row, "T-06", "appeal_open")
	brief := AppealBrief{
		CaseID: row.ID, UserID: userID, Kind: row.Kind, RuleVersion: row.RuleVersion,
		PolicyClause: row.PolicyClause, ReasonCategory: row.ReasonCategory,
		EvidenceRef: row.EvidenceRef, UserText: text,
	}
	j := s.judge
	_ = s.finish(nil)
	s.mu.Unlock()
	if j == nil {
		s.mu.Lock()
		_ = s.begin()
		if cur := s.appeals[a.ID]; cur != nil {
			cur.Status = AppealPendingReview
			a = cur
		}
		_ = s.finish(nil)
		s.mu.Unlock()
		return *a, nil
	}
	v, err := j.Decide(brief)
	s.mu.Lock()
	_ = s.begin()
	if cur := s.appeals[a.ID]; cur != nil {
		a = cur
	}
	s.applyVerdictLocked(a, v, err)
	_ = s.finish(nil)
	s.mu.Unlock()
	return *a, nil
}

// SubmitAppealWithGrant is the disposition-credential entry.
func (s *Service) SubmitAppealWithGrant(token, sanctionID, text string, now time.Time) (Appeal, error) {
	g, ok := s.UseGrant(token, now)
	if !ok {
		return Appeal{}, ErrRole
	}
	return s.SubmitAppeal(g.UserID, sanctionID, text)
}

// NoticesForGrant returns the user's admin notices without an ordinary session.
func (s *Service) NoticesForGrant(token string, now time.Time) ([]Notice, error) {
	g, ok := s.UseGrant(token, now)
	if !ok {
		return nil, ErrRole
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Notice
	for _, key := range s.noticeOrder {
		n := s.notices[key]
		if n != nil && n.UserID == g.UserID {
			out = append(out, *n)
		}
	}
	return out, nil
}

func (s *Service) applyVerdictLocked(a *Appeal, v Verdict, err error) {
	a.Attempts++
	if err != nil {
		a.Status = AppealPendingReview
		s.auditLocked("tellomi-ai", "appeal_unavailable", a.SanctionID, err.Error())
		return
	}
	v = sanitizeVerdict(v)
	a.UserReason = v.UserReason
	a.RuleRef = v.RuleRef
	a.EvidenceRefs = v.EvidenceRefs
	a.Model = v.Model
	a.PolicyVersion = v.PolicyVersion
	a.Action = v.Action
	row := s.rows[a.SanctionID]
	switch v.Action {
	case ActionLift:
		if row != nil && row.Status == StatusActive {
			row.Status = StatusLifted
			s.events = append(s.events, Event{ID: newID(), SanctionID: row.ID, Type: "ai_lift", Actor: "777001", At: s.clock(), Reason: v.UserReason})
			s.queueNoticeLocked(row, "T-07", "appeal_lift")
			s.refreshKnownLocked(row.UserID)
		}
		a.Status = AppealLifted
	case ActionUphold:
		a.Status = AppealUpheld
		if row != nil {
			s.queueNoticeLocked(row, "T-07", "appeal_uphold")
		}
	default:
		a.Status = AppealPendingReview
		a.Action = ActionInsufficient
	}
	s.auditLocked("777001", "appeal_"+a.Status, a.SanctionID, a.Model)
}

func sanitizeVerdict(v Verdict) Verdict {
	switch v.Action {
	case ActionLift, ActionUphold, ActionInsufficient:
	default:
		v.Action = ActionInsufficient
	}
	v.UserReason = strings.TrimSpace(v.UserReason)
	var refs []string
	for _, ref := range v.EvidenceRefs {
		if strings.Contains(strings.ToLower(ref), "reporter") || strings.Contains(ref, "private_chat") {
			continue
		}
		refs = append(refs, ref)
	}
	v.EvidenceRefs = refs
	return v
}

func (s *Service) queueNoticeLocked(row *Sanction, template, event string) {
	key := row.ID + ":" + event
	if _, ok := s.notices[key]; ok {
		return
	}
	body := render(template, row, s.clock())
	n := &Notice{Key: key, UserID: row.UserID, From: AdminUserID, Template: template, Body: body, Status: "pending"}
	s.notices[key] = n
	s.noticeOrder = append(s.noticeOrder, key)
}

// deliver tries pending notices after the sanction transaction has committed.
// A successful messenger accept followed by a lost status update is retried,
// so delivery is at-least-once, not exactly-once. A failed accept leaves status retry.
func (s *Service) deliver() {
	if s == nil {
		return
	}
	s.mu.Lock()
	sender := s.sender
	var batch []Notice
	if sender != nil {
		for _, key := range s.noticeOrder {
			n := s.notices[key]
			if n != nil && n.Status != "sent" {
				batch = append(batch, *n)
			}
		}
	}
	s.mu.Unlock()
	for _, n := range batch {
		err := sender.SendFromAdmin(n.UserID, n.Key, n.Body)
		s.mu.Lock()
		cur := s.notices[n.Key]
		if cur != nil && cur.Status != "sent" {
			cur.Attempts++
			if err != nil {
				cur.Status = "retry"
				s.auditLocked("777000", "notice_retry", n.Key, "")
			} else {
				cur.Status = "sent"
				s.auditLocked("777000", "notice_sent", n.Key, "")
			}
			_ = s.markNotice(cur)
		}
		s.mu.Unlock()
	}
}

// Flush retries pending notices. A sent key is not sent again.
func (s *Service) Flush() {
	if s.db != nil {
		s.mu.Lock()
		_ = s.begin()
		_ = s.finish(nil)
		s.mu.Unlock()
	}
	s.deliver()
}

func (s *Service) Notices(userID int64) []Notice {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Notice
	for _, key := range s.noticeOrder {
		n := s.notices[key]
		if n != nil && n.UserID == userID {
			out = append(out, *n)
		}
	}
	return out
}

func (s *Service) Audits() []Audit {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Audit(nil), s.audits...)
}

func (s *Service) Sanction(id string) (Sanction, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.rows[id]
	if row == nil {
		return Sanction{}, false
	}
	return *row, true
}

func (s *Service) Appeal(id string) (Appeal, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.appeals[id]
	if a == nil {
		return Appeal{}, false
	}
	return *a, true
}

// RevokeCount is how many times ordinary sessions were revoked for a user.
func (s *Service) RevokeCount(userID int64) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.revoked[userID]
}

func (s *Service) auditLocked(actor, action, caseID, detail string) {
	s.audits = append(s.audits, Audit{At: s.clock(), Actor: actor, Action: action, CaseID: caseID, Detail: detail})
}

// SendCodeShape is identical for unknown, normal, and sanctioned identifiers
// unless identifier reveal is separately enabled. It is off here.
func SendCodeShape(identifier string) string { return "sent" }
