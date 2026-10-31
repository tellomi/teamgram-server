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
	"github.com/teamgram/marmota/pkg/utils"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/botadmin"
	profile "github.com/teamgram/teamgram-server/app/bff/business/profile"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"

	"github.com/zeromicro/go-zero/core/mr"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// UsersGetFullUser
// users.getFullUser#b60f5918 id:InputUser = users.UserFull;
func (c *UsersCore) UsersGetFullUser(in *mtproto.TLUsersGetFullUser) (*mtproto.Users_UserFull, error) {
	var (
		peerId int64
		id     = mtproto.FromInputUser(c.MD.UserId, in.Id)
		me     *mtproto.ImmutableUser
		user   *mtproto.ImmutableUser
	)

	switch id.PeerType {
	case mtproto.PEER_SELF, mtproto.PEER_USER:
		peerId = id.PeerId
	default:
		err := mtproto.ErrUserIdInvalid
		c.Logger.Errorf("users.getFullUser - error: %v", err)
		return nil, err
	}

	mutableUsers, err := c.svcCtx.Dao.UserClient.UserGetMutableUsersV2(c.ctx, &userpb.TLUserGetMutableUsersV2{
		Id:      []int64{c.MD.UserId, peerId},
		Privacy: true,
		HasTo:   true,
		To:      []int64{c.MD.UserId, peerId},
	})
	if err != nil {
		c.Logger.Errorf("users.getFullUser - error: %v", err)
		return nil, err
	} else if len(mutableUsers.GetUsers()) == 0 {
		err = mtproto.ErrInternalServerError
		c.Logger.Errorf("users.getFullUser - error: %v", err)
		return nil, err
	}

	me, _ = mutableUsers.GetImmutableUser(c.MD.UserId)
	user, _ = mutableUsers.GetImmutableUser(peerId)

	if user == nil {
		err = mtproto.ErrInternalServerError
		c.Logger.Errorf("users.getFullUser - error: %v", err)
		return nil, err
	}

	userFull := mtproto.MakeTLUserFull(&mtproto.UserFull{
		Blocked:                  false,
		PhoneCallsAvailable:      c.MD.UserId != peerId,
		PhoneCallsPrivate:        false,
		CanPinMessage:            true,
		HasScheduled:             false,
		VideoCallsAvailable:      c.MD.UserId != peerId,
		VoiceMessagesForbidden:   false,
		TranslationsDisabled:     chatbox.Shared().TranslationsDisabled(c.MD.UserId, mtproto.MakePeerUtil(mtproto.PEER_USER, peerId)),
		StoriesPinnedAvailable:   false,
		BlockedMyStoriesFrom:     false,
		WallpaperOverridden:      false,
		Id:                       peerId,
		About:                    user.GetUser().GetAbout(),
		Settings:                 nil,
		PersonalPhoto:            nil,
		ProfilePhoto:             user.GetUser().GetProfilePhoto(),
		FallbackPhoto:            nil,
		NotifySettings:           nil,
		BotInfo:                  nil,
		PinnedMsgId:              nil,
		CommonChatsCount:         0,
		FolderId:                 nil,
		TtlPeriod:                nil,
		ThemeEmoticon:            nil,
		Theme:                    nil,
		PrivateForwardName:       nil,
		BotGroupAdminRights:      nil,
		BotBroadcastAdminRights:  nil,
		PremiumGifts:             nil,
		Wallpaper:                nil,
		Stories_FLAGPEERSTORIES:  nil,
		Stories_FLAGUSERSTORIES:  nil,
		BusinessWorkHours:        nil,
		BusinessLocation:         nil,
		BusinessGreetingMessage:  nil,
		BusinessAwayMessage:      nil,
		BusinessIntro:            nil,
		Birthday:                 nil,
		PersonalChannelId:        nil,
		PersonalChannelMessage:   nil,
		StargiftsCount:           nil,
		StarrefProgram:           nil,
		BotVerification:          nil,
		SendPaidMessagesStars:    nil,
		DisallowedGifts:          nil,
		StarsMyPendingRating:     nil,
		StarsMyPendingRatingDate: nil,
		MainTab:                  user.GetUser().GetMainTab(),
		SavedMusic:               nil,
		Note:                     nil,
	}).To_UserFull()

	// Premium is included. Gift prices would open a purchase flow, so they stay empty.
	userFull.PremiumGifts = nil
	// Business hours, location, greeting, away message, and intro are public
	// profile fields. The business RPCs write the same process-local book.
	// Default admin rights a bot asks for when it is added to a group or channel.
	// https://core.telegram.org/method/bots.setBotGroupDefaultAdminRights
	if group, channel := botadmin.DefaultAdminRights(peerId); group != nil || channel != nil {
		userFull.BotGroupAdminRights = group
		userFull.BotBroadcastAdminRights = channel
	}
	if card := profile.Shared().Card(peerId); card != nil {
		userFull.BusinessWorkHours = card.WorkHours
		userFull.BusinessLocation = card.Location
		userFull.BusinessGreetingMessage = card.Greeting
		userFull.BusinessAwayMessage = card.Away
		userFull.BusinessIntro = card.Intro
	}
	mr.FinishVoid(
		func() {
			// blocked
			if c.MD.UserId != peerId {
				blocked, _ := c.svcCtx.Dao.UserClient.UserBlockedByUser(
					c.ctx,
					&userpb.TLUserBlockedByUser{
						UserId:     c.MD.UserId,
						PeerUserId: peerId,
					})
				userFull.Blocked = mtproto.FromBool(blocked)
			}
		},
		func() {
			userFull.Settings, _ = c.svcCtx.Dao.UserClient.UserGetPeerSettings(c.ctx, &userpb.TLUserGetPeerSettings{
				UserId:   c.MD.UserId,
				PeerType: mtproto.PEER_USER,
				PeerId:   peerId,
			})
		},
		func() {
			userFull.NotifySettings, _ = c.svcCtx.Dao.UserClient.UserGetNotifySettings(c.ctx, &userpb.TLUserGetNotifySettings{
				UserId:   c.MD.UserId,
				PeerType: mtproto.PEER_USER,
				PeerId:   peerId,
			})
		},
		func() {
			if user.GetUser().GetBot() != nil {
				userFull.PhoneCallsAvailable = false
				userFull.PhoneCallsPrivate = false
				userFull.VideoCallsAvailable = false
				userFull.BotInfo, _ = c.svcCtx.Dao.UserClient.UserGetBotInfo(c.ctx, &userpb.TLUserGetBotInfo{
					BotId: peerId,
				})
			}
		},
		func() {
			// TODO: PinnedMsgId:         nil,
			if c.MD.UserId != peerId {
				usersChatIdList, _ := c.svcCtx.Dao.ChatClient.ChatGetUsersChatIdList(c.ctx, &chatpb.TLChatGetUsersChatIdList{
					Id: []int64{c.MD.UserId, peerId},
				})
				if usersChatIdList != nil && len(usersChatIdList.Datas) == 2 {
					commonChats := utils.Int64Intersect(
						usersChatIdList.Datas[0].ChatIdList,
						usersChatIdList.Datas[1].ChatIdList)
					userFull.CommonChatsCount = int32(len(commonChats))
				}
				// TODO: Fetch CommonChannelsCount
			}
		},
		func() {
			if peerId != c.MD.UserId {
				// theme_emoticon
				dialogExt, _ := c.svcCtx.Dao.DialogClient.DialogGetDialogById(c.ctx, &dialog.TLDialogGetDialogById{
					UserId:   c.MD.UserId,
					PeerType: mtproto.PEER_USER,
					PeerId:   peerId,
				})
				if dialogExt != nil {
					userFull.ThemeEmoticon = mtproto.MakeFlagsString(dialogExt.ThemeEmoticon)
					userFull.TtlPeriod = mtproto.MakeFlagsInt32(dialogExt.TtlPeriod)
					if dialogExt.WallpaperId != 0 && c.svcCtx.Dao.WallpaperPlugin != nil {
						userFull.Wallpaper = c.svcCtx.Dao.WallpaperPlugin.GetChatWallpaper(c.ctx, c.MD.UserId, dialogExt.WallpaperId)
						userFull.WallpaperOverridden = true
					}
				}
			}
		},
		func() {
			rules, _ := c.svcCtx.Dao.UserClient.UserGetPrivacy(c.ctx, &userpb.TLUserGetPrivacy{
				UserId:  peerId,
				KeyType: mtproto.VOICE_MESSAGES,
			})
			if rules != nil && len(rules.Datas) > 0 {
				allow := mtproto.CheckPrivacyIsAllow(
					peerId,
					rules.Datas,
					c.MD.UserId,
					func(id, checkId int64) bool {
						contact, _ := user.CheckContact(checkId)
						return contact
					},
					func(checkId int64, idList []int64) bool {
						// TODO
						chatIdList, _ := mtproto.SplitChatAndChannelIdList(idList)
						_ = chatIdList
						// return c.svcCtx.Dao.ChatClient.CheckParticipantIsExist(c.ctx, checkId, chatIdList)
						return false
					})
				userFull.VoiceMessagesForbidden = !allow
			}
		},
		func() {
			if user.GetUser().GetSavedMusic() != nil {
				rules, _ := c.svcCtx.Dao.UserClient.UserGetPrivacy(c.ctx, &userpb.TLUserGetPrivacy{
					UserId:  peerId,
					KeyType: mtproto.SAVED_MUSIC,
				})
				if rules != nil && len(rules.Datas) > 0 {
					allow := mtproto.CheckPrivacyIsAllow(
						peerId,
						rules.Datas,
						c.MD.UserId,
						func(id, checkId int64) bool {
							contact, _ := user.CheckContact(checkId)
							return contact
						},
						func(checkId int64, idList []int64) bool {
							// TODO
							chatIdList, _ := mtproto.SplitChatAndChannelIdList(idList)
							_ = chatIdList
							// return c.svcCtx.Dao.ChatClient.CheckParticipantIsExist(c.ctx, checkId, chatIdList)
							return false
						})
					if allow {
						userFull.SavedMusic = user.GetUser().GetSavedMusic()
					}
				}
			}
		})

	// TODO: FolderId:    0,

	// TODO: WallPaper

	// Active stories follow the story's privacy rules (everyone, contacts,
	// close friends, selected users). The plugin returns nil when nothing is visible.
	if c.svcCtx.Dao.StoryPlugin != nil {
		userFull.StoriesPinnedAvailable = c.svcCtx.Dao.StoryPlugin.GetStoriesPinnedAvailable(c.ctx, peerId, c.MD.UserId)
		userFull.BlockedMyStoriesFrom = c.svcCtx.Dao.StoryPlugin.GetBlockedMyStoriesFrom(c.ctx, peerId, c.MD.UserId)
		userFull.Stories_FLAGPEERSTORIES = c.svcCtx.Dao.StoryPlugin.GetActiveStories(c.ctx, peerId, c.MD.UserId)
	}

	chats := make([]*mtproto.Chat, 0)

	if c.svcCtx.Dao.PersonalChannelPlugin != nil {
		personalChannelId := user.GetUser().GetPersonalChannelId()
		if personalChannelId != 0 {
			userFull.PersonalChannelId = mtproto.MakeFlagsInt64(personalChannelId)
			pChannel, topMessageId := c.svcCtx.Dao.PersonalChannelPlugin.GetPersonalChannel(c.ctx, personalChannelId, c.MD.UserId)
			if pChannel != nil {
				userFull.PersonalChannelMessage = &wrapperspb.Int32Value{Value: topMessageId}
				chats = append(chats, pChannel)
			}
		}
	}

	if c.MD.UserId != peerId {
		// userFull.Birthday = user.Birthday()
		if user.Birthday() != nil {
			//if user.GetUser().GetSavedMusic() != nil {
			rules, _ := c.svcCtx.Dao.UserClient.UserGetPrivacy(c.ctx, &userpb.TLUserGetPrivacy{
				UserId:  peerId,
				KeyType: mtproto.BIRTHDAY,
			})
			if rules != nil && len(rules.Datas) > 0 {
				allow := mtproto.CheckPrivacyIsAllow(
					peerId,
					rules.Datas,
					c.MD.UserId,
					func(id, checkId int64) bool {
						contact, _ := user.CheckContact(checkId)
						return contact
					},
					func(checkId int64, idList []int64) bool {
						// TODO
						chatIdList, _ := mtproto.SplitChatAndChannelIdList(idList)
						_ = chatIdList
						// return c.svcCtx.Dao.ChatClient.CheckParticipantIsExist(c.ctx, checkId, chatIdList)
						return false
					})
				if allow {
					userFull.Birthday = user.Birthday()
				}
			}
			//}
		}
	} else {
		userFull.Birthday = user.Birthday()
	}

	if c.MD.UserId == 777000 {
		userFull.PhoneCallsAvailable = false
		userFull.PhoneCallsPrivate = false
		userFull.VideoCallsAvailable = false
	}

	unsafeUser := user.ToUnsafeUser(me)
	patchBotUsernameFromImmutable(unsafeUser, user)

	return mtproto.MakeTLUsersUserFull(&mtproto.Users_UserFull{
		FullUser: userFull,
		Chats:    chats,
		Users:    []*mtproto.User{unsafeUser},
	}).To_Users_UserFull(), nil
}
