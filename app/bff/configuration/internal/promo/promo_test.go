package promo

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

func ctor(raw []byte) uint32 {
	return uint32(raw[0]) | uint32(raw[1])<<8 | uint32(raw[2])<<16 | uint32(raw[3])<<24
}

func TestEmptyPromoEncodes(t *testing.T) {
	got := Empty()
	if got.GetPredicateName() != mtproto.Predicate_help_promoDataEmpty {
		t.Fatalf("predicate %s", got.GetPredicateName())
	}
	now := time.Now().Unix()
	if got.GetExpires() < int32(now)+refillAfter-2 || got.GetExpires() > int32(now)+refillAfter+2 {
		t.Fatalf("expires %d now %d", got.GetExpires(), now)
	}
	buf := mtproto.NewEncodeBuf(32)
	if err := got.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	raw := buf.GetBuf()
	if ctor(raw) != 0x98f6ac75 {
		t.Fatalf("constructor %x", ctor(raw))
	}
	decoded := &mtproto.Help_PromoData{}
	if err := decoded.Decode(mtproto.NewDecodeBuf(raw)); err != nil {
		t.Fatal(err)
	}
	if decoded.GetPredicateName() != mtproto.Predicate_help_promoDataEmpty {
		t.Fatalf("decoded %s", decoded.GetPredicateName())
	}
}

func TestHideRemovesAnnouncement(t *testing.T) {
	st := NewMemory()
	peer := mtproto.MakeTLPeerChannel(&mtproto.Peer{ChannelId: 42}).To_Peer()
	st.SetAnnouncement(&Announcement{
		Peer:       peer,
		PSAType:    "generic",
		PSAMessage: "read this",
	})
	ctx := context.Background()
	got := st.Get(ctx, 7)
	if got.GetPredicateName() != mtproto.Predicate_help_promoData {
		t.Fatalf("before hide %s", got.GetPredicateName())
	}
	if got.GetPeer().GetChannelId() != 42 || got.GetPsaType().GetValue() != "generic" {
		t.Fatalf("payload %+v", got)
	}
	if got.GetProxy() {
		t.Fatal("psa must not set the proxy flag")
	}
	buf := mtproto.NewEncodeBuf(256)
	if err := got.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	if ctor(buf.GetBuf()) != 0x8a4d87a {
		t.Fatalf("constructor %x", ctor(buf.GetBuf()))
	}

	in := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 42}).To_InputPeer()
	if err := st.Hide(ctx, 7, in); err != nil {
		t.Fatal(err)
	}
	after := st.Get(ctx, 7)
	if after.GetPredicateName() != mtproto.Predicate_help_promoDataEmpty {
		t.Fatalf("after hide %s", after.GetPredicateName())
	}
	other := st.Get(ctx, 8)
	if other.GetPredicateName() != mtproto.Predicate_help_promoData {
		t.Fatal("hide must be per user")
	}
}

func TestHideRejectsEmptyPeer(t *testing.T) {
	st := NewMemory()
	err := st.Hide(context.Background(), 7, mtproto.MakeTLInputPeerEmpty(&mtproto.InputPeer{}).To_InputPeer())
	if err != mtproto.ErrPeerIdInvalid {
		t.Fatalf("empty peer: %v", err)
	}
	if err := st.Hide(context.Background(), 7, nil); err != mtproto.ErrPeerIdInvalid {
		t.Fatalf("nil peer: %v", err)
	}
}

func TestNoAnnouncementIsEmpty(t *testing.T) {
	got := NewMemory().Get(context.Background(), 1)
	if got.GetPredicateName() != mtproto.Predicate_help_promoDataEmpty {
		t.Fatalf("predicate %s", got.GetPredicateName())
	}
}
