package mtproto

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestFixDataKeepsHistoryTTLPeriod(t *testing.T) {
	const period int32 = 86400
	date := int32(time.Now().Unix()) - 3600
	msg := MakeTLMessage(&Message{
		Date:      date,
		Message:   "later",
		TtlPeriod: &wrapperspb.Int32Value{Value: period},
	}).To_Message()

	msg.FixData()
	if got := msg.GetTtlPeriod().GetValue(); got != period {
		t.Fatalf("ttl_period = %d, want the stored period %d", got, period)
	}
	msg.FixData()
	if got := msg.GetTtlPeriod().GetValue(); got != period {
		t.Fatalf("second FixData ttl_period = %d", got)
	}

	upd := MakeTLUpdateEditMessage(&Update{Message_MESSAGE: msg}).To_Update()
	upd.FixData()
	if got := upd.GetMessage_MESSAGE().GetTtlPeriod().GetValue(); got != period {
		t.Fatalf("update ttl_period = %d", got)
	}
}
