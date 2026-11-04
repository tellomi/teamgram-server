package richmsg

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"github.com/teamgram/teamgram-server/pkg/contentsafety"
	"github.com/teamgram/teamgram-server/pkg/tellomiai"

	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func useModel(t *testing.T) {
	t.Helper()
	t.Setenv(tellomiai.EnvTranslate, "1")
	t.Setenv(tellomiai.EnvCompose, "1")
	t.Setenv(tellomiai.EnvAPIKey, "test-key")
	tellomiai.SetHook(func(_ context.Context, job tellomiai.Job) ([]string, error) {
		return mockPieces(job), nil
	})
	t.Cleanup(func() {
		tellomiai.SetHook(nil)
		tellomiai.SetModerate(nil)
	})
}

func mockPieces(job tellomiai.Job) []string {
	out := make([]string, len(job.Texts))
	for i, text := range job.Texts {
		trimmed := strings.TrimSpace(text)
		if job.Mode == tellomiai.ModeCompose && job.ToLang == "" {
			switch {
			case job.Proofread:
				out[i] = "proof:" + text
			case job.Emojify:
				out[i] = text + " 🙂"
			case job.Tone != "":
				out[i] = "tone:" + text
			default:
				out[i] = "ai:" + text
			}
			continue
		}
		if strings.HasPrefix(job.ToLang, "zh") {
			switch trimmed {
			case "hello":
				out[i] = strings.Replace(text, "hello", "你好", 1)
				continue
			case "world":
				out[i] = strings.Replace(text, "world", "世界", 1)
				continue
			}
		}
		if (job.ToLang == "en" || strings.HasPrefix(job.ToLang, "en-")) && trimmed == "thanks" {
			out[i] = strings.Replace(text, "thanks", "thank you", 1)
			continue
		}
		out[i] = "«" + text
	}
	return out
}

func markdown(text string) *mtproto.InputRichMessage {
	return mtproto.MakeTLInputRichMessageMarkdown(&mtproto.InputRichMessage{
		Markdown: text,
	}).To_InputRichMessage()
}

func paragraphInput(text string) *mtproto.InputRichMessage {
	plain := mtproto.MakeTLTextPlain(&mtproto.RichText{Text_STRING: text}).To_RichText()
	block := mtproto.MakeTLPageBlockParagraph(&mtproto.PageBlock{Text: plain}).To_PageBlock()
	return mtproto.MakeTLInputRichMessage(&mtproto.InputRichMessage{
		Blocks: []*mtproto.PageBlock{block},
	}).To_InputRichMessage()
}

func mustEncode(t *testing.T, obj interface {
	Encode(*mtproto.EncodeBuf, int32) error
}) {
	t.Helper()
	buf := mtproto.NewEncodeBuf(4096)
	if err := obj.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	if buf.GetBuf() == nil || len(buf.GetBuf()) == 0 {
		t.Fatal("empty encode")
	}
}

