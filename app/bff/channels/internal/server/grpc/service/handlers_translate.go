package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

// messageReader loads stored private and basic-group messages.
type messageReader interface {
	MessageGetUserMessageList(ctx context.Context, in *message.TLMessageGetUserMessageList) (*message.Vector_MessageBox, error)
}

// MessagesTranslateText
// messages.translateText#a5eec345 flags:# peer:flags.0?InputPeer id:flags.0?Vector<int> text:flags.1?Vector<TextWithEntities> to_lang:string tone:flags.2?string = messages.TranslatedText;
// https://core.telegram.org/method/messages.translateText
func (s TranslationService) MessagesTranslateText(ctx context.Context, in *mtproto.TLMessagesTranslateText) (*mtproto.Messages_TranslatedText, error) {
	if !chatbox.ValidToLang(in.GetToLang()) {
		return nil, mtproto.ErrToLangInvalid
	}
	if in.GetTone() != nil && len([]rune(in.GetTone().GetValue())) > 64 {
		return nil, mtproto.ErrToLangInvalid
	}
	texts := in.GetText_FLAGVECTORTEXTWITHENTITIES()
	var parts []textPart
	if len(texts) > 0 {
		for _, t := range texts {
			if t == nil {
				parts = append(parts, textPart{})
				continue
			}
			parts = append(parts, textPart{text: t.GetText(), entities: t.GetEntities()})
		}
	} else {
		self, _ := actor(ctx)
		peer := mtproto.FromInputPeer2(self, in.GetPeer())
		if peer == nil || !peer.IsUserOrChatOrChannel() {
			return nil, mtproto.ErrPeerIdInvalid
		}
		ids := in.GetId()
		if len(ids) == 0 {
			return nil, mtproto.ErrMessageIdsEmpty
		}
		loaded, err := s.loadParts(ctx, self, peer, ids)
		if err != nil {
			return nil, err
		}
		parts = loaded
	}
	out := make([]*mtproto.TextWithEntities, 0, len(parts))
	for _, part := range parts {
		one, err := chatbox.TranslateText(part.text, part.entities, in.GetToLang())
		if err != nil {
			return nil, err
		}
		out = append(out, one)
	}
	return mtproto.MakeTLMessagesTranslateResult(&mtproto.Messages_TranslatedText{
		Result: out,
	}).To_Messages_TranslatedText(), nil
}

// MessagesTogglePeerTranslations
// messages.togglePeerTranslations#e47cb579 flags:# disabled:flags.0?true peer:InputPeer = Bool;
// https://core.telegram.org/method/messages.togglePeerTranslations
func (TranslationService) MessagesTogglePeerTranslations(ctx context.Context, in *mtproto.TLMessagesTogglePeerTranslations) (*mtproto.Bool, error) {
	self, _ := actor(ctx)
	peer := mtproto.FromInputPeer2(self, in.GetPeer())
	if err := chatbox.Shared().TogglePeerTranslations(ctx, self, peer, in.GetDisabled()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

type textPart struct {
	text     string
	entities []*mtproto.MessageEntity
}

func (s TranslationService) loadParts(ctx context.Context, self int64, peer *mtproto.PeerUtil, ids []int32) ([]textPart, error) {
	missing := make([]int32, 0)
	found := map[int32]textPart{}
	for _, id := range ids {
		if id <= 0 {
			return nil, mtproto.ErrMessageIdInvalid
		}
		text, ents, ok := chatbox.Shared().LookupPlain(ctx, self, peer, id)
		if ok {
			found[id] = textPart{text: text, entities: ents}
			continue
		}
		missing = append(missing, id)
	}
	if len(missing) > 0 && peer.IsChannel() {
		for _, id := range missing {
			box, err := channel.Shared().MessageBox(ctx, self, peer.PeerId, id)
			if err != nil || box == nil || box.GetMessage() == nil || box.GetMessage().GetPredicateName() == mtproto.Predicate_messageEmpty {
				return nil, mtproto.ErrMessageIdInvalid
			}
			msg := box.GetMessage()
			found[id] = textPart{text: msg.GetMessage(), entities: msg.GetEntities()}
		}
		missing = nil
	}
	if len(missing) > 0 && s.Messages != nil {
		list, err := s.Messages.MessageGetUserMessageList(ctx, &message.TLMessageGetUserMessageList{
			UserId: self,
			IdList: missing,
		})
		if err == nil && list != nil {
			for _, box := range list.Datas {
				if box == nil || box.GetMessage() == nil || box.GetMessage().GetPredicateName() == mtproto.Predicate_messageEmpty {
					continue
				}
				msg := box.GetMessage()
				found[msg.GetId()] = textPart{text: msg.GetMessage(), entities: msg.GetEntities()}
			}
		}
	}
	out := make([]textPart, 0, len(ids))
	for _, id := range ids {
		part, ok := found[id]
		if !ok {
			return nil, mtproto.ErrMessageIdInvalid
		}
		out = append(out, part)
	}
	return out, nil
}
