package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// profileNameUpdates is what account.updateProfile pushes after a name change.
// updateUserName carries the new name. updateUser tells clients to replace
// the cached user. The users vector is the same name, without the self flag,
// so a contact session can apply it.
func profileNameUpdates(userID int64, first, last, username string) *mtproto.Updates {
	user := mtproto.MakeTLUser(&mtproto.User{
		Id:        userID,
		FirstName: mtproto.MakeFlagsString(first),
		LastName:  mtproto.MakeFlagsString(last),
		Username:  mtproto.MakeFlagsString(username),
	}).To_User()
	return mtproto.MakeUpdatesByUpdatesUsers(
		[]*mtproto.User{user},
		mtproto.MakeTLUpdateUserName(&mtproto.Update{
			UserId:    userID,
			FirstName: first,
			LastName:  last,
			Username:  username,
		}).To_Update(),
		mtproto.MakeTLUpdateUser(&mtproto.Update{UserId: userID}).To_Update(),
	)
}

// profileNamePeers is who else should see the new name: people this user
// added and people who added this user. The account itself is pushed separately.
func profileNamePeers(self int64, forward, reverse []int64) []int64 {
	seen := map[int64]struct{}{}
	var out []int64
	for _, id := range append(append([]int64{}, forward...), reverse...) {
		if id == 0 || id == self {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func (c *UserChannelProfilesCore) pushProfileName(me *mtproto.ImmutableUser) {
	if c == nil || c.MD == nil || me == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.SyncClient == nil {
		return
	}
	updates := profileNameUpdates(c.MD.UserId, me.FirstName(), me.LastName(), me.Username())
	_, _ = c.svcCtx.Dao.SyncClient.SyncUpdatesNotMe(c.ctx, &sync.TLSyncUpdatesNotMe{
		UserId:        c.MD.UserId,
		PermAuthKeyId: c.MD.PermAuthKeyId,
		Updates:       updates,
	})
	// SyncPushUpdates reaches every session of this account, including one
	// that updatesNotMe skips, and wakes a background session.
	_, _ = c.svcCtx.Dao.SyncClient.SyncPushUpdates(c.ctx, &sync.TLSyncPushUpdates{
		UserId:  c.MD.UserId,
		Updates: updates,
	})
	for _, id := range c.profileNameAudience() {
		_, _ = c.svcCtx.Dao.SyncClient.SyncPushUpdates(c.ctx, &sync.TLSyncPushUpdates{
			UserId:  id,
			Updates: updates,
		})
	}
}

func (c *UserChannelProfilesCore) profileNameAudience() []int64 {
	if c.svcCtx.Dao.UserClient == nil {
		return nil
	}
	var forward, reverse []int64
	if ids, err := c.svcCtx.Dao.UserClient.UserGetContactIdList(c.ctx, &userpb.TLUserGetContactIdList{UserId: c.MD.UserId}); err == nil && ids != nil {
		forward = ids.GetDatas()
	}
	if ids, err := c.svcCtx.Dao.UserClient.UserGetReverseContactIdList(c.ctx, &userpb.TLUserGetContactIdList{UserId: c.MD.UserId}); err == nil && ids != nil {
		reverse = ids.GetDatas()
	}
	return profileNamePeers(c.MD.UserId, forward, reverse)
}