func TestTranslateRichTextAndBlocks(t *testing.T) {
	t.Setenv(tellomiai.EnvTranslate, "")
	t.Setenv(tellomiai.EnvAPIKey, "")
	if _, err := Translate(context.Background(), 1, &mtproto.TLMessagesTranslateRichMessage{
		ToLang: "nope",
		Text:   []*mtproto.InputRichMessage{markdown("hello")},
	}, nil); status.Convert(err).Message() != "TO_LANG_INVALID" {
		t.Fatal(err)
	}
	long := ""
	for i := 0; i < 65; i++ {
		long += "a"
	}
	if _, err := Translate(context.Background(), 1, &mtproto.TLMessagesTranslateRichMessage{
		ToLang: "zh",
		Tone:   &wrapperspb.StringValue{Value: long},
		Text:   []*mtproto.InputRichMessage{markdown("hello")},
	}, nil); status.Convert(err).Message() != "TO_LANG_INVALID" {
		t.Fatal(err)
	}
	if _, err := Translate(context.Background(), 1, &mtproto.TLMessagesTranslateRichMessage{
		ToLang: "zh-hans",
		Text:   []*mtproto.InputRichMessage{markdown("hello")},
	}, nil); status.Convert(err).Message() != "TRANSLATE_REQ_FAILED" {
		t.Fatalf("flag off: %v", err)
	}
	useModel(t)
	out, err := Translate(context.Background(), 1, &mtproto.TLMessagesTranslateRichMessage{
		ToLang: "zh-hans",
		Text:   []*mtproto.InputRichMessage{markdown("**hello**"), paragraphInput("hello")},
	}, nil)
	if err != nil || len(out.GetResult()) != 2 || visible(out.GetResult()[0]) != "你好" || visible(out.GetResult()[1]) != "你好" {
		t.Fatalf("translate %v %#v", err, out)
	}
	if out.GetResult()[0].GetBlocks()[0].GetText().GetPredicateName() != mtproto.Predicate_textBold {
		t.Fatalf("bold dropped: %s", out.GetResult()[0].GetBlocks()[0].GetText().GetPredicateName())
	}
	linked, err := Translate(context.Background(), 1, &mtproto.TLMessagesTranslateRichMessage{
		ToLang: "zh",
		Text:   []*mtproto.InputRichMessage{markdown("[hello](https://tell.cc/a)")},
	}, nil)
	if err != nil || visible(linked.GetResult()[0]) != "你好" || linked.GetResult()[0].GetBlocks()[0].GetText().GetUrl() != "https://tell.cc/a" {
		t.Fatalf("link %v %#v", err, linked)
	}
	if out.GetResult()[1].GetBlocks()[0].GetPredicateName() != mtproto.Predicate_pageBlockParagraph ||
		out.GetResult()[1].GetBlocks()[0].GetText().GetPredicateName() != mtproto.Predicate_textPlain {
		t.Fatal("paragraph block was not kept")
	}
	inner := mtproto.MakeTLTextPlain(&mtproto.RichText{Text_STRING: "hello"}).To_RichText()
	bold := mtproto.MakeTLTextBold(&mtproto.RichText{Text_RICHTEXT: inner}).To_RichText()
	pre := mtproto.MakeTLPageBlockPreformatted(&mtproto.PageBlock{
		Text:     mtproto.MakeTLTextPlain(&mtproto.RichText{Text_STRING: "hello"}).To_RichText(),
		Language: "go",
	}).To_PageBlock()
	kept, err := Translate(context.Background(), 1, &mtproto.TLMessagesTranslateRichMessage{
		ToLang: "zh",
		Text: []*mtproto.InputRichMessage{
			mtproto.MakeTLInputRichMessage(&mtproto.InputRichMessage{
				Blocks: []*mtproto.PageBlock{
					mtproto.MakeTLPageBlockParagraph(&mtproto.PageBlock{Text: bold}).To_PageBlock(),
					pre,
				},
			}).To_InputRichMessage(),
		},
	}, nil)
	if err != nil || len(kept.GetResult()) != 1 || len(kept.GetResult()[0].GetBlocks()) != 2 {
		t.Fatalf("blocks %v %#v", err, kept)
	}
	if kept.GetResult()[0].GetBlocks()[0].GetText().GetPredicateName() != mtproto.Predicate_textBold ||
		richPlain(kept.GetResult()[0].GetBlocks()[0].GetText(), 0) != "你好" {
		t.Fatal("bold leaf was not translated in place")
	}
	if kept.GetResult()[0].GetBlocks()[1].GetPredicateName() != mtproto.Predicate_pageBlockPreformatted ||
		kept.GetResult()[0].GetBlocks()[1].GetLanguage() != "go" ||
		richPlain(kept.GetResult()[0].GetBlocks()[1].GetText(), 0) != "hello" {
		t.Fatal("code block was rewritten")
	}
	mustEncode(t, out)
	if out.GetResult()[0].GetPhotos() == nil || out.GetResult()[0].GetDocuments() == nil {
		t.Fatal("vectors")
	}

	if _, err := Translate(context.Background(), 1, &mtproto.TLMessagesTranslateRichMessage{ToLang: "en"}, nil); status.Convert(err).Message() != "PEER_ID_INVALID" {
		t.Fatal(err)
	}
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 4}).To_InputPeer()
	if _, err := Translate(context.Background(), 1, &mtproto.TLMessagesTranslateRichMessage{
		ToLang: "en", Peer: peer,
	}, nil); status.Convert(err).Message() != "MESSAGE_IDS_EMPTY" {
		t.Fatal(err)
	}
	if _, err := Translate(context.Background(), 1, &mtproto.TLMessagesTranslateRichMessage{
		ToLang: "en", Peer: peer, Id: []int32{0},
	}, nil); status.Convert(err).Message() != "MESSAGE_ID_INVALID" {
		t.Fatal(err)
	}

	chatbox.ResetShared()
	p := mtproto.MakePeerUtil(mtproto.PEER_USER, 4)
	chatbox.Shared().RememberPlain(context.Background(), 3, p, 11, "hello", []*mtproto.MessageEntity{{
		PredicateName: mtproto.Predicate_messageEntityBold,
		Offset:        0,
		Length:        5,
	}})
	byID, err := Translate(context.Background(), 3, &mtproto.TLMessagesTranslateRichMessage{
		ToLang: "zh", Peer: peer, Id: []int32{11},
	}, nil)
	if err != nil || visible(byID.GetResult()[0]) != "你好" {
		t.Fatalf("id %v %#v", err, byID)
	}
	if byID.GetResult()[0].GetBlocks()[0].GetText().GetPredicateName() != mtproto.Predicate_textBold {
		t.Fatal("stored bold dropped")
	}
	loaded, err := Translate(context.Background(), 9, &mtproto.TLMessagesTranslateRichMessage{
		ToLang: "en", Peer: peer, Id: []int32{8},
	}, func(context.Context, int64, *mtproto.PeerUtil, int32) (string, []*mtproto.MessageEntity, bool) {
		return "thanks", nil, true
	})
	if err != nil || visible(loaded.GetResult()[0]) != "thank you" {
		t.Fatalf("load %v %#v", err, loaded)
	}
}

