package ttlmedia

import "github.com/teamgram/proto/mtproto"

// Class is what a one-off scan should do with one dialog.
type Class int

const (
	// ClassSkipNoSignal is ttl media with no record that anyone opened it.
	// Historical photos and videos never had media_unread, so an unopened
	// copy and an opened copy look the same. Those stay.
	ClassSkipNoSignal Class = iota
	// ClassSkipUnopened is media_unread still set: the recipient has not
	// called readMessageContents.
	ClassSkipUnopened
	// ClassSkipNotDue is opened timed media whose countdown has not elapsed,
	// or a row that is already stripped.
	ClassSkipNotDue
	// ClassExpire is safe to rewrite to the expired form.
	ClassExpire
)

// OpenRow is one message_ttl_open row. Nil means the dialog was never
// recorded by this process.
type OpenRow struct {
	OpenedAt int32
	Stripped bool
}

// Classify decides a one-off action for one message copy.
//
// columnUnread is messages.media_unread. jsonUnread is the flag stored in
// message_data at send time. Read clears the column and leaves the JSON
// flag, which is how an already-opened voice (and, after this change, any
// ttl media) is distinguished from one that was never flagged.
//
// voiceRound opened before this change received updateReadMessagesContents
// without a date, so the official client blanked it immediately. Those
// copies are expired. Timed photos and videos from before this change have
// no open bit and are not expired here.
func Classify(ttl int32, columnUnread, jsonUnread, voiceRound bool, open *OpenRow, now int32) Class {
	if ttl <= 0 {
		return ClassSkipNoSignal
	}
	if open != nil && open.OpenedAt > 0 {
		if open.Stripped || !mtproto.TTLDue(ttl, open.OpenedAt, now) {
			return ClassSkipNotDue
		}
		return ClassExpire
	}
	if columnUnread {
		return ClassSkipUnopened
	}
	if !jsonUnread {
		return ClassSkipNoSignal
	}
	if mtproto.IsViewOnce(ttl) || voiceRound {
		return ClassExpire
	}
	return ClassSkipNotDue
}
