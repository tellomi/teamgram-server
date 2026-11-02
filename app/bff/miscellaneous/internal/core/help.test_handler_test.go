package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestHelpTestReturnsTrue(t *testing.T) {
	r, err := (&MiscellaneousCore{}).HelpTest(&mtproto.TLHelpTest{})
	if err != nil || !mtproto.FromBool(r) {
		t.Fatalf("help.test = %v, %v", r, err)
	}
}
