package service

import (
	"context"
	"sync"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// MemberView is one requested user. Live is false when the id does not exist
// or the account is deleted. Blocked is true when either user has blocked the
// other.
type MemberView struct {
	Live    bool
	Blocked bool
}

// Members resolves audience user ids. A nil Members fails issuing closed.
type Members interface {
	Lookup(ctx context.Context, owner int64, ids []int64) (map[int64]MemberView, error)
}

// AudienceUsers is the user RPC the audience check needs.
type AudienceUsers interface {
	UserGetMutableUsersV2(ctx context.Context, in *user.TLUserGetMutableUsersV2) (*mtproto.MutableUsers, error)
	UserCheckBlockUserList(ctx context.Context, in *user.TLUserCheckBlockUserList) (*user.Vector_Long, error)
	UserIsBlockedByUser(ctx context.Context, in *user.TLUserIsBlockedByUser) (*mtproto.Bool, error)
}

// UserMembers adapts the user RPC. Nil stays nil so issuing fails closed.
func UserMembers(users AudienceUsers) Members {
	if users == nil {
		return nil
	}
	return userMembers{users: users}
}

type userMembers struct {
	users AudienceUsers
}

const blockLookups = 16

func (u userMembers) Lookup(ctx context.Context, owner int64, ids []int64) (map[int64]MemberView, error) {
	if len(ids) == 0 {
		return map[int64]MemberView{}, nil
	}
	rows, err := u.users.UserGetMutableUsersV2(ctx, &user.TLUserGetMutableUsersV2{Id: append([]int64(nil), ids...)})
	if err != nil {
		return nil, err
	}
	if rows == nil {
		return nil, errLookup
	}
	live := make(map[int64]struct{}, len(ids))
	for _, row := range rows.GetUsers() {
		if row == nil || row.User == nil || row.Id() <= 0 || row.Deleted() {
			continue
		}
		live[row.Id()] = struct{}{}
	}
	blocked, err := u.blocked(ctx, owner, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]MemberView, len(ids))
	for _, id := range ids {
		_, ok := live[id]
		_, ban := blocked[id]
		out[id] = MemberView{Live: ok, Blocked: ban}
	}
	return out, nil
}

func (u userMembers) blocked(ctx context.Context, owner int64, ids []int64) (map[int64]struct{}, error) {
	fwd, err := u.users.UserCheckBlockUserList(ctx, &user.TLUserCheckBlockUserList{
		UserId: owner,
		Id:     append([]int64(nil), ids...),
	})
	if err != nil {
		return nil, err
	}
	if fwd == nil {
		return nil, errLookup
	}
	out := make(map[int64]struct{})
	for _, id := range fwd.GetDatas() {
		if id != 0 {
			out[id] = struct{}{}
		}
	}
	// skip is a snapshot. Goroutines only write out, so the loop must not
	// read that map while they run.
	skip := make(map[int64]struct{}, len(out))
	for id := range out {
		skip[id] = struct{}{}
	}
	var (
		mu    sync.Mutex
		first error
		wg    sync.WaitGroup
		sem   = make(chan struct{}, blockLookups)
	)
	for _, id := range ids {
		if _, ok := skip[id]; ok {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(id int64) {
			defer wg.Done()
			defer func() { <-sem }()
			flag, err := u.users.UserIsBlockedByUser(ctx, &user.TLUserIsBlockedByUser{
				UserId:     id,
				PeerUserId: owner,
			})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if first == nil {
					first = err
				}
				return
			}
			if mtproto.FromBool(flag) {
				out[id] = struct{}{}
			}
		}(id)
	}
	wg.Wait()
	if first != nil {
		return nil, first
	}
	return out, nil
}
