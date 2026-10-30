package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	msg_client "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// BotChatService serves inline bots, callback buttons, the attach menu,
// mini apps, and prepared inline messages.
//
// https://core.telegram.org/api/bots/inline
// https://core.telegram.org/api/bots/webapps
type BotChatService struct {
	mtproto.UnimplementedRPCInlineBotServer
	mtproto.UnimplementedRPCBotMenuServer
	mtproto.UnimplementedRPCMiniBotAppsServer
	mtproto.UnimplementedRPCMainMiniBotAppsServer
	mtproto.UnimplementedRPCPreparedInlineMessagesServer
	Msg msg_client.MsgClient
}

func strVal(v *wrapperspb.StringValue) string {
	if v == nil {
		return ""
	}
	return v.GetValue()
}

func deliver(ctx context.Context, c msg_client.MsgClient, self, auth int64, req *chatbox.SendReq) (*mtproto.Updates, error) {
	if req == nil || req.Message == nil {
		return nil, mtproto.ErrMessageEmpty
	}
	if c == nil {
		return req.Fallback, nil
	}
	return c.MsgSendMessageV2(ctx, &msg.TLMsgSendMessageV2{
		UserId:    self,
		AuthKeyId: auth,
		PeerType:  req.Peer.PeerType,
		PeerId:    req.Peer.PeerId,
		Message: []*msg.OutboxMessage{
			msg.MakeTLOutboxMessage(&msg.OutboxMessage{
				NoWebpage:  req.NoWebpage,
				Background: req.Background,
				RandomId:   req.RandomID,
				Message:    req.Message,
			}).To_OutboxMessage(),
		},
	})
}

func finish(ctx context.Context, c msg_client.MsgClient, self, auth, random int64, req *chatbox.SendReq, ready *mtproto.Updates) (*mtproto.Updates, error) {
	if ready != nil {
		chatbox.Shared().KeepRandom(ctx, self, random, ready)
		return ready, nil
	}
	ups, err := deliver(ctx, c, self, auth, req)
	if err != nil {
		return nil, err
	}
	chatbox.Shared().KeepRandom(ctx, self, random, ups)
	return ups, nil
}

func (s BotChatService) MessagesGetInlineBotResults(ctx context.Context, in *mtproto.TLMessagesGetInlineBotResults) (*mtproto.Messages_BotResults, error) {
	self, _ := selfID(ctx)
	return chatbox.Shared().InlineResults(ctx, self, in.GetBot(), in.GetPeer(), in.GetQuery(), in.GetOffset())
}

