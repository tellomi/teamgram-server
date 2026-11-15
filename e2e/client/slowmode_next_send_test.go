package e2e

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestSlowmodeNextSendDate is ISSUE-S-SLOWMODE-NEXTDATE-001.
// A member who just sent in a 30-second slow mode sees
// channelFull.slowmode_next_send_date within 2s of now+30. After
// SLOWMODE_WAIT_X, the same field is within 2s of now+X. An admin and the
// creator do not receive it. Turning slow mode off clears the field.
// Phones 9996610811–9996610813, code 11111, layer 232.
// 9996610801–9996610802 belong to the channel profile-photo test.
func TestSlowmodeNextSendDate(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(3 * time.Minute)
	defer cancel()
	owner, _ := dialPhone(t, ctx, cfg, "9996610811", "SlowOwn")
	admin, _ := dialPhone(t, ctx, cfg, "9996610812", "SlowAdm")
	member, _ := dialPhone(t, ctx, cfg, "9996610813", "SlowMem")
	adminID, adminHash, err := owner.ImportPhone(ctx, "9996610812", "SlowAdm")
	if err != nil {
		t.Fatal(err)
	}
	memberID, memberHash, err := owner.ImportPhone(ctx, "9996610813", "SlowMem")
	if err != nil {
		t.Fatal(err)
	}

	title := fmt.Sprintf("SlowRoom-%d", time.Now().Unix())
	groupID, groupHash, err := owner.CreateMegagroup(ctx, title)
	if err != nil {
		t.Fatal(err)
	}
	group := inputChannel(groupID, groupHash)
	if err = owner.InviteToChannel(ctx, group, inputUser(adminID, adminHash), inputUser(memberID, memberHash)); err != nil {
		t.Fatal(err)
	}
	adminGroup := waitChannel(t, ctx, admin, groupID)
	memberGroup := waitChannel(t, ctx, member, groupID)
	if _, err = owner.Invoke(ctx, &mtproto.TLChannelsEditAdmin{
		Constructor:     mtproto.CRC32_channels_editAdmin_9a98ad68,
		Channel:         group,
		UserId:          inputUser(adminID, adminHash),
		AdminRights:     mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{ChangeInfo: true}).To_ChatAdminRights(),
		Rank_FLAGSTRING: wrapperspb.String("mod"),
	}); err != nil {
		t.Fatal(err)
	}

	const seconds int32 = 30
	if _, err = owner.Invoke(ctx, &mtproto.TLChannelsToggleSlowMode{
		Constructor: mtproto.CRC32_channels_toggleSlowMode,
		Channel:     group,
		Seconds:     seconds,
	}); err != nil {
		t.Fatal(err)
	}
	if full := channelFull(t, ctx, member, memberGroup); full.GetSlowmodeSeconds().GetValue() != seconds || full.GetSlowmodeNextSendDate() != nil {
		t.Fatalf("before send seconds=%d next=%v", full.GetSlowmodeSeconds().GetValue(), full.GetSlowmodeNextSendDate())
	}

	peer := inputPeerChannel(groupID, memberGroup.GetAccessHash())
	if _, err = member.SendText(ctx, peer, fmt.Sprintf("slow-%d", time.Now().UnixNano()), nil); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	full := channelFull(t, ctx, member, memberGroup)
	got := full.GetSlowmodeNextSendDate()
	if got == nil {
		t.Fatal("member next send date missing")
	}
	next := int64(got.GetValue())
	// last-send + 30. Two seconds cover the client/server clock and the
	// unix-second boundary.
	if next < now+int64(seconds)-2 || next > now+int64(seconds)+2 {
		t.Fatalf("member next %d, want %d±2", next, now+int64(seconds))
	}
	if full.GetSlowmodeSeconds().GetValue() != seconds {
		t.Fatalf("slowmode_seconds %d", full.GetSlowmodeSeconds().GetValue())
	}

	_, err = member.SendText(ctx, peer, "slow-again", nil)
	rpc, ok := asRPCError(err)
	const waitPrefix = "SLOWMODE_WAIT_"
	if !ok || rpc.Code != 420 || !strings.HasPrefix(rpc.Message, waitPrefix) {
		t.Fatalf("second send: %v", err)
	}
	waitFor, convErr := strconv.Atoi(strings.TrimPrefix(rpc.Message, waitPrefix))
	if convErr != nil || waitFor <= 0 {
		t.Fatalf("SLOWMODE_WAIT seconds: %q", rpc.Message)
	}
	now = time.Now().Unix()
	again := channelFull(t, ctx, member, memberGroup).GetSlowmodeNextSendDate()
	if again == nil {
		t.Fatal("member next send date missing after SLOWMODE_WAIT")
	}
	want := now + int64(waitFor)
	if got := int64(again.GetValue()); got < want-2 || got > want+2 {
		t.Fatalf("next %d not within now+X±2 (now=%d X=%d)", got, now, waitFor)
	}

	adminPeer := inputPeerChannel(groupID, adminGroup.GetAccessHash())
	if _, err = admin.SendText(ctx, adminPeer, "admin-ok", nil); err != nil {
		t.Fatal(err)
	}
	ownerPeer := inputPeerChannel(groupID, groupHash)
	if _, err = owner.SendText(ctx, ownerPeer, "owner-ok", nil); err != nil {
		t.Fatal(err)
	}
	if channelFull(t, ctx, admin, adminGroup).GetSlowmodeNextSendDate() != nil {
		t.Fatal("admin next send date")
	}
	if channelFull(t, ctx, owner, group).GetSlowmodeNextSendDate() != nil {
		t.Fatal("creator next send date")
	}

	if _, err = owner.Invoke(ctx, &mtproto.TLChannelsToggleSlowMode{
		Constructor: mtproto.CRC32_channels_toggleSlowMode,
		Channel:     group,
		Seconds:     0,
	}); err != nil {
		t.Fatal(err)
	}
	off := channelFull(t, ctx, member, memberGroup)
	if off.GetSlowmodeSeconds() != nil || off.GetSlowmodeNextSendDate() != nil {
		t.Fatalf("disabled seconds=%v next=%v", off.GetSlowmodeSeconds(), off.GetSlowmodeNextSendDate())
	}
	if _, err = member.SendText(ctx, peer, "slow-off", nil); err != nil {
		t.Fatal(err)
	}
}
