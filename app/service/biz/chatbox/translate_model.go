package chatbox

import (
	"context"
	"errors"
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/tellomiai"
	"google.golang.org/protobuf/proto"
)

// RewritePlain rewrites text with job and shifts entity offsets.
// Callers validate to_lang before a translate job.
func RewritePlain(ctx context.Context, text string, entities []*mtproto.MessageEntity, job tellomiai.Job) (*mtproto.TextWithEntities, error) {
	return rewritePlain(ctx, text, entities, job)
}

func translateJob(in TranslateInput) tellomiai.Job {
	return tellomiai.Job{
		Mode:     tellomiai.ModeTranslate,
		ToLang:   in.ToLang,
		FromLang: in.FromLang,
		Tone:     in.Tone,
	}
}

// rewritePlain sends the translatable spans to the model and shifts entity
// offsets. Opaque spans (URLs, code, mentions, custom emoji) are copied.
func rewritePlain(ctx context.Context, text string, entities []*mtproto.MessageEntity, job tellomiai.Job) (*mtproto.TextWithEntities, error) {
	if !modelOn(job.Mode) {
		return nil, modelErr(job.Mode, tellomiai.ErrDisabled)
	}
	units := utf16.Encode([]rune(text))
	spans := cutSpans(units, entities)
	var pending []string
	for i := range spans {
		if spans[i].frozen {
			continue
		}
		spans[i].idx = len(pending)
		pending = append(pending, spans[i].text)
	}
	var rewritten []string
	if len(pending) > 0 {
		job.Texts = pending
		got, err := tellomiai.Rewrite(ctx, job)
		if err != nil {
			return nil, modelErr(job.Mode, err)
		}
		rewritten = got
	}
	outUnits := make([]uint16, 0, len(units))
	pos := map[int]int{0: 0}
	for _, span := range spans {
		piece := span.text
		if !span.frozen {
			piece = rewritten[span.idx]
		}
		outUnits = append(outUnits, utf16.Encode([]rune(piece))...)
		pos[span.end] = len(outUnits)
	}
	outEnt := shiftEntities(entities, pos, len(units))
	return mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{
		Text:     string(utf16.Decode(outUnits)),
		Entities: outEnt,
	}).To_TextWithEntities(), nil
}

type span struct {
	start, end int
	text       string
	frozen     bool
	idx        int
}

func cutSpans(units []uint16, entities []*mtproto.MessageEntity) []span {
	n := len(units)
	points := []int{0, n}
	for _, ent := range entities {
		if !entityCovers(ent, n) {
			continue
		}
		points = append(points, int(ent.GetOffset()), int(ent.GetOffset()+ent.GetLength()))
	}
	sort.Ints(points)
	uniq := points[:0]
	for _, p := range points {
		if len(uniq) == 0 || uniq[len(uniq)-1] != p {
			uniq = append(uniq, p)
		}
	}
	var spans []span
	for i := 0; i+1 < len(uniq); i++ {
		a, b := uniq[i], uniq[i+1]
		text := string(utf16.Decode(units[a:b]))
		spans = append(spans, span{
			start:  a,
			end:    b,
			text:   text,
			frozen: strings.TrimSpace(text) == "" || spanOpaque(entities, a, b),
		})
	}
	return spans
}

func entityCovers(ent *mtproto.MessageEntity, n int) bool {
	if ent == nil || ent.GetLength() <= 0 || ent.GetOffset() < 0 {
		return false
	}
	end := int64(ent.GetOffset()) + int64(ent.GetLength())
	return end <= int64(n)
}

func spanOpaque(entities []*mtproto.MessageEntity, start, end int) bool {
	for _, ent := range entities {
		if ent == nil || !opaqueEntity(ent.GetPredicateName()) {
			continue
		}
		if int(ent.GetOffset()) <= start && int(ent.GetOffset()+ent.GetLength()) >= end {
			return true
		}
	}
	return false
}

func opaqueEntity(name string) bool {
	switch name {
	case mtproto.Predicate_messageEntityUrl,
		mtproto.Predicate_messageEntityEmail,
		mtproto.Predicate_messageEntityPhone,
		mtproto.Predicate_messageEntityMention,
		mtproto.Predicate_messageEntityBotCommand,
		mtproto.Predicate_messageEntityCashtag,
		mtproto.Predicate_messageEntityBankCard,
		mtproto.Predicate_messageEntityCustomEmoji,
		mtproto.Predicate_messageEntityCode,
		mtproto.Predicate_messageEntityPre,
		mtproto.Predicate_messageEntityFormattedDate,
		mtproto.Predicate_messageEntityTonAddress:
		return true
	default:
		return false
	}
}

func shiftEntities(entities []*mtproto.MessageEntity, pos map[int]int, n int) []*mtproto.MessageEntity {
	out := make([]*mtproto.MessageEntity, 0, len(entities))
	for _, ent := range entities {
		if !entityCovers(ent, n) {
			continue
		}
		start := int(ent.GetOffset())
		end := start + int(ent.GetLength())
		ns, okS := pos[start]
		ne, okE := pos[end]
		if !okS || !okE || ne <= ns {
			continue
		}
		cp, ok := proto.Clone(ent).(*mtproto.MessageEntity)
		if !ok || cp == nil {
			continue
		}
		cp.Offset = int32(ns)
		cp.Length = int32(ne - ns)
		out = append(out, cp)
	}
	return out
}

func modelOn(mode string) bool {
	if mode == tellomiai.ModeCompose {
		return tellomiai.ComposeAdvertised()
	}
	return tellomiai.TranslateAdvertised()
}

func modelErr(mode string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, tellomiai.ErrTooLong) {
		return mtproto.ErrInputTextTooLong
	}
	if mode == tellomiai.ModeCompose {
		return mtproto.ErrAiComposeTimeout
	}
	return mtproto.ErrTranslateReqFailed
}
