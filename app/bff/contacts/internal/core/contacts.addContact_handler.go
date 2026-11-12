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
	"github.com/teamgram/teamgram-server/app/bff/contacts/book"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/statusbook"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// ContactsAddContact
/***
	## contacts.addContact
	Add an existing telegram user as contact.

	Use contacts.importContacts to add contacts by phone number, without knowing their Telegram ID.
**/
// contacts.addContact#e8f463d0 flags:# add_phone_privacy_exception:flags.0?true id:InputUser first_name:string last_name:string phone:string = Updates;
func (c *ContactsCore) ContactsAddContact(in *mtproto.TLContactsAddContact) (*mtproto.Updates, error) {
	// 400	CONTACT_NAME_EMPTY	Contact name empty.
	if err := book.ValidateNote(in.GetNote()); err != nil {
		c.Logger.Errorf("contacts.addContact - error: %v", err)
		return nil, err
	}
	if in.FirstName == "" && in.LastName == "" {
		err := mtproto.ErrContactNameEmpty
		c.Logger.Errorf("contacts.addContact - error: %v", err)
		return nil, err
	}

	id := mtproto.FromInputUser(c.MD.UserId, in.Id)

	// TODO: check inputUserFromMessage
	if !id.IsUser() || id.IsSelf() || id.PeerId == c.MD.UserId {
		err := mtproto.ErrContactIdInvalid
		c.Logger.Errorf("contacts.addContact - error: %v", err)
		return nil, err
	}

	users, err := c.svcCtx.Dao.UserClient.UserGetMutableUsersV2(c.ctx, &userpb.TLUserGetMutableUsersV2{
		Id:      []int64{c.MD.UserId, id.PeerId},
		Privacy: true,
		HasTo:   true,
		To:      []int64{id.PeerId},
	})
	if err != nil {
		c.Logger.Errorf("contacts.addContact - error: %v", err)
		err = mtproto.ErrContactIdInvalid
		return nil, err
	}

	if !users.CheckExistUser(c.MD.UserId, id.PeerId) {
		err = mtproto.ErrContactIdInvalid
		c.Logger.Errorf("contacts.addContact - error: %v", err)
		return nil, err
	}

	changeMutual, err := c.svcCtx.Dao.UserClient.UserAddContact(c.ctx, &userpb.TLUserAddContact{
		UserId:                   c.MD.UserId,
		AddPhonePrivacyException: mtproto.ToBool(in.AddPhonePrivacyException),
		Id:                       id.PeerId,
		FirstName:                in.FirstName,
		LastName:                 in.LastName,
		Phone:                    in.Phone,
	})
	if err != nil {
		c.Logger.Errorf("contacts.addContact - error: %v", err)
		err = mtproto.ErrContactIdInvalid
		return nil, err
	}

	cUser, _ := users.GetUnsafeUser(c.MD.UserId, id.PeerId)
	cUser.Contact = true
	cUser.MutualContact = mtproto.FromBool(changeMutual)
	cUser.FirstName = mtproto.MakeFlagsString(in.FirstName)
	cUser.LastName = mtproto.MakeFlagsString(in.LastName)
	cUser.Phone = mtproto.MakeFlagsString(in.Phone)
	if me, ok := users.GetImmutableUser(c.MD.UserId); ok && me != nil {
		if peer, ok := users.GetImmutableUser(id.PeerId); ok && peer != nil {
			me.Contacts = append(me.Contacts, mtproto.MakeTLContactData(&mtproto.ContactData{
				UserId:        c.MD.UserId,
				ContactUserId: id.PeerId,
			}).To_ContactData())
			if _, forMe := contactStatusRefresh(me, peer); forMe != nil {
				cUser.Status = forMe
			}
		}
	}

	me, _ := users.GetUnsafeUserSelf(c.MD.UserId)
	if in.GetNote() != nil {
		if err := c.relBook().PutNote(c.ctx, c.MD.UserId, id.PeerId, in.GetNote()); err != nil {
			c.Logger.Errorf("contacts.addContact - note: %v", err)
		}
	}

	// TODO(@benqi): 性能优化，复用users
	c.pushContactStatuses(c.MD.UserId, id.PeerId)
	statusbook.Shared().Forget(c.ctx, c.MD.UserId, id.PeerId)

	rUpdates := mtproto.MakeUpdatesByUpdatesUsers(
		[]*mtproto.User{me, cUser},
		mtproto.MakeTLUpdatePeerSettings(&mtproto.Update{
			Peer_PEER: id.ToPeer(),
			Settings: mtproto.MakeTLPeerSettings(&mtproto.PeerSettings{
				ReportSpam:             false,
				AddContact:             false,
				BlockContact:           false,
				ShareContact:           false,
				NeedContactsException:  false,
				ReportGeo:              false,
				Autoarchived:           false,
				InviteMembers:          false,
				RequestChatBroadcast:   false,
				BusinessBotPaused:      false,
				BusinessBotCanReply:    false,
				GeoDistance:            nil,
				RequestChatTitle:       nil,
				RequestChatDate:        nil,
				BusinessBotId:          nil,
				BusinessBotManageUrl:   nil,
				ChargePaidMessageStars: nil,
				RegistrationMonth:      nil,
				PhoneCountry:           nil,
				NameChangeDate:         nil,
				PhotoChangeDate:        nil,
			}).To_PeerSettings(),
		}).To_Update())

	return rUpdates, nil
}
