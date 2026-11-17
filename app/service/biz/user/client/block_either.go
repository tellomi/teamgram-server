package userclient

import (
	"context"

	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// EitherBlocked reports a block in either direction between a and b.
// A transport error is not a block: the caller keeps its previous rule.
func EitherBlocked(ctx context.Context, cli UserClient, a, b int64) bool {
	if cli == nil || a == 0 || b == 0 || a == b {
		return false
	}
	ids, err := cli.UserBlockedEitherWay(ctx, &user.TLUserCheckBlockUserList{
		UserId: a,
		Id:     []int64{b},
	})
	if err != nil || ids == nil {
		return false
	}
	for _, id := range ids.GetDatas() {
		if id == b {
			return true
		}
	}
	return false
}
