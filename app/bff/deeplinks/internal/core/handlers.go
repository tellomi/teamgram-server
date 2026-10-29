package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/brand"
)

// HelpGetDeepLinkInfo
// help.getDeepLinkInfo#3fedc75f path:string = help.DeepLinkInfo;
func (c *DeepLinksCore) HelpGetDeepLinkInfo(in *mtproto.TLHelpGetDeepLinkInfo) (*mtproto.Help_DeepLinkInfo, error) {
	// The path is a tell.cc route. There is no Telegram interstitial to show.
	_ = brand.Rewrite(in.GetPath())
	return mtproto.MakeTLHelpDeepLinkInfoEmpty(&mtproto.Help_DeepLinkInfo{}).To_Help_DeepLinkInfo(), nil
}

// HelpGetRecentMeUrls
// help.getRecentMeUrls#3dc0f114 referer:string = help.RecentMeUrls;
func (c *DeepLinksCore) HelpGetRecentMeUrls(in *mtproto.TLHelpGetRecentMeUrls) (*mtproto.Help_RecentMeUrls, error) {
	_ = brand.Rewrite(in.GetReferer())
	return mtproto.MakeTLHelpRecentMeUrls(&mtproto.Help_RecentMeUrls{
		Urls:  []*mtproto.RecentMeUrl{},
		Chats: []*mtproto.Chat{},
		Users: []*mtproto.User{},
	}).To_Help_RecentMeUrls(), nil
}
