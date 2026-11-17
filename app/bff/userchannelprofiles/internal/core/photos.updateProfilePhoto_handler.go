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
	mediapb "github.com/teamgram/teamgram-server/app/service/media/media"
)

// PhotosUpdateProfilePhoto
// photos.updateProfilePhoto#9e82039 flags:# fallback:flags.0?true bot:flags.1?InputUser id:InputPhoto = photos.Photo;
func (c *UserChannelProfilesCore) PhotosUpdateProfilePhoto(in *mtproto.TLPhotosUpdateProfilePhoto) (*mtproto.Photos_Photo, error) {
	var (
		photo *mtproto.Photo
	)

	// TODO: ALBUM_PHOTOS_TOO_MANY
	// inputPhotoEmpty clears the current avatar. Any other id has to be a
	// photo already in this user's album, and the access_hash has to match
	// the stored photo. A miss is PHOTO_ID_INVALID and does not write.
	photoID := in.GetId().GetId()
	if in.GetId().GetPredicateName() == mtproto.Predicate_inputPhotoEmpty {
		photoID = 0
	}
	if photoID != 0 {
		if err := c.requireOwnProfilePhoto(photoID, in.GetId().GetAccessHash()); err != nil {
			c.Logger.Errorf("photos.updateProfilePhoto - error: %v", err)
			return nil, err
		}
	}
	if in.GetFallback() && photoID > 0 {
		photoID = -photoID
	}
	updatedPhotoId, err := c.svcCtx.Dao.UserClient.UserUpdateProfilePhoto(c.ctx, &userpb.TLUserUpdateProfilePhoto{
		UserId: c.MD.UserId,
		Id:     photoID,
	})
	if err != nil {
		c.Logger.Errorf("photos.updateProfilePhoto - error: %v", err)
		return nil, err
	}

	if updatedPhotoId.V != 0 {
		photo, err = c.svcCtx.Dao.MediaClient.MediaGetPhoto(c.ctx, &mediapb.TLMediaGetPhoto{
			PhotoId: updatedPhotoId.V,
		})
		if err != nil {
			c.Logger.Errorf("photos.updateProfilePhoto - error: %v", err)
			return nil, err
		}
	}

	if photo == nil {
		photo = mtproto.MakeTLPhotoEmpty(nil).To_Photo()
	}

	me, err := c.svcCtx.Dao.UserClient.UserGetImmutableUser(
		c.ctx,
		&userpb.TLUserGetImmutableUser{
			Id:       c.MD.UserId,
			Privacy:  false,
			Contacts: nil,
		})
	if err != nil {
		c.Logger.Errorf("photos.updateProfilePhoto - error: %v", err)
		return nil, err
	}

	_, _ = c.svcCtx.Dao.SyncClient.SyncPushUpdates(c.ctx, &sync.TLSyncPushUpdates{
		UserId: c.MD.UserId,
		Updates: mtproto.MakeUpdatesByUpdatesUsers(
			[]*mtproto.User{me.ToSelfUser()},
			mtproto.MakeTLUpdateUser(&mtproto.Update{
				UserId: c.MD.UserId,
			}).To_Update()),
	})

	return mtproto.MakeTLPhotosPhoto(&mtproto.Photos_Photo{
		Photo: photo,
		Users: []*mtproto.User{me.ToSelfUser()},
	}).To_Photos_Photo(), nil
}

// requireOwnProfilePhoto accepts photoID only when that photo exists, the
// access hash matches, and the id is already in the caller's album or is
// their public fallback photo. photos.uploadProfilePhoto inserts a new
// upload itself and does not come through here.
func (c *UserChannelProfilesCore) requireOwnProfilePhoto(photoID, accessHash int64) error {
	if photoID <= 0 {
		return mtproto.ErrPhotoIdInvalid
	}
	photo, err := c.svcCtx.Dao.MediaClient.MediaGetPhoto(c.ctx, &mediapb.TLMediaGetPhoto{
		PhotoId: photoID,
	})
	if err != nil {
		return err
	}
	if !profilePhotoAccessMatches(photo, photoID, accessHash) {
		return mtproto.ErrPhotoIdInvalid
	}
	album, err := c.svcCtx.Dao.UserClient.UserGetProfilePhotos(c.ctx, &userpb.TLUserGetProfilePhotos{
		UserId: c.MD.UserId,
	})
	if err != nil {
		return err
	}
	if containsInt64(album.GetDatas(), photoID) {
		return nil
	}
	fallback, err := c.svcCtx.Dao.UserClient.UserGetFallbackPhoto(c.ctx, &userpb.TLUserGetProfilePhotos{
		UserId: c.MD.UserId,
	})
	if err != nil {
		return err
	}
	if fallback.GetId() == photoID && fallback.GetPredicateName() != mtproto.Predicate_photoEmpty {
		return nil
	}
	return mtproto.ErrPhotoIdInvalid
}

func profilePhotoAccessMatches(photo *mtproto.Photo, photoID, accessHash int64) bool {
	if photo == nil || photo.GetId() != photoID {
		return false
	}
	if photo.GetPredicateName() != mtproto.Predicate_photo {
		return false
	}
	return photo.GetAccessHash() == accessHash
}
