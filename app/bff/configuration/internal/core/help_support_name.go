package core

import "github.com/teamgram/teamgram-server/app/bff/helpdesk"

func supportLabel(lang, profileName string) string {
	return helpdesk.SupportName(lang, profileName)
}
