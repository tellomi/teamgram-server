// Package bots registers the bots RPC services on the bff process.
// Profile, preview, and mini-app methods that share an RPC service with
// messages or account are implemented on those services and call botadmin.
package bots

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/botadmin"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"

	"github.com/zeromicro/go-zero/zrpc"
)

// Service implements RPCBots, RPCBotVerificationIcons, and the bot method
// on RPCAffiliatePrograms.
type Service struct {
	mtproto.UnimplementedRPCBotsServer
	mtproto.UnimplementedRPCBotVerificationIconsServer
	mtproto.UnimplementedRPCAffiliateProgramsServer
	mtproto.UnimplementedRPCBotMenuButtonServer
	mtproto.UnimplementedRPCBotAdminRightServer
	mtproto.UnimplementedRPCInternalBotServer
}

// New serves bot administration. userConf is the user service, used to see
// whether a username is already taken.
func New(userConf zrpc.RpcClientConf) *Service {
	botadmin.UseUsers(userclient.NewUserClient(rpcx.GetCachedRpcClient(userConf)))
	return &Service{}
}

func selfID(ctx context.Context) int64 {
	md := metadata.RpcMetadataFromIncoming(ctx)
	if md == nil {
		return 0
	}
	return md.GetUserId()
}

func (s *Service) BotsSetBotMenuButton(ctx context.Context, in *mtproto.TLBotsSetBotMenuButton) (*mtproto.Bool, error) {
	var user *mtproto.InputUser
	var button *mtproto.BotMenuButton
	if in != nil {
		user = in.GetUserId()
		button = in.GetButton()
	}
	return botadmin.SetMenuButton(ctx, selfID(ctx), user, button)
}

func (s *Service) BotsGetBotMenuButton(ctx context.Context, in *mtproto.TLBotsGetBotMenuButton) (*mtproto.BotMenuButton, error) {
	var user *mtproto.InputUser
	if in != nil {
		user = in.GetUserId()
	}
	return botadmin.MenuButton(ctx, selfID(ctx), user)
}

func (s *Service) BotsSetBotGroupDefaultAdminRights(ctx context.Context, in *mtproto.TLBotsSetBotGroupDefaultAdminRights) (*mtproto.Bool, error) {
	var rights *mtproto.ChatAdminRights
	if in != nil {
		rights = in.GetAdminRights()
	}
	return botadmin.SetDefaultAdminRights(ctx, selfID(ctx), false, rights)
}

func (s *Service) BotsSetBotBroadcastDefaultAdminRights(ctx context.Context, in *mtproto.TLBotsSetBotBroadcastDefaultAdminRights) (*mtproto.Bool, error) {
	var rights *mtproto.ChatAdminRights
	if in != nil {
		rights = in.GetAdminRights()
	}
	return botadmin.SetDefaultAdminRights(ctx, selfID(ctx), true, rights)
}

func (s *Service) HelpSetBotUpdatesStatus(ctx context.Context, in *mtproto.TLHelpSetBotUpdatesStatus) (*mtproto.Bool, error) {
	pending := int32(0)
	message := ""
	if in != nil {
		pending = in.GetPendingUpdatesCount()
		message = in.GetMessage()
	}
	return botadmin.SetUpdatesStatus(ctx, selfID(ctx), pending, message)
}

// BotsSendCustomRequest implements bots.sendCustomRequest.
// No custom method is registered, so a well-formed call is METHOD_INVALID.
// https://core.telegram.org/method/bots.sendCustomRequest
func (s *Service) BotsSendCustomRequest(ctx context.Context, in *mtproto.TLBotsSendCustomRequest) (*mtproto.DataJSON, error) {
	if selfID(ctx) == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	method, raw := "", ""
	if in != nil {
		method = strings.TrimSpace(in.GetCustomMethod())
		if in.GetParams() != nil {
			raw = in.GetParams().GetData()
		}
	}
	if method == "" {
		return nil, mtproto.ErrMethodInvalid
	}
	if !json.Valid([]byte(raw)) {
		return nil, mtproto.ErrDataJsonInvalid
	}
	return nil, mtproto.ErrMethodInvalid
}

