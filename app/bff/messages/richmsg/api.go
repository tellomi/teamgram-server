// Package richmsg serves the layer-232 rich-text translate and AI compose
// methods official clients call from the chat screen.
//
// Translation uses the same lexicon as messages.translateText. There is no
// external translation provider. Proofread, emojify, and tone do not call a
// model: the original rich message is returned, matching
// messages.composeMessageWithAI. help.getAppConfig keeps
// tellomi_ai_compose_enabled false.
package richmsg

import (
	"context"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/messages/aicompose"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
)

// PlainFunc loads one stored message when the chatbox index does not have it.
type PlainFunc func(ctx context.Context, self int64, peer *mtproto.PeerUtil, id int32) (string, []*mtproto.MessageEntity, bool)

// Translate implements messages.translateRichMessage.
// https://core.telegram.org/method/messages.translateText
// The rich method is the same call with InputRichMessage instead of
// textWithEntities. A non-empty text vector wins over peer and id.
func Translate(ctx context.Context, self int64, in *mtproto.TLMessagesTranslateRichMessage, load PlainFunc) (*mtproto.Messages_TranslatedRichMessage, error) {
	if in == nil || !chatbox.ValidToLang(in.GetToLang()) {
		return nil, mtproto.ErrToLangInvalid
	}
	if in.GetTone() != nil && toneTooLong(in.GetTone().GetValue()) {
		return nil, mtproto.ErrToLangInvalid
	}
	texts := in.GetText()
	var out []*mtproto.RichMessage
	if len(texts) > 0 {
		out = make([]*mtproto.RichMessage, 0, len(texts))
		for _, text := range texts {
			one, err := translateInput(text, in.GetToLang())
			if err != nil {
				return nil, err
			}
			out = append(out, one)
		}
	} else {
		if in.GetPeer() == nil {
			return nil, mtproto.ErrPeerIdInvalid
		}
		peer := mtproto.FromInputPeer2(self, in.GetPeer())
		if peer == nil || !peer.IsUserOrChatOrChannel() {
			return nil, mtproto.ErrPeerIdInvalid
		}
		ids := in.GetId()
		if len(ids) == 0 {
			return nil, mtproto.ErrMessageIdsEmpty
		}
		out = make([]*mtproto.RichMessage, 0, len(ids))
		for _, id := range ids {
			if id <= 0 {
				return nil, mtproto.ErrMessageIdInvalid
			}
			text, ents, ok := chatbox.Shared().LookupPlain(ctx, self, peer, id)
			if !ok && load != nil {
				text, ents, ok = load(ctx, self, peer, id)
			}
			if !ok {
				return nil, mtproto.ErrMessageIdInvalid
			}
			one, err := copyPlain(text, ents, in.GetToLang())
			if err != nil {
				return nil, err
			}
			out = append(out, one)
		}
	}
	return mtproto.MakeTLMessagesTranslatedRichMessage(&mtproto.Messages_TranslatedRichMessage{
		Result: out,
	}).To_Messages_TranslatedRichMessage(), nil
}

// Compose implements messages.composeRichMessageWithAI.
// https://core.telegram.org/method/messages.composeMessageWithAI
// translate_to_lang uses the lexicon. The other modes keep the source text.
func Compose(in *mtproto.TLMessagesComposeRichMessageWithAI) (*mtproto.Messages_ComposedRichMessageWithAI, error) {
	if in == nil || inputEmpty(in.GetText()) {
		return nil, mtproto.ErrMessageEmpty
	}
	if !composeTask(in) {
		return nil, mtproto.ErrAiComposeTaskMissing
	}
	if in.GetTone() != nil {
		if err := aicompose.Accept(in.GetTone()); err != nil {
			return nil, err
		}
	}
	lang := ""
	if in.GetTranslateToLang() != nil {
		lang = strings.TrimSpace(in.GetTranslateToLang().GetValue())
	}
	var (
		msg *mtproto.RichMessage
		err error
	)
	if lang != "" {
		if !chatbox.ValidToLang(lang) {
			return nil, mtproto.ErrToLangInvalid
		}
		msg, err = translateInput(in.GetText(), lang)
	} else {
		// Proofread, emojify, and tone have no model. The draft stays as written.
		msg, err = passThrough(in.GetText())
	}
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessagesComposedRichMessageWithAI(&mtproto.Messages_ComposedRichMessageWithAI{
		Result: msg,
	}).To_Messages_ComposedRichMessageWithAI(), nil
}

func composeTask(in *mtproto.TLMessagesComposeRichMessageWithAI) bool {
	if in.GetProofread() || in.GetEmojify() || in.GetTone() != nil {
		return true
	}
	lang := in.GetTranslateToLang()
	return lang != nil && strings.TrimSpace(lang.GetValue()) != ""
}

// passThrough rebuilds the rich message without the lexicon.
func passThrough(in *mtproto.InputRichMessage) (*mtproto.RichMessage, error) {
	if in == nil {
		return paragraph("", false), nil
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputRichMessageHTML:
		return paragraph(htmlPlain(in.GetHtml()), in.GetRtl()), nil
	case mtproto.Predicate_inputRichMessageMarkdown:
		return paragraph(markdownPlain(in.GetMarkdown()), in.GetRtl()), nil
	default:
		return richFromBlocks(in.GetRtl(), in.GetBlocks()), nil
	}
}
