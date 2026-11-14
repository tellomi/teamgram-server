package profile

import (
	"fmt"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// PeerFacts is what the sender of an incoming private message looks like
// to the business account that may answer automatically.
type PeerFacts struct {
	Contact  bool
	Online   bool
	LastSeen int32
	Now      time.Time
}

// ConsumeAutoReply decides which quick-reply shortcuts to send and records
// that they were sent. Greeting goes out when the chat was idle for
// no_activity_days (a new chat always qualifies). Away goes out once per
// away window. The caller delivers the shortcuts.
func (b *Book) ConsumeAutoReply(owner, peer int64, facts PeerFacts) []int32 {
	if b == nil || owner == 0 || peer == 0 || owner == peer {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.reload(owner); err != nil {
		return nil
	}
	r := b.users[owner]
	if r == nil || (r.greeting == nil && r.away == nil) {
		return nil
	}
	if r.peers == nil {
		r.peers = map[int64]*peerNote{}
	}
	note := r.peers[peer]
	if note == nil {
		note = &peerNote{}
		r.peers[peer] = note
	}
	now := int32(facts.Now.Unix())
	if facts.Now.IsZero() {
		now = int32(b.now().Unix())
	}
	newChat := note.LastActivity == 0
	var out []int32
	if g := r.greeting; g != nil && matchRecipients(g.GetRecipients(), peer, facts.Contact, newChat) && greetingDue(g, note.LastActivity, now) {
		if g.GetShortcutId() > 0 {
			out = append(out, g.GetShortcutId())
			note.GreetingSent = now
		}
	}
	if a := r.away; a != nil && matchRecipients(a.GetRecipients(), peer, facts.Contact, newChat) {
		if active, window := awayWindow(a, r.hours, r.awayVersion, facts); active && a.GetShortcutId() > 0 {
			resend := note.AwayWindow != window || note.AwaySent == 0
			if !resend && a.GetOfflineOnly() && facts.LastSeen > note.AwaySent {
				resend = true
			}
			if resend {
				out = append(out, a.GetShortcutId())
				note.AwaySent = now
				note.AwayWindow = window
			}
		}
	}
	note.LastActivity = now
	_ = b.save(owner)
	return out
}

func greetingDue(g *mtproto.BusinessGreetingMessage, last, now int32) bool {
	if g == nil {
		return false
	}
	if last == 0 {
		return true
	}
	days := g.GetNoActivityDays()
	if days <= 0 {
		return false
	}
	return int64(now)-int64(last) >= int64(days)*86400
}

func awayWindow(a *mtproto.BusinessAwayMessage, hours *mtproto.BusinessWorkHours, version int32, facts PeerFacts) (bool, string) {
	if a == nil {
		return false, ""
	}
	if a.GetOfflineOnly() && facts.Online {
		return false, ""
	}
	now := facts.Now
	if now.IsZero() {
		now = time.Now()
	}
	switch a.GetSchedule().GetPredicateName() {
	case mtproto.Predicate_businessAwayMessageScheduleAlways:
		return true, fmt.Sprintf("always:%d", version)
	case mtproto.Predicate_businessAwayMessageScheduleOutsideWorkHours:
		if hours == nil || openNow(hours.GetTimezoneId(), hours.GetWeeklyOpen(), now) {
			return false, ""
		}
		return true, fmt.Sprintf("out:%d:%d", version, closedSince(hours, now))
	case mtproto.Predicate_businessAwayMessageScheduleCustom:
		start, end := a.GetSchedule().GetStartDate(), a.GetSchedule().GetEndDate()
		unix := int32(now.Unix())
		if start <= 0 || unix < start || unix >= end {
			return false, ""
		}
		return true, fmt.Sprintf("custom:%d:%d:%d", version, start, end)
	default:
		return false, ""
	}
}

func closedSince(hours *mtproto.BusinessWorkHours, now time.Time) int32 {
	if hours == nil {
		return int32(now.Unix())
	}
	t := now.Truncate(time.Minute)
	for i := 0; i < 8*24*60; i++ {
		prev := t.Add(-time.Minute)
		if openNow(hours.GetTimezoneId(), hours.GetWeeklyOpen(), prev) {
			return int32(t.Unix())
		}
		t = prev
	}
	return int32(now.Unix())
}

func matchRecipients(rec *mtproto.BusinessRecipients, peer int64, contact, newChat bool) bool {
	if rec == nil {
		return false
	}
	selected := false
	for _, id := range rec.GetUsers() {
		if id == peer {
			selected = true
			break
		}
	}
	if rec.GetExcludeSelected() && selected {
		return false
	}
	if !rec.GetExcludeSelected() && selected {
		return true
	}
	if rec.GetContacts() && contact {
		return true
	}
	if rec.GetNonContacts() && !contact {
		return true
	}
	if rec.GetExistingChats() && !newChat {
		return true
	}
	if rec.GetNewChats() && newChat {
		return true
	}
	if rec.GetExcludeSelected() && !rec.GetContacts() && !rec.GetNonContacts() && !rec.GetExistingChats() && !rec.GetNewChats() {
		return true
	}
	return false
}
