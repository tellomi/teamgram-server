package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type fakeUsers struct {
	mu       sync.Mutex
	rows     []*mtproto.ImmutableUser
	forward  []int64
	reverse  map[int64]bool
	err      error
	fwdOwner int64
	revPeers []int64
}

func (f *fakeUsers) UserGetMutableUsersV2(_ context.Context, in *user.TLUserGetMutableUsersV2) (*mtproto.MutableUsers, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &mtproto.MutableUsers{Users: f.rows}, nil
}

func (f *fakeUsers) UserCheckBlockUserList(_ context.Context, in *user.TLUserCheckBlockUserList) (*user.Vector_Long, error) {
	f.mu.Lock()
	f.fwdOwner = in.GetUserId()
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return &user.Vector_Long{Datas: append([]int64(nil), f.forward...)}, nil
}

func (f *fakeUsers) UserIsBlockedByUser(_ context.Context, in *user.TLUserIsBlockedByUser) (*mtproto.Bool, error) {
	f.mu.Lock()
	f.revPeers = append(f.revPeers, in.GetUserId())
	peer := in.GetPeerUserId()
	f.mu.Unlock()
	if peer != 42 {
		return nil, errors.New("reverse check asked the wrong owner")
	}
	if f.reverse[in.GetUserId()] {
		return mtproto.BoolTrue, nil
	}
	return mtproto.BoolFalse, nil
}

func userRow(id int64, deleted bool) *mtproto.ImmutableUser {
	return &mtproto.ImmutableUser{User: &mtproto.UserData{Id: id, Deleted: deleted}}
}

func TestUserMembersLiveAndBothBlockDirections(t *testing.T) {
	f := &fakeUsers{
		rows:    []*mtproto.ImmutableUser{userRow(7, false), userRow(8, true), userRow(9, false), userRow(11, false)},
		forward: []int64{9},
		reverse: map[int64]bool{11: true},
	}
	views, err := UserMembers(f).Lookup(context.Background(), 42, []int64{7, 8, 9, 10, 11})
	if err != nil {
		t.Fatal(err)
	}
	if !views[7].Live || views[7].Blocked {
		t.Fatalf("live %+v", views[7])
	}
	if views[8].Live || views[10].Live {
		t.Fatalf("deleted or missing looked live: %+v %+v", views[8], views[10])
	}
	if !views[9].Live || !views[9].Blocked {
		t.Fatalf("forward block %+v", views[9])
	}
	if !views[11].Live || !views[11].Blocked {
		t.Fatalf("reverse block %+v", views[11])
	}
	if f.fwdOwner != 42 {
		t.Fatalf("forward owner %d", f.fwdOwner)
	}
	for _, id := range f.revPeers {
		if id == 9 {
			t.Fatal("forward-blocked id was checked in reverse")
		}
	}
}

func TestUserMembersNilClient(t *testing.T) {
	if UserMembers(nil) != nil {
		t.Fatal("nil client installed a directory")
	}
}