// BotsAnswerWebhookJSONQuery implements bots.answerWebhookJSONQuery.
// Webhook queries are not issued here.
// https://core.telegram.org/method/bots.answerWebhookJSONQuery
func (s *Service) BotsAnswerWebhookJSONQuery(ctx context.Context, in *mtproto.TLBotsAnswerWebhookJSONQuery) (*mtproto.Bool, error) {
	self := selfID(ctx)
	if err := botCaller(ctx, self); err != nil {
		return nil, err
	}
	raw := ""
	query := int64(0)
	if in != nil {
		query = in.GetQueryId()
		if in.GetData() != nil {
			raw = in.GetData().GetData()
		}
	}
	if !json.Valid([]byte(raw)) {
		return nil, mtproto.ErrDataJsonInvalid
	}
	if query == 0 {
		return nil, mtproto.ErrQueryIdInvalid
	}
	return nil, mtproto.ErrQueryIdInvalid
}

func botCaller(ctx context.Context, self int64) error {
	if self == 0 {
		return mtproto.ErrAuthKeyUnregistered
	}
	if !botadmin.CallerIsBot(ctx, self) {
		return mtproto.ErrUserBotRequired
	}
	return nil
}

func (s *Service) BotsSetBotCommands(ctx context.Context, in *mtproto.TLBotsSetBotCommands) (*mtproto.Bool, error) {
	return botadmin.SetCommands(ctx, selfID(ctx), in.GetScope(), in.GetLangCode(), in.GetCommands())
}

func (s *Service) BotsResetBotCommands(ctx context.Context, in *mtproto.TLBotsResetBotCommands) (*mtproto.Bool, error) {
	return botadmin.ResetCommands(ctx, selfID(ctx), in.GetScope(), in.GetLangCode())
}

func (s *Service) BotsGetBotCommands(ctx context.Context, in *mtproto.TLBotsGetBotCommands) (*mtproto.Vector_BotCommand, error) {
	return botadmin.Commands(ctx, selfID(ctx), in.GetScope(), in.GetLangCode())
}

func (s *Service) BotsSetBotInfo(ctx context.Context, in *mtproto.TLBotsSetBotInfo) (*mtproto.Bool, error) {
	return botadmin.SetBotInfo(ctx, selfID(ctx), in.GetBot(), in.GetLangCode(), in.GetName(), in.GetAbout(), in.GetDescription())
}

func (s *Service) BotsGetBotInfoDCD914FD(ctx context.Context, in *mtproto.TLBotsGetBotInfoDCD914FD) (*mtproto.Bots_BotInfo, error) {
	return botadmin.GetBotInfo(ctx, selfID(ctx), in.GetBot(), in.GetLangCode())
}

func (s *Service) BotsGetAdminedBots(ctx context.Context, _ *mtproto.TLBotsGetAdminedBots) (*mtproto.Vector_User, error) {
	return botadmin.AdminedBots(selfID(ctx))
}

func (s *Service) BotsCheckUsername(ctx context.Context, in *mtproto.TLBotsCheckUsername) (*mtproto.Bool, error) {
	return botadmin.CheckUsername(ctx, selfID(ctx), in.GetAdditional(), in.GetUsername())
}

func (s *Service) BotsCreateBot(ctx context.Context, in *mtproto.TLBotsCreateBot) (*mtproto.User, error) {
	return botadmin.CreateBot(ctx, selfID(ctx), in.GetViaDeeplink(), in.GetName(), in.GetUsername(), in.GetManagerId())
}

func (s *Service) BotsExportBotToken(ctx context.Context, in *mtproto.TLBotsExportBotToken) (*mtproto.Bots_ExportedBotToken, error) {
	return botadmin.ExportToken(ctx, selfID(ctx), in.GetBot(), mtproto.FromBool(in.GetRevoke()))
}

func (s *Service) BotsGetAccessSettings(ctx context.Context, in *mtproto.TLBotsGetAccessSettings) (*mtproto.Bots_AccessSettings, error) {
	return botadmin.AccessSettings(ctx, selfID(ctx), in.GetBot())
}

func (s *Service) BotsEditAccessSettings(ctx context.Context, in *mtproto.TLBotsEditAccessSettings) (*mtproto.Bool, error) {
	return botadmin.EditAccessSettings(ctx, selfID(ctx), in.GetBot(), in.GetRestricted(), in.GetAddUsers())
}

