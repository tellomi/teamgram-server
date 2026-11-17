package channel

import (
	"context"
	"errors"
	"time"

	bizchat "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
)

// pickActivePermanent is the admin's current primary link. The comparison
// matches PrimaryExported: a later row with the same date replaces the earlier
// one. Revoked rows and additional links are not primaries.
func pickActivePermanent(rows []*StoredInvite, adminID int64) *StoredInvite {
	var best *StoredInvite
	for _, row := range rows {
		if row == nil {
			continue
		}
		if row.AdminID == adminID && row.Permanent && !row.Revoked && (best == nil || row.Date >= best.Date) {
			best = row
		}
	}
	return best
}

func (s *Service) insertStored(ctx context.Context, db inviteLocker, inv *StoredInvite) (*StoredInvite, error) {
	var err error
	for i := 0; i < 5; i++ {
		inv.Link = bizchat.GenChannelInviteHash()
		err = db.InsertInvite(ctx, inv)
		if err == nil {
			return inv, nil
		}
		if !errors.Is(err, errInviteExists) {
			return nil, err
		}
	}
	return nil, err
}

// ensureOwnPrimary returns this admin's active permanent invite. An existing
// row is not rewritten. A missing one is inserted once, under the channel
// invite lock, so a concurrent read or revoke cannot insert a second primary.
// The fast path does not take the lock. Callers must already know this admin
// is allowed to invite.
func (s *Service) ensureOwnPrimary(ctx context.Context, channelID, adminID int64) (*StoredInvite, error) {
	rows, err := s.store.ListInvites(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if cur := pickActivePermanent(rows, adminID); cur != nil {
		return cur, nil
	}
	var saved *StoredInvite
	err = s.store.WithInviteLock(ctx, channelID, func(ctx context.Context, tx inviteLocker) error {
		locked, err := tx.ListInvites(ctx, channelID)
		if err != nil {
			return err
		}
		if cur := pickActivePermanent(locked, adminID); cur != nil {
			saved = cur
			return nil
		}
		saved, err = s.insertStored(ctx, tx, &StoredInvite{
			ChannelID: channelID,
			AdminID:   adminID,
			Permanent: true,
			Date:      time.Now().Unix(),
		})
		return err
	})
	return saved, err
}
