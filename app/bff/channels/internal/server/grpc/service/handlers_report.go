package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/phone/secretchat"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
)

// MessagesReportFC78AF9B is messages.report.
// https://core.telegram.org/method/messages.report
func (ReportsService) MessagesReportFC78AF9B(ctx context.Context, in *mtproto.TLMessagesReportFC78AF9B) (*mtproto.ReportResult, error) {
	self, _ := actor(ctx)
	peer := mtproto.FromInputPeer2(self, in.GetPeer())
	return chatbox.Shared().ReportMessages(ctx, self, peer, in.GetId(), in.GetOption(), in.GetMessage())
}

// MessagesReport8953AB4E is the previous messages.report constructor.
// It takes a ReportReason and returns Bool.
func (ReportsService) MessagesReport8953AB4E(ctx context.Context, in *mtproto.TLMessagesReport8953AB4E) (*mtproto.Bool, error) {
	self, _ := actor(ctx)
	peer := mtproto.FromInputPeer2(self, in.GetPeer())
	reason := ""
	if in.GetReason() != nil {
		reason = in.GetReason().GetPredicateName()
	}
	return chatbox.Shared().ReportLegacy(ctx, self, peer, in.GetId(), reason, in.GetMessage())
}

// MessagesReportReadMetrics
// messages.reportReadMetrics#4067c5e6 peer:InputPeer metrics:Vector<InputMessageReadMetric> = Bool;
func (ReportsService) MessagesReportReadMetrics(ctx context.Context, in *mtproto.TLMessagesReportReadMetrics) (*mtproto.Bool, error) {
	self, _ := actor(ctx)
	peer := mtproto.FromInputPeer2(self, in.GetPeer())
	if err := chatbox.Shared().ReportReadMetrics(ctx, self, peer, in.GetMetrics()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

// MessagesReportMusicListen
// messages.reportMusicListen#ddbcd819 id:InputDocument listened_duration:int = Bool;
func (ReportsService) MessagesReportMusicListen(ctx context.Context, in *mtproto.TLMessagesReportMusicListen) (*mtproto.Bool, error) {
	self, _ := actor(ctx)
	if err := chatbox.Shared().ReportMusicListen(ctx, self, in.GetId(), in.GetListenedDuration()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

// MessagesReportEncryptedSpam
// messages.reportEncryptedSpam#4b0c8c0f peer:InputEncryptedChat = Bool;
// https://core.telegram.org/method/messages.reportEncryptedSpam
func (ReportsService) MessagesReportEncryptedSpam(ctx context.Context, in *mtproto.TLMessagesReportEncryptedSpam) (*mtproto.Bool, error) {
	self, _ := actor(ctx)
	if self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if err := secretchat.Default().Report(self, in.GetPeer()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

// MessagesReportReaction
// messages.reportReaction#3f64c076 peer:InputPeer id:int reaction_peer:InputPeer = Bool;
func (ReactionsService) MessagesReportReaction(ctx context.Context, in *mtproto.TLMessagesReportReaction) (*mtproto.Bool, error) {
	self, _ := actor(ctx)
	peer := mtproto.FromInputPeer2(self, in.GetPeer())
	reactionPeer := mtproto.FromInputPeer2(self, in.GetReactionPeer())
	if err := chatbox.Shared().ReportReaction(ctx, self, peer, in.GetId(), reactionPeer); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
