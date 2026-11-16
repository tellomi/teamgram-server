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
	"context"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// DialogSetChatTheme
// dialog.setChatTheme user_id:long peer_type:int peer_id:long theme_emoticon:string = Bool;
func (c *DialogCore) DialogSetChatTheme(in *dialog.TLDialogSetChatTheme) (*mtproto.Bool, error) {
	keys := []string{dialog.GetDialogCacheKeyByPeer(in.UserId, in.PeerType, in.PeerId)}
	if in.PeerType == mtproto.PEER_USER && in.PeerId != 0 && in.PeerId != in.UserId {
		keys = append(keys, dialog.GetDialogCacheKeyByPeer(in.PeerId, mtproto.PEER_USER, in.UserId))
	}
	_, _, err := c.svcCtx.Dao.ExecInvalidate(
		c.ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			if _, err := c.svcCtx.Dao.DialogsDAO.UpdateCustomMap(ctx, map[string]interface{}{
				"theme_emoticon": in.ThemeEmoticon,
			}, in.UserId, in.PeerType, in.PeerId); err != nil {
				return 0, 0, err
			}
			if in.PeerType == mtproto.PEER_USER && in.PeerId != 0 && in.PeerId != in.UserId {
				if _, err := c.svcCtx.Dao.DialogsDAO.UpdateCustomMap(ctx, map[string]interface{}{
					"theme_emoticon": in.ThemeEmoticon,
				}, in.PeerId, mtproto.PEER_USER, in.UserId); err != nil {
					return 0, 0, err
				}
			}
			return 0, 1, nil
		},
		keys...)
	if err != nil {
		c.Logger.Errorf("dialog.setChatTheme - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
