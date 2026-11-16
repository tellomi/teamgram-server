package channel

import "context"

// JoinTop is the channel message id that already existed when this member
// joined. It is recorded on the three join paths. A missing entry is a
// member from before this field existed: those rows keep the older
// date-only cutoff. Zero is not stored, so an empty channel stays on that
// older cutoff, which has nothing to hide.
func joinTop(f *Features, userID int64) int32 {
	if f == nil || f.JoinTop == nil || userID == 0 {
		return 0
	}
	return f.JoinTop[userID]
}

func (s *Service) noteJoinBoundary(ctx context.Context, channelID, userID int64, top int32) {
	if s == nil || channelID == 0 || userID == 0 || top <= 0 {
		return
	}
	f := s.features(ctx, channelID)
	if f.JoinTop == nil {
		f.JoinTop = map[int64]int32{}
	}
	f.JoinTop[userID] = top
	_ = s.saveFeatures(ctx, channelID, f)
}

// messageBeforeJoin reports a message that existed at join. A recorded
// join top includes a message from the same unix second when its id is not
// past that top. The join service message and anything sent after it have
// a higher id, so they stay visible. Without a recorded top, only an
// earlier second is before join.
func messageBeforeJoin(p *Participant, stored *StoredMessage, top int32) bool {
	if p == nil || stored == nil {
		return false
	}
	if top > 0 && stored.ID > 0 {
		return stored.ID <= top
	}
	return stored.Date < p.Date
}

// prehistoryBlocks hides messages posted before this member joined when the
// channel has hidden prehistory. Admins and the creator may still read
// those messages. loadErr means the member row could not be read: that
// fails closed only while prehistory is hidden, so a lookup error does not
// return the body. A nil viewer with no error is someone who is not a
// member; that case is unchanged.
func prehistoryBlocks(f *Features, viewer *Participant, stored *StoredMessage, loadErr error) bool {
	if f == nil || !f.HiddenPrehistory || stored == nil {
		return false
	}
	if loadErr != nil {
		return true
	}
	if viewer == nil || viewer.IsAdmin() {
		return false
	}
	return messageBeforeJoin(viewer, stored, joinTop(f, viewer.UserID))
}

// PrehistoryGate is one viewer's join cutoff for messages.searchGlobal.
// Hidden is false when the channel does not hide prehistory.
type PrehistoryGate struct {
	Hidden  bool
	Admin   bool
	Joined  int32
	JoinTop int32
	// Unknown is a failed member-row read. Blocks then hides every message
	// while prehistory is on.
	Unknown bool
}

// PrehistoryGate loads the viewer's cutoff. A missing channel does not hide
// anything: searchGlobal still returns the rows the user already has.
func (s *Service) PrehistoryGate(ctx context.Context, user, channelID int64) PrehistoryGate {
	if s == nil || channelID == 0 {
		return PrehistoryGate{}
	}
	f := s.features(ctx, channelID)
	if !f.HiddenPrehistory {
		return PrehistoryGate{}
	}
	viewer, err := s.store.GetParticipant(ctx, channelID, user)
	if err != nil {
		return PrehistoryGate{Hidden: true, Unknown: true}
	}
	if viewer == nil {
		return PrehistoryGate{Hidden: true}
	}
	return PrehistoryGate{Hidden: true, Admin: viewer.IsAdmin(), Joined: viewer.Date, JoinTop: joinTop(f, user)}
}

// Blocks reports whether this message date is before the viewer joined.
// It has no message id, so a recorded join top cannot separate two messages
// in the same second. Callers that have an id use BlocksMessage.
func (g PrehistoryGate) Blocks(date int32) bool {
	return g.BlocksMessage(date, 0)
}

// BlocksMessage applies the same cutoff as getHistory. id 0 keeps the
// date-only rule. A failed member read hides the message.
func (g PrehistoryGate) BlocksMessage(date, id int32) bool {
	if !g.Hidden || g.Admin {
		return false
	}
	if g.Unknown {
		return true
	}
	if g.JoinTop > 0 && id > 0 {
		return id <= g.JoinTop
	}
	return date < g.Joined
}
