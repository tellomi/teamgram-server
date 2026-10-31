package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/accountassets"
	"github.com/teamgram/teamgram-server/app/bff/botadmin"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
)

// AccountGetBotBusinessConnection implements account.getBotBusinessConnection.
// https://core.telegram.org/method/account.getBotBusinessConnection
func (ConnectedBotsService) AccountGetBotBusinessConnection(ctx context.Context, in *mtproto.TLAccountGetBotBusinessConnection) (*mtproto.Updates, error) {
	self, _ := selfID(ctx)
	id := ""
	if in != nil {
		id = in.GetConnectionId()
	}
	return accountassets.BotBusinessConnection(self, id)
}

// MessagesSetGameScore implements messages.setGameScore.
// This process does not keep game messages, so a well-formed call still
// names a message that is not a game.
// https://core.telegram.org/method/messages.setGameScore
func (GamesService) MessagesSetGameScore(ctx context.Context, in *mtproto.TLMessagesSetGameScore) (*mtproto.Updates, error) {
	self, _ := selfID(ctx)
	if err := requireBot(ctx, self); err != nil {
		return nil, err
	}
	if in == nil || in.GetScore() < 0 {
		return nil, mtproto.ErrScoreInvalid
	}
	if _, err := chatboxPeer(self, in.GetPeer()); err != nil {
		return nil, err
	}
	if _, err := gameUser(self, in.GetUserId()); err != nil {
		return nil, err
	}
	if in.GetId() == 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	return nil, mtproto.ErrMessageIdInvalid
}

// MessagesSetInlineGameScore implements messages.setInlineGameScore.
// https://core.telegram.org/method/messages.setInlineGameScore
func (GamesService) MessagesSetInlineGameScore(ctx context.Context, in *mtproto.TLMessagesSetInlineGameScore) (*mtproto.Bool, error) {
	self, _ := selfID(ctx)
	if err := requireBot(ctx, self); err != nil {
		return nil, err
	}
	if in == nil || in.GetScore() < 0 {
		return nil, mtproto.ErrScoreInvalid
	}
	if _, err := gameUser(self, in.GetUserId()); err != nil {
		return nil, err
	}
	if !inlineIDOK(in.GetId()) {
		return nil, mtproto.ErrMessageIdInvalid
	}
	return nil, mtproto.ErrMessageIdInvalid
}

// MessagesGetGameHighScores implements messages.getGameHighScores.
// https://core.telegram.org/method/messages.getGameHighScores
func (GamesService) MessagesGetGameHighScores(ctx context.Context, in *mtproto.TLMessagesGetGameHighScores) (*mtproto.Messages_HighScores, error) {
	self, _ := selfID(ctx)
	if err := requireBot(ctx, self); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrMessageIdInvalid
	}
	if _, err := chatboxPeer(self, in.GetPeer()); err != nil {
		return nil, err
	}
	if _, err := gameUser(self, in.GetUserId()); err != nil {
		return nil, err
	}
	if in.GetId() == 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	return nil, mtproto.ErrMessageIdInvalid
}

// MessagesGetInlineGameHighScores implements messages.getInlineGameHighScores.
// https://core.telegram.org/method/messages.getInlineGameHighScores
func (GamesService) MessagesGetInlineGameHighScores(ctx context.Context, in *mtproto.TLMessagesGetInlineGameHighScores) (*mtproto.Messages_HighScores, error) {
	self, _ := selfID(ctx)
	if err := requireBot(ctx, self); err != nil {
		return nil, err
	}
	if in == nil || !inlineIDOK(in.GetId()) {
		return nil, mtproto.ErrMessageIdInvalid
	}
	if _, err := gameUser(self, in.GetUserId()); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMessageIdInvalid
}

