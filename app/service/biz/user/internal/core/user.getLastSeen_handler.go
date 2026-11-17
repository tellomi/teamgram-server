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
	"errors"

	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// UserGetLastSeen
// user.getLastSeen id:long = LastSeenData;
func (c *UserCore) UserGetLastSeen(in *user.TLUserGetLastSeen) (*user.LastSeenData, error) {
	do, err := c.svcCtx.Dao.GetLastSeenAt(c.ctx, in.Id)
	if err != nil || do == nil {
		if mapped := presenceReadError(err); errors.Is(mapped, user.ErrPresenceNotFound) {
			return nil, mapped
		}
		c.Logger.Errorf("user.getLastSeen - error: %v", err)
		return nil, err
	}

	return user.MakeTLLastSeenData(&user.LastSeenData{
		UserId:     in.Id,
		LastSeenAt: do.LastSeenAt,
		Expires:    do.Expires,
	}).To_LastSeenData(), nil
}

// presenceReadError maps a missing row to ErrPresenceNotFound. Any other
// read error is returned unchanged so the caller does not treat a database
// failure as "this user was offline".
func presenceReadError(err error) error {
	if err == nil || errors.Is(err, sqlc.ErrNotFound) {
		return user.ErrPresenceNotFound
	}
	return err
}
