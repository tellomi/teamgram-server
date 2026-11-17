/*
 * WARNING! All changes made in this file will be lost!
 *   Created from by 'dalgen'
 *
 * Copyright (c) 2024-present,  Teamgram Authors.
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package dataobject

type UserNotifySettingsDO struct {
	Id                  int64  `db:"id" json:"id"`
	UserId              int64  `db:"user_id" json:"user_id"`
	PeerType            int32  `db:"peer_type" json:"peer_type"`
	PeerId              int64  `db:"peer_id" json:"peer_id"`
	TopMsgId            int32  `db:"top_msg_id" json:"top_msg_id"`
	ShowPreviews        int32  `db:"show_previews" json:"show_previews"`
	Silent              int32  `db:"silent" json:"silent"`
	MuteUntil           int32  `db:"mute_until" json:"mute_until"`
	Sound               string `db:"sound" json:"sound"`
	StoriesMuted        int32  `db:"stories_muted" json:"stories_muted"`
	StoriesHideSender   int32  `db:"stories_hide_sender" json:"stories_hide_sender"`
	IosSound            string `db:"ios_sound" json:"ios_sound"`
	AndroidSound        string `db:"android_sound" json:"android_sound"`
	OtherSound          string `db:"other_sound" json:"other_sound"`
	StoriesIosSound     string `db:"stories_ios_sound" json:"stories_ios_sound"`
	StoriesAndroidSound string `db:"stories_android_sound" json:"stories_android_sound"`
	StoriesOtherSound   string `db:"stories_other_sound" json:"stories_other_sound"`
	Deleted             bool   `db:"deleted" json:"deleted"`
}
