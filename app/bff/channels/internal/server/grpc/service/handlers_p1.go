package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/accountrest"
	"github.com/teamgram/teamgram-server/app/bff/botadmin"
	"github.com/teamgram/teamgram-server/app/bff/channels/internal/historysplit"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// FragmentService serves channel username order and activation, and the
// account username order and activation methods on RPCFragment.
type FragmentService struct {
	mtproto.UnimplementedRPCFragmentServer
	Users userclient.UserClient
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

// AccountReorderUsernames
// account.reorderUsernames#ef500eab order:Vector<string> = Bool;
// https://core.telegram.org/method/account.reorderUsernames
func (s FragmentService) AccountReorderUsernames(ctx context.Context, in *mtproto.TLAccountReorderUsernames) (*mtproto.Bool, error) {
	self, _ := actor(ctx)
	s.seedPrimaryUsername(ctx, self)
	var order []string
	if in != nil {
		order = in.GetOrder()
	}
	ok, err := accountrest.ReorderUsernames(self, order)
	if err == nil && s.Users != nil {
		_, _ = s.Users.UserReorderUsernames(ctx, &user.TLUserReorderUsernames{
			PeerType:     mtproto.PEER_USER,
			PeerId:       self,
			UsernameList: order,
		})
	}
	return ok, err
}

// AccountToggleUsername
// account.toggleUsername#58d6b376 username:string active:Bool = Bool;
// https://core.telegram.org/method/account.toggleUsername
func (s FragmentService) AccountToggleUsername(ctx context.Context, in *mtproto.TLAccountToggleUsername) (*mtproto.Bool, error) {
	self, _ := actor(ctx)
	s.seedPrimaryUsername(ctx, self)
	username := ""
	var active *mtproto.Bool
	if in != nil {
		username = in.GetUsername()
		active = in.GetActive()
	}
	ok, err := accountrest.ToggleUsername(self, username, mtproto.FromBool(active))
	if err == nil && s.Users != nil {
		_, _ = s.Users.UserToggleUsername(ctx, &user.TLUserToggleUsername{
			PeerType: mtproto.PEER_USER,
			PeerId:   self,
			Username: username,
			Active:   active,
		})
	}
	return ok, err
}

func (s FragmentService) BotsReorderUsernames(ctx context.Context, in *mtproto.TLBotsReorderUsernames) (*mtproto.Bool, error) {
	self, _ := actor(ctx)
	return botadmin.ReorderUsernames(ctx, self, in.GetBot(), in.GetOrder())
}

func (s FragmentService) BotsToggleUsername(ctx context.Context, in *mtproto.TLBotsToggleUsername) (*mtproto.Bool, error) {
	self, _ := actor(ctx)
	return botadmin.ToggleUsername(ctx, self, in.GetBot(), in.GetUsername(), mtproto.FromBool(in.GetActive()))
}

func (s FragmentService) seedPrimaryUsername(ctx context.Context, self int64) {
	if s.Users == nil || self == 0 {
		return
	}
	data, err := s.Users.UserGetAccountUsername(ctx, &user.TLUserGetAccountUsername{UserId: self})
	if err != nil || data.GetUsername() == "" {
		return
	}
	accountrest.SeedPrimary(self, data.GetUsername(), data.GetActive())
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

// BoostsService serves the channel boost threshold and premium.getBoosts*
// plus premium.applyBoost. Story boost RPCs on this service stay unimplemented.
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

// TakeoutService serves channels.getLeftChannels, messages.getSplitRanges,
// and account takeout sessions.
type TakeoutService struct {
	mtproto.UnimplementedRPCTakeoutServer
}

func (TakeoutService) ChannelsGetLeftChannels(ctx context.Context, in *mtproto.TLChannelsGetLeftChannels) (*mtproto.Messages_Chats, error) {
	self, _ := actor(ctx)
	return channel.Shared().GetLeftChannels(ctx, self, in.GetOffset())
}

// MessagesGetSplitRanges
// messages.getSplitRanges#1cff7e08 = Vector<MessageRange>;
func (TakeoutService) MessagesGetSplitRanges(ctx context.Context, _ *mtproto.TLMessagesGetSplitRanges) (*mtproto.Vector_MessageRange, error) {
	if self, _ := actor(ctx); self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	return historysplit.Ranges(), nil
}

// AccountInitTakeoutSession
// account.initTakeoutSession#8ef3eab0 flags:# contacts:flags.0?true message_users:flags.1?true message_chats:flags.2?true message_megagroups:flags.3?true message_channels:flags.4?true files:flags.5?true file_max_size:flags.5?long = account.Takeout;
// https://core.telegram.org/method/account.initTakeoutSession
func (TakeoutService) AccountInitTakeoutSession(ctx context.Context, in *mtproto.TLAccountInitTakeoutSession) (*mtproto.Account_Takeout, error) {
	self, _ := actor(ctx)
	return accountrest.InitTakeoutSession(self, in)
}

// AccountFinishTakeoutSession
// account.finishTakeoutSession#1d2652ee flags:# success:flags.0?true = Bool;
// https://core.telegram.org/method/account.finishTakeoutSession
func (TakeoutService) AccountFinishTakeoutSession(ctx context.Context, in *mtproto.TLAccountFinishTakeoutSession) (*mtproto.Bool, error) {
	self, _ := actor(ctx)
	var takeoutID int64
	if md := metadata.RpcMetadataFromIncoming(ctx); md != nil {
		takeoutID = md.GetTakeoutId()
	}
	success := false
	if in != nil {
		success = in.GetSuccess()
	}
	return accountrest.FinishTakeoutSession(self, takeoutID, success)
}

// TranslationService serves channels.toggleAutotranslation,
// messages.translateText, and messages.togglePeerTranslations.
type TranslationService struct {
	mtproto.UnimplementedRPCTranslationServer
	Messages messageReader
}

func (TranslationService) ChannelsToggleAutotranslation(ctx context.Context, in *mtproto.TLChannelsToggleAutotranslation) (*mtproto.Updates, error) {
	self, auth := actor(ctx)
	return channel.Shared().ToggleAutotranslation(ctx, self, auth, in.GetChannel(), mtproto.FromBool(in.GetEnabled()))
}

// PaidMessageService serves channels.updatePaidMessagesPrice and the account
// paid-message revenue methods.
type PaidMessageService struct {
	mtproto.UnimplementedRPCPaidMessageServer
}

func (PaidMessageService) ChannelsUpdatePaidMessagesPrice(ctx context.Context, in *mtproto.TLChannelsUpdatePaidMessagesPrice) (*mtproto.Updates, error) {
	self, auth := actor(ctx)
	return channel.Shared().UpdatePaidMessagesPrice(ctx, self, auth, in.GetChannel(), in.GetSendPaidMessagesStars(), in.GetBroadcastMessagesAllowed())
}

// AccountGetPaidMessagesRevenue
// account.getPaidMessagesRevenue#19ba4a67 flags:# parent_peer:flags.0?InputPeer user_id:InputUser = account.PaidMessagesRevenue;
// https://core.telegram.org/method/account.getPaidMessagesRevenue
func (PaidMessageService) AccountGetPaidMessagesRevenue(ctx context.Context, in *mtproto.TLAccountGetPaidMessagesRevenue) (*mtproto.Account_PaidMessagesRevenue, error) {
	self, _ := actor(ctx)
	if in == nil {
		in = &mtproto.TLAccountGetPaidMessagesRevenue{}
	}
	return accountrest.GetPaidMessagesRevenue(self, in.GetParentPeer(), in.GetUserId())
}

// AccountToggleNoPaidMessagesException
// account.toggleNoPaidMessagesException#fe2eda76 flags:# refund_charged:flags.0?true require_payment:flags.2?true parent_peer:flags.1?InputPeer user_id:InputUser = Bool;
// https://core.telegram.org/method/account.toggleNoPaidMessagesException
func (PaidMessageService) AccountToggleNoPaidMessagesException(ctx context.Context, in *mtproto.TLAccountToggleNoPaidMessagesException) (*mtproto.Bool, error) {
	self, _ := actor(ctx)
	if in == nil {
		in = &mtproto.TLAccountToggleNoPaidMessagesException{}
	}
	return accountrest.ToggleNoPaidMessagesException(self, in.GetParentPeer(), in.GetUserId(), in.GetRefundCharged(), in.GetRequirePayment())
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
