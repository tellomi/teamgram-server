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

package server

import (
	"flag"

	"github.com/teamgram/proto/mtproto"
	account_helper "github.com/teamgram/teamgram-server/app/bff/account"
	authorization_helper "github.com/teamgram/teamgram-server/app/bff/authorization"
	autodownload_helper "github.com/teamgram/teamgram-server/app/bff/autodownload"
	"github.com/teamgram/teamgram-server/app/bff/bff/internal/config"
	channels_helper "github.com/teamgram/teamgram-server/app/bff/channels"
	chatinvites_helper "github.com/teamgram/teamgram-server/app/bff/chatinvites"
	chats_helper "github.com/teamgram/teamgram-server/app/bff/chats"
	configuration_helper "github.com/teamgram/teamgram-server/app/bff/configuration"
	contacts_helper "github.com/teamgram/teamgram-server/app/bff/contacts"
	deeplinks_helper "github.com/teamgram/teamgram-server/app/bff/deeplinks"
	dialogs_helper "github.com/teamgram/teamgram-server/app/bff/dialogs"
	drafts_helper "github.com/teamgram/teamgram-server/app/bff/drafts"
	files_helper "github.com/teamgram/teamgram-server/app/bff/files"
	folders_helper "github.com/teamgram/teamgram-server/app/bff/folders"
	langpack_helper "github.com/teamgram/teamgram-server/app/bff/langpack"
	messages_helper "github.com/teamgram/teamgram-server/app/bff/messages"
	miscellaneous_helper "github.com/teamgram/teamgram-server/app/bff/miscellaneous"
	notification_helper "github.com/teamgram/teamgram-server/app/bff/notification"
	nsfw_helper "github.com/teamgram/teamgram-server/app/bff/nsfw"
	passkeyhelper "github.com/teamgram/teamgram-server/app/bff/passkey"
	passport_helper "github.com/teamgram/teamgram-server/app/bff/passport"
	phone_helper "github.com/teamgram/teamgram-server/app/bff/phone"
	premium_helper "github.com/teamgram/teamgram-server/app/bff/premium"
	privacysettingshelper "github.com/teamgram/teamgram-server/app/bff/privacysettings"
	qrcode_helper "github.com/teamgram/teamgram-server/app/bff/qrcode"
	savedmessagedialogshelper "github.com/teamgram/teamgram-server/app/bff/savedmessagedialogs"
	sponsoredmessages_helper "github.com/teamgram/teamgram-server/app/bff/sponsoredmessages"
	stickers_helper "github.com/teamgram/teamgram-server/app/bff/stickers"
	stories_helper "github.com/teamgram/teamgram-server/app/bff/stories"
	tos_helper "github.com/teamgram/teamgram-server/app/bff/tos"
	twofa_helper "github.com/teamgram/teamgram-server/app/bff/twofa"
	updates_helper "github.com/teamgram/teamgram-server/app/bff/updates"
	userchannelprofileshelper "github.com/teamgram/teamgram-server/app/bff/userchannelprofiles"
	usernames_helper "github.com/teamgram/teamgram-server/app/bff/usernames"
	users_helper "github.com/teamgram/teamgram-server/app/bff/users"
	webbrowserhelper "github.com/teamgram/teamgram-server/app/bff/webbrowser"
	"github.com/teamgram/teamgram-server/pkg/bootstrap"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
)

var configFile = flag.String("f", "etc/bff.yaml", "the config file")

type Server struct {
	grpcSrv *zrpc.RpcServer
}

func New() *Server {
	return new(Server)
}

