package core

import (
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/premium"
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

// senderPremium reports a paid users.premium subscription.
// user.Premium() is true for every account while TELLOMI_PREMIUM_EVERYONE
// is on. That switch raises feature badges and the limits help.getAppConfig
// advertises. Caption length stays on the official rule: 1024 UTF-16 units,
// or 2048 with an unexpired subscription. A lookup failure uses 1024.
func (c *MessagesCore) senderPremium() (paid bool) {
	if c == nil || c.MD == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil || c.MD.UserId == 0 {
		return false
	}
	// Test doubles embed a nil user client. The promoted method then panics.
	defer func() { _ = recover() }()
	users, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
		Id: []int64{c.MD.UserId},
	})
	if err != nil || users == nil {
		return false
	}
	user, ok := users.GetImmutableUser(c.MD.UserId)
	if !ok || user == nil || user.User == nil || user.Deleted() {
		return false
	}
	var expire int64
	if user.User.GetPremiumExpireDate() != nil {
		expire = user.User.GetPremiumExpireDate().GetValue()
	}
	return premium.ActiveSubscription(user.User.GetPremium(), expire, time.Now().Unix())
}

// editIsCaption reports whether this edit's text is a media caption.
// A new photo, file, or other InputMedia is a caption. inputMediaEmpty
// removes media, so the text is a message. A link-preview card is not a
// caption. When the request omits media, the stored message decides.
func editIsCaption(existing *mtproto.MessageMedia, incoming *mtproto.InputMedia) bool {
	if incoming != nil {
		switch incoming.GetPredicateName() {
		case "", mtproto.Predicate_inputMediaEmpty, mtproto.Predicate_inputMediaWebPage:
			return false
		default:
			return true
		}
	}
	if existing == nil {
		return false
	}
	switch existing.GetPredicateName() {
	case "", mtproto.Predicate_messageMediaEmpty, mtproto.Predicate_messageMediaWebPage:
		return false
	default:
		return true
	}
}

func (c *MessagesCore) rejectOverlong(text string, caption bool) error {
	if caption {
		if c.captionLimitExceeded(text) {
			return mtproto.ErrMediaCaptionTooLong
		}
		return nil
	}
	if textTooLong(text) {
		return mtproto.ErrMessageTooLong
	}
	return nil
}

// channelStoredMedia reads the post being edited when the request does not
// send a new InputMedia. A miss leaves the text on the message cap.
func (c *MessagesCore) channelStoredMedia(peer *mtproto.PeerUtil, id int32) *mtproto.MessageMedia {
	if c == nil || c.MD == nil || peer == nil || peer.PeerType != mtproto.PEER_CHANNEL || id == 0 {
		return nil
	}
	defer func() { _ = recover() }()
	res, err := channel.Shared().GetMessages(c.ctx, c.MD.UserId, mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  peer.PeerId,
		AccessHash: peer.AccessHash,
	}).To_InputChannel(), []int32{id})
	if err != nil || res == nil {
		return nil
	}
	for _, m := range res.GetMessages() {
		if m != nil && m.GetId() == id && m.GetPredicateName() != mtproto.Predicate_messageEmpty {
			return m.GetMedia()
		}
	}
	return nil
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
