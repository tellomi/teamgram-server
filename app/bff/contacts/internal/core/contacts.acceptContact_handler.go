// Copyright 2022 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/statusbook"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// ContactsAcceptContact
// contacts.acceptContact#f831a20f id:InputUser = Updates;
// https://core.telegram.org/method/contacts.acceptContact
//
// If the add-contact action bar is active (peerSettings.add_contact), add that
// user as a contact and clear the bar. The returned Updates carry the user as
// the caller now sees them.
func (c *ContactsCore) ContactsAcceptContact(in *mtproto.TLContactsAcceptContact) (*mtproto.Updates, error) {
	if err := c.requireSelf(); err != nil {
		return nil, err
	}
	id, err := userPeer(c.MD.UserId, in.GetId())
	if err != nil {
		c.Logger.Errorf("contacts.acceptContact - error: %v", err)
		return nil, err
	}
	settings, err := c.svcCtx.Dao.UserGetPeerSettings(c.ctx, &userpb.TLUserGetPeerSettings{
		UserId:   c.MD.UserId,
		PeerType: mtproto.PEER_USER,
		PeerId:   id.PeerId,
	})
	if err != nil || settings == nil || !settings.GetAddContact() {
		c.Logger.Errorf("contacts.acceptContact - add_contact is not set")
		return nil, mtproto.ErrContactAddMissing
	}
	user, err := c.loadUser(c.MD.UserId, id.PeerId)
	if err != nil {
		c.Logger.Errorf("contacts.acceptContact - error: %v", err)
		return nil, mtproto.ErrContactIdInvalid
	}
	// acceptContact shares the caller's phone with the accepted user, the same
	// exception contacts.addContact sets with add_phone_privacy_exception.
	mutual, err := c.svcCtx.Dao.UserAddContact(c.ctx, &userpb.TLUserAddContact{
		UserId:                   c.MD.UserId,
		AddPhonePrivacyException: mtproto.ToBool(true),
		Id:                       id.PeerId,
		FirstName:                flagString(user.GetFirstName()),
		LastName:                 flagString(user.GetLastName()),
		Phone:                    flagString(user.GetPhone()),
	})
	if err != nil {
		c.Logger.Errorf("contacts.acceptContact - error: %v", err)
		return nil, mtproto.ErrContactIdInvalid
	}
	// user.addContact already deleted the stranger bar. Do not write the old
	// report_spam and block_contact flags back.
	cleared := mtproto.MakeTLPeerSettings(&mtproto.PeerSettings{}).To_PeerSettings()

	user.Contact = true
	user.MutualContact = mtproto.FromBool(mutual)
	if err := statusbook.Shared().Forget(c.ctx, c.MD.UserId, id.PeerId); err != nil {
		c.Logger.Errorf("contacts.acceptContact forget audience: %v", err)
		return nil, err
	}
	c.pushAcceptedPhone(id.PeerId)
	me, meErr := c.loadUser(c.MD.UserId, c.MD.UserId)
	users := []*mtproto.User{user}
	if meErr == nil && me != nil {
		users = []*mtproto.User{me, user}
	}
	return mtproto.MakeUpdatesByUpdatesUsers(users, mtproto.MakeTLUpdatePeerSettings(&mtproto.Update{
		Peer_PEER: id.ToPeer(),
		Settings:  cleared,
	}).To_Update()), nil
}

// pushAcceptedPhone tells the accepted user that this phone is now visible,
// and the caller's other sessions that the phone-number rule changed.
func (c *ContactsCore) pushAcceptedPhone(peerID int64) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.SyncClient == nil || c.MD == nil || peerID == 0 {
		return
	}
	users, err := c.svcCtx.Dao.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
		Id: []int64{c.MD.UserId, peerID},
		To: []int64{peerID},
	})
	if err != nil || users == nil {
		c.Logger.Errorf("contacts.acceptContact - push users: %v", err)
		return
	}
	_, _ = c.svcCtx.SyncClient.SyncPushUpdates(c.ctx, &sync.TLSyncPushUpdates{
		UserId: peerID,
		Updates: mtproto.MakeUpdatesByUpdatesUsers(
			users.GetUserListByIdList(peerID, c.MD.UserId),
			mtproto.MakeTLUpdatePeerSettings(&mtproto.Update{
				Peer_PEER: mtproto.MakePeerUser(c.MD.UserId),
				Settings:  mtproto.MakeTLPeerSettings(&mtproto.PeerSettings{}).To_PeerSettings(),
			}).To_Update(),
		),
	})
	rules, err := c.svcCtx.Dao.UserGetPrivacy(c.ctx, &userpb.TLUserGetPrivacy{
		UserId:  c.MD.UserId,
		KeyType: mtproto.PHONE_NUMBER,
	})
	if err != nil || rules == nil {
		c.Logger.Errorf("contacts.acceptContact - privacy rules: %v", err)
		return
	}
	_, _ = c.svcCtx.SyncClient.SyncUpdatesNotMe(c.ctx, &sync.TLSyncUpdatesNotMe{
		UserId:        c.MD.UserId,
		PermAuthKeyId: c.MD.PermAuthKeyId,
		Updates: mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdatePrivacy(&mtproto.Update{
			Key:   mtproto.ToPrivacyKey(mtproto.PHONE_NUMBER),
			Rules: rules.GetDatas(),
		}).To_Update()),
	})
}
