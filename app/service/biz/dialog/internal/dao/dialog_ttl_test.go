package dao

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"
)

func TestApplyStoredTTLReplacesAStaleCacheValue(t *testing.T) {
	ext := dialog.MakeTLDialogExt(&dialog.DialogExt{
		Dialog: mtproto.MakeTLDialog(&mtproto.Dialog{
			Peer: mtproto.MakePeerUser(20),
		}).To_Dialog(),
	}).To_DialogExt()
	applyStoredTTL(ext, 86400)
	if ext.GetTtlPeriod() != 86400 || ext.GetDialog().GetTtlPeriod().GetValue() != 86400 {
		t.Fatalf("stored timer ext %d flag %#v", ext.GetTtlPeriod(), ext.GetDialog().GetTtlPeriod())
	}
	applyStoredTTL(ext, 0)
	if ext.GetDialog().GetTtlPeriod() != nil {
		t.Fatalf("cleared timer still set: %#v", ext.GetDialog().GetTtlPeriod())
	}
}

func TestMakeDialogTTLPeriodOnlyWhenPositive(t *testing.T) {
	d := &Dao{}
	cases := []struct {
		period int32
		want   int32
		set    bool
	}{
		{period: 0, set: false},
		{period: -1, set: false},
		{period: 86400, want: 86400, set: true},
	}
	for _, tc := range cases {
		got := d.MakeDialog(&dataobject.DialogsDO{
			UserId:    10,
			PeerType:  mtproto.PEER_USER,
			PeerId:    20,
			TtlPeriod: tc.period,
		})
		flag := got.GetDialog().GetTtlPeriod()
		if tc.set {
			if flag == nil || flag.GetValue() != tc.want || got.GetTtlPeriod() != tc.want {
				t.Fatalf("period %d flag %#v ext %d", tc.period, flag, got.GetTtlPeriod())
			}
			continue
		}
		if flag != nil {
			t.Fatalf("period %d emitted %#v", tc.period, flag)
		}
	}
}
