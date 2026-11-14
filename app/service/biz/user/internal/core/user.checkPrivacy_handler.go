package core

import (
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// UserCheckPrivacy
// user.checkPrivacy flags:# user_id:int key_type:int peer_id:int is_contact:flags.0?true = Bool;
//
// user_id is the owner of the rules. peer_id is the user who wants to do
// the thing. The result is whether peer_id is allowed.
func (c *UserCore) UserCheckPrivacy(in *user.TLUserCheckPrivacy) (*mtproto.Bool, error) {
	if in == nil || in.UserId == 0 || in.PeerId == 0 {
		return mtproto.BoolFalse, nil
	}
	if in.UserId == in.PeerId {
		return mtproto.BoolTrue, nil
	}
	rules, err := c.UserGetPrivacy(&user.TLUserGetPrivacy{
		UserId:  in.UserId,
		KeyType: in.KeyType,
	})
	if err != nil || rules == nil {
		return mtproto.BoolFalse, nil
	}
	sub := mtproto.PrivacySubject{UserID: in.PeerId}
	if row := c.svcCtx.Dao.GetUserContact(c.ctx, in.UserId, in.PeerId); row != nil {
		sub.Contact = true
		sub.CloseFriend = row.GetCloseFriend()
	}
	if peer := c.svcCtx.Dao.GetCacheUserData(c.ctx, in.PeerId); peer != nil && peer.UserData != nil {
		data := peer.UserData
		sub.Bot = data.GetUserType() == user.UserTypeBot || data.GetBot() != nil
		sub.Premium = data.GetPremium()
		if exp := data.GetPremiumExpireDate().GetValue(); exp > 0 && exp <= time.Now().Unix() {
			sub.Premium = false
		}
	}
	sub.Member = func(chats []int64) (bool, bool) {
		return mtproto.LookupChatMember(c.ctx, in.PeerId, chats)
	}
	return mtproto.ToBool(mtproto.EvaluatePrivacy(rules.GetDatas(), sub)), nil
}
