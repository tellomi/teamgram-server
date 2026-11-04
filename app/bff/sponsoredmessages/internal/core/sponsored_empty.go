package core

import "github.com/teamgram/proto/mtproto"

func sponsoredEmpty() *mtproto.Messages_SponsoredMessages {
	return mtproto.MakeTLMessagesSponsoredMessagesEmpty(&mtproto.Messages_SponsoredMessages{}).To_Messages_SponsoredMessages()
}

func sponsoredReported() *mtproto.Channels_SponsoredMessageReportResult {
	return mtproto.MakeTLChannelsSponsoredMessageReportResultReported(&mtproto.Channels_SponsoredMessageReportResult{}).To_Channels_SponsoredMessageReportResult()
}
