package mtproto

import (
	"math"
	"testing"
	"time"
)

func TestActiveBannedRightsDropsExpiredMute(t *testing.T) {
	const now int32 = 1_700_000_000
	live := MakeTLChatBannedRights(&ChatBannedRights{
		SendMessages: true,
		UntilDate:    now + 60,
	}).To_ChatBannedRights()
	if got := ActiveBannedRights(live, now); got == nil || !got.GetSendMessages() || got.GetUntilDate() != now+60 {
		t.Fatalf("live mute = %#v", got)
	}

	expired := MakeTLChatBannedRights(&ChatBannedRights{
		SendMessages: true,
		UntilDate:    now - 1,
	}).To_ChatBannedRights()
	if got := ActiveBannedRights(expired, now); got != nil {
		t.Fatalf("expired mute still issued: %#v", got)
	}
	if !expired.Expired(now) {
		t.Fatal("until_date in the past was not expired")
	}

	forever := MakeTLChatBannedRights(&ChatBannedRights{
		SendMessages: true,
		UntilDate:    math.MaxInt32,
	}).To_ChatBannedRights()
	if ActiveBannedRights(forever, now) == nil || forever.Expired(now) {
		t.Fatal("0x7fffffff mute was treated as expired")
	}
	zero := MakeTLChatBannedRights(&ChatBannedRights{SendMessages: true}).To_ChatBannedRights()
	if zero.Expired(now) {
		t.Fatal("until_date 0 was treated as expired")
	}

	bits := int64(expired.ToBannedRights())
	if IssuedBannedRights(bits, now) != nil {
		t.Fatal("stored expired mute was issued")
	}
	if IssuedBannedRights(int64(live.ToBannedRights()), now) == nil {
		t.Fatal("stored live mute was dropped")
	}

	wall := int32(time.Now().Unix())
	basic := &MutableChat{Chat: &ImmutableChat{DefaultBannedRights: MakeTLChatBannedRights(&ChatBannedRights{
		SendMessages: true,
		UntilDate:    wall - 30,
	}).To_ChatBannedRights()}}
	if basic.DefaultBannedRights() != nil {
		t.Fatal("basic group still issued an expired default ban")
	}
	open := &MutableChat{Chat: &ImmutableChat{DefaultBannedRights: MakeTLChatBannedRights(&ChatBannedRights{
		SendMessages: true,
		UntilDate:    wall + 3600,
	}).To_ChatBannedRights()}}
	if got := open.DefaultBannedRights(); got == nil || !got.GetSendMessages() {
		t.Fatalf("live default ban = %#v", got)
	}
}
