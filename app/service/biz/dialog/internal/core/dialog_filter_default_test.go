package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

func TestDialogFilterDefaultJSONRoundTrip(t *testing.T) {
	first, err := dialogFilterDefaultJSON()
	if err != nil {
		t.Fatal(err)
	}
	second, err := dialogFilterDefaultJSON()
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first == "" {
		t.Fatalf("json %q %q", first, second)
	}
	got, err := mtproto.UnmarshalDialogFilter(first)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetPredicateName() != mtproto.Predicate_dialogFilterDefault {
		t.Fatalf("predicate %s json %s", got.GetPredicateName(), first)
	}
	if got.GetId() != dialog.DialogFilterDefaultID {
		t.Fatalf("id %d", got.GetId())
	}
}
