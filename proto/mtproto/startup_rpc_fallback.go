package mtproto

import (
	"reflect"
	"strings"
	"time"
)

// StartupRPCResult is the typed empty or NotModified reply for an RPC the
// official iOS and Android clients call during startup. ok is false when the
// method is not on that list. Callers then return 400 METHOD_NOT_IMPL.
// A gRPC "unknown service" must not become 500: clients retry 500 and stay
// on "Updating...".
func StartupRPCResult(object TLObject) (TLObject, bool) {
	if object == nil {
		return nil, false
	}
	switch in := object.(type) {
	case *TLHelpGetConfig:
		now := int32(time.Now().Unix())
		return MakeTLConfig(&Config{
			Date:      now,
			Expires:   now + 3600,
			TestMode:  BoolFalse,
			ThisDc:    1,
			DcOptions: []*DcOption{},
		}).To_Config(), true
	case *TLHelpGetNearestDc:
		return MakeTLNearestDc(&NearestDc{Country: "", ThisDc: 1, NearestDc: 1}).To_NearestDc(), true
	case *TLHelpGetCdnConfig:
		return MakeTLCdnConfig(&CdnConfig{PublicKeys: []*CdnPublicKey{}}).To_CdnConfig(), true
	case *TLHelpTest, *TLHelpSaveAppLog:
		return BoolTrue, true
	case *TLHelpGetAppUpdate:
		return MakeTLHelpNoAppUpdate(nil).To_Help_AppUpdate(), true
	case *TLHelpGetPremiumPromo:
		return MakeTLHelpPremiumPromo(&Help_PremiumPromo{
			StatusText:     "",
			StatusEntities: []*MessageEntity{},
			VideoSections:  []string{},
			Videos:         []*Document{},
			PeriodOptions:  []*PremiumSubscriptionOption{},
			Users:          []*User{},
		}).To_Help_PremiumPromo(), true
	case *TLHelpGetAppConfig61E3F854:
		return MakeTLHelpAppConfigNotModified(nil).To_Help_AppConfig(), true
	case *TLHelpGetAppConfig98914110:
		return MakeTLJsonNull(nil).To_JSONValue(), true
	case *TLPhoneGetCallConfig:
		return MakeTLDataJSON(&DataJSON{Data: "{}"}).To_DataJSON(), true
	case *TLLangpackGetDifference:
		return MakeTLLangPackDifference(&LangPackDifference{
			LangCode:    in.GetLangCode(),
			FromVersion: in.GetFromVersion(),
			Version:     in.GetFromVersion(),
			Strings:     []*LangPackString{},
		}).To_LangPackDifference(), true
	case *TLAccountGetAutoDownloadSettings:
		one := disabledAutoDownload()
		return MakeTLAccountAutoDownloadSettings(&Account_AutoDownloadSettings{
			Low: one, Medium: disabledAutoDownload(), High: disabledAutoDownload(),
		}).To_Account_AutoDownloadSettings(), true
	case *TLUpdatesGetState:
		return emptyUpdatesState(), true
	case *TLUpdatesGetDifference:
		return MakeTLUpdatesDifferenceEmpty(&Updates_Difference{
			Date: int32(time.Now().Unix()),
		}).To_Updates_Difference(), true
	case *TLUpdatesGetChannelDifference:
		return MakeTLUpdatesChannelDifferenceEmpty(&Updates_ChannelDifference{
			Final: true,
			Pts:   in.GetPts(),
		}).To_Updates_ChannelDifference(), true
	case *TLMessagesReceivedQueue:
		return &Vector_Long{Datas: []int64{}}, true
	case *TLMessagesReceivedMessages:
		return &Vector_ReceivedNotifyMessage{Datas: []*ReceivedNotifyMessage{}}, true
	case *TLMessagesGetDialogs:
		return MakeTLMessagesDialogs(&Messages_Dialogs{
			Dialogs: []*Dialog{}, Messages: []*Message{}, Chats: []*Chat{}, Users: []*User{},
		}).To_Messages_Dialogs(), true
	case *TLMessagesGetPinnedDialogs, *TLMessagesGetPeerDialogs:
		return MakeTLMessagesPeerDialogs(&Messages_PeerDialogs{
			Dialogs: []*Dialog{}, Messages: []*Message{}, Chats: []*Chat{}, Users: []*User{},
			State: emptyUpdatesState(),
		}).To_Messages_PeerDialogs(), true
	case *TLMessagesGetPinnedSavedDialogs:
		return MakeTLMessagesSavedDialogs(&Messages_SavedDialogs{
			Dialogs: []*SavedDialog{}, Messages: []*Message{}, Chats: []*Chat{}, Users: []*User{},
		}).To_Messages_SavedDialogs(), true
	case *TLMessagesGetDefaultHistoryTTL:
		return MakeTLDefaultHistoryTTL(&DefaultHistoryTTL{}).To_DefaultHistoryTTL(), true
	case *TLMessagesGetAllStickers, *TLMessagesGetEmojiStickers, *TLMessagesGetMaskStickers:
		return MakeTLMessagesAllStickersNotModified(nil).To_Messages_AllStickers(), true
	case *TLMessagesGetStickerSet:
		return MakeTLMessagesStickerSetNotModified(nil).To_Messages_StickerSet(), true
	case *TLMessagesGetRecentStickers:
		return MakeTLMessagesRecentStickersNotModified(nil).To_Messages_RecentStickers(), true
	case *TLMessagesGetFavedStickers:
		return MakeTLMessagesFavedStickersNotModified(nil).To_Messages_FavedStickers(), true
	case *TLMessagesGetSavedGifs:
		return MakeTLMessagesSavedGifsNotModified(nil).To_Messages_SavedGifs(), true
	case *TLMessagesGetFeaturedStickers:
		return MakeTLMessagesFeaturedStickersNotModified(nil).To_Messages_FeaturedStickers(), true
	case *TLMessagesGetEmojiKeywords:
		return MakeTLEmojiKeywordsDifference(&EmojiKeywordsDifference{
			LangCode: in.GetLangCode(), Keywords: []*EmojiKeyword{},
		}).To_EmojiKeywordsDifference(), true
	case *TLMessagesGetEmojiKeywordsDifference:
		return MakeTLEmojiKeywordsDifference(&EmojiKeywordsDifference{
			LangCode: in.GetLangCode(), FromVersion: in.GetFromVersion(), Version: in.GetFromVersion(),
			Keywords: []*EmojiKeyword{},
		}).To_EmojiKeywordsDifference(), true
	case *TLMessagesGetEmojiKeywordsLanguages:
		return &Vector_EmojiLanguage{Datas: []*EmojiLanguage{}}, true
	case *TLMessagesGetEmojiGroups, *TLMessagesGetEmojiStatusGroups, *TLMessagesGetEmojiProfilePhotoGroups, *TLMessagesGetEmojiStickerGroups:
		return MakeTLMessagesEmojiGroupsNotModified(nil).To_Messages_EmojiGroups(), true
	case *TLAccountGetDefaultEmojiStatuses, *TLAccountGetRecentEmojiStatuses, *TLAccountGetCollectibleEmojiStatuses, *TLAccountGetChannelDefaultEmojiStatuses:
		return MakeTLAccountEmojiStatusesNotModified(nil).To_Account_EmojiStatuses(), true
	case *TLAccountGetDefaultProfilePhotoEmojis, *TLAccountGetDefaultGroupPhotoEmojis, *TLAccountGetDefaultBackgroundEmojis, *TLAccountGetChannelRestrictedStatusEmojis:
		return MakeTLEmojiListNotModified(nil).To_EmojiList(), true
	case *TLMessagesGetTopReactions, *TLMessagesGetRecentReactions, *TLMessagesGetDefaultTagReactions:
		return MakeTLMessagesReactionsNotModified(nil).To_Messages_Reactions(), true
	case *TLMessagesGetSavedReactionTags:
		return MakeTLMessagesSavedReactionTagsNotModified(nil).To_Messages_SavedReactionTags(), true
	case *TLMessagesGetEmojiGameInfo:
		return MakeTLMessagesEmojiGameUnavailable(nil).To_Messages_EmojiGameInfo(), true
	case *TLAccountGetNotifySettings:
		return MakeTLPeerNotifySettings(&PeerNotifySettings{}).To_PeerNotifySettings(), true
	case *TLAccountGetContactSignUpNotification:
		return BoolFalse, true
	case *TLAccountGetReactionsNotifySettings:
		return MakeTLReactionsNotifySettings(&ReactionsNotifySettings{
			Sound:        MakeTLNotificationSoundNone(nil).To_NotificationSound(),
			ShowPreviews: BoolFalse,
		}).To_ReactionsNotifySettings(), true
	case *TLUsersGetFullUser:
		return MakeTLUsersUserFull(&Users_UserFull{
			FullUser: MakeTLUserFull(&UserFull{
				Settings:       MakeTLPeerSettings(&PeerSettings{}).To_PeerSettings(),
				NotifySettings: MakeTLPeerNotifySettings(&PeerNotifySettings{}).To_PeerNotifySettings(),
			}).To_UserFull(),
			Chats: []*Chat{},
			Users: []*User{MakeTLUserEmpty(&User{}).To_User()},
		}).To_Users_UserFull(), true
	case *TLContactsGetContacts:
		return MakeTLContactsContactsNotModified(nil).To_Contacts_Contacts(), true
	case *TLMessagesGetUnreadMentions, *TLMessagesGetScheduledMessages, *TLMessagesGetQuickReplyMessages:
		return emptyMessages(), true
	default:
		name := requestName(object)
		if strings.HasPrefix(name, "TLStories") || name == "TLUsersGetStoriesMaxIDs" || name == "TLContactsToggleStoriesHidden" {
			return storiesFallback(name)
		}
		return nil, false
	}
}

