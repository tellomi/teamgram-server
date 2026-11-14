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
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	mediapb "github.com/teamgram/teamgram-server/app/service/media/media"
)

// PhotosGetUserPhotos
// photos.getUserPhotos#91cd32a8 user_id:InputUser offset:int max_id:long limit:int = photos.Photos;
func (c *UserChannelProfilesCore) PhotosGetUserPhotos(in *mtproto.TLPhotosGetUserPhotos) (*mtproto.Photos_Photos, error) {
	userId := mtproto.FromInputUser(c.MD.UserId, in.UserId)
	switch userId.PeerType {
	case mtproto.PEER_SELF, mtproto.PEER_USER:
	default:
		err := mtproto.ErrUserIdInvalid
		c.Logger.Errorf("photos.getUserPhotos - error: %v", err)
		return nil, err
	}

	self := userId.PeerId == c.MD.UserId
	// The person who was blocked does not receive the blocker's album, even
	// when profile-photo privacy would allow it. A public fallback is not a
	// substitute for that block.
	if !self && c.ownerBlockedViewer(userId.PeerId) {
		return c.photosResult(nil), nil
	}
	fallback, _ := c.svcCtx.Dao.UserClient.UserGetFallbackPhoto(c.ctx, &userpb.TLUserGetProfilePhotos{
		UserId: userId.PeerId,
	})
	if !self && !c.profilePhotoAllowed(userId.PeerId) {
		return c.photosResult(fallbackOnly(fallback)), nil
	}

	cachePhotos, err := c.svcCtx.Dao.UserClient.UserGetProfilePhotos(c.ctx, &userpb.TLUserGetProfilePhotos{
		UserId: userId.PeerId,
	})
	if err != nil {
		c.Logger.Errorf("photos.getUserPhotos - error: %v", err)
		return nil, err
	}

	ids := append([]int64{}, cachePhotos.GetDatas()...)
	if self && fallback.GetId() != 0 && !containsInt64(ids, fallback.GetId()) {
		ids = append(ids, fallback.GetId())
	}
	photos := make([]*mtproto.Photo, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		photo, err := c.svcCtx.Dao.MediaClient.MediaGetPhoto(c.ctx, &mediapb.TLMediaGetPhoto{PhotoId: id})
		if err != nil {
			c.Logger.Errorf("photos.getUserPhotos - error: %v", err)
			continue
		}
		if photo != nil {
			photos = append(photos, photo)
		}
	}
	return c.photosResult(photos), nil
}

func (c *UserChannelProfilesCore) ownerBlockedViewer(owner int64) bool {
	if c == nil || c.MD == nil || c.MD.UserId == owner || owner == 0 {
		return false
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return false
	}
	blocked, err := c.svcCtx.Dao.UserClient.UserBlockedByUser(c.ctx, &userpb.TLUserBlockedByUser{
		UserId:     owner,
		PeerUserId: c.MD.UserId,
	})
	return err == nil && mtproto.FromBool(blocked)
}

func (c *UserChannelProfilesCore) profilePhotoAllowed(owner int64) bool {
	if c == nil || c.MD == nil || c.MD.UserId == owner {
		return true
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return true
	}
	ok, err := c.svcCtx.Dao.UserClient.UserCheckPrivacy(c.ctx, &userpb.TLUserCheckPrivacy{
		UserId:  owner,
		KeyType: mtproto.PROFILE_PHOTO,
		PeerId:  c.MD.UserId,
	})
	if err != nil {
		return true
	}
	return mtproto.FromBool(ok)
}

func (c *UserChannelProfilesCore) photosResult(photos []*mtproto.Photo) *mtproto.Photos_Photos {
	if photos == nil {
		photos = []*mtproto.Photo{}
	}
	return mtproto.MakeTLPhotosPhotos(&mtproto.Photos_Photos{
		Photos: photos,
		Users:  []*mtproto.User{},
	}).To_Photos_Photos()
}

func fallbackOnly(photo *mtproto.Photo) []*mtproto.Photo {
	if photo == nil || photo.GetId() == 0 {
		return []*mtproto.Photo{}
	}
	return []*mtproto.Photo{photo}
}

func containsInt64(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}
