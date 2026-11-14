package paidreaction

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
)

func TestPaidReactionIsClosed(t *testing.T) {
	Reset()
	channel := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 15, AccessHash: 1}).To_InputPeer()
	user := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 4}).To_InputPeer()
	random := time.Now().Unix()<<32 | 7
	if _, err := Send(3, user, 8, 1, random, nil); status.Convert(err).Message() != "PAYMENT_UNSUPPORTED" {
		t.Fatalf("user peer: %v", err)
	}
	if _, err := Send(3, channel, 8, 1, 5, nil); status.Convert(err).Message() != "PAYMENT_UNSUPPORTED" {
		t.Fatalf("expired id: %v", err)
	}
	if _, err := Send(3, channel, 8, 2, random, mtproto.MakeTLPaidReactionPrivacyAnonymous(nil).To_PaidReactionPrivacy()); status.Convert(err).Message() != "PAYMENT_UNSUPPORTED" {
		t.Fatalf("channel: %v", err)
	}
	if err := Toggle(3, channel, 8, mtproto.MakeTLPaidReactionPrivacyAnonymous(nil).To_PaidReactionPrivacy()); status.Convert(err).Message() != "REACTION_EMPTY" {
		t.Fatalf("toggle: %v", err)
	}
}