// FallbackForUnregisteredService is the client-facing result when bff has
// no gRPC service for the method.
func FallbackForUnregisteredService(object TLObject) TLObject {
	if r, ok := StartupRPCResult(object); ok {
		return r
	}
	return NewRpcError(ErrMethodNotImpl)
}

func requestName(object TLObject) string {
	rt := reflect.TypeOf(object)
	if rt == nil {
		return ""
	}
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}
	return rt.Name()
}

func emptyUpdatesState() *Updates_State {
	return MakeTLUpdatesState(&Updates_State{}).To_Updates_State()
}

func emptyMessages() *Messages_Messages {
	return MakeTLMessagesMessages(&Messages_Messages{
		Messages: []*Message{},
		Chats:    []*Chat{},
		Users:    []*User{},
	}).To_Messages_Messages()
}

func emptyUpdates() *Updates {
	return MakeTLUpdates(&Updates{
		Date:    int32(time.Now().Unix()),
		Updates: []*Update{},
		Users:   []*User{},
		Chats:   []*Chat{},
	}).To_Updates()
}

func disabledAutoDownload() *AutoDownloadSettings {
	return MakeTLAutoDownloadSettings(&AutoDownloadSettings{Disabled: true}).To_AutoDownloadSettings()
}

func emptyStories() *Stories_Stories {
	return MakeTLStoriesStories(&Stories_Stories{
		Stories: []*StoryItem{}, Chats: []*Chat{}, Users: []*User{},
	}).To_Stories_Stories()
}

