package p1rest

import (
	"strings"

	"github.com/teamgram/proto/mtproto"
)

// statsPage is the page messages.getStatsURL opens. There is no hosted
// statistics graph; the URL does not use tg:statsrefresh, so the Android
// webview does not call the method again.
const statsPage = "https://tellomi.cn/"

func composeAI(in *mtproto.TLMessagesComposeMessageWithAI) (*mtproto.Messages_ComposedMessageWithAI, error) {
	if in == nil || in.GetText() == nil || strings.TrimSpace(in.GetText().GetText()) == "" {
		return nil, mtproto.ErrMessageEmpty
	}
	if !aiTask(in) {
		return nil, mtproto.ErrAiComposeTaskMissing
	}
	// No composer model is configured. Returning the original text is a
	// schema-valid refusal: the draft stays, and the client does not hang.
	return mtproto.MakeTLMessagesComposedMessageWithAI(&mtproto.Messages_ComposedMessageWithAI{
		ResultText: in.GetText(),
	}).To_Messages_ComposedMessageWithAI(), nil
}

func aiTask(in *mtproto.TLMessagesComposeMessageWithAI) bool {
	if in.GetProofread() || in.GetEmojify() || in.GetTone() != nil {
		return true
	}
	lang := in.GetTranslateToLang()
	return lang != nil && strings.TrimSpace(lang.GetValue()) != ""
}

func emojiGame() *mtproto.Messages_EmojiGameInfo {
	return mtproto.MakeTLMessagesEmojiGameUnavailable(&mtproto.Messages_EmojiGameInfo{}).To_Messages_EmojiGameInfo()
}

func statsURL(self int64, in *mtproto.TLMessagesGetStatsURL) (*mtproto.StatsURL, error) {
	if _, err := channelOnly(self, in.GetPeer()); err != nil {
		return nil, err
	}
	_ = in.GetDark()
	_ = in.GetParams()
	return mtproto.MakeTLStatsURL(&mtproto.StatsURL{Url: statsPage}).To_StatsURL(), nil
}

func (s *store) suggest(self int64, in *mtproto.TLMessagesToggleSuggestedPostApproval) (*mtproto.Updates, error) {
	ref, _, err := peerOf(self, in.GetPeer())
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.suggested[todoKey{user: self, kind: ref.kind, id: ref.id, msg: in.GetMsgId()}] = in.GetReject()
	s.mu.Unlock()
	return mtproto.MakeEmptyUpdates(), nil
}
