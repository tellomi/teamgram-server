package plugin

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestRemakeMessageKeepsCustomEmoji(t *testing.T) {
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Message: "hi",
		Entities: []*mtproto.MessageEntity{
			mtproto.MakeTLMessageEntityCustomEmoji(&mtproto.MessageEntity{
				Offset:     0,
				Length:     2,
				DocumentId: 42,
			}).To_MessageEntity(),
		},
	}).To_Message()

	out := RemakeMessage(context.Background(), nil, msg, 1, true, nil)
	var found bool
	for _, entity := range out.GetEntities() {
		if entity.GetPredicateName() == mtproto.Predicate_messageEntityCustomEmoji && entity.GetDocumentId() == 42 {
			found = true
		}
	}
	if !found {
		t.Fatalf("custom emoji entity was dropped: %#v", out.GetEntities())
	}
}
