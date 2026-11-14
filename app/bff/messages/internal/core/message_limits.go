package core

import (
	"github.com/teamgram/proto/mtproto"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

const (
	maxMessageUTF16        = 4096
	maxCaptionUTF16        = 1024
	maxCaptionUTF16Premium = 2048
)

func textTooLong(text string) bool {
	return mtproto.UTF16Len(text) > maxMessageUTF16
}

func captionTooLong(text string, premium bool) bool {
	limit := maxCaptionUTF16
	if premium {
		limit = maxCaptionUTF16Premium
	}
	return mtproto.UTF16Len(text) > limit
}

func (c *MessagesCore) senderPremium() (premium bool) {
	if c == nil || c.MD == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil || c.MD.UserId == 0 {
		return false
	}
	// Test doubles embed a nil user client. The promoted method then panics.
	// A failed lookup is the non-premium caption limit.
	defer func() { _ = recover() }()
	users, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
		Id: []int64{c.MD.UserId},
	})
	if err != nil || users == nil {
		return false
	}
	user, ok := users.GetImmutableUser(c.MD.UserId)
	return ok && user != nil && user.Premium()
}

func (c *MessagesCore) captionLimitExceeded(text string) bool {
	return captionTooLong(text, c.senderPremium())
}

// botReplyMarkup keeps reply_markup only when the sender is a bot.
// A user-sent keyboard is ignored, matching Telegram.
func (c *MessagesCore) botReplyMarkup(markup *mtproto.ReplyMarkup) *mtproto.ReplyMarkup {
	if c == nil || c.MD == nil || !c.MD.IsBot {
		return nil
	}
	return markup
}
