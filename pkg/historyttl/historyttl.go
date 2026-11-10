// Package historyttl is the auto-delete timer for cloud chats.
//
// Official clients store the timer on the dialog (dialogs.ttl_period, or the
// chat/channel row for groups) and stamp that value onto each message as
// ttl_period when it is sent. The countdown starts at the message date, for
// every participant. A later change of the dialog timer does not rewrite
// messages that were already sent. Messages sent while the timer was off
// stay. When date + ttl_period is reached the message is removed from every
// participant's box.
package historyttl

import (
	"encoding/json"
	"math"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Action is one messageActionSetMessagesTTL in a dialog, in send order.
type Action struct {
	Date   int32
	Period int32
}

// Apply writes the dialog timer onto a message that is about to be stored.
// A positive period sets ttl_period. Zero clears it, which is how a message
// sent while the timer is off is kept.
func Apply(msg *mtproto.Message, period int32) {
	if msg == nil {
		return
	}
	if period > 0 {
		msg.TtlPeriod = &wrapperspb.Int32Value{Value: period}
		return
	}
	msg.TtlPeriod = nil
}

// PeriodOf reads ttl_period from a message. A missing flag is 0.
func PeriodOf(msg *mtproto.Message) int32 {
	if msg == nil || msg.TtlPeriod == nil {
		return 0
	}
	return msg.TtlPeriod.Value
}

// Expired reports whether a stored message must be removed at now.
// ttl 0 never expires. A missing send date is left alone.
func Expired(date, ttl, now int32) bool {
	if ttl <= 0 || date <= 0 || now <= 0 {
		return false
	}
	if ttl > math.MaxInt32-date {
		return false
	}
	return now >= date+ttl
}

// PeriodAt is the timer that was in force when a message was sent.
// actions are the set-timer service messages in the same dialog.
// A message dated before every action was sent while no timer applied, so
// the result is 0. The service message itself is included: its date equals
// the action date, and it takes the new period.
func PeriodAt(actions []Action, messageDate int32) int32 {
	var found bool
	var when, period int32
	for _, a := range actions {
		if a.Date <= 0 || a.Date > messageDate {
			continue
		}
		if !found || a.Date >= when {
			found = true
			when = a.Date
			period = a.Period
		}
	}
	if !found || period <= 0 {
		return 0
	}
	return period
}

// ParseAction reads a stored message body. JSON is the user/chat box
// encoding. Not a set-timer service message returns false.
func ParseAction(raw string) (Action, bool) {
	if raw == "" {
		return Action{}, false
	}
	var msg mtproto.Message
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		return Action{}, false
	}
	return actionOf(&msg)
}

func actionOf(msg *mtproto.Message) (Action, bool) {
	if msg == nil || msg.GetAction().GetPredicateName() != mtproto.Predicate_messageActionSetMessagesTTL {
		return Action{}, false
	}
	return Action{Date: msg.GetDate(), Period: msg.GetAction().GetPeriod()}, true
}

// Box is one participant's copy of a message.
type Box struct {
	UserID   int64
	BoxID    int32
	SenderID int64
	PeerType int32
	PeerID   int64
	Date     int32
	TTL      int32
}

// Head is the dialog row the delete has to keep consistent.
type Head struct {
	ReadInbox int32
	Unread    int32
	Top       int32
	Date      int64
}

// UnreadAfter removes incoming boxes that were still unread.
// The reader's own outgoing boxes do not change the badge.
func UnreadAfter(head Head, self int64, deleted []Box) int32 {
	left := head.Unread
	for _, box := range deleted {
		if box.SenderID == self {
			continue
		}
		if box.BoxID > head.ReadInbox {
			left--
		}
	}
	if left < 0 {
		return 0
	}
	return left
}

// Due keeps boxes whose timer has elapsed. Already chosen ids are not repeated.
func Due(boxes []Box, now int32) []Box {
	seen := map[[2]int64]struct{}{}
	var out []Box
	for _, box := range boxes {
		if box.UserID == 0 || box.BoxID == 0 || !Expired(box.Date, box.TTL, now) {
			continue
		}
		key := [2]int64{box.UserID, int64(box.BoxID)}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, box)
	}
	return out
}