// MessagesEditInlineBotMessage implements messages.editInlineBotMessage.
// No sent inline message is stored for later edits.
// https://core.telegram.org/method/messages.editInlineBotMessage
func (s BotChatService) MessagesEditInlineBotMessage(ctx context.Context, in *mtproto.TLMessagesEditInlineBotMessage) (*mtproto.Bool, error) {
	self, _ := selfID(ctx)
	if err := requireBot(ctx, self); err != nil {
		return nil, err
	}
	if in == nil || !inlineIDOK(in.GetId()) {
		return nil, mtproto.ErrMessageIdInvalid
	}
	if in.GetMessage() == nil && in.GetMedia() == nil && in.GetReplyMarkup() == nil && in.GetRichMessage() == nil {
		return nil, mtproto.ErrMessageEmpty
	}
	return nil, mtproto.ErrMessageIdInvalid
}

// MessagesSendWebViewResultMessage implements messages.sendWebViewResultMessage.
// https://core.telegram.org/method/messages.sendWebViewResultMessage
func (s BotChatService) MessagesSendWebViewResultMessage(ctx context.Context, in *mtproto.TLMessagesSendWebViewResultMessage) (*mtproto.WebViewMessageSent, error) {
	self, auth := selfID(ctx)
	if self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	query, result := "", (*mtproto.InputBotInlineResult)(nil)
	if in != nil {
		query = in.GetBotQueryId()
		result = in.GetResult()
	}
	req, sent, err := chatbox.Shared().TakeWebViewResult(ctx, self, query, result)
	if err != nil {
		return nil, err
	}
	if _, err = deliver(ctx, s.Msg, self, auth, req); err != nil {
		return nil, err
	}
	return sent, nil
}

// MessagesSetBotGuestChatResult implements messages.setBotGuestChatResult.
// Guest chats are not offered, so the query id is unknown.
// https://core.telegram.org/method/messages.setBotGuestChatResult
func (BotChatService) MessagesSetBotGuestChatResult(ctx context.Context, in *mtproto.TLMessagesSetBotGuestChatResult) (*mtproto.InputBotInlineMessageID, error) {
	self, _ := selfID(ctx)
	if err := requireBot(ctx, self); err != nil {
		return nil, err
	}
	if in == nil || in.GetQueryId() == 0 {
		return nil, mtproto.ErrQueryIdInvalid
	}
	if in.GetResult() == nil || in.GetResult().GetId() == "" {
		return nil, mtproto.ErrResultIdEmpty
	}
	return nil, mtproto.ErrQueryIdInvalid
}

func requireBot(ctx context.Context, self int64) error {
	if self == 0 {
		return mtproto.ErrAuthKeyUnregistered
	}
	if !botadmin.CallerIsBot(ctx, self) {
		return mtproto.ErrUserBotRequired
	}
	return nil
}

func gameUser(self int64, in *mtproto.InputUser) (int64, error) {
	if in == nil || in.GetPredicateName() == "" || in.GetPredicateName() == mtproto.Predicate_inputUserEmpty {
		return 0, mtproto.ErrUserIdInvalid
	}
	p := mtproto.FromInputUser(self, in)
	if p == nil || p.PeerId <= 0 {
		return 0, mtproto.ErrUserIdInvalid
	}
	return p.PeerId, nil
}

func chatboxPeer(self int64, in *mtproto.InputPeer) (*mtproto.PeerUtil, error) {
	if in == nil || in.GetPredicateName() == "" || in.GetPredicateName() == mtproto.Predicate_inputPeerEmpty {
		return nil, mtproto.ErrPeerIdInvalid
	}
	p := mtproto.FromInputPeer2(self, in)
	if p == nil || p.PeerId == 0 && p.PeerType != mtproto.PEER_SELF {
		return nil, mtproto.ErrPeerIdInvalid
	}
	return p, nil
}

func inlineIDOK(in *mtproto.InputBotInlineMessageID) bool {
	if in == nil {
		return false
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputBotInlineMessageID:
		return in.GetId_INT64() != 0
	case mtproto.Predicate_inputBotInlineMessageID64:
		return in.GetId_INT32() != 0
	default:
		return false
	}
}
