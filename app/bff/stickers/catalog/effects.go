package catalog

import (
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/emojiblock"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// AvailableEffects is messages.getAvailableEffects.
// Each effect reuses the reaction icon documents so upload.getFile can
// serve the same bytes. A matching hash returns
// messages.availableEffectsNotModified.
// https://core.telegram.org/method/messages.getAvailableEffects
func (s *Service) AvailableEffects(hash int32) *mtproto.Messages_AvailableEffects {
	var effects []*mtproto.AvailableEffect
	var docs []*mtproto.Document
	var ids []int64
	seenDoc := map[int64]bool{}
	for _, spec := range reactionSpecs() {
		alt := emojiFromCode(spec.code)
		if alt == "" || emojiblock.Blocked(alt) {
			continue
		}
		webp, tgs := reactionArt(spec.code)
		if len(webp) == 0 || len(tgs) == 0 || !tgsCanvas512(tgs) {
			continue
		}
		staticID := stableID("reaction", "static", spec.code)
		animID := stableID("reaction", "anim", spec.code)
		effectID := stableID("effect", spec.code)
		if !seenDoc[staticID] {
			seenDoc[staticID] = true
			docs = append(docs, reactionIconDocument(staticID, "image/webp", spec.code+".webp", alt, 72, 72, len(webp)))
		}
		if !seenDoc[animID] {
			seenDoc[animID] = true
			docs = append(docs, reactionIconDocument(animID, "application/x-tgsticker", spec.code+".tgs", alt, 512, 512, len(tgs)))
		}
		effects = append(effects, mtproto.MakeTLAvailableEffect(&mtproto.AvailableEffect{
			Id:                effectID,
			Emoticon:          alt,
			StaticIconId:      wrapperspb.Int64(staticID),
			EffectStickerId:   animID,
			EffectAnimationId: wrapperspb.Int64(animID),
		}).To_AvailableEffect())
		ids = append(ids, effectID)
	}
	sum := int32(hash64(ids))
	if hash != 0 && hash == sum {
		return mtproto.MakeTLMessagesAvailableEffectsNotModified(&mtproto.Messages_AvailableEffects{}).To_Messages_AvailableEffects()
	}
	if effects == nil {
		effects = []*mtproto.AvailableEffect{}
	}
	if docs == nil {
		docs = []*mtproto.Document{}
	}
	return mtproto.MakeTLMessagesAvailableEffects(&mtproto.Messages_AvailableEffects{
		Hash:      sum,
		Effects:   effects,
		Documents: docs,
	}).To_Messages_AvailableEffects()
}

// DefaultTagReactions is the short list messages.getDefaultTagReactions
// offers for saved-message tags. It follows the picker order, not usage counts.
func (s *Service) DefaultTagReactions(hash int64) *mtproto.Messages_Reactions {
	var emos []string
	for _, spec := range requiredReactions {
		emo := emojiFromCode(spec.code)
		if emo == "" || emojiblock.Blocked(emo) {
			continue
		}
		emos = append(emos, emo)
	}
	return reactionPage(emos, 0, hash)
}

// EmojiURL is messages.getEmojiURL. The URL names the keyword pack for
// lang. Keyword search itself stays on messages.getEmojiKeywords.
// https://core.telegram.org/method/messages.getEmojiURL
func (s *Service) EmojiURL(lang string) (*mtproto.EmojiURL, error) {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if !emojiLangOK(lang) {
		return nil, mtproto.ErrLangCodeInvalid
	}
	return mtproto.MakeTLEmojiURL(&mtproto.EmojiURL{
		Url: "https://telegram.org/resources/emoji/keywords/" + lang,
	}).To_EmojiURL(), nil
}

func emojiLangOK(lang string) bool {
	if lang == "" || len(lang) > 8 {
		return false
	}
	for _, r := range lang {
		if r >= 'a' && r <= 'z' || r == '-' {
			continue
		}
		return false
	}
	return strings.Contains(lang, "-") || len(lang) == 2
}
