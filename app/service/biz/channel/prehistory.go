package channel

import (
	"context"

	"github.com/teamgram/proto/mtproto"
)

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
	// saveFeatures merges this key onto the row stored at write time.
	// Two invites therefore keep both users.
	return s.saveFeatures(ctx, channelID, f)
}

// repairJoinBoundary fills a missing join top for a member who is already
// in the channel. It uses the inbox cursor only while every live message
// that cursor covers is still in the member's join second. That cursor is
// the top recorded at join and not yet moved.
//
// Once the cursor covers a later second, the original join id is gone.
// member.Date only has second resolution, and the join service message is
// not a reliable top: messages can commit after the boundary is stored and
// before that service message gets its id. Inventing a top from either one
// can hide a message sent after the member joined. This function leaves
// that member on the date cutoff. A same-second message from before the
// real join can still be visible. Recovering it needs the top id captured
// at join, which new joins store in JoinTop and old rows do not have.
func (s *Service) repairJoinBoundary(ctx context.Context, ch *Channel, member *Participant) error {
	if s == nil || s.store == nil || ch == nil || member == nil || !member.InChannel() {
		return nil
	}
	f, err := s.mutableFeatures(ctx, ch.ID)
	if err != nil {
		return err
	}
	if f == nil || !f.HiddenPrehistory || joinTop(f, member.UserID) > 0 {
		return nil
	}
	st, err := s.store.GetRead(ctx, ch.ID, member.UserID)
	if err != nil {
		return err
	}
	inbox := int32(0)
	if st != nil {
		inbox = st.ReadInboxMaxID
	}
	msgs, err := s.store.ListMessages(ctx, ch.ID)
	if err != nil {
		return err
	}
	top, ok := joinBoundaryCandidate(member, inbox, msgs)
	if !ok {
		return nil
	}
	if f.JoinTop == nil {
		f.JoinTop = map[int64]int32{}
	}
	f.JoinTop[member.UserID] = top
	return s.saveFeatures(ctx, ch.ID, f)
}

// joinBoundaryCandidate is the inbox cursor when every live message it
// covers is still in the member's join second. A later second means the
// cursor is not a join snapshot.
func joinBoundaryCandidate(member *Participant, inbox int32, msgs []*StoredMessage) (int32, bool) {
	if member == nil || !member.InChannel() || inbox <= 0 {
		return 0, false
	}
	for _, m := range msgs {
		if m == nil || m.Deleted {
			continue
		}
		if m.ID <= inbox && m.Date > member.Date {
			return 0, false
		}
	}
	return inbox, true
}

// commitJoin grants StateNormal only after the inbox cursor and the join
// boundary are stored. A failed read or write returns before that grant, so
// an undo that itself fails cannot leave a normal member on the date-only
// cutoff. A nil participant is not a member. That is not a read error.
// keepOutbox preserves an outbox cursor the user already earned. Join and
// import start a new inbox row instead, which is what those RPCs stored.
func (s *Service) commitJoin(ctx context.Context, ch *Channel, np *Participant, keepOutbox bool) error {
	if s == nil || s.store == nil || ch == nil || np == nil || ch.ID == 0 || np.UserID == 0 {
		return mtproto.ErrInternalServerError
	}
	cur, err := s.store.GetParticipant(ctx, ch.ID, np.UserID)
	if err != nil {
		return err
	}
	if cur != nil && cur.State == StateKicked {
		return mtproto.ErrUserBannedInChannel
	}
	if cur != nil && cur.InChannel() {
		return s.repairJoinBoundary(ctx, ch, cur)
	}
	st, err := s.store.GetRead(ctx, ch.ID, np.UserID)
	if err != nil {
		return err
	}
	if keepOutbox && st != nil {
		st.ReadInboxMaxID = ch.TopMessage
	} else {
		st = &ReadState{ChannelID: ch.ID, UserID: np.UserID, ReadInboxMaxID: ch.TopMessage}
	}
	if st.ChannelID == 0 {
		st.ChannelID = ch.ID
	}
	st.UserID = np.UserID
	if err = s.store.SaveRead(ctx, st); err != nil {
		return err
	}
	if err = s.noteJoinBoundary(ctx, ch.ID, np.UserID, ch.TopMessage); err != nil {
		return err
	}
	np.ChannelID = ch.ID
	np.State = StateNormal
	return s.store.PutParticipant(ctx, np)
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