func (s *Server) Initialize() error {
	var c config.Config
	conf.MustLoad(*configFile, &c)
	bootstrap.Wait("bff", bootstrap.Deps{
		Kafka: bootstrap.ProducerBrokers(c.SyncClient),
		MySQL: []string{c.ChannelMySQL, c.StoryMySQL},
		Redis: bootstrap.RedisHosts(c.KV),
		Etcd:  c.Etcd.Hosts,
	})

	logx.Infov(c)
	channelBridge := channels_helper.Bridge{}

	s.grpcSrv = zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		// tos_helper
		mtproto.RegisterRPCDeepLinksServer(
			grpcServer,
			deeplinks_helper.New(deeplinks_helper.Config{
				RpcServerConf: c.RpcServerConf,
			}))

		mtproto.RegisterRPCTosServer(
			grpcServer,
			tos_helper.New(tos_helper.Config{
				RpcServerConf: c.RpcServerConf,
				Mysql:         c.ChannelMySQL,
				Tos:           c.Tos,
			}))

		mtproto.RegisterRPCLangpackServer(
			grpcServer,
			langpack_helper.New(langpack_helper.Config{
				RpcServerConf: c.RpcServerConf,
			}))

		// configuration_helper
		configSvc := configuration_helper.New(configuration_helper.Config{
			RpcServerConf: c.RpcServerConf,
			MysqlDSN:      c.ChannelMySQL,
		})
		mtproto.RegisterRPCConfigurationServer(grpcServer, configSvc)
		mtproto.RegisterRPCPromoDataServer(grpcServer, configSvc)

		// qrcode_helper
		mtproto.RegisterRPCQrCodeServer(
			grpcServer,
			qrcode_helper.New(
				qrcode_helper.Config{
					RpcServerConf:     c.RpcServerConf,
					KV:                c.KV,
					UserClient:        c.BizServiceClient,
					AuthSessionClient: c.AuthSessionClient,
					SyncClient:        c.SyncClient,
				},
				nil))

		// miscellaneous_helper
		mtproto.RegisterRPCMiscellaneousServer(
			grpcServer,
			miscellaneous_helper.New(miscellaneous_helper.Config{
				RpcServerConf: c.RpcServerConf,
			}))

		// authorization_helper
		mtproto.RegisterRPCAuthorizationServer(
			grpcServer,
			authorization_helper.New(
				authorization_helper.Config{
					RpcServerConf:             c.RpcServerConf,
					KV:                        c.KV,
					Code:                      c.Code,
					UserClient:                c.BizServiceClient,
					AuthsessionClient:         c.AuthSessionClient,
					ChatClient:                c.BizServiceClient,
					StatusClient:              c.StatusClient,
					SyncClient:                c.SyncClient,
					MsgClient:                 c.MsgClient,
					SignInMessage:             c.SignInMessage,
					SignInServiceNotification: c.SignInServiceNotification,
					TestPhones:                c.TestPhones,
				},
				nil,
				nil))

		stickerSvc := stickers_helper.New(stickers_helper.Config{MySQL: c.ChannelMySQL})
		mtproto.RegisterRPCStickersServer(grpcServer, stickerSvc)
		mtproto.RegisterRPCEmojiServer(grpcServer, stickerSvc)
		mtproto.RegisterRPCCustomEmojisServer(grpcServer, stickerSvc)
		mtproto.RegisterRPCEmojiCategoriesServer(grpcServer, stickers_helper.NewEmojiCategories())

		// premium_helper
		premiumSvc := premium_helper.New(premium_helper.Config{
			RpcServerConf: c.RpcServerConf,
			UserClient:    c.BizServiceClient,
			SyncClient:    c.SyncClient,
		})
		mtproto.RegisterRPCPremiumServer(grpcServer, premiumSvc)
		mtproto.RegisterRPCEmojiStatusServer(grpcServer, premiumSvc)
		mtproto.RegisterRPCAccentColorsServer(grpcServer, premiumSvc)
		mtproto.RegisterRPCTranscriptionServer(grpcServer, premiumSvc)

		// chatinvites_helper
		mtproto.RegisterRPCChatInvitesServer(
			grpcServer,
			chatinvites_helper.New(chatinvites_helper.Config{
				RpcServerConf: c.RpcServerConf,
				UserClient:    c.BizServiceClient,
				ChatClient:    c.BizServiceClient,
				MsgClient:     c.MsgClient,
				SyncClient:    c.SyncClient,
			}))

		mtproto.RegisterRPCChannelsServer(
			grpcServer,
			channels_helper.New(channels_helper.Config{
				RpcServerConf: c.RpcServerConf,
				ChannelMySQL:  c.ChannelMySQL,
				UserClient:    c.BizServiceClient,
				DialogClient:  c.BizServiceClient,
				MediaClient:   c.MediaClient,
				SyncClient:    c.SyncClient,
			}))
		mtproto.RegisterRPCForumsServer(grpcServer, channels_helper.NewForums())
		mtproto.RegisterRPCReactionsServer(grpcServer, channels_helper.NewReactions())
		mtproto.RegisterRPCMessageThreadsServer(grpcServer, channels_helper.NewThreads(c.BizServiceClient, c.BizServiceClient, c.MsgClient, c.SyncClient))
		mtproto.RegisterRPCStatisticsServer(grpcServer, channels_helper.NewStats())

		storyMySQL := c.StoryMySQL
		if storyMySQL == "" {
			storyMySQL = c.ChannelMySQL
		}
		mtproto.RegisterRPCStoriesServer(
			grpcServer,
			stories_helper.New(stories_helper.Config{
				RpcServerConf: c.RpcServerConf,
				StoryMySQL:    storyMySQL,
				UserClient:    c.BizServiceClient,
				MediaClient:   c.MediaClient,
				SyncClient:    c.SyncClient,
			}))

		phoneCfg := c.Phone
		if phoneCfg.MySQL == "" {
			phoneCfg.MySQL = c.ChannelMySQL
		}
		phoneSvc := phone_helper.New(phoneCfg, phone_helper.Clients{
			User: c.BizServiceClient,
			Chat: c.BizServiceClient,
			Msg:  c.MsgClient,
			Sync: c.SyncClient,
		})
		mtproto.RegisterRPCVoipCallsServer(grpcServer, phoneSvc)
		mtproto.RegisterRPCGroupCallsServer(grpcServer, phoneSvc)
		mtproto.RegisterRPCSecretChatsServer(grpcServer, phoneSvc)

		// chats_helper
		mtproto.RegisterRPCChatsServer(
			grpcServer,
			chats_helper.New(chats_helper.Config{
				RpcServerConf:     c.RpcServerConf,
				UserClient:        c.BizServiceClient,
				ChatClient:        c.BizServiceClient,
				MsgClient:         c.MsgClient,
				DialogClient:      c.BizServiceClient,
				SyncClient:        c.SyncClient,
				MediaClient:       c.MediaClient,
				AuthsessionClient: c.AuthSessionClient,
				IdgenClient:       c.IdgenClient,
				MessageClient:     c.BizServiceClient,
			}))

		// files_helper
		mtproto.RegisterRPCFilesServer(
			grpcServer,
			files_helper.New(files_helper.Config{
				RpcServerConf: c.RpcServerConf,
				DfsClient:     c.DfsClient,
				UserClient:    c.BizServiceClient,
				MediaClient:   c.MediaClient,
			}, stickers_helper.Thumb{}, nil))

		// passport_helper
		mtproto.RegisterRPCPassportServer(
			grpcServer,
			passport_helper.New(passport_helper.Config{
				RpcServerConf:     c.RpcServerConf,
				AuthsessionClient: c.AuthSessionClient,
				UserClient:        c.BizServiceClient,
			}))

		// updates_helper
		mtproto.RegisterRPCUpdatesServer(
			grpcServer,
			updates_helper.New(updates_helper.Config{
				RpcServerConf:     c.RpcServerConf,
				UpdatesClient:     c.BizServiceClient,
				UserClient:        c.BizServiceClient,
				ChatClient:        c.BizServiceClient,
				AuthsessionClient: c.AuthSessionClient,
			}))

		// contacts_helper
		contactsSvc := contacts_helper.New(
			contacts_helper.Config{
				RpcServerConf: c.RpcServerConf,
				KV:            c.KV,
				UserClient:    c.BizServiceClient,
				ChatClient:    c.BizServiceClient,
				SyncClient:    c.SyncClient,
			},
			channelBridge)
		mtproto.RegisterRPCContactsServer(grpcServer, contactsSvc)
		mtproto.RegisterRPCProfileLinksServer(grpcServer, contactsSvc)
		mtproto.RegisterRPCNoPaidMessagesServer(grpcServer, contactsSvc)

		// dialogs_helper
		mtproto.RegisterRPCDialogsServer(
			grpcServer,
			dialogs_helper.New(dialogs_helper.Config{
				RpcServerConf: c.RpcServerConf,
				UpdatesClient: c.BizServiceClient,
				UserClient:    c.BizServiceClient,
				ChatClient:    c.BizServiceClient,
				DialogClient:  c.BizServiceClient,
				SyncClient:    c.SyncClient,
				MessageClient: c.BizServiceClient,
			}, channelBridge))

		folderSvc := folders_helper.New(folders_helper.Config{
			RpcServerConf: c.RpcServerConf,
			DialogClient:  c.BizServiceClient,
			UserClient:    c.BizServiceClient,
			SyncClient:    c.SyncClient,
		})
		mtproto.RegisterRPCFoldersServer(grpcServer, folderSvc)
		mtproto.RegisterRPCFolderTagsServer(grpcServer, folderSvc)

		// drafts_helper
		mtproto.RegisterRPCDraftsServer(
			grpcServer,
			drafts_helper.New(drafts_helper.Config{
				RpcServerConf: c.RpcServerConf,
				DialogClient:  c.BizServiceClient,
				UserClient:    c.BizServiceClient,
				SyncClient:    c.SyncClient,
				ChatClient:    c.BizServiceClient,
			}, channelBridge))

		// autodownload_helper
		mtproto.RegisterRPCAutoDownloadServer(
			grpcServer,
			autodownload_helper.New(autodownload_helper.Config{
				RpcServerConf: c.RpcServerConf,
			}))

		// messages_helper
		mtproto.RegisterRPCMessagesServer(
			grpcServer,
			messages_helper.New(messages_helper.Config{
				RpcServerConf: c.RpcServerConf,
				UserClient:    c.BizServiceClient,
				ChatClient:    c.BizServiceClient,
				MsgClient:     c.MsgClient,
				DialogClient:  c.BizServiceClient,
				IdgenClient:   c.IdgenClient,
				MessageClient: c.BizServiceClient,
				MediaClient:   c.MediaClient,
				SyncClient:    c.SyncClient,
			}, nil))

		// notification_helper
		notifySvc := notification_helper.New(notification_helper.Config{
			RpcServerConf: c.RpcServerConf,
			UserClient:    c.BizServiceClient,
			ChatClient:    c.BizServiceClient,
			SyncClient:    c.SyncClient,
			MysqlDSN:      c.ChannelMySQL,
		}, channelBridge)
		mtproto.RegisterRPCNotificationServer(grpcServer, notifySvc)
		mtproto.RegisterRPCReactionNotificationServer(grpcServer, notifySvc)

		twofaSvc := twofa_helper.New()
		mtproto.RegisterRPCTwoFaServer(grpcServer, twofaSvc)
		mtproto.RegisterRPCPaymentsServer(grpcServer, twofaSvc)

		// users_helper
		mtproto.RegisterRPCUsersServer(
			grpcServer,
			users_helper.New(
				users_helper.Config{
					RpcServerConf: c.RpcServerConf,
					UserClient:    c.BizServiceClient,
					ChatClient:    c.BizServiceClient,
					DialogClient:  c.BizServiceClient,
				},
				stories_helper.Plugin{},
				nil,
				nil))

		// nsfw_helper
		mtproto.RegisterRPCNsfwServer(
			grpcServer,
			nsfw_helper.New(nsfw_helper.Config{
				RpcServerConf: c.RpcServerConf,
				UserClient:    c.BizServiceClient,
			}))

		// sponsoredmessages_helper
		mtproto.RegisterRPCSponsoredMessagesServer(
			grpcServer,
			sponsoredmessages_helper.New(sponsoredmessages_helper.Config{
				RpcServerConf: c.RpcServerConf,
			}))

		// account_helper
		mtproto.RegisterRPCAccountServer(
			grpcServer,
			account_helper.New(
				account_helper.Config{
					RpcServerConf:     c.RpcServerConf,
					KV:                c.KV,
					UserClient:        c.BizServiceClient,
					AuthsessionClient: c.AuthSessionClient,
					ChatClient:        c.BizServiceClient,
					SyncClient:        c.SyncClient,
				},
				nil,
				nil))
		// usernames_helper
		mtproto.RegisterRPCUsernamesServer(
			grpcServer,
			usernames_helper.New(usernames_helper.Config{
				RpcServerConf: c.RpcServerConf,
				UserClient:    c.BizServiceClient,
				ChatClient:    c.BizServiceClient,
				SyncClient:    c.SyncClient,
			}, channelBridge))

		// privacysettingshelper
		mtproto.RegisterRPCPrivacySettingsServer(
			grpcServer,
			privacysettingshelper.New(privacysettingshelper.Config{
				RpcServerConf:     c.RpcServerConf,
				UserClient:        c.BizServiceClient,
				AuthsessionClient: c.AuthSessionClient,
				ChatClient:        c.BizServiceClient,
				SyncClient:        c.SyncClient,
			}))

		// savedmessagedialogshelper
		mtproto.RegisterRPCSavedMessageDialogsServer(
			grpcServer,
			savedmessagedialogshelper.New(savedmessagedialogshelper.Config{
				RpcServerConf: c.RpcServerConf,
				UpdatesClient: c.BizServiceClient,
				UserClient:    c.BizServiceClient,
				ChatClient:    c.BizServiceClient,
				DialogClient:  c.BizServiceClient,
				SyncClient:    c.SyncClient,
				MessageClient: c.BizServiceClient,
			}))

		// userchannelprofileshelper
		mtproto.RegisterRPCUserChannelProfilesServer(
			grpcServer,
			userchannelprofileshelper.New(userchannelprofileshelper.Config{
				RpcServerConf: c.RpcServerConf,
				MediaClient:   c.MediaClient,
				UserClient:    c.BizServiceClient,
				SyncClient:    c.SyncClient,
			}))

		mtproto.RegisterRPCPasskeyServer(
			grpcServer,
			passkeyhelper.New(passkeyhelper.Config{
				RpcServerConf: c.RpcServerConf,
			}))

		mtproto.RegisterRPCWebBrowserServer(
			grpcServer,
			webbrowserhelper.New(webbrowserhelper.Config{
				RpcServerConf: c.RpcServerConf,
			}))
	})

	// logx.Must(err)

	go func() {
		s.grpcSrv.Start()
	}()
	return nil
}

func (s *Server) RunLoop() {
}

func (s *Server) Destroy() {
	s.grpcSrv.Stop()
}
