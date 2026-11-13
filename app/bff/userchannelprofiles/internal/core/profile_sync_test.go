package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestProfileNameUpdates(t *testing.T) {
	up := profileNameUpdates(7, "Ada", "Lovelace", "ada")
	if len(up.GetUsers()) != 1 || up.GetUsers()[0].GetId() != 7 {
		t.Fatalf("users %#v", up.GetUsers())
	}
	if up.GetUsers()[0].GetFirstName().GetValue() != "Ada" || up.GetUsers()[0].GetSelf() {
		t.Fatalf("public user %#v", up.GetUsers()[0])
	}
	var name, user bool
	for _, upd := range up.GetUpdates() {
		switch upd.GetPredicateName() {
		case mtproto.Predicate_updateUserName:
			name = true
			if upd.GetFirstName() != "Ada" || upd.GetLastName() != "Lovelace" || upd.GetUsername() != "ada" || upd.GetUserId() != 7 {
				t.Fatalf("updateUserName %#v", upd)
			}
		case mtproto.Predicate_updateUser:
			user = upd.GetUserId() == 7
		}
	}
	if !name || !user {
		t.Fatalf("updates %#v", up.GetUpdates())
	}
	if err := up.Encode(mtproto.NewEncodeBuf(512), 232); err != nil {
		t.Fatal(err)
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