func TestComposeRichUsesModel(t *testing.T) {
	t.Setenv(tellomiai.EnvCompose, "")
	t.Setenv(tellomiai.EnvAPIKey, "")
	ctx := context.Background()
	if _, err := Compose(ctx, nil); status.Convert(err).Message() != "MESSAGE_EMPTY" {
		t.Fatal(err)
	}
	if _, err := Compose(ctx, &mtproto.TLMessagesComposeRichMessageWithAI{}); status.Convert(err).Message() != "MESSAGE_EMPTY" {
		t.Fatal(err)
	}
	text := markdown("hello there")
	if _, err := Compose(ctx, &mtproto.TLMessagesComposeRichMessageWithAI{Text: text}); status.Convert(err).Message() != "AI_COMPOSE_TASK_MISSING" {
		t.Fatal(err)
	}
	if _, err := Compose(ctx, &mtproto.TLMessagesComposeRichMessageWithAI{
		Text: text,
		Tone: mtproto.MakeTLInputAiComposeToneDefault(&mtproto.InputAiComposeTone{Tone: "not-a-tone"}).To_InputAiComposeTone(),
	}); status.Convert(err).Message() != "AICOMPOSE_TONE_INVALID" {
		t.Fatal(err)
	}
	if _, err := Compose(ctx, &mtproto.TLMessagesComposeRichMessageWithAI{
		Proofread: true,
		Text:      text,
	}); status.Convert(err).Message() != "AICOMPOSE_TIMEOUT" {
		t.Fatalf("flag off: %v", err)
	}
	useModel(t)
	kept, err := Compose(ctx, &mtproto.TLMessagesComposeRichMessageWithAI{
		Proofread: true,
		Text:      text,
		Tone:      mtproto.MakeTLInputAiComposeToneDefault(&mtproto.InputAiComposeTone{Tone: "friendly"}).To_InputAiComposeTone(),
	})
	if err != nil || visible(kept.GetResult()) != "proof:hello there" {
		t.Fatalf("proofread %v %#v", err, kept)
	}
	mustEncode(t, kept)
	emoji, err := Compose(ctx, &mtproto.TLMessagesComposeRichMessageWithAI{
		Emojify: true,
		Text:    markdown("hello"),
	})
	if err != nil || visible(emoji.GetResult()) != "hello 🙂" {
		t.Fatalf("emoji %v %#v", err, emoji)
	}
	zh, err := Compose(ctx, &mtproto.TLMessagesComposeRichMessageWithAI{
		Text:            markdown("hello"),
		TranslateToLang: &wrapperspb.StringValue{Value: "zh-hans"},
	})
	if err != nil || visible(zh.GetResult()) != "你好" {
		t.Fatalf("compose translate %v %#v", err, zh)
	}
	if _, err := Compose(ctx, &mtproto.TLMessagesComposeRichMessageWithAI{
		Proofread:       true,
		Text:            text,
		TranslateToLang: &wrapperspb.StringValue{Value: "nope"},
	}); status.Convert(err).Message() != "TO_LANG_INVALID" {
		t.Fatal(err)
	}
	tellomiai.SetHook(func(context.Context, tellomiai.Job) ([]string, error) {
		return nil, errors.New("down")
	})
	if _, err := Compose(ctx, &mtproto.TLMessagesComposeRichMessageWithAI{
		Proofread: true,
		Text:      markdown("hello"),
	}); status.Convert(err).Message() != "AICOMPOSE_TIMEOUT" {
		t.Fatalf("provider: %v", err)
	}
	tellomiai.SetHook(func(context.Context, tellomiai.Job) ([]string, error) {
		return []string{"你好"}, nil
	})
	tellomiai.SetModerate(func(context.Context, string) string { return contentsafety.Blocked })
	if _, err := Translate(ctx, 1, &mtproto.TLMessagesTranslateRichMessage{
		ToLang: "zh",
		Text:   []*mtproto.InputRichMessage{markdown("hello")},
	}, nil); status.Convert(err).Message() != "TRANSLATE_REQ_FAILED" {
		t.Fatalf("safety: %v", err)
	}
}