func (s BotChatService) MessagesSetInlineBotResults(ctx context.Context, in *mtproto.TLMessagesSetInlineBotResults) (*mtproto.Bool, error) {
	self, _ := selfID(ctx)
	if err := chatbox.Shared().AnswerInlineQuery(ctx, self, in.GetQueryId(), in.GetGallery(), in.GetPrivate(), in.GetCacheTime(), strVal(in.GetNextOffset()),
		in.GetSwitchPm().GetText(), in.GetSwitchPm().GetStartParam(), in.GetSwitchWebview().GetText(), in.GetSwitchWebview().GetUrl(), in.GetResults()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s BotChatService) MessagesSendInlineBotResult(ctx context.Context, in *mtproto.TLMessagesSendInlineBotResult) (*mtproto.Updates, error) {
	self, auth := selfID(ctx)
	if ups, ok := chatbox.Shared().SeenRandom(ctx, self, in.GetRandomId()); ok {
		return ups, nil
	}
	when := int32(0)
	if in.GetScheduleDate() != nil {
		when = in.GetScheduleDate().GetValue()
	}
	req, ready, err := chatbox.Shared().TakeInlineResult(ctx, self, in.GetPeer(), in.GetQueryId(), in.GetId(), in.GetRandomId(), in.GetHideVia(), in.GetSilent(), in.GetBackground(), when, in.GetReplyTo())
	if err != nil {
		return nil, err
	}
	return finish(ctx, s.Msg, self, auth, in.GetRandomId(), req, ready)
}

func (s BotChatService) MessagesGetBotCallbackAnswer(ctx context.Context, in *mtproto.TLMessagesGetBotCallbackAnswer) (*mtproto.Messages_BotCallbackAnswer, error) {
	self, _ := selfID(ctx)
	_, _ = chatbox.Shared().NoteCallback(ctx, self, in.GetPeer(), in.GetMsgId(), in.GetData())
	return chatbox.Shared().CallbackAnswer(ctx, self, in.GetPeer(), in.GetMsgId(), in.GetData(), in.GetPassword() != nil)
}

func (s BotChatService) MessagesSetBotCallbackAnswer(ctx context.Context, in *mtproto.TLMessagesSetBotCallbackAnswer) (*mtproto.Bool, error) {
	if err := chatbox.Shared().AnswerCallback(ctx, in.GetQueryId(), in.GetAlert(), strVal(in.GetMessage()), strVal(in.GetUrl()), in.GetCacheTime()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s BotChatService) MessagesSendBotRequestedPeer(ctx context.Context, in *mtproto.TLMessagesSendBotRequestedPeer) (*mtproto.Updates, error) {
	self, auth := selfID(ctx)
	msgID := in.GetMsgId_INT32()
	if in.GetMsgId_FLAGINT32() != nil {
		msgID = in.GetMsgId_FLAGINT32().GetValue()
	}
	peers := in.GetRequestedPeers()
	if len(peers) == 0 && in.GetRequestedPeer() != nil {
		peers = []*mtproto.InputPeer{in.GetRequestedPeer()}
	}
	req, err := chatbox.Shared().RequestedPeers(ctx, self, in.GetPeer(), msgID, in.GetButtonId(), peers)
	if err != nil {
		return nil, err
	}
	return deliver(ctx, s.Msg, self, auth, req)
}

func (s BotChatService) MessagesGetAttachMenuBots(ctx context.Context, in *mtproto.TLMessagesGetAttachMenuBots) (*mtproto.AttachMenuBots, error) {
	self, _ := selfID(ctx)
	return chatbox.Shared().AttachBots(ctx, self, in.GetHash())
}

func (s BotChatService) MessagesGetAttachMenuBot(ctx context.Context, in *mtproto.TLMessagesGetAttachMenuBot) (*mtproto.AttachMenuBotsBot, error) {
	self, _ := selfID(ctx)
	return chatbox.Shared().AttachBot(ctx, self, in.GetBot())
}

func (s BotChatService) MessagesToggleBotInAttachMenu(ctx context.Context, in *mtproto.TLMessagesToggleBotInAttachMenu) (*mtproto.Bool, error) {
	self, _ := selfID(ctx)
	return chatbox.Shared().ToggleAttach(ctx, self, in.GetBot(), mtproto.FromBool(in.GetEnabled()), in.GetWriteAllowed())
}

func (s BotChatService) MessagesRequestWebView(ctx context.Context, in *mtproto.TLMessagesRequestWebView) (*mtproto.WebViewResult, error) {
	self, _ := selfID(ctx)
	botID, err := botID(self, in.GetBot())
	if err != nil {
		return nil, err
	}
	peer, err := inputPeer(self, in.GetPeer())
	if err != nil {
		return nil, err
	}
	return chatbox.Shared().OpenWebView(ctx, chatboxOpen(self, botID, peer, in.GetFullscreen(), in.GetCompact(), strVal(in.GetStartParam()), strVal(in.GetUrl()), "menu"))
}

func (s BotChatService) MessagesProlongWebView(ctx context.Context, in *mtproto.TLMessagesProlongWebView) (*mtproto.Bool, error) {
	self, _ := selfID(ctx)
	botID, err := botID(self, in.GetBot())
	if err != nil {
		return nil, err
	}
	peer, err := inputPeer(self, in.GetPeer())
	if err != nil {
		return nil, err
	}
	return chatbox.Shared().ProlongWebView(ctx, self, botID, in.GetQueryId(), peer)
}

func (s BotChatService) MessagesRequestSimpleWebView413A3E73(ctx context.Context, in *mtproto.TLMessagesRequestSimpleWebView413A3E73) (*mtproto.WebViewResult, error) {
	self, _ := selfID(ctx)
	botID, err := botID(self, in.GetBot())
	if err != nil {
		return nil, err
	}
	kind := "simple"
	return chatbox.Shared().OpenWebView(ctx, chatboxOpen(self, botID, nil, in.GetFullscreen(), in.GetCompact(), strVal(in.GetStartParam()), strVal(in.GetUrl()), kind))
}

func (s BotChatService) MessagesRequestAppWebView53618BCE(ctx context.Context, in *mtproto.TLMessagesRequestAppWebView53618BCE) (*mtproto.WebViewResult, error) {
	self, _ := selfID(ctx)
	id, short, err := chatbox.Shared().AppByRef(ctx, self, in.GetApp())
	if err != nil {
		return nil, err
	}
	peer, err := inputPeer(self, in.GetPeer())
	if err != nil {
		return nil, err
	}
	return chatbox.Shared().OpenWebView(ctx, chatboxOpen(self, id, peer, in.GetFullscreen(), in.GetCompact(), strVal(in.GetStartParam()), short, "app"))
}

func (s BotChatService) MessagesRequestMainWebView(ctx context.Context, in *mtproto.TLMessagesRequestMainWebView) (*mtproto.WebViewResult, error) {
	self, _ := selfID(ctx)
	botID, err := botID(self, in.GetBot())
	if err != nil {
		return nil, err
	}
	peer, err := inputPeer(self, in.GetPeer())
	if err != nil {
		return nil, err
	}
	return chatbox.Shared().OpenWebView(ctx, chatboxOpen(self, botID, peer, in.GetFullscreen(), in.GetCompact(), strVal(in.GetStartParam()), "", "main"))
}

func (s BotChatService) MessagesRequestChatJoinWebView(ctx context.Context, in *mtproto.TLMessagesRequestChatJoinWebView) (*mtproto.WebViewResult, error) {
	self, _ := selfID(ctx)
	return chatbox.Shared().OpenWebView(ctx, chatboxOpen(self, in.GetQueryId(), nil, false, false, "", "", "join"))
}

func (s BotChatService) MessagesGetBotApp(ctx context.Context, in *mtproto.TLMessagesGetBotApp) (*mtproto.Messages_BotApp, error) {
	self, _ := selfID(ctx)
	return chatbox.Shared().GetBotApp(ctx, self, in.GetApp(), in.GetHash())
}

func (s BotChatService) MessagesSendWebViewData(ctx context.Context, in *mtproto.TLMessagesSendWebViewData) (*mtproto.Updates, error) {
	self, auth := selfID(ctx)
	if ups, ok := chatbox.Shared().SeenRandom(ctx, self, in.GetRandomId()); ok {
		return ups, nil
	}
	req, err := chatbox.Shared().WebViewData(ctx, self, in.GetBot(), in.GetRandomId(), in.GetButtonText(), in.GetData())
	if err != nil {
		return nil, err
	}
	return finish(ctx, s.Msg, self, auth, in.GetRandomId(), req, nil)
}

func (s BotChatService) MessagesSavePreparedInlineMessage(ctx context.Context, in *mtproto.TLMessagesSavePreparedInlineMessage) (*mtproto.Messages_BotPreparedInlineMessage, error) {
	self, _ := selfID(ctx)
	forUser, err := botID(self, in.GetUserId())
	if err != nil {
		return nil, err
	}
	return chatbox.Shared().SavePrepared(ctx, self, forUser, in.GetResult(), in.GetPeerTypes())
}

func (s BotChatService) MessagesGetPreparedInlineMessage(ctx context.Context, in *mtproto.TLMessagesGetPreparedInlineMessage) (*mtproto.Messages_PreparedInlineMessage, error) {
	self, _ := selfID(ctx)
	return chatbox.Shared().PreparedInline(ctx, self, in.GetBot(), in.GetId())
}

func botID(self int64, in *mtproto.InputUser) (int64, error) {
	if in == nil {
		return 0, mtproto.ErrBotInvalid
	}
	p := mtproto.FromInputUser(self, in)
	if p == nil || p.PeerId <= 0 {
		return 0, mtproto.ErrBotInvalid
	}
	return p.PeerId, nil
}

func inputPeer(self int64, in *mtproto.InputPeer) (*mtproto.PeerUtil, error) {
	if in == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	p := mtproto.FromInputPeer2(self, in)
	if p != nil && p.IsSelf() {
		p.PeerType = mtproto.PEER_USER
		p.PeerId = self
	}
	if p == nil || !p.IsUserOrChatOrChannel() || p.PeerId == 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	return p, nil
}

func chatboxOpen(self, botID int64, peer *mtproto.PeerUtil, fullscreen, compact bool, start, raw, kind string) chatbox.WebOpen {
	return chatbox.WebOpen{
		Self: self, BotID: botID, Peer: peer, Fullscreen: fullscreen, Compact: compact,
		StartParam: start, URL: raw, Kind: kind,
	}
}
