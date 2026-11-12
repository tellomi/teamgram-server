package richmsg

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"github.com/teamgram/teamgram-server/pkg/tellomiai"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const walkLimit = 32

var htmlTag = regexp.MustCompile(`<[^>]*>`)
var mdLink = regexp.MustCompile(`\[([^\[\]]+)\]\([^)]+\)`)

type slot struct {
	text string
	set  func(string)
}

func htmlPlain(s string) string {
	s = htmlTag.ReplaceAllString(s, "")
	s = strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&#39;", "'",
		"&nbsp;", " ",
	).Replace(s)
	return strings.TrimSpace(s)
}

func markdownPlain(s string) string {
	s = mdLink.ReplaceAllString(s, "$1")
	s = strings.NewReplacer("**", "", "__", "", "`", "", "*", "", "_", "").Replace(s)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#"))
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func inputEmpty(in *mtproto.InputRichMessage) bool {
	if in == nil {
		return true
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputRichMessageHTML:
		return strings.TrimSpace(htmlPlain(in.GetHtml())) == "" && len(in.GetFiles()) == 0
	case mtproto.Predicate_inputRichMessageMarkdown:
		return strings.TrimSpace(markdownPlain(in.GetMarkdown())) == "" && len(in.GetFiles()) == 0
	default:
		if len(in.GetPhotos()) > 0 || len(in.GetDocuments()) > 0 || len(in.GetUsers()) > 0 {
			return false
		}
		return !blockContent(in.GetBlocks(), 0)
	}
}

func blockContent(blocks []*mtproto.PageBlock, depth int) bool {
	if depth > walkLimit {
		return false
	}
	for _, block := range blocks {
		if block == nil {
			continue
		}
		switch block.GetPredicateName() {
		case mtproto.Predicate_pageBlockPhoto, mtproto.Predicate_pageBlockVideo,
			mtproto.Predicate_pageBlockAudio, mtproto.Predicate_pageBlockEmbed,
			mtproto.Predicate_pageBlockMap, mtproto.Predicate_pageBlockCollage,
			mtproto.Predicate_pageBlockSlideshow:
			return true
		}
		if strings.TrimSpace(richPlain(block.GetText(), depth+1)) != "" ||
			strings.TrimSpace(richPlain(block.GetAuthor_RICHTEXT(), depth+1)) != "" ||
			strings.TrimSpace(richPlain(block.GetCaption_RICHTEXT(), depth+1)) != "" ||
			strings.TrimSpace(richPlain(block.GetTitle(), depth+1)) != "" {
			return true
		}
		if cap := block.GetCaption_PAGECAPTION(); cap != nil {
			if strings.TrimSpace(richPlain(cap.GetText(), depth+1)) != "" || strings.TrimSpace(richPlain(cap.GetCredit(), depth+1)) != "" {
				return true
			}
		}
		if blockContent(block.GetBlocks(), depth+1) || blockContent(block.GetItems_VECTORPAGEBLOCK(), depth+1) {
			return true
		}
		if cover := block.GetCover(); cover != nil && blockContent([]*mtproto.PageBlock{cover}, depth+1) {
			return true
		}
		for _, item := range block.GetItems_VECTORPAGELISTITEM() {
			if item != nil && (strings.TrimSpace(richPlain(item.GetText(), depth+1)) != "" || blockContent(item.GetBlocks(), depth+1)) {
				return true
			}
		}
		for _, item := range block.GetItems_VECTORPAGELISTORDEREDITEM() {
			if item != nil && (strings.TrimSpace(richPlain(item.GetText(), depth+1)) != "" || blockContent(item.GetBlocks(), depth+1)) {
				return true
			}
		}
		for _, row := range block.GetRows() {
			if row == nil {
				continue
			}
			for _, cell := range row.GetCells() {
				if cell != nil && strings.TrimSpace(richPlain(cell.GetText(), depth+1)) != "" {
					return true
				}
			}
		}
	}
	return false
}

func richPlain(text *mtproto.RichText, depth int) string {
	if text == nil || depth > walkLimit {
		return ""
	}
	switch text.GetPredicateName() {
	case mtproto.Predicate_textPlain:
		return text.GetText_STRING()
	case mtproto.Predicate_textMath:
		return text.GetSource()
	case mtproto.Predicate_textConcat:
		var b strings.Builder
		for _, part := range text.GetTexts() {
			b.WriteString(richPlain(part, depth+1))
		}
		return b.String()
	case "", mtproto.Predicate_textEmpty, mtproto.Predicate_textImage, mtproto.Predicate_textCustomEmoji:
		return ""
	default:
		if child := text.GetText_RICHTEXT(); child != nil {
			return richPlain(child, depth+1)
		}
		var b strings.Builder
		for _, part := range text.GetTexts() {
			b.WriteString(richPlain(part, depth+1))
		}
		if old := text.GetOldText(); old != nil {
			b.WriteString(richPlain(old, depth+1))
		}
		return b.String()
	}
}

func rewriteInput(ctx context.Context, in *mtproto.InputRichMessage, job tellomiai.Job) (*mtproto.RichMessage, error) {
	if in == nil {
		return paragraph("", false), nil
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputRichMessageHTML:
		plain, ents := parseHTML(in.GetHtml())
		return plainToRich(ctx, plain, ents, in.GetRtl(), job)
	case mtproto.Predicate_inputRichMessageMarkdown:
		plain, ents := parseMarkdown(in.GetMarkdown())
		return plainToRich(ctx, plain, ents, in.GetRtl(), job)
	default:
		blocks := cloneBlocks(in.GetBlocks())
		if err := rewriteBlocks(ctx, blocks, job); err != nil {
			return nil, err
		}
		return richFromBlocks(in.GetRtl(), blocks), nil
	}
}

func plainToRich(ctx context.Context, plain string, ents []*mtproto.MessageEntity, rtl bool, job tellomiai.Job) (*mtproto.RichMessage, error) {
	out, err := chatbox.RewritePlain(ctx, plain, ents, job)
	if err != nil {
		return nil, err
	}
	return richFromPlain(out.GetText(), out.GetEntities(), rtl), nil
}

func rewriteBlocks(ctx context.Context, blocks []*mtproto.PageBlock, job tellomiai.Job) error {
	var slots []slot
	collectBlocks(blocks, &slots, 0)
	return applySlots(ctx, slots, job)
}

func applySlots(ctx context.Context, slots []slot, job tellomiai.Job) error {
	if len(slots) == 0 {
		return nil
	}
	texts := make([]string, len(slots))
	for i, one := range slots {
		texts[i] = one.text
	}
	job.Texts = texts
	out, err := tellomiai.Rewrite(ctx, job)
	if err != nil {
		return failJob(job, err)
	}
	for i, one := range slots {
		one.set(out[i])
	}
	return nil
}

func failJob(job tellomiai.Job, err error) error {
	if errors.Is(err, tellomiai.ErrTooLong) {
		return mtproto.ErrInputTextTooLong
	}
	if errors.Is(err, tellomiai.ErrSidecarBusy) {
		return mtproto.ErrTranslateBusy
	}
	if job.Mode == tellomiai.ModeCompose {
		return mtproto.ErrAiComposeTimeout
	}
	return mtproto.ErrTranslateReqFailed
}

func cloneBlocks(blocks []*mtproto.PageBlock) []*mtproto.PageBlock {
	out := make([]*mtproto.PageBlock, 0, len(blocks))
	for _, block := range blocks {
		if block == nil {
			continue
		}
		cp, ok := proto.Clone(block).(*mtproto.PageBlock)
		if !ok || cp == nil {
			continue
		}
		out = append(out, cp)
	}
	return out
}

func collectBlocks(blocks []*mtproto.PageBlock, slots *[]slot, depth int) {
	if depth > walkLimit {
		return
	}
	for _, block := range blocks {
		if block == nil {
			continue
		}
		freeze := block.GetPredicateName() == mtproto.Predicate_pageBlockPreformatted
		collectRich(block.GetText(), freeze, slots, depth+1)
		collectRich(block.GetAuthor_RICHTEXT(), false, slots, depth+1)
		collectRich(block.GetCaption_RICHTEXT(), false, slots, depth+1)
		collectRich(block.GetTitle(), false, slots, depth+1)
		if cap := block.GetCaption_PAGECAPTION(); cap != nil {
			collectRich(cap.GetText(), false, slots, depth+1)
			collectRich(cap.GetCredit(), false, slots, depth+1)
		}
		collectBlocks(block.GetBlocks(), slots, depth+1)
		collectBlocks(block.GetItems_VECTORPAGEBLOCK(), slots, depth+1)
		if cover := block.GetCover(); cover != nil {
			collectBlocks([]*mtproto.PageBlock{cover}, slots, depth+1)
		}
		for _, item := range block.GetItems_VECTORPAGELISTITEM() {
			if item == nil {
				continue
			}
			collectRich(item.GetText(), false, slots, depth+1)
			collectBlocks(item.GetBlocks(), slots, depth+1)
		}
		for _, item := range block.GetItems_VECTORPAGELISTORDEREDITEM() {
			if item == nil {
				continue
			}
			collectRich(item.GetText(), false, slots, depth+1)
			collectBlocks(item.GetBlocks(), slots, depth+1)
		}
		for _, row := range block.GetRows() {
			if row == nil {
				continue
			}
			for _, cell := range row.GetCells() {
				if cell == nil {
					continue
				}
				collectRich(cell.GetText(), false, slots, depth+1)
			}
		}
		for _, article := range block.GetArticles() {
			if article == nil {
				continue
			}
			collectValue(article.GetTitle(), slots)
			collectValue(article.GetDescription(), slots)
			collectValue(article.GetAuthor(), slots)
		}
	}
}

func collectValue(v *wrapperspb.StringValue, slots *[]slot) {
	if v == nil || strings.TrimSpace(v.GetValue()) == "" {
		return
	}
	captured := v
	*slots = append(*slots, slot{text: v.GetValue(), set: func(s string) { captured.Value = s }})
}

func collectRich(text *mtproto.RichText, freeze bool, slots *[]slot, depth int) {
	if text == nil || depth > walkLimit || freeze {
		return
	}
	switch text.GetPredicateName() {
	case mtproto.Predicate_textPlain:
		if strings.TrimSpace(text.GetText_STRING()) == "" {
			return
		}
		captured := text
		*slots = append(*slots, slot{text: text.GetText_STRING(), set: func(s string) { captured.Text_STRING = s }})
	case mtproto.Predicate_textMath, mtproto.Predicate_textFixed,
		mtproto.Predicate_textEmpty, mtproto.Predicate_textImage, mtproto.Predicate_textCustomEmoji:
		return
	case mtproto.Predicate_textConcat:
		for _, part := range text.GetTexts() {
			collectRich(part, false, slots, depth+1)
		}
	default:
		collectRich(text.GetText_RICHTEXT(), false, slots, depth+1)
		for _, part := range text.GetTexts() {
			collectRich(part, false, slots, depth+1)
		}
		collectRich(text.GetOldText(), false, slots, depth+1)
	}
}

func paragraph(text string, rtl bool) *mtproto.RichMessage {
	plain := mtproto.MakeTLTextPlain(&mtproto.RichText{Text_STRING: text}).To_RichText()
	block := mtproto.MakeTLPageBlockParagraph(&mtproto.PageBlock{Text: plain}).To_PageBlock()
	return richFromBlocks(rtl, []*mtproto.PageBlock{block})
}

func richFromBlocks(rtl bool, blocks []*mtproto.PageBlock) *mtproto.RichMessage {
	if blocks == nil {
		blocks = []*mtproto.PageBlock{}
	}
	return mtproto.MakeTLRichMessage(&mtproto.RichMessage{
		Rtl:       rtl,
		Blocks:    blocks,
		Photos:    []*mtproto.Photo{},
		Documents: []*mtproto.Document{},
	}).To_RichMessage()
}

func copyPlain(ctx context.Context, text string, entities []*mtproto.MessageEntity, job tellomiai.Job) (*mtproto.RichMessage, error) {
	out, err := chatbox.RewritePlain(ctx, text, entities, job)
	if err != nil {
		return nil, err
	}
	return richFromPlain(out.GetText(), out.GetEntities(), false), nil
}

func visible(msg *mtproto.RichMessage) string {
	if msg == nil {
		return ""
	}
	var b strings.Builder
	for _, block := range msg.GetBlocks() {
		if block == nil {
			continue
		}
		b.WriteString(richPlain(block.GetText(), 0))
	}
	return b.String()
}

func toneTooLong(tone string) bool {
	return utf8.RuneCountInString(tone) > 64
}
