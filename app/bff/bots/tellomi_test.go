package bots

import (
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/tellomiai"
)

func TestBuildAIMessageHasNoFooter(t *testing.T) {
	got := buildAIMessage(9, "你好", true, 7)
	if got.UserId != tellomiai.AccountID || got.PeerId != 9 || got.PeerType != mtproto.PEER_USER {
		t.Fatalf("envelope %+v", got)
	}
	if len(got.Message) != 1 || got.Message[0].RandomId != 7 {
		t.Fatal(got.Message)
	}
	msg := got.Message[0].Message
	if msg.Message != "你好" || strings.Contains(msg.Message, tellomiai.Label) || msg.FromId.GetUserId() != tellomiai.AccountID || msg.PeerId.GetUserId() != 9 {
		t.Fatalf("message %+v", msg)
	}
	if len(msg.Entities) != 0 {
		t.Fatalf("assistant reply still has a label entity: %+v", msg.Entities)
	}
	plain := buildAIMessage(9, tellomiai.DailyMessage, false, 1)
	if len(plain.Message[0].Message.Entities) != 0 {
		t.Fatal("quota text was marked as a model reply")
	}
}

func TestBuildAIMessageStillBoldsReviewLabel(t *testing.T) {
	text := tellomiai.WithLabel("请确认")
	msg := buildAIMessage(9, text, true, 7).Message[0].Message
	if len(msg.Entities) != 1 {
		t.Fatalf("entities %+v", msg.Entities)
	}
	ent := msg.Entities[0]
	prefix := strings.TrimSuffix(text, tellomiai.Label)
	if ent.Offset != int32(mtproto.UTF16Len(prefix)) || ent.Length != int32(mtproto.UTF16Len(tellomiai.Label)) {
		t.Fatalf("entity offset %d length %d", ent.Offset, ent.Length)
	}
	if ent.GetPredicateName() != "messageEntityBold" && !strings.HasPrefix(ent.PredicateName, "messageEntityBold") {
		t.Fatalf("entity %s", ent.PredicateName)
	}
}

func TestBuildAIGroupMessageRepliesWithoutLabel(t *testing.T) {
	got := buildAIGroupMessage(mtproto.PEER_CHANNEL, 70, 15, "你好", true, 3)
	if got.UserId != tellomiai.AccountID || got.PeerType != mtproto.PEER_CHANNEL || got.PeerId != 70 {
		t.Fatalf("envelope %+v", got)
	}
	msg := got.Message[0].Message
	if msg.Message != "你好" || len(msg.Entities) != 0 || msg.GetReplyTo().GetReplyToMsgId() != 15 {
		t.Fatalf("message %+v reply %+v", msg, msg.GetReplyTo())
	}
	if msg.PeerId.GetChannelId() != 70 || msg.FromId.GetUserId() != tellomiai.AccountID {
		t.Fatalf("peers %+v from %+v", msg.PeerId, msg.FromId)
	}
	chat := buildAIGroupMessage(mtproto.PEER_CHAT, 4, 9, "好", true, 1)
	if chat.PeerType != mtproto.PEER_CHAT || chat.Message[0].Message.PeerId.GetChatId() != 4 {
		t.Fatalf("basic group %+v", chat)
	}
}
