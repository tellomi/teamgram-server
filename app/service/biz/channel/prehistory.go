package channel

import "context"

// prehistoryBlocks hides messages posted before this member joined when the
// channel has hidden prehistory. Admins and the creator keep the old history.
func prehistoryBlocks(f *Features, viewer *Participant, stored *StoredMessage) bool {
	if f == nil || !f.HiddenPrehistory || stored == nil {
		return false
	}
	if viewer == nil || viewer.IsAdmin() {
		return false
	}
	return stored.Date < viewer.Date
}

// PrehistoryGate is one viewer's join cutoff for messages.searchGlobal.
// Hidden is false when the channel does not hide prehistory.
type PrehistoryGate struct {
	Hidden bool
	Admin  bool
	Joined int32
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
	if err != nil || viewer == nil {
		return PrehistoryGate{Hidden: true}
	}
	return PrehistoryGate{Hidden: true, Admin: viewer.IsAdmin(), Joined: viewer.Date}
}

// Blocks reports whether this message date is before the viewer joined.
func (g PrehistoryGate) Blocks(date int32) bool {
	if !g.Hidden || g.Admin {
		return false
	}
	return date < g.Joined
}
