package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/premium"

	"google.golang.org/protobuf/proto"
)

const (
	maxMessageUTF16 = 4096
)

func textTooLong(text string) bool {
	return mtproto.UTF16Len(text) > maxMessageUTF16
}

// senderMember is the effective membership help.getAppConfig uses.
// user.Premium() follows TELLOMI_PREMIUM_EVERYONE and an unexpired
// subscription. A lookup failure is a non-member; premium.Limit still
// raises the cap while the everyone switch is on, matching Advertised.
func (c *MessagesCore) senderMember() (member bool) {
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
	return ok && user != nil && user.Premium()
}

// captionLimit is premium.Limit(FeatureCaption) for this sender. That is the
// same number help.getAppConfig publishes as caption_length_limit_default
// or caption_length_limit_premium.
func (c *MessagesCore) captionLimit() int {
	return premium.Limit(premium.FeatureCaption, c.senderMember())
}

func (c *MessagesCore) captionLimitExceeded(text string) bool {
	return mtproto.UTF16Len(text) > c.captionLimit()
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

// storedEditIsCaption uses the stored media when the request does not send
// a new InputMedia. A lookup that does not resolve the row uses the caption
// cap. Treating that miss as a plain message would allow a caption past the
// advertised limit.
func storedEditIsCaption(media *mtproto.MessageMedia, incoming *mtproto.InputMedia, resolved bool) bool {
	if incoming != nil || resolved {
		return editIsCaption(media, incoming)
	}
	return true
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

// channelStoredMedia reads the post Edit will load. ok is false when that
// row cannot be read. The caller must not treat that as "no media".
func (c *MessagesCore) channelStoredMedia(peer *mtproto.PeerUtil, id int32) (media *mtproto.MessageMedia, ok bool) {
	if c == nil || peer == nil || peer.PeerType != mtproto.PEER_CHANNEL || id == 0 {
		return nil, false
	}
	defer func() { _ = recover() }()
	body, err := channel.Shared().MessageBody(c.ctx, peer.PeerId, id)
	if err != nil || body == nil {
		return nil, false
	}
	return body.GetMedia(), true
}

// scheduledStoredMedia reads the scheduled row this edit will rewrite.
// ok is false when that row cannot be read.
func (c *MessagesCore) scheduledStoredMedia(peer *mtproto.PeerUtil, id int32) (media *mtproto.MessageMedia, ok bool) {
	if c == nil || c.MD == nil || peer == nil || id == 0 {
		return nil, false
	}
	defer func() { _ = recover() }()
	page, err := chatbox.Shared().ScheduledByID(c.ctx, c.MD.UserId, peer, []int32{id})
	if err != nil || page == nil {
		return nil, false
	}
	for _, m := range page.GetMessages() {
		if m != nil && m.GetId() == id && m.GetPredicateName() != mtproto.Predicate_messageEmpty {
			return m.GetMedia(), true
		}
	}
	return nil, false
}

// stagePrivateEdit applies text, entities, markup, and replacement media the
// way a channel edit does. Entities the request resends stay on the message
// so MESSAGE_NOT_MODIFIED can see them. Entities are cleared only when the
// request sets new text and omits the field.
func stagePrivateEdit(before *mtproto.Message, in *mtproto.TLMessagesEditMessage, media *mtproto.MessageMedia) *mtproto.Message {
	out := proto.Clone(before).(*mtproto.Message)
	if in == nil {
		return out
	}
	if in.GetMessage() != nil {
		out.Message = in.GetMessage().GetValue()
		if in.Entities == nil {
			out.Entities = nil
		}
	}
	if in.Entities != nil {
		out.Entities = in.Entities
	}
	if in.ReplyMarkup != nil {
		out.ReplyMarkup = in.ReplyMarkup
	}
	if media != nil {
		out.Media = media
	}
	return out
}

// botReplyMarkup keeps reply_markup only when the sender is a bot.
// A user-sent keyboard is ignored, matching Telegram.
// Session metadata leaves IsBot unset, so the user row is the check:
// a known bot id, then user.isBot. A lookup failure drops the markup.
func (c *MessagesCore) botReplyMarkup(markup *mtproto.ReplyMarkup) *mtproto.ReplyMarkup {
	if markup == nil || !c.senderIsBot() {
		return nil
	}
	return markup
}

func (c *MessagesCore) senderIsBot() bool {
	if c == nil || c.MD == nil || c.MD.UserId == 0 {
		return false
	}
	if c.MD.IsBot || userpb.GetBotNameById(c.MD.UserId) != "" {
		return true
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return false
	}
	defer func() { _ = recover() }()
	ok, err := c.svcCtx.Dao.UserClient.UserIsBot(c.ctx, &userpb.TLUserIsBot{Id: c.MD.UserId})
	return err == nil && mtproto.FromBool(ok)
}
