package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/passport/book"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// loadBotHook lets tests supply a bot without a user RPC.
var loadBotHook func(c *PassportCore, botID int64) (*mtproto.User, string, error)

func (c *PassportCore) book() *book.Book {
	if c != nil && c.svcCtx != nil && c.svcCtx.Book != nil {
		return c.svcCtx.Book
	}
	return book.Shared()
}

func (c *PassportCore) requireUser() (int64, error) {
	if c == nil || c.MD == nil || c.MD.UserId == 0 {
		return 0, mtproto.ErrUserIdInvalid
	}
	if c.MD.IsBot {
		return 0, mtproto.ErrBotMethodInvalid
	}
	return c.MD.UserId, nil
}

func (c *PassportCore) requireBot() (int64, error) {
	if c == nil || c.MD == nil || c.MD.UserId == 0 {
		return 0, mtproto.ErrUserIdInvalid
	}
	if !c.MD.IsBot {
		return 0, mtproto.ErrUserBotRequired
	}
	return c.MD.UserId, nil
}

func (c *PassportCore) loadBot(botID int64) (*mtproto.User, string, error) {
	if botID <= 0 {
		return nil, "", mtproto.ErrBotInvalid
	}
	if loadBotHook != nil {
		return loadBotHook(c, botID)
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.UserClient == nil {
		return nil, "", mtproto.ErrBotInvalid
	}
	self := c.MD.UserId
	users, err := c.svcCtx.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
		Id: []int64{self, botID},
	})
	if err != nil || users == nil {
		return nil, "", mtproto.ErrBotInvalid
	}
	list := users.GetUserListByIdList(self, botID)
	if len(list) == 0 || list[0] == nil || list[0].GetDeleted() || !list[0].GetBot() {
		return nil, "", mtproto.ErrBotInvalid
	}
	policy := ""
	info, err := c.svcCtx.UserClient.UserGetBotInfo(c.ctx, &userpb.TLUserGetBotInfo{BotId: botID})
	if err == nil && info != nil && info.GetPrivacyPolicyUrl() != nil {
		policy = info.GetPrivacyPolicyUrl().GetValue()
	}
	return list[0], policy, nil
}

func resolveTarget(in *mtproto.InputUser, botID int64) (int64, error) {
	if in == nil {
		return 0, mtproto.ErrUserIdInvalid
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputUserSelf, mtproto.Predicate_inputUserEmpty:
		return 0, mtproto.ErrUserIdInvalid
	case mtproto.Predicate_inputUser, mtproto.Predicate_inputUserFromMessage, "":
		id := in.GetUserId()
		if id <= 0 || id == botID {
			return 0, mtproto.ErrUserIdInvalid
		}
		return id, nil
	default:
		return 0, mtproto.ErrUserIdInvalid
	}
}
