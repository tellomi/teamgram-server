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
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// ContactsSearch
// contacts.search#11f812d8 q:string limit:int = contacts.Found;
func (c *ContactsCore) ContactsSearch(in *mtproto.TLContactsSearch) (*mtproto.Contacts_Found, error) {
	var (
		limit = in.GetLimit()
	)

	if limit > 50 {
		limit = 50
	}
	if limit == 0 {
		limit = 50
	}

	q := in.Q

	if q == "" {
		err := mtproto.ErrSearchQueryEmpty
		c.Logger.Errorf("contacts.search - error: %v", err)
		return nil, err
	}

	if q[0] == '@' {
		q = q[1:]
	}

	if len(q) < 3 {
		err := mtproto.ErrQueryTooShort
		c.Logger.Errorf("contacts.search - error: %v", err)
		return nil, err
	}

	var (
		// The caller is not a search hit. Seeding the id list with UserId
		// put them in every result, including a query that matches nobody.
		idHelper = mtproto.NewIDListHelper()
	)

	found := mtproto.MakeTLContactsFound(&mtproto.Contacts_Found{
		MyResults: []*mtproto.Peer{},
		Results:   []*mtproto.Peer{},
		Users:     []*mtproto.User{},
		Chats:     []*mtproto.Chat{},
	}).To_Contacts_Found()

	// Contacts are not excluded. A hit that is in the caller's address book
	// goes to my_results, including a match on the contact note rather than
	// the account name. The caller is never a hit.

	// Check query string and limit
	// contactIDs is the caller's address book. my_results is membership in
	// that book, not CheckContact on the other user (that is their book).
	contactIDs := map[int64]struct{}{}
	if len(q) >= 3 && limit > 0 {
		if book, err := c.svcCtx.Dao.UserClient.UserGetContactList(c.ctx, &userpb.TLUserGetContactList{
			UserId: c.MD.UserId,
		}); err != nil {
			c.Logger.Errorf("contacts.search notes - error: %v", err)
		} else {
			for _, row := range book.GetDatas() {
				id := row.GetContactUserId()
				if id == 0 || id == c.MD.UserId {
					continue
				}
				contactIDs[id] = struct{}{}
				if !contactNoteMatch(q, row.GetFirstName().GetValue(), row.GetLastName().GetValue()) {
					continue
				}
				idHelper.PickByPeerUtil(mtproto.PEER_USER, id)
			}
		}

		// c.Logger.Debugf("q: %s", q)
		// Contacts stay in the search. They are split into my_results below.
		// Only the caller is excluded.
		rVList, err := c.svcCtx.Dao.UserClient.UserSearchUsername(c.ctx, &userpb.TLUserSearchUsername{
			Q:                q,
			ExcludedContacts: []int64{c.MD.UserId},
			Limit:            limit,
		})
		if err != nil {
			c.Logger.Errorf("contacts.search - error: %v", err)
			return found, nil
		}

		for _, v := range rVList.GetDatas() {
			// c.Logger.Debugf("v: %v", v)
			idHelper.PickByPeer(v.Peer)
		}

		rVList2, err := c.svcCtx.Dao.UserClient.UserSearch(c.ctx, &userpb.TLUserSearch{
			Q:                q,
			ExcludedContacts: []int64{c.MD.UserId},
			Offset:           0,
			Limit:            limit,
		})

		for _, v := range rVList2.GetIdList() {
			idHelper.PickByPeerUtil(mtproto.PEER_USER, v)
		}
	}

	// Public channel and group titles, including Chinese. Username search
	// above only matches the username table.
	if pubs, err := channel.Shared().SearchPublic(c.ctx, c.MD.UserId, q, limit); err != nil {
		c.Logger.Errorf("contacts.search public - error: %v", err)
	} else {
		for _, ch := range pubs {
			idHelper.PickByPeerUtil(mtproto.PEER_CHANNEL, ch.ID)
		}
	}

	idHelper.Visit(
		func(userIdList []int64) {
			users, _ := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx,
				&userpb.TLUserGetMutableUsers{
					Id: userIdList,
				})

			var kept []int64
			users.Visit(func(it *mtproto.ImmutableUser) {
				if it.Deleted() || it.Id() == c.MD.UserId {
					return
				}
				kind := contactUser
				if it.IsBot() {
					kind = contactBot
				}
				if !contactHitAllowed(in.GetBroadcasts(), in.GetBots(), kind) {
					return
				}
				kept = append(kept, it.Id())

				peer := mtproto.MakeTLPeerUser(&mtproto.Peer{
					UserId: it.Id(),
				})
				if callerSaved(contactIDs, it.Id()) {
					found.MyResults = append(found.MyResults, peer.To_Peer())
				} else {
					found.Results = append(found.Results, peer.To_Peer())
				}
			})

			found.Users = users.GetUserListByIdList(c.MD.UserId, kept...)
		},
		func(chatIdList []int64) {
		},
		func(channelIdList []int64) {
			if c.svcCtx.Plugin != nil {
				chats := c.svcCtx.Plugin.GetChannelListByIdList(c.ctx, c.MD.UserId, channelIdList...)
				for _, ch := range chats {
					if ch.PredicateName == mtproto.Predicate_chatEmpty || ch.PredicateName == mtproto.Predicate_channelForbidden {
						continue
					}
					if channel.Shared().Hidden(c.ctx, ch.GetId()) {
						continue
					}
					kind := contactGroup
					if ch.GetBroadcast() && !ch.GetMegagroup() {
						kind = contactChannel
					}
					if !contactHitAllowed(in.GetBroadcasts(), in.GetBots(), kind) {
						continue
					}
					found.Chats = append(found.Chats, ch)
					found.Results = append(found.Results, mtproto.MakePeerChannel(ch.GetId()))
				}
			} else {
				c.Logger.Errorf("contacts.search blocked, License key from https://teamgram.net required to unlock enterprise features.")
			}
		})

	return found, nil
}
