package richmsg

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

const walkLimit = 32

var htmlTag = regexp.MustCompile(`<[^>]*>`)
var mdLink = regexp.MustCompile(`\[([^\[\]]+)\]\([^)]+\)`)

func translateString(text, toLang string) (string, error) {
	out, err := chatbox.TranslateText(text, nil, toLang)
	if err != nil {
		return "", err
	}
	return out.GetText(), nil
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

func translateInput(in *mtproto.InputRichMessage, toLang string) (*mtproto.RichMessage, error) {
	if in == nil {
		return paragraph("", false), nil
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputRichMessageHTML:
		plain := htmlPlain(in.GetHtml())
		out, err := translateString(plain, toLang)
		if err != nil {
			return nil, err
		}
		return paragraph(out, in.GetRtl()), nil
	case mtproto.Predicate_inputRichMessageMarkdown:
		plain := markdownPlain(in.GetMarkdown())
		out, err := translateString(plain, toLang)
		if err != nil {
			return nil, err
		}
		return paragraph(out, in.GetRtl()), nil
	default:
		if err := translateBlocks(in.GetBlocks(), toLang, 0); err != nil {
			return nil, err
		}
		return richFromBlocks(in.GetRtl(), in.GetBlocks()), nil
	}
}

func translateBlocks(blocks []*mtproto.PageBlock, toLang string, depth int) error {
	if depth > walkLimit {
		return nil
	}
	for _, block := range blocks {
		if block == nil {
			continue
		}
		if err := translateRich(block.GetText(), toLang, depth+1); err != nil {
			return err
		}
		if err := translateRich(block.GetAuthor_RICHTEXT(), toLang, depth+1); err != nil {
			return err
		}
		if err := translateRich(block.GetCaption_RICHTEXT(), toLang, depth+1); err != nil {
			return err
		}
		if err := translateRich(block.GetTitle(), toLang, depth+1); err != nil {
			return err
		}
		if cap := block.GetCaption_PAGECAPTION(); cap != nil {
			if err := translateRich(cap.GetText(), toLang, depth+1); err != nil {
				return err
			}
			if err := translateRich(cap.GetCredit(), toLang, depth+1); err != nil {
				return err
			}
		}
		if err := translateBlocks(block.GetBlocks(), toLang, depth+1); err != nil {
			return err
		}
		if err := translateBlocks(block.GetItems_VECTORPAGEBLOCK(), toLang, depth+1); err != nil {
			return err
		}
		if cover := block.GetCover(); cover != nil {
			if err := translateBlocks([]*mtproto.PageBlock{cover}, toLang, depth+1); err != nil {
				return err
			}
		}
		for _, item := range block.GetItems_VECTORPAGELISTITEM() {
			if item == nil {
				continue
			}
			if err := translateRich(item.GetText(), toLang, depth+1); err != nil {
				return err
			}
			if err := translateBlocks(item.GetBlocks(), toLang, depth+1); err != nil {
				return err
			}
		}
		for _, item := range block.GetItems_VECTORPAGELISTORDEREDITEM() {
			if item == nil {
				continue
			}
			if err := translateRich(item.GetText(), toLang, depth+1); err != nil {
				return err
			}
			if err := translateBlocks(item.GetBlocks(), toLang, depth+1); err != nil {
				return err
			}
		}
		for _, row := range block.GetRows() {
			if row == nil {
				continue
			}
			for _, cell := range row.GetCells() {
				if cell == nil {
					continue
				}
				if err := translateRich(cell.GetText(), toLang, depth+1); err != nil {
					return err
				}
			}
		}
		for _, article := range block.GetArticles() {
			if article == nil {
				continue
			}
			if err := translateValue(article.GetTitle(), toLang); err != nil {
				return err
			}
			if err := translateValue(article.GetDescription(), toLang); err != nil {
				return err
			}
			if err := translateValue(article.GetAuthor(), toLang); err != nil {
				return err
			}
		}
	}
	return nil
}

func translateValue(v *wrapperspb.StringValue, toLang string) error {
	if v == nil {
		return nil
	}
	out, err := translateString(v.GetValue(), toLang)
	if err != nil {
		return err
	}
	v.Value = out
	return nil
}

func translateRich(text *mtproto.RichText, toLang string, depth int) error {
	if text == nil || depth > walkLimit {
		return nil
	}
	switch text.GetPredicateName() {
	case mtproto.Predicate_textPlain:
		out, err := translateString(text.GetText_STRING(), toLang)
		if err != nil {
			return err
		}
		text.Text_STRING = out
		return nil
	case mtproto.Predicate_textMath:
		out, err := translateString(text.GetSource(), toLang)
		if err != nil {
			return err
		}
		text.Source = out
		return nil
	case mtproto.Predicate_textConcat:
		for _, part := range text.GetTexts() {
			if err := translateRich(part, toLang, depth+1); err != nil {
				return err
			}
		}
		return nil
	case mtproto.Predicate_textEmpty, mtproto.Predicate_textImage, mtproto.Predicate_textCustomEmoji:
		return nil
	default:
		if err := translateRich(text.GetText_RICHTEXT(), toLang, depth+1); err != nil {
			return err
		}
		for _, part := range text.GetTexts() {
			if err := translateRich(part, toLang, depth+1); err != nil {
				return err
			}
		}
		return translateRich(text.GetOldText(), toLang, depth+1)
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

func copyPlain(text string, entities []*mtproto.MessageEntity, toLang string) (*mtproto.RichMessage, error) {
	out, err := chatbox.TranslateText(text, entities, toLang)
	if err != nil {
		return nil, err
	}
	return paragraph(out.GetText(), false), nil
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
