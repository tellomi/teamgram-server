package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestNormalizeCreateTTLOff(t *testing.T) {
	for _, ttl := range []*wrapperspb.Int32Value{nil, wrapperspb.Int32(0)} {
		period, err := normalizeCreateTTL(ttl)
		if err != nil || period != 0 {
			t.Fatalf("ttl %v -> %d %v", ttl, period, err)
		}
		if action := createTTLAction(period, 0, 7); action != nil {
			t.Fatalf("off produced %s period=%d auto=%v", action.GetPredicateName(), action.GetPeriod(), action.GetAutoSettingFrom())
		}
	}
}

func TestNormalizeCreateTTLRejectsOutOfRange(t *testing.T) {
	for _, v := range []int32{-1, 366*86400 + 1} {
		if _, err := normalizeCreateTTL(wrapperspb.Int32(v)); err != mtproto.ErrTtlPeriodInvalid {
			t.Fatalf("ttl %d err = %v", v, err)
		}
	}
}

func TestCreateTTLActionGlobalVersusExplicit(t *testing.T) {
	global := createTTLAction(86400, 86400, 42)
	if global.GetPredicateName() != mtproto.Predicate_messageActionSetMessagesTTL || global.GetPeriod() != 86400 || global.GetAutoSettingFrom().GetValue() != 42 {
		t.Fatalf("global action = %#v", global)
	}
	explicit := createTTLAction(3600, 86400, 42)
	if explicit.GetPeriod() != 3600 || explicit.GetAutoSettingFrom() != nil {
		t.Fatalf("explicit action = %#v", explicit)
	}
	unknownDefault := createTTLAction(86400, 0, 42)
	if unknownDefault.GetAutoSettingFrom() != nil {
		t.Fatalf("missing account default must not claim the global flag: %#v", unknownDefault)
	}
}
