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
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// UserUpdateProfilePhoto
// user.updateProfilePhoto user_id:long id:long = Int64;
func (c *UserCore) UserUpdateProfilePhoto(in *user.TLUserUpdateProfilePhoto) (*mtproto.Int64, error) {
	// A negative id is the public fallback photo. The internal request has no
	// fallback flag, and the id itself is always positive.
	photoID := in.GetId()
	if photoID < 0 {
		return &mtproto.Int64{V: c.svcCtx.Dao.UpdateFallbackPhoto(c.ctx, in.GetUserId(), -photoID)}, nil
	}
	rV := c.svcCtx.Dao.UpdateProfilePhoto(
		c.ctx,
		in.GetUserId(),
		photoID)

	return &mtproto.Int64{
		V: rV,
	}, nil
}
