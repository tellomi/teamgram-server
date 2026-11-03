package channel

import (
	"context"

	"github.com/teamgram/proto/mtproto"
)

// PickSuccessor chooses who becomes creator when an account is deleted.
// The earliest remaining administrator wins (smallest join date, then the
// smallest user id). With no administrator, the earliest remaining ordinary
// member wins. Banned and already-left participants are not eligible.
// A false result means the leaving user is the only member.
//
// Telegram will not let a creator leave (channels.leaveChannel returns
// USER_CREATOR) and channels.editCreator demands the cloud password, which
// account deletion cannot prompt for. Transferring first leaves the chat
// with an owner who can still appoint admins. An ownerless channel would
// be stuck: no remaining member can pass the password check.
func PickSuccessor(parts []*Participant, leaving int64) (int64, bool) {
	var adminID, memberID int64
	var adminDate, memberDate int32
	adminSet, memberSet := false, false
	for _, p := range parts {
		if p == nil || p.UserID == 0 || p.UserID == leaving || p.State != StateNormal {
			continue
		}
		switch p.Role {
		case RoleAdmin:
			if !adminSet || p.Date < adminDate || (p.Date == adminDate && p.UserID < adminID) {
				adminSet = true
				adminID = p.UserID
				adminDate = p.Date
			}
		case RoleMember:
			if !memberSet || p.Date < memberDate || (p.Date == memberDate && p.UserID < memberID) {
				memberSet = true
				memberID = p.UserID
				memberDate = p.Date
			}
		}
	}
	if adminSet {
		return adminID, true
	}
	if memberSet {
		return memberID, true
	}
	return 0, false
}

// LeaveDeletedAccount removes userID from every channel and supergroup.
// A creator is transferred with PickSuccessor, then left. The sole member
// deactivates the chat. Messages already in the channel are not deleted, so
// clients still render them with the sender as a deleted account.
// A second call finds nothing left to do.
func (s *Service) LeaveDeletedAccount(ctx context.Context, userID int64) error {
	if s == nil || userID == 0 {
		return nil
	}
	list, err := s.store.ListByUser(ctx, userID)
	if err != nil {
		return err
	}
	for _, ch := range list {
		if ch == nil || ch.Deleted {
			continue
		}
		p, err := s.store.GetParticipant(ctx, ch.ID, userID)
		if err != nil {
			return err
		}
		if p == nil || !p.InChannel() {
			continue
		}
		if p.IsCreator() {
			parts, err := s.store.ListParticipants(ctx, ch.ID)
			if err != nil {
				return err
			}
			to, ok := PickSuccessor(parts, userID)
			if !ok {
				p.State = StateLeft
				if err = s.store.PutParticipant(ctx, p); err != nil {
					return err
				}
				n, err := s.store.CountMembers(ctx, ch.ID)
				if err != nil {
					return err
				}
				ch.ParticipantsCount = n
				ch.Deleted = true
				ch.Version++
				if err = s.store.SaveChannel(ctx, ch); err != nil {
					return err
				}
				continue
			}
			target, err := s.store.GetParticipant(ctx, ch.ID, to)
			if err != nil {
				return err
			}
			if target == nil || !target.InChannel() {
				return mtproto.ErrUserNotParticipant
			}
			p.Role = RoleAdmin
			p.AdminRights = allAdminRights()
			target.Role = RoleCreator
			target.AdminRights = allAdminRights()
			if err = s.store.PutParticipant(ctx, p); err != nil {
				return err
			}
			if err = s.store.PutParticipant(ctx, target); err != nil {
				return err
			}
			ch.CreatorID = to
			ch.Version++
			if err = s.store.SaveChannel(ctx, ch); err != nil {
				return err
			}
		}
		in := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
			ChannelId:  ch.ID,
			AccessHash: ch.AccessHash,
		}).To_InputChannel()
		if _, err = s.Leave(ctx, userID, 0, in); err != nil {
			return err
		}
	}
	return nil
}
