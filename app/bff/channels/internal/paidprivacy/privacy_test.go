package paidprivacy

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func ctor(raw []byte) uint32 {
	return uint32(raw[0]) | uint32(raw[1])<<8 | uint32(raw[2])<<16 | uint32(raw[3])<<24
}

func TestDefaultPaidReactionPrivacy(t *testing.T) {
	got := DefaultUpdates()
	if got.GetPredicateName() != mtproto.Predicate_updates {
		t.Fatalf("predicate %s", got.GetPredicateName())
	}
	if len(got.GetUpdates()) != 1 {
		t.Fatalf("updates %d", len(got.GetUpdates()))
	}
	upd := got.GetUpdates()[0]
	if upd.GetPredicateName() != mtproto.Predicate_updatePaidReactionPrivacy {
		t.Fatalf("update %s", upd.GetPredicateName())
	}
	priv := upd.GetPrivate_PAIDREACTIONPRIVACY()
	if priv.GetPredicateName() != mtproto.Predicate_paidReactionPrivacyDefault {
		t.Fatalf("privacy %s", priv.GetPredicateName())
	}
	if priv.GetPeer() != nil {
		t.Fatal("default privacy has no peer")
	}

	buf := mtproto.NewEncodeBuf(64)
	if err := got.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	raw := buf.GetBuf()
	if ctor(raw) != 0x74ae4240 {
		t.Fatalf("updates constructor %x", ctor(raw))
	}
	decoded := &mtproto.Updates{}
	if err := decoded.Decode(mtproto.NewDecodeBuf(raw)); err != nil {
		t.Fatal(err)
	}
	if decoded.GetUpdates()[0].GetPrivate_PAIDREACTIONPRIVACY().GetPredicateName() != mtproto.Predicate_paidReactionPrivacyDefault {
		t.Fatal("decode dropped paidReactionPrivacyDefault")
	}
}
