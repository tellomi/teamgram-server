package e2e

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestBasicGroupCreatorRequiresPassword is messages.editChatCreator.
// A basic group used to accept any non-empty SRP A. It now uses the same
// gate as channels.editCreator.
func TestBasicGroupCreatorRequiresPassword(t *testing.T) {
	cfg := requireE2E(t)
	a, aSelf := loginNamed(t, cfg, ProfileIOS, "basic_creator_a", "R187A")
	loginNamed(t, cfg, ProfileAndroid, "basic_creator_b", "R187B")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	phone, err := cfg.Phone("basic_creator_b")
	if err != nil {
		t.Fatal(err)
	}
	bID, bHash, err := a.ImportPhone(ctx, phone.Number, "R187B")
	if err != nil {
		t.Fatal(err)
	}
	chatID, err := a.CreateChat(ctx, "R187Box", inputUser(bID, bHash))
	if err != nil {
		t.Fatal(err)
	}
	if got := basicGroupCreator(t, ctx, a, chatID); got != aSelf.ID {
		t.Fatalf("creator %d", got)
	}

	emptyErr := editBasicGroupCreator(ctx, a, chatID, bID, bHash, mtproto.MakeTLInputCheckPasswordEmpty(&mtproto.InputCheckPasswordSRP{}).To_InputCheckPasswordSRP())
	if rpc, ok := asRPCError(emptyErr); !ok || rpc.Message != "PASSWORD_MISSING" {
		t.Fatalf("empty proof: %v", emptyErr)
	}
	fake := mtproto.MakeTLInputCheckPasswordSRP(&mtproto.InputCheckPasswordSRP{
		SrpId: 1,
		A:     bytes.Repeat([]byte{0x11}, 256),
		M1:    bytes.Repeat([]byte{0x22}, 32),
	}).To_InputCheckPasswordSRP()
	fakeErr := editBasicGroupCreator(ctx, a, chatID, bID, bHash, fake)
	if rpc, ok := asRPCError(fakeErr); !ok || rpc.Message != "PASSWORD_MISSING" {
		t.Fatalf("fake proof without a password: %v", fakeErr)
	}
	if got := basicGroupCreator(t, ctx, a, chatID); got != aSelf.ID {
		t.Fatalf("creator changed to %d", got)
	}

	if err = setCloudPassword(ctx, a, "r187-secret"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = disableCloudPassword(cctx, a, "r187-secret")
	})
	freshErr := editBasicGroupCreator(ctx, a, chatID, bID, bHash, fake)
	rpc, ok := asRPCError(freshErr)
	if !ok || !passwordGateRejected(rpc.Message) {
		t.Fatalf("fake proof with a password: %v", freshErr)
	}
	if got := basicGroupCreator(t, ctx, a, chatID); got != aSelf.ID {
		t.Fatalf("fresh password transferred creator to %d", got)
	}
}

func passwordGateRejected(message string) bool {
	return message == "PASSWORD_HASH_INVALID" ||
		strings.HasPrefix(message, "PASSWORD_TOO_FRESH") ||
		strings.HasPrefix(message, "SESSION_TOO_FRESH")
}

func editBasicGroupCreator(ctx context.Context, c *Client, chatID, userID, userHash int64, pass *mtproto.InputCheckPasswordSRP) error {
	_, err := c.Invoke(ctx, &mtproto.TLMessagesEditChatCreator{
		Constructor: mtproto.CRC32_messages_editChatCreator,
		Peer:        inputPeerChat(chatID),
		UserId:      inputUser(userID, userHash),
		Password:    pass,
	})
	return err
}

func basicGroupCreator(t *testing.T, ctx context.Context, c *Client, chatID int64) int64 {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLMessagesGetFullChat{
		Constructor: mtproto.CRC32_messages_getFullChat,
		ChatId:      chatID,
	})
	if err != nil {
		t.Fatal(err)
	}
	full := asChatFull(t, obj)
	if full.GetParticipants() == nil {
		t.Fatal("no participants")
	}
	for _, p := range full.GetParticipants().GetParticipants() {
		if p != nil && p.GetPredicateName() == mtproto.Predicate_chatParticipantCreator {
			return p.GetUserId()
		}
	}
	t.Fatal("no creator")
	return 0
}
