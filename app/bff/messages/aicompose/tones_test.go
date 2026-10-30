package aicompose

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
)

func TestTonesCreateListAndExample(t *testing.T) {
	if _, err := Create(0, &mtproto.TLAicomposeCreateTone{Title: "A", Prompt: "B"}); status.Convert(err).Message() != "AUTH_KEY_UNREGISTERED" {
		t.Fatal(err)
	}
	if _, err := Create(9, &mtproto.TLAicomposeCreateTone{Title: "  ", Prompt: "hello"}); status.Convert(err).Message() != "AICOMPOSE_TONE_TITLE_INVALID" {
		t.Fatal(err)
	}
	if _, err := Create(9, &mtproto.TLAicomposeCreateTone{Title: "Warm", Prompt: " "}); status.Convert(err).Message() != "AICOMPOSE_TONE_INVALID" {
		t.Fatal(err)
	}
	tone, err := Create(9, &mtproto.TLAicomposeCreateTone{Title: "Warm", Prompt: "Be warm.", EmojiId: 5, DisplayAuthor: true})
	if err != nil || tone.GetTitle() != "Warm" || !tone.GetCreator() || tone.GetSlug() == "" || tone.GetAuthorId().GetValue() != 9 {
		t.Fatalf("create %#v %v", tone, err)
	}
	list, err := List(9, 0)
	if err != nil || list.GetPredicateName() != mtproto.Predicate_aicompose_tones || len(list.GetTones()) < 5 {
		t.Fatalf("list %#v %v", list, err)
	}
	again, err := List(9, list.GetHash())
	if err != nil || again.GetPredicateName() != mtproto.Predicate_aicompose_tonesNotModified {
		t.Fatalf("not modified %#v %v", again, err)
	}
	ex, err := Example(9, &mtproto.TLAicomposeGetToneExample{
		Tone: &mtproto.InputAiComposeTone{PredicateName: mtproto.Predicate_inputAiComposeToneDefault, Tone: "friendly"},
		Num:  0,
	})
	if err != nil || ex.GetFrom().GetText() == "" || ex.GetTo().GetText() == "" {
		t.Fatalf("example %#v %v", ex, err)
	}
	got, err := Get(9, &mtproto.TLAicomposeGetTone{Tone: &mtproto.InputAiComposeTone{
		PredicateName: mtproto.Predicate_inputAiComposeToneID,
		Id:            tone.GetId(),
		AccessHash:    tone.GetAccessHash(),
	}})
	if err != nil || len(got.GetTones()) != 1 || got.Tones[0].GetPrompt().GetValue() != "Be warm." {
		t.Fatalf("get %#v %v", got, err)
	}
	if _, err := Delete(8, &mtproto.TLAicomposeDeleteTone{Tone: &mtproto.InputAiComposeTone{
		PredicateName: mtproto.Predicate_inputAiComposeToneID,
		Id:            tone.GetId(),
		AccessHash:    tone.GetAccessHash(),
	}}); status.Convert(err).Message() != "AICOMPOSE_TONE_INVALID" {
		t.Fatal(err)
	}
	if _, err := Delete(9, &mtproto.TLAicomposeDeleteTone{Tone: &mtproto.InputAiComposeTone{
		PredicateName: mtproto.Predicate_inputAiComposeToneID,
		Id:            tone.GetId(),
		AccessHash:    tone.GetAccessHash(),
	}}); err != nil {
		t.Fatal(err)
	}
}
