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
	"github.com/teamgram/teamgram-server/app/bff/business/profile"
	"github.com/teamgram/teamgram-server/app/bff/contacts/book"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// ContactsGetLocated
// contacts.getLocated#d348bc44 flags:# background:flags.1?true geo_point:InputGeoPoint self_expires:flags.0?int = Updates;
// https://core.telegram.org/method/contacts.getLocated
// https://core.telegram.org/api/nearby
//
// Returns users who are currently advertising a location within 50km.
// self_expires publishes (or, when 0, hides) the caller's point. A business
// location, a missing profile photo, or a profile photo that is not visible
// to everyone prevents publishing. Geogroups are not indexed.
func (c *ContactsCore) ContactsGetLocated(in *mtproto.TLContactsGetLocated) (*mtproto.Updates, error) {
	if err := c.requireSelf(); err != nil {
		return nil, err
	}
	lat, long, err := book.ParseGeo(in.GetGeoPoint())
	if err != nil {
		c.Logger.Errorf("contacts.getLocated - error: %v", err)
		return nil, err
	}
	now := nowUnix()
	if in.GetSelfExpires() != nil {
		if err := c.publishSelf(lat, long, in.GetSelfExpires().GetValue(), now); err != nil {
			c.Logger.Errorf("contacts.getLocated - error: %v", err)
			return nil, err
		}
	}
	var located []*mtproto.PeerLocated
	if exp, ok := c.relBook().SelfExpires(c.ctx, c.MD.UserId, now); ok {
		located = append(located, mtproto.MakeTLPeerSelfLocated(&mtproto.PeerLocated{Expires: exp}).To_PeerLocated())
	}
	var ids []int64
	for _, n := range c.relBook().Nearby(c.ctx, c.MD.UserId, lat, long, now) {
		located = append(located, mtproto.MakeTLPeerLocated(&mtproto.PeerLocated{
			Peer:     mtproto.MakePeerUser(n.UserID),
			Expires:  n.Expires,
			Distance: n.Distance,
		}).To_PeerLocated())
		ids = append(ids, n.UserID)
	}
	upd := mtproto.MakeTLUpdatePeerLocated(&mtproto.Update{Peers: located}).To_Update()
	return mtproto.MakeUpdatesByUpdatesUsers(c.topUsers(ids), upd), nil
}

func (c *ContactsCore) publishSelf(lat, long float64, ttl, now int32) error {
	if ttl == 0 {
		return c.relBook().Hide(c.ctx, c.MD.UserId)
	}
	if card := profile.Shared().Card(c.MD.UserId); card != nil && card.Location != nil {
		return mtproto.ErrBusinessAddressActive
	}
	user, err := c.loadUser(c.MD.UserId, c.MD.UserId)
	if err != nil {
		return mtproto.ErrNotAcceptableUserpicUploadRequired
	}
	photo := user.GetPhoto()
	if photo == nil || photo.GetPredicateName() == "" || photo.GetPredicateName() == mtproto.Predicate_userProfilePhotoEmpty {
		return mtproto.ErrNotAcceptableUserpicUploadRequired
	}
	if !c.profilePhotoEveryone() {
		return mtproto.ErrUserpicPrivacyRequired
	}
	return c.relBook().Publish(c.ctx, c.MD.UserId, lat, long, book.ExpiresUnix(now, ttl))
}

func (c *ContactsCore) profilePhotoEveryone() bool {
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return true
	}
	rules, err := c.svcCtx.Dao.UserGetPrivacy(c.ctx, &userpb.TLUserGetPrivacy{
		UserId:  c.MD.UserId,
		KeyType: mtproto.PROFILE_PHOTO,
	})
	if err != nil || rules == nil || len(rules.GetDatas()) == 0 {
		return true
	}
	allowAll := false
	for _, rule := range rules.GetDatas() {
		switch rule.GetPredicateName() {
		case mtproto.Predicate_privacyValueAllowAll:
			allowAll = true
		case mtproto.Predicate_privacyValueDisallowAll,
			mtproto.Predicate_privacyValueDisallowContacts,
			mtproto.Predicate_privacyValueDisallowUsers,
			mtproto.Predicate_privacyValueAllowContacts,
			mtproto.Predicate_privacyValueAllowUsers,
			mtproto.Predicate_privacyValueAllowCloseFriends,
			mtproto.Predicate_privacyValueAllowPremium:
			return false
		}
	}
	return allowAll
}
