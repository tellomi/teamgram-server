// Package bots registers the bots RPC services on the bff process.
// Profile, preview, and mini-app methods that share an RPC service with
// messages or account are implemented on those services and call botadmin.
package bots

import (
	"context"

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

// The rest of RPCAffiliatePrograms is payments. Those methods stay
// METHOD_NOT_IMPL, the same 400 the session returned when the service was
// not routed. Payments are outside this change.

func (s *Service) PaymentsGetConnectedStarRefBots(context.Context, *mtproto.TLPaymentsGetConnectedStarRefBots) (*mtproto.Payments_ConnectedStarRefBots, error) {
	return nil, mtproto.ErrMethodNotImpl
}

func (s *Service) PaymentsGetConnectedStarRefBot(context.Context, *mtproto.TLPaymentsGetConnectedStarRefBot) (*mtproto.Payments_ConnectedStarRefBots, error) {
	return nil, mtproto.ErrMethodNotImpl
}

func (s *Service) PaymentsGetSuggestedStarRefBots(context.Context, *mtproto.TLPaymentsGetSuggestedStarRefBots) (*mtproto.Payments_SuggestedStarRefBots, error) {
	return nil, mtproto.ErrMethodNotImpl
}

func (s *Service) PaymentsConnectStarRefBot(context.Context, *mtproto.TLPaymentsConnectStarRefBot) (*mtproto.Payments_ConnectedStarRefBots, error) {
	return nil, mtproto.ErrMethodNotImpl
}

func (s *Service) PaymentsEditConnectedStarRefBot(context.Context, *mtproto.TLPaymentsEditConnectedStarRefBot) (*mtproto.Payments_ConnectedStarRefBots, error) {
	return nil, mtproto.ErrMethodNotImpl
}
