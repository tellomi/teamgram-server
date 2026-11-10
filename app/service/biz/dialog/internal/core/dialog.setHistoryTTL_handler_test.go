package core

import (
	"errors"
	"testing"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

type ttlUpdate struct {
	tx       *sqlx.Tx
	userID   int64
	peerType int32
	peerID   int64
	ttl      int32
}

type recordingTTLStore struct {
	updates []ttlUpdate
	failAt  int
}

func (s *recordingTTLStore) UpdateCustomMapTx(tx *sqlx.Tx, cMap map[string]interface{}, userId int64, peerType int32, peerId int64) (int64, error) {
	ttl, _ := cMap["ttl_period"].(int32)
	s.updates = append(s.updates, ttlUpdate{tx: tx, userID: userId, peerType: peerType, peerID: peerId, ttl: ttl})
	if s.failAt > 0 && len(s.updates) == s.failAt {
		return 0, errors.New("ttl write failed")
	}
	return 1, nil
}

func TestApplyHistoryTTLWritesBothSidesInOneTransaction(t *testing.T) {
	store := &recordingTTLStore{}
	in := &dialog.TLDialogSetHistoryTTL{UserId: 10, PeerType: mtproto.PEER_USER, PeerId: 20, TtlPeriod: 86400}
	var tx sqlx.Tx
	if err := applyHistoryTTL(&tx, store, in); err != nil {
		t.Fatal(err)
	}
	if len(store.updates) != 2 {
		t.Fatalf("updates %d", len(store.updates))
	}
	if store.updates[0].tx != &tx || store.updates[1].tx != &tx {
		t.Fatal("the two sides were not written on the same transaction")
	}
	if store.updates[0].userID != 10 || store.updates[0].peerID != 20 || store.updates[0].ttl != 86400 {
		t.Fatalf("owner side %+v", store.updates[0])
	}
	if store.updates[1].userID != 20 || store.updates[1].peerType != mtproto.PEER_USER || store.updates[1].peerID != 10 || store.updates[1].ttl != 86400 {
		t.Fatalf("peer side %+v", store.updates[1])
	}
}

func TestApplyHistoryTTLReturnsThePeerSideError(t *testing.T) {
	store := &recordingTTLStore{failAt: 2}
	in := &dialog.TLDialogSetHistoryTTL{UserId: 10, PeerType: mtproto.PEER_USER, PeerId: 20, TtlPeriod: 60}
	err := applyHistoryTTL(nil, store, in)
	if err == nil {
		t.Fatal("peer write succeeded")
	}
	if len(store.updates) != 2 {
		t.Fatalf("updates %d, want both attempts so the caller can roll the transaction back", len(store.updates))
	}
}

func TestApplyHistoryTTLSkipsTheReverseRow(t *testing.T) {
	cases := []struct {
		name string
		in   *dialog.TLDialogSetHistoryTTL
	}{
		{"chat", &dialog.TLDialogSetHistoryTTL{UserId: 10, PeerType: mtproto.PEER_CHAT, PeerId: 30, TtlPeriod: 60}},
		{"channel", &dialog.TLDialogSetHistoryTTL{UserId: 10, PeerType: mtproto.PEER_CHANNEL, PeerId: 40, TtlPeriod: 60}},
		{"saved", &dialog.TLDialogSetHistoryTTL{UserId: 10, PeerType: mtproto.PEER_SELF, PeerId: 10, TtlPeriod: 60}},
		{"self user", &dialog.TLDialogSetHistoryTTL{UserId: 10, PeerType: mtproto.PEER_USER, PeerId: 10, TtlPeriod: 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &recordingTTLStore{}
			if err := applyHistoryTTL(nil, store, tc.in); err != nil {
				t.Fatal(err)
			}
			if len(store.updates) != 1 || store.updates[0].userID != tc.in.UserId || store.updates[0].peerID != tc.in.PeerId {
				t.Fatalf("updates %+v", store.updates)
			}
		})
	}
}

func TestHistoryTTLCacheKeysCoverBothPrivateSides(t *testing.T) {
	keys := historyTTLCacheKeys(10, mtproto.PEER_USER, 20)
	want := []string{
		dialog.GetDialogCacheKeyByPeer(10, mtproto.PEER_USER, 20),
		dialog.GetDialogCacheKeyByPeer(20, mtproto.PEER_USER, 10),
	}
	if len(keys) != len(want) || keys[0] != want[0] || keys[1] != want[1] {
		t.Fatalf("keys %v", keys)
	}
	one := historyTTLCacheKeys(10, mtproto.PEER_CHAT, 30)
	if len(one) != 1 || one[0] != dialog.GetDialogCacheKeyByPeer(10, mtproto.PEER_CHAT, 30) {
		t.Fatalf("chat keys %v", one)
	}
}
