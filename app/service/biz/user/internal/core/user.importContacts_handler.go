/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package core

import (
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// UserImportContacts
// user.importContacts user_id:long contacts:Vector<InputContact> = UserImportedContacts;
func (c *UserCore) UserImportContacts(in *user.TLUserImportContacts) (*user.UserImportedContacts, error) {
	if len(in.GetContacts()) == 0 {
		return emptyImportedContacts(), nil
	}

	myUserData := c.svcCtx.Dao.GetCacheUserData(c.ctx, in.UserId)
	if myUserData == nil {
		c.Logger.Errorf("UserImportContacts - myUserData == nil")
		return nil, mtproto.ErrInternalServerError
	}

	// Every spelling of the number is queried ("+86 138-0013-8000" and
	// "8613800138000") so a formatted address-book entry matches signup.
	lookup := PlanContactImport(in.UserId, in.Contacts, nil, nil, nil).LookupPhones
	rows, err := c.svcCtx.Dao.UsersDAO.SelectUsersByPhoneList(c.ctx, lookup)
	if err != nil {
		c.Logger.Errorf("UserImportContacts - SelectUsersByPhoneList: %v", err)
		return nil, err
	}
	registered := make([]RegisteredPhone, 0, len(rows))
	for i := range rows {
		registered = append(registered, RegisteredPhone{
			ID:      rows[i].Id,
			Phone:   rows[i].Phone,
			Deleted: rows[i].Deleted,
		})
	}

	plan := PlanContactImport(
		in.UserId,
		in.Contacts,
		registered,
		myUserData.ContactIdList,
		myUserData.ReverseContactIdList,
	)

	importedContacts := make([]*mtproto.ImportedContact, 0, len(plan.Matches))
	idList := make([]int64, 0, len(plan.UpdateUserIDs))
	now := time.Now().Unix()

	for _, match := range plan.Matches {
		do := &dataobject.UserContactsDO{
			OwnerUserId:      in.UserId,
			ContactUserId:    match.UserID,
			ContactPhone:     match.Phone,
			ContactFirstName: match.FirstName,
			ContactLastName:  match.LastName,
			Mutual:           match.Reverse,
			CloseFriend:      false,
			StoriesHidden:    false,
			IsDeleted:        false,
			Date2:            now,
		}

		if match.AlreadyMine {
			_, _ = c.svcCtx.Dao.UserContactsDAO.UpdateContactName(
				c.ctx,
				do.ContactFirstName,
				do.ContactLastName,
				do.OwnerUserId,
				do.ContactUserId)
			if match.Reverse {
				_, _ = c.svcCtx.Dao.UserContactsDAO.UpdateMutual(c.ctx, true, do.OwnerUserId, do.ContactUserId)
				_, _ = c.svcCtx.Dao.UserContactsDAO.UpdateMutual(c.ctx, true, do.ContactUserId, do.OwnerUserId)
			}
		} else if match.Reverse {
			_, _ = c.svcCtx.Dao.UserContactsDAO.UpdateMutual(c.ctx, true, do.ContactUserId, do.OwnerUserId)
			_, _, _ = c.svcCtx.Dao.UserContactsDAO.InsertOrUpdate(c.ctx, do)
		} else {
			_, _, _ = c.svcCtx.Dao.ImportedContactsDAO.InsertOrUpdate(c.ctx, &dataobject.ImportedContactsDO{
				UserId:         do.ContactUserId,
				ImportedUserId: do.OwnerUserId,
			})
			_, _, _ = c.svcCtx.Dao.UserContactsDAO.InsertOrUpdate(c.ctx, do)
		}

		importedContacts = append(importedContacts, mtproto.MakeTLImportedContact(&mtproto.ImportedContact{
			UserId:   match.UserID,
			ClientId: match.ClientID,
		}).To_ImportedContact())
	}
	idList = append(idList, plan.UpdateUserIDs...)

	for _, pending := range plan.Unregistered {
		if pending.Phone == "" {
			continue
		}
		_, _, _ = c.svcCtx.Dao.UnregisteredContactsDAO.InsertOrUpdate(c.ctx, &dataobject.UnregisteredContactsDO{
			Phone:           pending.Phone,
			ImporterUserId:  in.UserId,
			ImportFirstName: pending.FirstName,
			ImportLastName:  pending.LastName,
		})
	}

	c.svcCtx.Dao.ClearContactCaches(c.ctx, in.UserId, idList...)
	users, _ := c.UserGetMutableUsers(&user.TLUserGetMutableUsers{
		Id: append(append([]int64{}, idList...), in.UserId),
		To: idList,
	})
	var userList []*mtproto.User
	if users != nil {
		userList = users.GetUserListByIdList(in.UserId, idList...)
	}
	if userList == nil {
		userList = []*mtproto.User{}
	}

	return user.MakeTLUserImportedContacts(&user.UserImportedContacts{
		Imported:       importedContacts,
		PopularInvites: []*mtproto.PopularContact{},
		RetryContacts:  []int64{},
		Users:          userList,
		UpdateIdList:   plan.UpdateUserIDs,
	}).To_UserImportedContacts(), nil
}

func emptyImportedContacts() *user.UserImportedContacts {
	return user.MakeTLUserImportedContacts(&user.UserImportedContacts{
		Imported:       []*mtproto.ImportedContact{},
		PopularInvites: []*mtproto.PopularContact{},
		RetryContacts:  []int64{},
		Users:          []*mtproto.User{},
		UpdateIdList:   []int64{},
	}).To_UserImportedContacts()
}
