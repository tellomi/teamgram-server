package core

import (
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// UserBlockedEitherWay returns the subset of id that has a block in either
// direction with user_id. One pair of queries covers the whole list.
func (c *UserCore) UserBlockedEitherWay(in *user.TLUserCheckBlockUserList) (*user.Vector_Long, error) {
	out := &user.Vector_Long{Datas: []int64{}}
	if in.GetUserId() <= 0 || len(in.GetId()) == 0 {
		return out, nil
	}
	ids, err := c.svcCtx.Dao.BlockedEitherWay(c.ctx, in.GetUserId(), in.GetId())
	if err != nil {
		return nil, err
	}
	if len(ids) > 0 {
		out.Datas = ids
	}
	return out, nil
}
