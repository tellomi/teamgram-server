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

package dao

import (
	"context"
	"fmt"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	// v2 drops cached rows from before sound and stories columns were read.
	userNotifySettingsKeyPrefix = "user_notify_settings_v2"
)

func genUserNotifySettingsCacheKey(id int64, peerType int32, peerId int64) string {
	return fmt.Sprintf("%s_%d_%d_%d", userNotifySettingsKeyPrefix, id, peerType, peerId)
}

func (d *Dao) GetUserNotifySettings(ctx context.Context, id int64, peerType int32, peerId int64) (*mtproto.PeerNotifySettings, error) {
	settings := mtproto.MakeTLPeerNotifySettings(nil).To_PeerNotifySettings()

	err := d.CachedConn.QueryRow(
		ctx,
		settings,
		genUserNotifySettingsCacheKey(id, peerType, peerId),
		func(ctx context.Context, conn *sqlx.DB, v interface{}) error {
			do, err := d.UserNotifySettingsDAO.Select(ctx, id, peerType, peerId)
			if err != nil {
				// c.Logger.Errorf("user.getNotifySettings - error: %v", err)
				return err
			}

			var (
				notifySettings = v.(*mtproto.PeerNotifySettings)
			)

			if do == nil {
				switch peerType {
				case mtproto.PEER_USERS,
					mtproto.PEER_CHATS,
					mtproto.PEER_BROADCASTS:
					applyDefaultPeerNotifySettings(notifySettings)
				}
			} else {
				setPeerNotifySettingsByDO(notifySettings, do)
			}

			return nil
		})
	if err != nil {
		return nil, err
	}

	return settings, nil
}

func applyDefaultPeerNotifySettings(settings *mtproto.PeerNotifySettings) {
	def := mtproto.MakeDefaultPeerNotifySettings()
	settings.ShowPreviews = def.ShowPreviews
	settings.Silent = def.Silent
	settings.MuteUntil = def.MuteUntil
	settings.Sound = def.Sound
	settings.IosSound = def.IosSound
	settings.AndroidSound = def.AndroidSound
	settings.OtherSound = def.OtherSound
	settings.StoriesMuted = def.StoriesMuted
	settings.StoriesHideSender = def.StoriesHideSender
	settings.StoriesIosSound = def.StoriesIosSound
	settings.StoriesAndroidSound = def.StoriesAndroidSound
	settings.StoriesOtherSound = def.StoriesOtherSound
}

func setPeerNotifySettingsByDO(settings *mtproto.PeerNotifySettings, do *dataobject.UserNotifySettingsDO) {
	filled := peerNotifySettingsFromDO(do)
	settings.ShowPreviews = filled.ShowPreviews
	settings.Silent = filled.Silent
	settings.MuteUntil = filled.MuteUntil
	settings.Sound = filled.Sound
	settings.IosSound = filled.IosSound
	settings.AndroidSound = filled.AndroidSound
	settings.OtherSound = filled.OtherSound
	settings.StoriesMuted = filled.StoriesMuted
	settings.StoriesHideSender = filled.StoriesHideSender
	settings.StoriesIosSound = filled.StoriesIosSound
	settings.StoriesAndroidSound = filled.StoriesAndroidSound
	settings.StoriesOtherSound = filled.StoriesOtherSound
}

// PeerNotifySettingsFromDO builds the TL object stored for one row.
// -1 and an empty sound are unset. A row that only has the legacy sound
// column exposes that sound on every platform until a client writes its slot.
func PeerNotifySettingsFromDO(do *dataobject.UserNotifySettingsDO) *mtproto.PeerNotifySettings {
	return peerNotifySettingsFromDO(do)
}

func peerNotifySettingsFromDO(do *dataobject.UserNotifySettingsDO) *mtproto.PeerNotifySettings {
	settings := mtproto.MakeTLPeerNotifySettings(nil).To_PeerNotifySettings()
	if do == nil {
		return settings
	}
	if do.ShowPreviews != -1 {
		settings.ShowPreviews = mtproto.ToBool(do.ShowPreviews == 1)
	}
	if do.Silent != -1 {
		settings.Silent = mtproto.ToBool(do.Silent == 1)
	}
	if do.MuteUntil != -1 {
		settings.MuteUntil = &wrapperspb.Int32Value{Value: do.MuteUntil}
	}
	if do.Sound != "" && do.Sound != "-1" {
		settings.Sound = &wrapperspb.StringValue{Value: do.Sound}
	}
	if do.StoriesMuted != -1 {
		settings.StoriesMuted = mtproto.ToBool(do.StoriesMuted == 1)
	}
	if do.StoriesHideSender != -1 {
		settings.StoriesHideSender = mtproto.ToBool(do.StoriesHideSender == 1)
	}
	ios := mtproto.DecodeNotificationSound(do.IosSound)
	android := mtproto.DecodeNotificationSound(do.AndroidSound)
	other := mtproto.DecodeNotificationSound(do.OtherSound)
	if ios == nil && android == nil && other == nil {
		if legacy := mtproto.DecodeNotificationSound(do.Sound); legacy != nil {
			ios, android, other = legacy, legacy, legacy
		}
	}
	settings.IosSound = ios
	settings.AndroidSound = android
	settings.OtherSound = other
	sios := mtproto.DecodeNotificationSound(do.StoriesIosSound)
	sandroid := mtproto.DecodeNotificationSound(do.StoriesAndroidSound)
	sother := mtproto.DecodeNotificationSound(do.StoriesOtherSound)
	settings.StoriesIosSound = sios
	settings.StoriesAndroidSound = sandroid
	settings.StoriesOtherSound = sother
	return settings
}

func makeDOByPeerNotifySettings(settings *mtproto.PeerNotifySettings) map[string]interface{} {
	return mtproto.NotifyColumnUpdates(settings)
}

func (d *Dao) SetUserPeerNotifySettings(ctx context.Context, id int64, peerType int32, peerId int64, settings *mtproto.PeerNotifySettings) error {
	_, _, err := d.CachedConn.Exec(
		ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			cMap := makeDOByPeerNotifySettings(settings)
			return d.UserNotifySettingsDAO.InsertOrUpdateExt(ctx, id, peerType, peerId, cMap)
		},
		genUserNotifySettingsCacheKey(id, peerType, peerId))

	return err
}
