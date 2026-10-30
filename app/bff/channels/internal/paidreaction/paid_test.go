package paidreaction

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
)

func TestPaidReaction(t *testing.T) {
	Reset()
	channel := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 15, AccessHash: 1}).To_InputPeer()
	user := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 4}).To_InputPeer()
	random := time.Now().Unix()<<32 | 7
	if _, err := Send(3, user, 8, 1, random, nil); status.Convert(err).Message() != "CHANNEL_INVALID" {
		t.Fatalf("user peer: %v", err)
	}
	if _, err := Send(3, channel, 0, 1, random, nil); status.Convert(err).Message() != "MESSAGE_ID_INVALID" {
		t.Fatalf("msg: %v", err)
	}
	if _, err := Send(3, channel, 8, 0, random, nil); status.Convert(err).Message() != "REACTIONS_COUNT_INVALID" {
		t.Fatalf("count: %v", err)
	}
	if _, err := Send(3, channel, 8, 1, 0, nil); status.Convert(err).Message() != "RANDOM_ID_EMPTY" {
		t.Fatalf("random: %v", err)
	}
	if _, err := Send(3, channel, 8, 1, 5, nil); status.Convert(err).Message() != "RANDOM_ID_EXPIRED" {
		t.Fatalf("expired: %v", err)
	}
	if err := Toggle(3, channel, 8, mtproto.MakeTLPaidReactionPrivacyAnonymous(nil).To_PaidReactionPrivacy()); status.Convert(err).Message() != "REACTION_EMPTY" {
		t.Fatalf("toggle empty: %v", err)
	}

	up, err := Send(3, channel, 8, 2, random, mtproto.MakeTLPaidReactionPrivacyAnonymous(nil).To_PaidReactionPrivacy())
	if err != nil {
		t.Fatal(err)
	}
	if err := up.Encode(mtproto.NewEncodeBuf(512), mtproto.Layer); err != nil {
		t.Fatal(err)
	}
	upd := up.Updates[0]
	if upd.GetPredicateName() != mtproto.Predicate_updateMessageReactions || upd.GetMsgId_INT32() != 8 {
		t.Fatalf("update %+v", upd)
	}
	results := upd.GetReactions_MESSAGEREACTIONS().GetResults()
	if len(results) != 1 || results[0].GetCount() != 2 || results[0].GetReaction_REACTION().GetPredicateName() != mtproto.Predicate_reactionPaid {
		t.Fatalf("results %+v", results)
	}
	reactors := upd.GetReactions_MESSAGEREACTIONS().GetTopReactors()
	if len(reactors) != 1 || !reactors[0].GetAnonymous() || reactors[0].GetPeerId() != nil {
		t.Fatalf("reactor %+v", reactors)
	}

	again, err := Send(3, channel, 8, 2, random, nil)
	if err != nil || again.Updates[0].GetReactions_MESSAGEREACTIONS().GetResults()[0].GetCount() != 2 {
		t.Fatal("duplicate random id should not add stars")
	}
	more := time.Now().Unix()<<32 | 8
	sum, err := Send(3, channel, 8, 3, more, mtproto.MakeTLPaidReactionPrivacyDefault(nil).To_PaidReactionPrivacy())
	if err != nil || sum.Updates[0].GetReactions_MESSAGEREACTIONS().GetResults()[0].GetCount() != 5 {
		t.Fatalf("sum %+v %v", sum, err)
	}
	if sum.Updates[0].GetReactions_MESSAGEREACTIONS().GetTopReactors()[0].GetAnonymous() {
		t.Fatal("default privacy is not anonymous")
	}
	if err := Toggle(3, channel, 8, mtproto.MakeTLPaidReactionPrivacyAnonymous(nil).To_PaidReactionPrivacy()); err != nil {
		t.Fatal(err)
	}
}
