package bots

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type fakeNames struct {
	in  *user.TLUserGetListByUsernameList
	out *user.Vector_UsernameData
	err error
}

func (f *fakeNames) UserGetListByUsernameList(_ context.Context, in *user.TLUserGetListByUsernameList) (*user.Vector_UsernameData, error) {
	f.in = in
	return f.out, f.err
}

func TestUserDirectoryKeepsUsersOnly(t *testing.T) {
	fake := &fakeNames{out: &user.Vector_UsernameData{Datas: []*user.UsernameData{
		{Username: "Erel", Peer: mtproto.MakePeerUser(9)},
		{Username: "news", Peer: mtproto.MakePeerChannel(4)},
		{Username: "ghost"},
	}}}
	got, err := (userDirectory{c: fake}).Users(context.Background(), []string{"erel", "news", "ghost"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["erel"] != 9 {
		t.Fatalf("%v", got)
	}
	if len(fake.in.GetNames()) != 3 || fake.in.GetNames()[0] != "erel" {
		t.Fatalf("asked %v", fake.in.GetNames())
	}

	fake.err = errors.New("down")
	if _, err = (userDirectory{c: fake}).Users(context.Background(), []string{"erel"}); err == nil {
		t.Fatal("lookup error was dropped")
	}
	if _, err = (userDirectory{}).Users(context.Background(), []string{"erel"}); err == nil {
		t.Fatal("nil client was treated as nobody")
	}
}
