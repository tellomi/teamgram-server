package dao

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"
)

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
			if flag == nil || flag.GetValue() != tc.want {
				t.Fatalf("period %d flag %#v", tc.period, flag)
			}
			continue
		}
		if flag != nil {
			t.Fatalf("period %d emitted %#v", tc.period, flag)
		}
	}
}
