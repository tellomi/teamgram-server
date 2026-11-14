package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestDeactivateAllUsernamesAndForgedEmoji covers
// ISSUE-S-USERNAME-DEACT-001 (row 99), ISSUE-S-CHAN-EMOJI-INVALID-001 (row 293),
// and ISSUE-S-CHAN-EMOJISET-001 (row 294).
// Phone 9996610456, code 11111.
func TestDeactivateAllUsernamesAndForgedEmoji(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	owner, _ := dialPhone(t, ctx, cfg, "9996610456", "Own456")
	groupID, groupHash, err := owner.CreateMegagroup(ctx, "NameRoom")
	if err != nil {
		t.Fatal(err)
	}
	channel := inputChannel(groupID, groupHash)
	name := fmt.Sprintf("edeact%d", time.Now().Unix()%100000000)
	if _, err = owner.Invoke(ctx, &mtproto.TLChannelsUpdateUsername{
		Constructor: mtproto.CRC32_channels_updateUsername,
		Channel:     channel,
		Username:    name,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = owner.Invoke(ctx, &mtproto.TLContactsResolveUsername{
		Constructor: mtproto.CRC32_contacts_resolveUsername_f93ccba3,
		Username:    name,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = owner.Invoke(ctx, &mtproto.TLChannelsDeactivateAllUsernames{
		Constructor: mtproto.CRC32_channels_deactivateAllUsernames,
		Channel:     channel,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = owner.Invoke(ctx, &mtproto.TLContactsResolveUsername{
		Constructor: mtproto.CRC32_contacts_resolveUsername_f93ccba3,
		Username:    name,
	}); err == nil || !strings.Contains(err.Error(), "USERNAME_NOT_OCCUPIED") {
		t.Fatalf("resolve after deactivate: %v", err)
	}
	if _, err = owner.Invoke(ctx, &mtproto.TLChannelsUpdateEmojiStatus{
		Constructor: mtproto.CRC32_channels_updateEmojiStatus,
		Channel:     channel,
		EmojiStatus: mtproto.MakeTLEmojiStatus(&mtproto.EmojiStatus{DocumentId: 7}).To_EmojiStatus(),
	}); err == nil || !strings.Contains(err.Error(), "DOCUMENT_INVALID") {
		t.Fatalf("emoji status: %v", err)
	}
	if _, err = owner.Invoke(ctx, &mtproto.TLChannelsSetEmojiStickers{
		Constructor: mtproto.CRC32_channels_setEmojiStickers,
		Channel:     channel,
		Stickerset:  mtproto.MakeTLInputStickerSetID(&mtproto.InputStickerSet{Id: 999999001}).To_InputStickerSet(),
	}); err == nil || !strings.Contains(err.Error(), "STICKERSET_INVALID") {
		t.Fatalf("emoji set: %v", err)
	}
}
