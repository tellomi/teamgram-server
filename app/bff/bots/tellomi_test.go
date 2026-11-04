package bots

import (
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/tellomiai"
)

func TestBuildAIMessageLabelsTheReply(t *testing.T) {
	text := tellomiai.WithLabel("你好")
	got := buildAIMessage(9, text, true, 7)
	if got.UserId != tellomiai.AccountID || got.PeerId != 9 || got.PeerType != mtproto.PEER_USER {
		t.Fatalf("envelope %+v", got)
	}
	if len(got.Message) != 1 || got.Message[0].RandomId != 7 {
		t.Fatal(got.Message)
	}
	msg := got.Message[0].Message
	if msg.Message != text || msg.FromId.GetUserId() != tellomiai.AccountID || msg.PeerId.GetUserId() != 9 {
		t.Fatalf("message %+v", msg)
	}
	if len(msg.Entities) != 1 {
		t.Fatalf("entities %+v", msg.Entities)
	}
	ent := msg.Entities[0]
	prefix := strings.TrimSuffix(text, tellomiai.Label)
	if ent.Offset != int32(mtproto.UTF16Len(prefix)) || ent.Length != int32(mtproto.UTF16Len(tellomiai.Label)) {
		t.Fatalf("entity offset %d length %d", ent.Offset, ent.Length)
	}
	if !strings.HasPrefix(ent.PredicateName, "messageEntityBold") && ent.GetPredicateName() != "messageEntityBold" {
		t.Fatalf("entity %s", ent.PredicateName)
	}
	plain := buildAIMessage(9, tellomiai.DailyMessage, false, 1)
	if len(plain.Message[0].Message.Entities) != 0 {
		t.Fatal("quota text was marked as a model reply")
	}
}
