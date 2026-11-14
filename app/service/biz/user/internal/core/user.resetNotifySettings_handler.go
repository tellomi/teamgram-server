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

// UserResetNotifySettings
// user.resetNotifySettings user_id:int = Bool;
func (c *UserCore) UserResetNotifySettings(in *user.TLUserResetNotifySettings) (*mtproto.Bool, error) {
	if err := c.svcCtx.Dao.ResetNotifySettings(c.ctx, in.UserId); err != nil {
		c.Logger.Errorf("user.resetNotifySettings - error: %v", err)
		return nil, err
	}

	return mtproto.BoolTrue, nil
}
