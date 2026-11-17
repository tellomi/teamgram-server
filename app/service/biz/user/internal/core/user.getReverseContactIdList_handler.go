package core

import (
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// UserGetReverseContactIdList returns the ids of users who added this user.
// It reads the cached id list and does not load contact rows.
func (c *UserCore) UserGetReverseContactIdList(in *user.TLUserGetContactIdList) (*user.Vector_Long, error) {
	out := &user.Vector_Long{Datas: []int64{}}
	if in.GetUserId() <= 0 {
		return out, nil
	}
	data := c.svcCtx.Dao.GetCacheUserData(c.ctx, in.GetUserId())
	if len(data.GetReverseContactIdList()) > 0 {
		out.Datas = data.GetReverseContactIdList()
	}
	return out, nil
}
