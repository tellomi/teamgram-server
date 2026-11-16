package profile

import (
	"fmt"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// PeerFacts is what the sender of an incoming private message looks like
// to the business account that may answer automatically.
//
// PriorMessage is the unix time of the newest private message already in
// the dialog before the one that just arrived. HistoryKnown is false when
// that lookup failed. A missing ledger row is not a new chat.
// ContactKnown and PresenceKnown stay false when those lookups fail. An
// unknown contact is not a non-contact, and unknown presence is not offline.
type PeerFacts struct {
	Contact       bool
	ContactKnown  bool
	Online        bool
	PresenceKnown bool
	LastSeen      int32
	Now           time.Time
	PriorMessage  int32
	HistoryKnown  bool
}

// ConsumeAutoReply decides which quick-reply shortcuts to send and records
// that they were sent. Greeting goes out only when dialog history says the
// chat is new (no_activity_days == 0) or the previous message is at least
// no_activity_days old. Away goes out once per away window. The caller
// delivers the shortcuts.
func (b *Book) ConsumeAutoReply(owner, peer int64, facts PeerFacts) []int32 {
	if b == nil || owner == 0 || peer == 0 || owner == peer {
		return nil
	}
	var out []int32
	// Save the ledger before returning shortcut ids. A failed write returns
	// nothing, so the caller does not send a reply the book will not remember.
	err := b.update(owner, func(*[]string) error {
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
		newChat := facts.HistoryKnown && facts.PriorMessage == 0
		if g := r.greeting; g != nil && matchRecipients(g.GetRecipients(), peer, facts.Contact, facts.ContactKnown, newChat) && greetingDue(g, facts, now) && (note.GreetingSent == 0 || facts.PriorMessage > note.GreetedPrior) {
			if g.GetShortcutId() > 0 {
				out = append(out, g.GetShortcutId())
				note.GreetingSent = now
				note.GreetedPrior = facts.PriorMessage
			}
		}
		if a := r.away; a != nil && matchRecipients(a.GetRecipients(), peer, facts.Contact, facts.ContactKnown, newChat) {
			if active, window := awayWindow(a, r.hours, r.awayVersion, facts); active && a.GetShortcutId() > 0 {
				resend := note.AwayWindow != window || note.AwaySent == 0
				if !resend && a.GetOfflineOnly() && facts.PresenceKnown && facts.LastSeen > note.AwaySent {
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
		return nil
	})
	if err != nil {
		return nil
	}
	return out
}

func greetingDue(g *mtproto.BusinessGreetingMessage, facts PeerFacts, now int32) bool {
	if g == nil || !facts.HistoryKnown {
		return false
	}
	// no_activity_days == 0 is only for a dialog with no earlier message.
	if facts.PriorMessage == 0 {
		return true
	}
	days := g.GetNoActivityDays()
	if days <= 0 {
		return false
	}
	return int64(now)-int64(facts.PriorMessage) >= int64(days)*86400
}

func awayWindow(a *mtproto.BusinessAwayMessage, hours *mtproto.BusinessWorkHours, version int32, facts PeerFacts) (bool, string) {
	if a == nil {
		return false, ""
	}
	// Unknown presence is not offline. offline_only stays quiet until last seen is known.
	if a.GetOfflineOnly() && (!facts.PresenceKnown || facts.Online) {
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

func matchRecipients(rec *mtproto.BusinessRecipients, peer int64, contact, contactKnown, newChat bool) bool {
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
	if rec.GetContacts() && contactKnown && contact {
		return true
	}
	if rec.GetNonContacts() && contactKnown && !contact {
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
