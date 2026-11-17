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

// UserDeleteProfilePhotos
// user.deleteProfilePhotos user_id:long id:Vector<long> = Int64;
func (c *UserCore) UserDeleteProfilePhotos(in *user.TLUserDeleteProfilePhotos) (*mtproto.Int64, error) {
	main, err := c.svcCtx.Dao.DeleteUserProfilePhotos(c.ctx, in.GetUserId(), in.GetId())
	if err != nil {
		c.Logger.Errorf("user.deleteProfilePhotos - error: %v", err)
		return nil, err
	}
	return &mtproto.Int64{V: main}, nil
}
