package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
)

// FragmentService serves channel username order and activation.
type FragmentService struct {
	mtproto.UnimplementedRPCFragmentServer
}

func (FragmentService) ChannelsReorderUsernames(ctx context.Context, in *mtproto.TLChannelsReorderUsernames) (*mtproto.Bool, error) {
	self, auth := actor(ctx)
	return channel.Shared().ReorderUsernames(ctx, self, auth, in.GetChannel(), in.GetOrder())
}

func (FragmentService) ChannelsToggleUsername(ctx context.Context, in *mtproto.TLChannelsToggleUsername) (*mtproto.Bool, error) {
	self, auth := actor(ctx)
	return channel.Shared().ToggleUsername(ctx, self, auth, in.GetChannel(), in.GetUsername(), mtproto.FromBool(in.GetActive()))
}

func (FragmentService) ChannelsDeactivateAllUsernames(ctx context.Context, in *mtproto.TLChannelsDeactivateAllUsernames) (*mtproto.Bool, error) {
	self, auth := actor(ctx)
	return channel.Shared().DeactivateAllUsernames(ctx, self, auth, in.GetChannel())
}

// AntiSpamService serves native antispam toggles and false-positive reports.
type AntiSpamService struct {
	mtproto.UnimplementedRPCAntiSpamServer
}

func (AntiSpamService) ChannelsToggleAntiSpam(ctx context.Context, in *mtproto.TLChannelsToggleAntiSpam) (*mtproto.Updates, error) {
	self, auth := actor(ctx)
	return channel.Shared().ToggleAntiSpam(ctx, self, auth, in.GetChannel(), mtproto.FromBool(in.GetEnabled()))
}

func (AntiSpamService) ChannelsReportAntiSpamFalsePositive(ctx context.Context, in *mtproto.TLChannelsReportAntiSpamFalsePositive) (*mtproto.Bool, error) {
	self, _ := actor(ctx)
	return channel.Shared().ReportAntiSpamFalsePositive(ctx, self, in.GetChannel(), in.GetMsgId())
}

// BoostsService serves the channel boost threshold. Other boost RPCs on this
// service stay unimplemented.
type BoostsService struct {
	mtproto.UnimplementedRPCBoostsServer
}

func (BoostsService) ChannelsSetBoostsToUnblockRestrictions(ctx context.Context, in *mtproto.TLChannelsSetBoostsToUnblockRestrictions) (*mtproto.Updates, error) {
	self, auth := actor(ctx)
	return channel.Shared().SetBoostsToUnblockRestrictions(ctx, self, auth, in.GetChannel(), in.GetBoosts())
}

// RecommendationsService serves channels.getChannelRecommendations.
type RecommendationsService struct {
	mtproto.UnimplementedRPCChannelRecommendationsServer
}

func (RecommendationsService) ChannelsGetChannelRecommendations(ctx context.Context, in *mtproto.TLChannelsGetChannelRecommendations) (*mtproto.Messages_Chats, error) {
	self, _ := actor(ctx)
	return channel.Shared().GetChannelRecommendations(ctx, self, in.GetChannel())
}

// TakeoutService serves channels.getLeftChannels. Other takeout RPCs stay unimplemented.
type TakeoutService struct {
	mtproto.UnimplementedRPCTakeoutServer
}

func (TakeoutService) ChannelsGetLeftChannels(ctx context.Context, in *mtproto.TLChannelsGetLeftChannels) (*mtproto.Messages_Chats, error) {
	self, _ := actor(ctx)
	return channel.Shared().GetLeftChannels(ctx, self, in.GetOffset())
}

// TranslationService serves channels.toggleAutotranslation.
type TranslationService struct {
	mtproto.UnimplementedRPCTranslationServer
}

func (TranslationService) ChannelsToggleAutotranslation(ctx context.Context, in *mtproto.TLChannelsToggleAutotranslation) (*mtproto.Updates, error) {
	self, auth := actor(ctx)
	return channel.Shared().ToggleAutotranslation(ctx, self, auth, in.GetChannel(), mtproto.FromBool(in.GetEnabled()))
}

// PaidMessageService serves channels.updatePaidMessagesPrice.
type PaidMessageService struct {
	mtproto.UnimplementedRPCPaidMessageServer
}

func (PaidMessageService) ChannelsUpdatePaidMessagesPrice(ctx context.Context, in *mtproto.TLChannelsUpdatePaidMessagesPrice) (*mtproto.Updates, error) {
	self, auth := actor(ctx)
	return channel.Shared().UpdatePaidMessagesPrice(ctx, self, auth, in.GetChannel(), in.GetSendPaidMessagesStars(), in.GetBroadcastMessagesAllowed())
}

// ReportsService serves channels.reportSpam and the report methods that the
// session previously answered with an empty proxy result.
type ReportsService struct {
	mtproto.UnimplementedRPCReportsServer
}

func (ReportsService) ChannelsReportSpam(ctx context.Context, in *mtproto.TLChannelsReportSpam) (*mtproto.Bool, error) {
	self, _ := actor(ctx)
	return channel.Shared().ReportSpam(ctx, self, in.GetChannel(), in.GetParticipant(), in.GetId())
}

func (ReportsService) AccountReportPeer(ctx context.Context, in *mtproto.TLAccountReportPeer) (*mtproto.Bool, error) {
	self, _ := actor(ctx)
	return channel.Shared().RecordPeerReport(ctx, self, in.GetPeer(), reasonName(in.GetReason()), in.GetMessage(), 0)
}

func (ReportsService) AccountReportProfilePhoto(ctx context.Context, in *mtproto.TLAccountReportProfilePhoto) (*mtproto.Bool, error) {
	self, _ := actor(ctx)
	var photoID int64
	if in.GetPhotoId() != nil {
		photoID = in.GetPhotoId().GetId()
	}
	return channel.Shared().RecordPeerReport(ctx, self, in.GetPeer(), reasonName(in.GetReason()), in.GetMessage(), photoID)
}

func (ReportsService) MessagesReportSpam(ctx context.Context, in *mtproto.TLMessagesReportSpam) (*mtproto.Bool, error) {
	self, _ := actor(ctx)
	return channel.Shared().RecordPeerReport(ctx, self, in.GetPeer(), "messages.reportSpam", "", 0)
}

func reasonName(reason *mtproto.ReportReason) string {
	if reason == nil {
		return ""
	}
	return reason.GetPredicateName()
}

func (ForumsService) ChannelsToggleViewForumAsMessages(ctx context.Context, in *mtproto.TLChannelsToggleViewForumAsMessages) (*mtproto.Updates, error) {
	self, auth := actor(ctx)
	return channel.Shared().ToggleViewForumAsMessages(ctx, self, auth, in.GetChannel(), mtproto.FromBool(in.GetEnabled()))
}
