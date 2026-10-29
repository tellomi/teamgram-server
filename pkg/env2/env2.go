// Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
//  All rights reserved.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package env2

import (
	"flag"
	"strings"
)

var (
	MyAppName = "Teamgram"
	MyWebSite = "teamgram.net"
	TDotMe    = "tell.cc"
	// MyWebClientSite = "web.teamgram.net"
	MyTgScheme = "tg"
	// IosAppStoreId   = ""
	// AndroidPackage  = "org.chatengine.messenger"

	// PredefinedUser - auto register
	PredefinedUser = false
	// PredefinedUser2
	// predefined2 - auto register
	PredefinedUser2 = false

	MyTgCDNHost = ""
)

func init() {
	flag.StringVar(&MyAppName, "app_name", "Teamgram", "app_name")
	flag.StringVar(&MyWebSite, "site_name", "teamgram.net", "site_name")
	flag.StringVar(&TDotMe, "t.me", "tell.cc", "public short-link host")
	// flag.StringVar(&MyWebClientSite, "webclient", "web.teamgram.net", "web.teamgram.net")
	flag.StringVar(&MyTgScheme, "tg", "tg", "tg")
	// flag.StringVar(&IosAppStoreId, "app_store_id", "", "")
	// flag.StringVar(&AndroidPackage, "android_package", "org.chatengine.messenger", "org.chatengine.messenger")
	flag.BoolVar(&PredefinedUser, "predefined", false, "predefined")
	flag.BoolVar(&PredefinedUser2, "predefined2", false, "predefined2")
	flag.StringVar(&MyTgCDNHost, "cdn", "", "cdn_host")
}

func IsTDotMe(me string) bool {
	switch strings.ToLower(me) {
	case "tell.cc", "t.me", "telegram.me", "telegram.dog", "teamgram.me":
		return true
	case strings.ToLower(TDotMe):
		return true
	}

	return false
}
