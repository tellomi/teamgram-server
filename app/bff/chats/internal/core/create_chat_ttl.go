package core

import (
	"github.com/teamgram/proto/mtproto"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

// normalizeCreateTTL accepts the ttl_period flag on messages.createChat and
// channels.createChannel. Absent and 0 both mean Off. A negative value, or a
// value past one year, is TTL_PERIOD_INVALID, same as messages.setHistoryTTL.
func normalizeCreateTTL(ttl *wrapperspb.Int32Value) (int32, error) {
	if ttl == nil {
		return 0, nil
	}
	v := ttl.GetValue()
	if v < 0 || v > 366*86400 {
		return 0, mtproto.ErrTtlPeriodInvalid
	}
	return v, nil
}

// createTTLAction is the service message written into a new basic group.
// period 0 is not an action: official Telegram sends no
// messageActionSetMessagesTTL when the picker is Off. auto_setting_from is
// set only when this period is the account's global default. Android renders
// that flag as "self-destruct timer set for all chats"; a timer chosen for
// this group alone does not carry it.
func createTTLAction(period, accountDefault int32, userID int64) *mtproto.MessageAction {
	if period <= 0 {
		return nil
	}
	action := mtproto.MakeMessageActionSetMessagesTTL(period)
	if accountDefault > 0 && accountDefault == period && userID != 0 {
		action.AutoSettingFrom = mtproto.MakeFlagsInt64(userID)
	}
	return action
}

func (c *ChatsCore) accountDefaultTTL() int32 {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil || c.MD == nil {
		return 0
	}
	def, err := c.svcCtx.Dao.UserClient.UserGetDefaultHistoryTTL(c.ctx, &userpb.TLUserGetDefaultHistoryTTL{
		UserId: c.MD.UserId,
	})
	if err != nil || def == nil {
		if err != nil {
			c.Logger.Errorf("messages.createChat - default history ttl: %v", err)
		}
		return 0
	}
	return def.GetPeriod()
}
