package e2e

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestMentionNameWithoutUsername covers ISSUE-S-MENTIONNAME-001 (row 77).
// A name mention of a user with no username must be stored as
// messageEntityMentionName, not dropped.
func TestMentionNameWithoutUsername(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(90 * time.Second)
	defer cancel()
	a, _ := dialPhone(t, ctx, cfg, "9996610441", "MentA")
	_, _ = dialPhone(t, ctx, cfg, "9996610442", "MentB")
	bID, bHash, err := a.ImportPhone(ctx, "9996610442", "MentB")
	if err != nil {
		t.Fatal(err)
	}
	peer := inputPeerUser(bID, bHash)
	text := "hi Bob"
	id, err := a.SendText(ctx, peer, text, []*mtproto.MessageEntity{
		mtproto.MakeTLInputMessageEntityMentionName(&mtproto.MessageEntity{
			Offset: 3,
			Length: 3,
			UserId_INPUTUSER: mtproto.MakeTLInputUser(&mtproto.InputUser{
				UserId:     bID,
				AccessHash: bHash,
			}).To_InputUser(),
		}).To_MessageEntity(),
	})
	if err != nil {
		t.Fatal(err)
	}
	// The outbox row is written by the inbox consumer after sendMessage returns.
	waitCtx, cancelWait := contextTimeout(8 * time.Second)
	defer cancelWait()
	var saw bool
	err = waitUntil(waitCtx, 200*time.Millisecond, func() (bool, error) {
		msgs, err := a.History(waitCtx, peer, 100)
		if err != nil {
			return false, err
		}
		for _, m := range msgs {
			if m.GetMessage() != text && m.GetId() != id {
				continue
			}
			saw = true
			for _, entity := range m.GetEntities() {
				if entity.GetPredicateName() == mtproto.Predicate_messageEntityMentionName && entity.GetUserId_INT64() == bID {
					return true, nil
				}
			}
			return false, nil
		}
		return false, nil
	})
	if err != nil {
		if saw {
			t.Fatalf("message %d %q was stored without messageEntityMentionName: %v", id, text, err)
		}
		t.Fatalf("message %d %q missing from history: %v", id, text, err)
	}
}
