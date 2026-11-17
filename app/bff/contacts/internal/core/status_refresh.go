package core

import (
	"errors"
	"time"

	"github.com/teamgram/proto/mtproto"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// contactStatusRefresh is what each side may see after one user adds the
// other. forPeer is this user's status as the new contact is allowed to see
// it. forMe is the contact's status as this user is allowed to see it,
// including the rule that hiding your own last seen also hides theirs.
func contactStatusRefresh(me, peer *mtproto.ImmutableUser) (forPeer, forMe *mtproto.UserStatus) {
	if me == nil || peer == nil || me.Id() == 0 || peer.Id() == 0 || me.Id() == peer.Id() {
		return nil, nil
	}
	if viewed := me.ToUnsafeUser(peer); viewed != nil {
		forPeer = viewed.Status
	}
	if viewed := peer.ToUnsafeUser(me); viewed != nil {
		forMe = viewed.Status
	}
	return forPeer, forMe
}

func setForwardContacts(user *mtproto.ImmutableUser, ids []int64) {
	if user == nil {
		return
	}
	rows := make([]*mtproto.ContactData, 0, len(ids))
	for _, id := range ids {
		if id == 0 || id == user.Id() {
			continue
		}
		rows = append(rows, mtproto.MakeTLContactData(&mtproto.ContactData{
			UserId:        user.Id(),
			ContactUserId: id,
		}).To_ContactData())
	}
	user.Contacts = rows
}

func userStatusPush(userID int64, status *mtproto.UserStatus) *mtproto.Updates {
	if status == nil {
		return nil
	}
	return mtproto.MakeTLUpdateShort(&mtproto.Updates{
		Update: mtproto.MakeTLUpdateUserStatus(&mtproto.Update{
			UserId:            userID,
			Status_USERSTATUS: status,
		}).To_Update(),
		Date: int32(time.Now().Unix()),
	}).To_Updates()
}

// pushContactStatuses delivers the two current statuses once a contact edge
// exists. Both devices of the adder and the other user receive the view the
// privacy rules allow, without waiting for the next account.updateStatus.
func (c *ContactsCore) pushContactStatuses(selfID, peerID int64) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.SyncClient == nil || selfID == 0 || peerID == 0 || selfID == peerID {
		return
	}
	blocked, err := c.blockedEither(selfID, peerID)
	if err != nil || blocked {
		if err != nil {
			c.Logger.Errorf("contacts status refresh - block check: %v", err)
		}
		return
	}
	got, err := c.svcCtx.Dao.UserGetMutableUsersV2(c.ctx, &userpb.TLUserGetMutableUsersV2{
		Id:      []int64{selfID, peerID},
		Privacy: true,
		HasTo:   true,
		To:      []int64{selfID, peerID},
	})
	if err != nil || got == nil {
		c.Logger.Errorf("contacts status refresh - load users: %v", err)
		return
	}
	me, okMe := got.GetImmutableUser(selfID)
	peer, okPeer := got.GetImmutableUser(peerID)
	if !okMe || !okPeer {
		return
	}
	forPeer, forMe := contactStatusRefresh(me, peer)
	if up := userStatusPush(selfID, forPeer); up != nil {
		_, _ = c.svcCtx.SyncClient.SyncPushUpdates(c.ctx, &syncpb.TLSyncPushUpdates{UserId: peerID, Updates: up})
	}
	if up := userStatusPush(peerID, forMe); up != nil {
		_, _ = c.svcCtx.SyncClient.SyncPushUpdates(c.ctx, &syncpb.TLSyncPushUpdates{UserId: selfID, Updates: up})
	}
}

var errStatusBlockCheckUnavailable = errors.New("status block check unavailable")

func interpretBlockChecks(err1 error, blocked1 bool, err2 error, blocked2 bool) (bool, error) {
	if err1 != nil {
		return true, err1
	}
	if err2 != nil {
		return true, err2
	}
	return blocked1 || blocked2, nil
}

func (c *ContactsCore) blockedEither(selfID, peerID int64) (bool, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil {
		return true, errStatusBlockCheckUnavailable
	}
	one, err1 := c.svcCtx.Dao.UserBlockedByUser(c.ctx, &userpb.TLUserBlockedByUser{UserId: selfID, PeerUserId: peerID})
	two, err2 := c.svcCtx.Dao.UserBlockedByUser(c.ctx, &userpb.TLUserBlockedByUser{UserId: peerID, PeerUserId: selfID})
	return interpretBlockChecks(err1, mtproto.FromBool(one), err2, mtproto.FromBool(two))
}
