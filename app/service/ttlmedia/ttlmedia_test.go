package ttlmedia

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestClassify(t *testing.T) {
	now := int32(1_700_000_000)
	view := mtproto.ViewOnceTTL
	cases := []struct {
		name       string
		ttl        int32
		column     bool
		jsonUnread bool
		voice      bool
		open       *OpenRow
		want       Class
	}{
		{"unopened voice", view, true, true, true, nil, ClassSkipUnopened},
		{"opened view-once voice", view, false, true, true, nil, ClassExpire},
		{"opened timed voice already blanked by the client", 10, false, true, true, nil, ClassExpire},
		{"historical photo has no open bit", view, false, false, false, nil, ClassSkipNoSignal},
		{"unopened photo after the flag is stored", view, true, true, false, nil, ClassSkipUnopened},
		{"opened view-once photo", view, false, true, false, nil, ClassExpire},
		{"timed photo without an open time", 10, false, true, false, nil, ClassSkipNotDue},
		{"recorded and still inside the window", 10, false, true, false, &OpenRow{OpenedAt: now - 3}, ClassSkipNotDue},
		{"recorded and due", 10, false, true, false, &OpenRow{OpenedAt: now - 10}, ClassExpire},
		{"already stripped", view, false, false, true, &OpenRow{OpenedAt: now - 5, Stripped: true}, ClassSkipNotDue},
		{"not ttl", 0, false, true, true, nil, ClassSkipNoSignal},
	}
	for _, tc := range cases {
		got := Classify(tc.ttl, tc.column, tc.jsonUnread, tc.voice, tc.open, now)
		if got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}
