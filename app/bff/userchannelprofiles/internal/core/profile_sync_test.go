package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestProfileNameUpdates(t *testing.T) {
	up := profileNameUpdates(7, "Ada", "Lovelace", "ada")
	if len(up.GetUsers()) != 0 || len(up.GetChats()) != 0 {
		t.Fatalf("partial user must not be attached: users=%#v chats=%#v", up.GetUsers(), up.GetChats())
	}
	if len(up.GetUpdates()) != 1 || up.GetUpdates()[0].GetPredicateName() != mtproto.Predicate_updateUserName {
		t.Fatalf("updates %#v", up.GetUpdates())
	}
	upd := up.GetUpdates()[0]
	if upd.GetFirstName() != "Ada" || upd.GetLastName() != "Lovelace" || upd.GetUsername() != "ada" || upd.GetUserId() != 7 {
		t.Fatalf("updateUserName %#v", upd)
	}
	buf := mtproto.NewEncodeBuf(512)
	if err := up.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	decoded := &mtproto.Updates{}
	if err := decoded.Decode(mtproto.NewDecodeBuf(buf.GetBuf())); err != nil {
		t.Fatal(err)
	}
	if len(decoded.GetUsers()) != 0 || decoded.GetUpdates()[0].GetPredicateName() != mtproto.Predicate_updateUserName {
		t.Fatalf("wire users=%#v updates=%#v", decoded.GetUsers(), decoded.GetUpdates())
	}
}

func TestProfileNamePeers(t *testing.T) {
	got := profileNamePeers(7, []int64{7, 8, 0, 8}, []int64{9, 8, 7})
	if len(got) != 2 || got[0] != 8 || got[1] != 9 {
		t.Fatalf("peers %v", got)
	}
	if len(profileNamePeers(1, nil, nil)) != 0 {
		t.Fatal("empty")
	}
}
