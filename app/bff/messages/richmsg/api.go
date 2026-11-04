// Package richmsg serves the layer-232 rich-text translate and AI compose
// methods official clients call from the chat screen.
//
// Both call the Bailian client in pkg/tellomiai (qwen-plus, then deepseek-v3).
// Translation runs only when TELLOMI_TRANSLATE=1 and TELLOMI_AI_API_KEY are
// set. Compose runs only when TELLOMI_AI_COMPOSE=1 and the same key are set,
// which is also when help.getAppConfig reports tellomi_ai_compose_enabled.
// A flag that is off, or a model error, is an official RPC error.
// The source text is not returned in place of a model result.
package richmsg

import (
	"context"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/messages/aicompose"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"github.com/teamgram/teamgram-server/pkg/tellomiai"
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
	tone := ""
	if in.GetTone() != nil {
		tone = in.GetTone().GetValue()
		if toneTooLong(tone) {
			return nil, mtproto.ErrToLangInvalid
		}
	}
	type item struct {
		in   *mtproto.InputRichMessage
		text string
		ents []*mtproto.MessageEntity
		byID bool
	}
	var items []item
	texts := in.GetText()
	if len(texts) > 0 {
		items = make([]item, 0, len(texts))
		for _, text := range texts {
			items = append(items, item{in: text})
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
		items = make([]item, 0, len(ids))
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
			items = append(items, item{text: text, ents: ents, byID: true})
		}
	}
	if !tellomiai.TranslateAdvertised() {
		return nil, mtproto.ErrTranslateReqFailed
	}
	job := tellomiai.Job{Mode: tellomiai.ModeTranslate, ToLang: in.GetToLang(), Tone: tone}
	out := make([]*mtproto.RichMessage, 0, len(items))
	for _, item := range items {
		var (
			one *mtproto.RichMessage
			err error
		)
		if item.byID {
			one, err = copyPlain(ctx, item.text, item.ents, job)
		} else {
			one, err = rewriteInput(ctx, item.in, job)
		}
		if err != nil {
			return nil, err
		}
		out = append(out, one)
	}
	return mtproto.MakeTLMessagesTranslatedRichMessage(&mtproto.Messages_TranslatedRichMessage{
		Result: out,
	}).To_Messages_TranslatedRichMessage(), nil
}

// Compose implements messages.composeRichMessageWithAI.
// https://core.telegram.org/method/messages.composeMessageWithAI
func Compose(ctx context.Context, in *mtproto.TLMessagesComposeRichMessageWithAI) (*mtproto.Messages_ComposedRichMessageWithAI, error) {
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
	if lang != "" && !chatbox.ValidToLang(lang) {
		return nil, mtproto.ErrToLangInvalid
	}
	if !tellomiai.ComposeAdvertised() {
		return nil, mtproto.ErrAiComposeTimeout
	}
	tone := ""
	if in.GetTone() != nil {
		var err error
		tone, err = aicompose.Instruction(in.GetTone())
		if err != nil {
			return nil, err
		}
	}
	msg, err := rewriteInput(ctx, in.GetText(), tellomiai.Job{
		Mode:      tellomiai.ModeCompose,
		ToLang:    lang,
		Tone:      tone,
		Proofread: in.GetProofread(),
		Emojify:   in.GetEmojify(),
	})
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
