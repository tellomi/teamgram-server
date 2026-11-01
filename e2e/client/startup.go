package e2e

import (
	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// startupCall is one RPC official iOS and Android clients issue while the
// chat list still says Updating. A new regression adds a row here.
type startupCall struct {
	Name string
	Req  mtproto.TLObject
}

func startupCalls(profile Profile) []startupCall {
	emptyPeer := inputPeerEmpty()
	selfPeer := inputPeerSelf()
	notify := func(name string, peer *mtproto.InputNotifyPeer) startupCall {
		return startupCall{name, &mtproto.TLAccountGetNotifySettings{
			Constructor: mtproto.CRC32_account_getNotifySettings,
			Peer:        peer,
		}}
	}
	privacy := func(name string, key *mtproto.InputPrivacyKey) startupCall {
		return startupCall{name, &mtproto.TLAccountGetPrivacy{
			Constructor: mtproto.CRC32_account_getPrivacy,
			Key:         key,
		}}
	}
	return []startupCall{
		{"help.getConfig", &mtproto.TLHelpGetConfig{Constructor: mtproto.CRC32_help_getConfig}},
		{"help.getNearestDc", &mtproto.TLHelpGetNearestDc{Constructor: mtproto.CRC32_help_getNearestDc}},
		{"help.getAppConfig", &mtproto.TLHelpGetAppConfig61E3F854{Constructor: mtproto.CRC32_help_getAppConfig61E3F854}},
		{"help.getTermsOfServiceUpdate", &mtproto.TLHelpGetTermsOfServiceUpdate{Constructor: mtproto.CRC32_help_getTermsOfServiceUpdate}},
		{"help.getCountriesList", &mtproto.TLHelpGetCountriesList{Constructor: mtproto.CRC32_help_getCountriesList, LangCode: profile.LangCode}},
		{"help.getPeerColors", &mtproto.TLHelpGetPeerColors{Constructor: mtproto.CRC32_help_getPeerColors}},
		{"help.getPeerProfileColors", &mtproto.TLHelpGetPeerProfileColors{Constructor: mtproto.CRC32_help_getPeerProfileColors}},
		{"help.getPremiumPromo", &mtproto.TLHelpGetPremiumPromo{Constructor: mtproto.CRC32_help_getPremiumPromo}},
		{"help.getTimezonesList", &mtproto.TLHelpGetTimezonesList{Constructor: mtproto.CRC32_help_getTimezonesList}},
		{"help.getCdnConfig", &mtproto.TLHelpGetCdnConfig{Constructor: mtproto.CRC32_help_getCdnConfig}},
		{"langpack.getLanguages", &mtproto.TLLangpackGetLanguages{Constructor: mtproto.CRC32_langpack_getLanguages_42c6978f, LangPack: profile.LangPack}},
		{"langpack.getLangPack", &mtproto.TLLangpackGetLangPack{Constructor: mtproto.CRC32_langpack_getLangPack_f2f2330a, LangPack: profile.LangPack, LangCode: profile.LangCode}},
		{"langpack.getDifference", &mtproto.TLLangpackGetDifference{Constructor: mtproto.CRC32_langpack_getDifference, LangPack: profile.LangPack, LangCode: profile.LangCode}},
		{"updates.getState", &mtproto.TLUpdatesGetState{Constructor: mtproto.CRC32_updates_getState}},
		{"updates.getDifference", &mtproto.TLUpdatesGetDifference{Constructor: mtproto.CRC32_updates_getDifference_19c2f763, PtsTotalLimit: &wrapperspb.Int32Value{Value: 100}}},
		{"messages.getDialogs", &mtproto.TLMessagesGetDialogs{Constructor: mtproto.CRC32_messages_getDialogs, OffsetPeer: emptyPeer, Limit: 100}},
		{"messages.getPinnedDialogs", &mtproto.TLMessagesGetPinnedDialogs{Constructor: mtproto.CRC32_messages_getPinnedDialogs}},
		{"messages.getPeerDialogs", &mtproto.TLMessagesGetPeerDialogs{Constructor: mtproto.CRC32_messages_getPeerDialogs, Peers: []*mtproto.InputDialogPeer{}}},
		{"contacts.getContacts", &mtproto.TLContactsGetContacts{Constructor: mtproto.CRC32_contacts_getContacts}},
		{"contacts.getStatuses", &mtproto.TLContactsGetStatuses{Constructor: mtproto.CRC32_contacts_getStatuses}},
		notify("account.getNotifySettings/users", mtproto.MakeTLInputNotifyUsers(nil).To_InputNotifyPeer()),
		notify("account.getNotifySettings/chats", mtproto.MakeTLInputNotifyChats(nil).To_InputNotifyPeer()),
		notify("account.getNotifySettings/channels", mtproto.MakeTLInputNotifyBroadcasts(nil).To_InputNotifyPeer()),
		{"account.updateStatus", &mtproto.TLAccountUpdateStatus{Constructor: mtproto.CRC32_account_updateStatus, Offline: mtproto.MakeTLBoolFalse(nil).To_Bool()}},
		{"account.getPassword", &mtproto.TLAccountGetPassword{Constructor: mtproto.CRC32_account_getPassword}},
		{"account.getAccountTTL", &mtproto.TLAccountGetAccountTTL{Constructor: mtproto.CRC32_account_getAccountTTL}},
		{"account.getContentSettings", &mtproto.TLAccountGetContentSettings{Constructor: mtproto.CRC32_account_getContentSettings}},
		{"account.getGlobalPrivacySettings", &mtproto.TLAccountGetGlobalPrivacySettings{Constructor: mtproto.CRC32_account_getGlobalPrivacySettings}},
		{"account.getAutoSaveSettings", &mtproto.TLAccountGetAutoSaveSettings{Constructor: mtproto.CRC32_account_getAutoSaveSettings}},
		{"account.getAutoDownloadSettings", &mtproto.TLAccountGetAutoDownloadSettings{Constructor: mtproto.CRC32_account_getAutoDownloadSettings}},
		{"account.getContactSignUpNotification", &mtproto.TLAccountGetContactSignUpNotification{Constructor: mtproto.CRC32_account_getContactSignUpNotification}},
		{"account.getReactionsNotifySettings", &mtproto.TLAccountGetReactionsNotifySettings{Constructor: mtproto.CRC32_account_getReactionsNotifySettings}},
		{"account.getThemes", &mtproto.TLAccountGetThemes{Constructor: mtproto.CRC32_account_getThemes, Format: profile.LangPack}},
		{"account.getSavedRingtones", &mtproto.TLAccountGetSavedRingtones{Constructor: mtproto.CRC32_account_getSavedRingtones}},
		{"account.getDefaultEmojiStatuses", &mtproto.TLAccountGetDefaultEmojiStatuses{Constructor: mtproto.CRC32_account_getDefaultEmojiStatuses}},
		{"account.getRecentEmojiStatuses", &mtproto.TLAccountGetRecentEmojiStatuses{Constructor: mtproto.CRC32_account_getRecentEmojiStatuses}},
		{"account.getCollectibleEmojiStatuses", &mtproto.TLAccountGetCollectibleEmojiStatuses{Constructor: mtproto.CRC32_account_getCollectibleEmojiStatuses}},
		{"account.getChannelDefaultEmojiStatuses", &mtproto.TLAccountGetChannelDefaultEmojiStatuses{Constructor: mtproto.CRC32_account_getChannelDefaultEmojiStatuses}},
		{"account.getDefaultProfilePhotoEmojis", &mtproto.TLAccountGetDefaultProfilePhotoEmojis{Constructor: mtproto.CRC32_account_getDefaultProfilePhotoEmojis}},
		privacy("account.getPrivacy/statusTimestamp", mtproto.MakeTLInputPrivacyKeyStatusTimestamp(nil).To_InputPrivacyKey()),
		privacy("account.getPrivacy/chatInvite", mtproto.MakeTLInputPrivacyKeyChatInvite(nil).To_InputPrivacyKey()),
		privacy("account.getPrivacy/phoneCall", mtproto.MakeTLInputPrivacyKeyPhoneCall(nil).To_InputPrivacyKey()),
		privacy("account.getPrivacy/phoneP2P", mtproto.MakeTLInputPrivacyKeyPhoneP2P(nil).To_InputPrivacyKey()),
		privacy("account.getPrivacy/forwards", mtproto.MakeTLInputPrivacyKeyForwards(nil).To_InputPrivacyKey()),
		privacy("account.getPrivacy/profilePhoto", mtproto.MakeTLInputPrivacyKeyProfilePhoto(nil).To_InputPrivacyKey()),
		privacy("account.getPrivacy/phoneNumber", mtproto.MakeTLInputPrivacyKeyPhoneNumber(nil).To_InputPrivacyKey()),
		privacy("account.getPrivacy/addedByPhone", mtproto.MakeTLInputPrivacyKeyAddedByPhone(nil).To_InputPrivacyKey()),
		privacy("account.getPrivacy/voiceMessages", mtproto.MakeTLInputPrivacyKeyVoiceMessages(nil).To_InputPrivacyKey()),
		privacy("account.getPrivacy/about", mtproto.MakeTLInputPrivacyKeyAbout(nil).To_InputPrivacyKey()),
		privacy("account.getPrivacy/birthday", mtproto.MakeTLInputPrivacyKeyBirthday(nil).To_InputPrivacyKey()),
		{"users.getFullUser", &mtproto.TLUsersGetFullUser{Constructor: mtproto.CRC32_users_getFullUser, Id: inputUserSelf()}},
		{"messages.getAllStickers", &mtproto.TLMessagesGetAllStickers{Constructor: mtproto.CRC32_messages_getAllStickers}},
		{"messages.getEmojiStickers", &mtproto.TLMessagesGetEmojiStickers{Constructor: mtproto.CRC32_messages_getEmojiStickers}},
		{"messages.getFeaturedStickers", &mtproto.TLMessagesGetFeaturedStickers{Constructor: mtproto.CRC32_messages_getFeaturedStickers}},
		{"messages.getFeaturedEmojiStickers", &mtproto.TLMessagesGetFeaturedEmojiStickers{Constructor: mtproto.CRC32_messages_getFeaturedEmojiStickers}},
		{"messages.getRecentStickers", &mtproto.TLMessagesGetRecentStickers{Constructor: mtproto.CRC32_messages_getRecentStickers}},
		{"messages.getFavedStickers", &mtproto.TLMessagesGetFavedStickers{Constructor: mtproto.CRC32_messages_getFavedStickers}},
		{"messages.getMaskStickers", &mtproto.TLMessagesGetMaskStickers{Constructor: mtproto.CRC32_messages_getMaskStickers}},
		{"messages.getSavedGifs", &mtproto.TLMessagesGetSavedGifs{Constructor: mtproto.CRC32_messages_getSavedGifs}},
		{"messages.getStickers", &mtproto.TLMessagesGetStickers{Constructor: mtproto.CRC32_messages_getStickers, Emoticon: "👍"}},
		{"messages.getEmojiStickerGroups", &mtproto.TLMessagesGetEmojiStickerGroups{Constructor: mtproto.CRC32_messages_getEmojiStickerGroups}},
		{"messages.getEmojiStatusGroups", &mtproto.TLMessagesGetEmojiStatusGroups{Constructor: mtproto.CRC32_messages_getEmojiStatusGroups}},
		{"messages.getEmojiProfilePhotoGroups", &mtproto.TLMessagesGetEmojiProfilePhotoGroups{Constructor: mtproto.CRC32_messages_getEmojiProfilePhotoGroups}},
		{"messages.getEmojiGroups", &mtproto.TLMessagesGetEmojiGroups{Constructor: mtproto.CRC32_messages_getEmojiGroups}},
		{"messages.getAvailableReactions", &mtproto.TLMessagesGetAvailableReactions{Constructor: mtproto.CRC32_messages_getAvailableReactions}},
		{"messages.getTopReactions", &mtproto.TLMessagesGetTopReactions{Constructor: mtproto.CRC32_messages_getTopReactions, Limit: 100}},
		{"messages.getRecentReactions", &mtproto.TLMessagesGetRecentReactions{Constructor: mtproto.CRC32_messages_getRecentReactions, Limit: 50}},
		{"messages.getDefaultTagReactions", &mtproto.TLMessagesGetDefaultTagReactions{Constructor: mtproto.CRC32_messages_getDefaultTagReactions}},
		{"messages.getAvailableEffects", &mtproto.TLMessagesGetAvailableEffects{Constructor: mtproto.CRC32_messages_getAvailableEffects}},
		{"messages.getCustomEmojiDocuments", &mtproto.TLMessagesGetCustomEmojiDocuments{Constructor: mtproto.CRC32_messages_getCustomEmojiDocuments, DocumentId: []int64{}}},
		{"messages.getEmojiKeywords", &mtproto.TLMessagesGetEmojiKeywords{Constructor: mtproto.CRC32_messages_getEmojiKeywords, LangCode: profile.LangCode}},
		{"messages.getEmojiKeywordsDifference", &mtproto.TLMessagesGetEmojiKeywordsDifference{Constructor: mtproto.CRC32_messages_getEmojiKeywordsDifference, LangCode: profile.LangCode}},
		{"messages.getEmojiKeywordsLanguages", &mtproto.TLMessagesGetEmojiKeywordsLanguages{Constructor: mtproto.CRC32_messages_getEmojiKeywordsLanguages, LangCodes: []string{profile.LangCode}}},
		{"messages.getDialogFilters", &mtproto.TLMessagesGetDialogFiltersF19ED96D{Constructor: mtproto.CRC32_messages_getDialogFiltersF19ED96D}},
		{"messages.getSuggestedDialogFilters", &mtproto.TLMessagesGetSuggestedDialogFilters{Constructor: mtproto.CRC32_messages_getSuggestedDialogFilters}},
		{"messages.getAttachMenuBots", &mtproto.TLMessagesGetAttachMenuBots{Constructor: mtproto.CRC32_messages_getAttachMenuBots}},
		{"messages.getDefaultHistoryTTL", &mtproto.TLMessagesGetDefaultHistoryTTL{Constructor: mtproto.CRC32_messages_getDefaultHistoryTTL}},
		{"messages.getQuickReplies", &mtproto.TLMessagesGetQuickReplies{Constructor: mtproto.CRC32_messages_getQuickReplies}},
		{"messages.getSavedDialogs", &mtproto.TLMessagesGetSavedDialogs{Constructor: mtproto.CRC32_messages_getSavedDialogs_5381d21a, OffsetPeer: emptyPeer, Limit: 20}},
		{"messages.getStickerSet/animatedEmoji", &mtproto.TLMessagesGetStickerSet{
			Constructor: mtproto.CRC32_messages_getStickerSet_c8a0ec74,
			Stickerset:  mtproto.MakeTLInputStickerSetAnimatedEmoji(nil).To_InputStickerSet(),
		}},
		{"phone.getCallConfig", &mtproto.TLPhoneGetCallConfig{Constructor: mtproto.CRC32_phone_getCallConfig}},
		{"stories.getAllStories", &mtproto.TLStoriesGetAllStories{Constructor: mtproto.CRC32_stories_getAllStories}},
		{"stories.getPeerMaxIDs", &mtproto.TLStoriesGetPeerMaxIDs78499170{
			Constructor: mtproto.CRC32_stories_getPeerMaxIDs78499170,
			Id:          []*mtproto.InputPeer{selfPeer},
		}},
	}
}
