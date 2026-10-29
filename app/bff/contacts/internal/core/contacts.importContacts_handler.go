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
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// ContactsImportContacts
// contacts.importContacts#2c800be5 contacts:Vector<InputContact> = contacts.ImportedContacts;
func (c *ContactsCore) ContactsImportContacts(in *mtproto.TLContactsImportContacts) (*mtproto.Contacts_ImportedContacts, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		c.Logger.Errorf("contacts.importContacts - missing user")
		return nil, mtproto.ErrUserIdInvalid
	}
	if len(in.GetContacts()) == 0 {
		return emptyImportedContacts(), nil
	}

	imported, err := c.svcCtx.Dao.UserClient.UserImportContacts(c.ctx, &userpb.TLUserImportContacts{
		UserId:   c.MD.UserId,
		Contacts: in.Contacts,
	})
	if err != nil {
		c.Logger.Errorf("contacts.importContacts - error: %v", err)
		return nil, err
	}
	if imported == nil {
		return emptyImportedContacts(), nil
	}

	c.pushImportContactUpdates(imported.GetUpdateIdList())

	users := imported.GetUsers()
	if users == nil {
		users = []*mtproto.User{}
	}
	popular := imported.GetPopularInvites()
	if popular == nil {
		popular = []*mtproto.PopularContact{}
	}
	retry := imported.GetRetryContacts()
	if retry == nil {
		retry = []int64{}
	}
	rows := imported.GetImported()
	if rows == nil {
		rows = []*mtproto.ImportedContact{}
	}

	return mtproto.MakeTLContactsImportedContacts(&mtproto.Contacts_ImportedContacts{
		Imported:       rows,
		PopularInvites: popular,
		RetryContacts:  retry,
		Users:          users,
	}).To_Contacts_ImportedContacts(), nil
}

func emptyImportedContacts() *mtproto.Contacts_ImportedContacts {
	return mtproto.MakeTLContactsImportedContacts(&mtproto.Contacts_ImportedContacts{
		Imported:       []*mtproto.ImportedContact{},
		PopularInvites: []*mtproto.PopularContact{},
		RetryContacts:  []int64{},
		Users:          []*mtproto.User{},
	}).To_Contacts_ImportedContacts()
}

// pushImportContactUpdates tells the importer's other sessions and each
// matched user. Telegram delivers updatePeerSettings plus the user as the
// recipient sees them, with contact / mutual_contact set from the saved link.
func (c *ContactsCore) pushImportContactUpdates(peerIDs []int64) {
	if len(peerIDs) == 0 || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.SyncClient == nil || c.MD == nil {
		return
	}

	selfID := c.MD.UserId
	users, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
		Id: append([]int64{selfID}, peerIDs...),
		To: peerIDs,
	})
	if err != nil || users == nil {
		c.Logger.Errorf("contacts.importContacts - load users for updates: %v", err)
		return
	}

	updates := make([]*mtproto.Update, 0, len(peerIDs))
	for _, id := range peerIDs {
		updates = append(updates, importPeerSettings(id))
	}
	_, _ = c.svcCtx.SyncClient.SyncUpdatesNotMe(c.ctx, &sync.TLSyncUpdatesNotMe{
		UserId:        selfID,
		PermAuthKeyId: c.MD.PermAuthKeyId,
		Updates: mtproto.MakeUpdatesByUpdatesUsers(
			users.GetUserListByIdList(selfID, peerIDs...),
			updates...,
		),
	})

	for _, id := range peerIDs {
		_, _ = c.svcCtx.SyncClient.SyncPushUpdates(c.ctx, &sync.TLSyncPushUpdates{
			UserId: id,
			Updates: mtproto.MakeUpdatesByUpdatesUsers(
				users.GetUserListByIdList(id, selfID),
				importPeerSettings(selfID),
			),
		})
	}
}

func importPeerSettings(userID int64) *mtproto.Update {
	return mtproto.MakeTLUpdatePeerSettings(&mtproto.Update{
		Peer_PEER: mtproto.MakePeerUser(userID),
		Settings:  mtproto.MakeTLPeerSettings(&mtproto.PeerSettings{}).To_PeerSettings(),
	}).To_Update()
}
