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

func (s *Service) noteJoinBoundary(ctx context.Context, channelID, userID int64, top int32) error {
	if s == nil || channelID == 0 || userID == 0 || top <= 0 {
		return nil
	}
	f, err := s.mutableFeatures(ctx, channelID)
	if err != nil {
		return err
	}
	if f.JoinTop == nil {
		f.JoinTop = map[int64]int32{}
	}
	f.JoinTop[userID] = top
	return s.saveFeatures(ctx, channelID, f)
}

// undoJoin drops a membership whose inbox cursor or join top could not be
// saved. Leaving that row in place would fall back to the date-only cutoff
// and show a same-second message.
func (s *Service) undoJoin(ctx context.Context, channelID, userID int64) {
	if s == nil || s.store == nil || channelID == 0 || userID == 0 {
		return
	}
	p, err := s.store.GetParticipant(ctx, channelID, userID)
	if err != nil || p == nil {
		return
	}
	p.State = StateLeft
	_ = s.store.PutParticipant(ctx, p)
}

// pinnedHidden reports a pin the viewer must not be told about. The id
// stays in storage for members who can see that message.
func (s *Service) pinnedHidden(ctx context.Context, viewer int64, p *Participant, feat *Features, channelID int64) bool {
	if feat == nil || feat.PinnedMsgID <= 0 || s == nil || s.store == nil {
		return false
	}
	stored, err := s.store.GetMessage(ctx, channelID, feat.PinnedMsgID)
	if err != nil || stored == nil {
		return true
	}
	if p == nil && feat.HiddenPrehistory {
		return true
	}
	return prehistoryBlocks(feat, p, stored, nil)
}

// hidesStored is the body gate for search and other readers that walk
// messages from more than one channel. A features or member-row error hides
// the row.
func (s *Service) hidesStored(ctx context.Context, viewer int64, stored *StoredMessage) bool {
	if s == nil || stored == nil {
		return true
	}
	f, err := s.mutableFeatures(ctx, stored.ChannelID)
	if err != nil {
		return true
	}
	p, perr := s.store.GetParticipant(ctx, stored.ChannelID, viewer)
	return prehistoryBlocks(f, p, stored, perr)
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
	// Unknown is a failed member-row read or a failed feature read. Blocks
	// then hides every message. A feature error cannot be treated as
	// "prehistory is off".
	Unknown bool
}

// PrehistoryGate loads the viewer's cutoff. A missing channel does not hide
// anything: searchGlobal still returns the rows the user already has.
// A failed feature read is unknown: the caller must not return the body.
func (s *Service) PrehistoryGate(ctx context.Context, user, channelID int64) PrehistoryGate {
	if s == nil || channelID == 0 {
		return PrehistoryGate{}
	}
	f, err := s.mutableFeatures(ctx, channelID)
	if err != nil {
		return PrehistoryGate{Hidden: true, Unknown: true}
	}
	if f == nil || !f.HiddenPrehistory {
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
