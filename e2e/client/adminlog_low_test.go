package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestMigrateAgainAndFootballDice covers spreadsheet rows 190 and 204.
// Phones 9996610723 and 9996610724.
func TestMigrateAgainAndFootballDice(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	a, _ := dialPhone(t, ctx, cfg, "9996610723", "lowA")
	_, bSelf := dialPhone(t, ctx, cfg, "9996610724", "lowB")

	chatID, err := a.CreateChat(ctx, "again room", inputUser(bSelf.ID, bSelf.AccessHash))
	if err != nil {
		t.Fatal(err)
	}
	migrate := &mtproto.TLMessagesMigrateChat{
		Constructor: mtproto.CRC32_messages_migrateChat,
		ChatId:      chatID,
	}
	if _, err := a.Invoke(ctx, migrate); err != nil {
		t.Fatal(err)
	}
	_, err = a.Invoke(ctx, migrate)
	rpc, ok := asRPCError(err)
	if !ok || rpc.Message != "CHAT_ID_INVALID" {
		t.Fatalf("second migrate: %v", err)
	}

	sent, err := a.Invoke(ctx, &mtproto.TLMessagesSendMedia{
		Constructor: mtproto.CRC32_messages_sendMedia_7852834e,
		Peer:        inputPeerUser(bSelf.ID, bSelf.AccessHash),
		Media: mtproto.MakeTLInputMediaDice(&mtproto.InputMedia{
			Emoticon: "⚽",
		}).To_InputMedia(),
		RandomId: randomID(),
	})
	if err != nil {
		t.Fatal(err)
	}
	var value int32
	for _, m := range allMessages(sent) {
		if m.GetMedia().GetEmoticon() == "⚽" {
			value = m.GetMedia().GetValue()
		}
	}
	if value < 1 || value > 5 {
		t.Fatalf("football value %d", value)
	}
}