func storiesFallback(name string) (TLObject, bool) {
	switch name {
	case "TLStoriesGetAllStories":
		return MakeTLStoriesAllStoriesNotModified(&Stories_AllStories{
			StealthMode: MakeTLStoriesStealthMode(nil).To_StoriesStealthMode(),
		}).To_Stories_AllStories(), true
	case "TLStoriesGetPinnedStories", "TLStoriesGetStoriesArchive", "TLStoriesGetStoriesByID", "TLStoriesGetAlbumStories":
		return emptyStories(), true
	case "TLStoriesGetPeerStories":
		return MakeTLStoriesPeerStories(&Stories_PeerStories{
			Stories: MakeTLPeerStories(&PeerStories{
				Peer:    MakeTLPeerUser(&Peer{}).To_Peer(),
				Stories: []*StoryItem{},
			}).To_PeerStories(),
			Chats: []*Chat{},
			Users: []*User{},
		}).To_Stories_PeerStories(), true
	case "TLStoriesGetUserStories":
		return MakeTLStoriesUserStories(&Stories_UserStories{
			Stories: MakeTLUserStories(&UserStories{Stories: []*StoryItem{}}).To_UserStories(),
			Users:   []*User{},
		}).To_Stories_UserStories(), true
	case "TLStoriesGetStoryViewsList":
		return MakeTLStoriesStoryViewsList(&Stories_StoryViewsList{
			Views: []*StoryView{}, Chats: []*Chat{}, Users: []*User{},
		}).To_Stories_StoryViewsList(), true
	case "TLStoriesGetStoriesViews":
		return MakeTLStoriesStoryViews(&Stories_StoryViews{
			Views: []*StoryViews{}, Users: []*User{},
		}).To_Stories_StoryViews(), true
	case "TLStoriesGetStoryReactionsList":
		return MakeTLStoriesStoryReactionsList(&Stories_StoryReactionsList{
			Reactions: []*StoryReaction{}, Chats: []*Chat{}, Users: []*User{},
		}).To_Stories_StoryReactionsList(), true
	case "TLStoriesSearchPosts":
		return MakeTLStoriesFoundStories(&Stories_FoundStories{
			Stories: []*FoundStory{}, Chats: []*Chat{}, Users: []*User{},
		}).To_Stories_FoundStories(), true
	case "TLStoriesGetAlbums":
		return MakeTLStoriesAlbumsNotModified(nil).To_Stories_Albums(), true
	case "TLStoriesGetChatsToSend":
		return MakeTLMessagesChats(&Messages_Chats{Chats: []*Chat{}}).To_Messages_Chats(), true
	case "TLStoriesCanSendStory30EB63F0":
		return MakeTLStoriesCanSendStoryCount(&Stories_CanSendStoryCount{}).To_Stories_CanSendStoryCount(), true
	case "TLStoriesExportStoryLink":
		return MakeTLExportedStoryLink(&ExportedStoryLink{}).To_ExportedStoryLink(), true
	case "TLStoriesReport19D8EB45":
		return MakeTLReportResultReported(nil).To_ReportResult(), true
	case "TLStoriesCreateAlbum", "TLStoriesUpdateAlbum":
		return MakeTLStoryAlbum(&StoryAlbum{}).To_StoryAlbum(), true
	case "TLStoriesDeleteStories", "TLStoriesTogglePinned", "TLStoriesReadStories", "TLStoriesGetPeerMaxIDs535983C3", "TLUsersGetStoriesMaxIDs":
		return &Vector_Int{Datas: []int32{}}, true
	case "TLStoriesGetPeerMaxIDs78499170":
		return &Vector_RecentStory{Datas: []*RecentStory{}}, true
	case "TLStoriesSendStory", "TLStoriesEditStory", "TLStoriesActivateStealthMode", "TLStoriesSendReaction",
		"TLStoriesGetAllReadPeerStories", "TLStoriesGetAllReadUserStories", "TLStoriesStartLive":
		return emptyUpdates(), true
	case "TLStoriesToggleAllStoriesHidden", "TLStoriesIncrementStoryViews", "TLStoriesTogglePeerStoriesHidden",
		"TLStoriesTogglePinnedToTop", "TLStoriesReorderAlbums", "TLStoriesDeleteAlbum",
		"TLStoriesCanSendStoryC7DFDFDD", "TLStoriesReport1923FA8C", "TLStoriesCanSendStoryB100D45D",
		"TLStoriesReportC95BE06A", "TLContactsToggleStoriesHidden":
		return BoolTrue, true
	default:
		return nil, false
	}
}