func (s *Service) BotsSetJoinChatResults(ctx context.Context, in *mtproto.TLBotsSetJoinChatResults) (*mtproto.Bool, error) {
	return botadmin.SetJoinResult(ctx, selfID(ctx), in.GetQueryId(), in.GetResult())
}

func (s *Service) BotsAddUsername(ctx context.Context, in *mtproto.TLBotsAddUsername) (*mtproto.Bool, error) {
	return botadmin.AddUsername(ctx, selfID(ctx), in.GetBot(), in.GetUsername())
}

func (s *Service) BotsRemoveUsername(ctx context.Context, in *mtproto.TLBotsRemoveUsername) (*mtproto.Bool, error) {
	return botadmin.RemoveUsername(ctx, selfID(ctx), in.GetBot(), in.GetUsername())
}

func (s *Service) BotsGetBotInfo75EC12E6(ctx context.Context, in *mtproto.TLBotsGetBotInfo75EC12E6) (*mtproto.Vector_String, error) {
	return botadmin.BotInfoLangs(selfID(ctx), in.GetLangCode())
}

func (s *Service) BotsSetCustomVerification(ctx context.Context, in *mtproto.TLBotsSetCustomVerification) (*mtproto.Bool, error) {
	return botadmin.SetCustomVerification(ctx, selfID(ctx), in.GetEnabled(), in.GetBot(), in.GetPeer(), botadmin.DescriptionText(in))
}

func (s *Service) BotsGetBotRecommendations(ctx context.Context, in *mtproto.TLBotsGetBotRecommendations) (*mtproto.Users_Users, error) {
	return botadmin.Recommendations(ctx, selfID(ctx), in.GetBot())
}

func (s *Service) BotsUpdateStarRefProgram(ctx context.Context, in *mtproto.TLBotsUpdateStarRefProgram) (*mtproto.StarRefProgram, error) {
	return botadmin.UpdateStarRefProgram(ctx, selfID(ctx), in.GetBot(), in.GetCommissionPermille(), in.GetDurationMonths())
}

// Affiliate connections live next to the bot's star ref program.

func (s *Service) PaymentsGetConnectedStarRefBots(ctx context.Context, in *mtproto.TLPaymentsGetConnectedStarRefBots) (*mtproto.Payments_ConnectedStarRefBots, error) {
	return botadmin.ConnectedStarRefBots(selfID(ctx), in)
}

func (s *Service) PaymentsGetConnectedStarRefBot(ctx context.Context, in *mtproto.TLPaymentsGetConnectedStarRefBot) (*mtproto.Payments_ConnectedStarRefBots, error) {
	var peer *mtproto.InputPeer
	var bot *mtproto.InputUser
	if in != nil {
		peer = in.GetPeer()
		bot = in.GetBot()
	}
	return botadmin.ConnectedStarRefBot(selfID(ctx), peer, bot)
}

func (s *Service) PaymentsGetSuggestedStarRefBots(ctx context.Context, in *mtproto.TLPaymentsGetSuggestedStarRefBots) (*mtproto.Payments_SuggestedStarRefBots, error) {
	return botadmin.SuggestedStarRefBots(selfID(ctx), in)
}

func (s *Service) PaymentsConnectStarRefBot(ctx context.Context, in *mtproto.TLPaymentsConnectStarRefBot) (*mtproto.Payments_ConnectedStarRefBots, error) {
	var peer *mtproto.InputPeer
	var bot *mtproto.InputUser
	if in != nil {
		peer = in.GetPeer()
		bot = in.GetBot()
	}
	return botadmin.ConnectStarRefBot(selfID(ctx), peer, bot)
}

func (s *Service) PaymentsEditConnectedStarRefBot(ctx context.Context, in *mtproto.TLPaymentsEditConnectedStarRefBot) (*mtproto.Payments_ConnectedStarRefBots, error) {
	var peer *mtproto.InputPeer
	link := ""
	revoked := false
	if in != nil {
		peer = in.GetPeer()
		link = in.GetLink()
		revoked = in.GetRevoked()
	}
	return botadmin.EditConnectedStarRefBot(selfID(ctx), peer, link, revoked)
}
