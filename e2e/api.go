package e2e

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func randomID() int64 {
	var b [8]byte
	_, _ = rand.Read(b[:])
	id := int64(binary.LittleEndian.Uint64(b[:]) & 0x7fffffffffffffff)
	if id == 0 {
		id = 1
	}
	return id
}

// SendText sends a private, group, or channel message and returns its id.
func (c *Client) SendText(ctx context.Context, peer *mtproto.InputPeer, text string, entities []*mtproto.MessageEntity) (int32, error) {
	res, err := c.Invoke(ctx, &mtproto.TLMessagesSendMessage{
		Constructor: mtproto.CRC32_messages_sendMessage_fef48f62,
		Peer:        peer,
		Message:     text,
		RandomId:    randomID(),
		Entities:    entities,
	})
	if err != nil {
		return 0, err
	}
	id := messageID(res)
	if id == 0 {
		return 0, fmt.Errorf("messages.sendMessage returned %T without a message id", res)
	}
	return id, nil
}

func (c *Client) ReadHistory(ctx context.Context, peer *mtproto.InputPeer, maxID int32) error {
	_, err := c.Invoke(ctx, &mtproto.TLMessagesReadHistory{
		Constructor: mtproto.CRC32_messages_readHistory,
		Peer:        peer,
		MaxId:       maxID,
	})
	return err
}

func (c *Client) ReadChannelHistory(ctx context.Context, channel *mtproto.InputChannel, maxID int32) error {
	_, err := c.Invoke(ctx, &mtproto.TLChannelsReadHistory{
		Constructor: mtproto.CRC32_channels_readHistory,
		Channel:     channel,
		MaxId:       maxID,
	})
	return err
}

func (c *Client) State(ctx context.Context) (pts, qts, date int32, err error) {
	res, err := c.Invoke(ctx, &mtproto.TLUpdatesGetState{
		Constructor: mtproto.CRC32_updates_getState,
	})
	if err != nil {
		return 0, 0, 0, err
	}
	st, ok := res.(*mtproto.TLUpdatesState)
	if !ok {
		return 0, 0, 0, fmt.Errorf("updates.getState returned %T", res)
	}
	return st.GetPts(), st.GetQts(), st.GetDate(), nil
}

func (c *Client) Difference(ctx context.Context, pts, qts, date int32) (mtproto.TLObject, error) {
	return c.Invoke(ctx, &mtproto.TLUpdatesGetDifference{
		Constructor:   mtproto.CRC32_updates_getDifference_19c2f763,
		Pts:           pts,
		Date:          date,
		Qts:           qts,
		PtsTotalLimit: &wrapperspb.Int32Value{Value: 100},
	})
}

func (c *Client) History(ctx context.Context, peer *mtproto.InputPeer, limit int32) ([]*mtproto.Message, error) {
	res, err := c.Invoke(ctx, &mtproto.TLMessagesGetHistory{
		Constructor: mtproto.CRC32_messages_getHistory,
		Peer:        peer,
		Limit:       limit,
	})
	if err != nil {
		return nil, err
	}
	msgs, ok := messagesOf(res)
	if !ok {
		return nil, fmt.Errorf("messages.getHistory returned %T", res)
	}
	return msgs, nil
}

type dialogPage struct {
	Dialogs []*mtproto.Dialog
	Chats   []*mtproto.Chat
	Users   []*mtproto.User
}

func (c *Client) dialogPage(ctx context.Context) (dialogPage, error) {
	res, err := c.Invoke(ctx, &mtproto.TLMessagesGetDialogs{
		Constructor: mtproto.CRC32_messages_getDialogs,
		OffsetPeer:  inputPeerEmpty(),
		Limit:       100,
	})
	if err != nil {
		return dialogPage{}, err
	}
	switch d := res.(type) {
	case *mtproto.TLMessagesDialogs:
		return dialogPage{Dialogs: d.GetDialogs(), Chats: d.GetChats(), Users: d.GetUsers()}, nil
	case *mtproto.TLMessagesDialogsSlice:
		return dialogPage{Dialogs: d.GetDialogs(), Chats: d.GetChats(), Users: d.GetUsers()}, nil
	case *mtproto.TLMessagesDialogsNotModified:
		return dialogPage{}, nil
	default:
		return dialogPage{}, fmt.Errorf("messages.getDialogs returned %T", res)
	}
}

func (c *Client) Dialogs(ctx context.Context) ([]*mtproto.Dialog, error) {
	page, err := c.dialogPage(ctx)
	return page.Dialogs, err
}

// OutboxMaxID returns dialog.read_outbox_max_id for the peer, and whether
// the dialog is present.
func (c *Client) OutboxMaxID(ctx context.Context, kind string, peerID int64) (int32, bool, error) {
	dialogs, err := c.Dialogs(ctx)
	if err != nil {
		return 0, false, err
	}
	for _, d := range dialogs {
		if d == nil || d.GetPeer() == nil {
			continue
		}
		var id int64
		switch kind {
		case "user":
			id = d.GetPeer().GetUserId()
		case "chat":
			id = d.GetPeer().GetChatId()
		case "channel":
			id = d.GetPeer().GetChannelId()
		}
		if id == peerID {
			return d.GetReadOutboxMaxId(), true, nil
		}
	}
	return 0, false, nil
}

func (c *Client) ChatAccess(ctx context.Context, chatID int64) (int64, bool, error) {
	page, err := c.dialogPage(ctx)
	if err != nil {
		return 0, false, err
	}
	for _, chat := range page.Chats {
		if chat != nil && chat.GetId() == chatID {
			return chatAccessHash(chat), true, nil
		}
	}
	return 0, false, nil
}

func (c *Client) CreateChat(ctx context.Context, title string, users ...*mtproto.InputUser) (int64, error) {
	res, err := c.Invoke(ctx, &mtproto.TLMessagesCreateChat9CB126E{
		Constructor: mtproto.CRC32_messages_createChat9CB126E,
		Users:       users,
		Title:       title,
	})
	if err != nil {
		return 0, err
	}
	for _, chat := range chatsOf(res) {
		if chat == nil || chat.GetId() == 0 {
			continue
		}
		if chat.GetMegagroup() || chat.GetBroadcast() {
			continue
		}
		if title == "" || chat.GetTitle() == title || chat.GetPredicateName() == mtproto.Predicate_chat {
			return chat.GetId(), nil
		}
	}
	return 0, fmt.Errorf("messages.createChat returned %T without a basic group", res)
}

func (c *Client) CreateMegagroup(ctx context.Context, title string) (id, accessHash int64, err error) {
	res, err := c.Invoke(ctx, &mtproto.TLChannelsCreateChannel{
		Constructor: mtproto.CRC32_channels_createChannel_91006707,
		Megagroup:   true,
		Title:       title,
		About:       "",
	})
	if err != nil {
		return 0, 0, err
	}
	for _, chat := range chatsOf(res) {
		if chat == nil || !chat.GetMegagroup() {
			continue
		}
		return chat.GetId(), chatAccessHash(chat), nil
	}
	return 0, 0, fmt.Errorf("channels.createChannel returned %T without a megagroup", res)
}

func (c *Client) InviteToChannel(ctx context.Context, channel *mtproto.InputChannel, users ...*mtproto.InputUser) error {
	_, err := c.Invoke(ctx, &mtproto.TLChannelsInviteToChannel199F3A6C{
		Constructor: mtproto.CRC32_channels_inviteToChannel199F3A6C,
		Channel:     channel,
		Users:       users,
	})
	return err
}

func (c *Client) SendReaction(ctx context.Context, peer *mtproto.InputPeer, msgID int32, emoticon string) (mtproto.TLObject, error) {
	return c.Invoke(ctx, &mtproto.TLMessagesSendReaction{
		Constructor:                 mtproto.CRC32_messages_sendReaction_d30d78d4,
		AddToRecent:                 true,
		Peer:                        peer,
		MsgId:                       msgID,
		Reaction_FLAGVECTORREACTION: []*mtproto.Reaction{reactionEmoji(emoticon)},
	})
}

func chatAccessHash(chat *mtproto.Chat) int64 {
	if chat.GetAccessHash_INT64() != 0 {
		return chat.GetAccessHash_INT64()
	}
	if chat.GetAccessHash_FLAGINT64() != nil {
		return chat.GetAccessHash_FLAGINT64().GetValue()
	}
	return 0
}

func messageID(obj mtproto.TLObject) int32 {
	switch v := obj.(type) {
	case *mtproto.TLUpdateShortSentMessage:
		return v.GetId()
	}
	for _, m := range allMessages(obj) {
		if m != nil && m.GetId() != 0 {
			return m.GetId()
		}
	}
	return 0
}

func messagesOf(obj mtproto.TLObject) ([]*mtproto.Message, bool) {
	switch v := obj.(type) {
	case *mtproto.TLMessagesMessages:
		return v.GetMessages(), true
	case *mtproto.TLMessagesMessagesSlice:
		return v.GetMessages(), true
	case *mtproto.TLMessagesChannelMessages:
		return v.GetMessages(), true
	default:
		return nil, false
	}
}

func allMessages(obj mtproto.TLObject) []*mtproto.Message {
	if msgs, ok := messagesOf(obj); ok {
		return msgs
	}
	var out []*mtproto.Message
	switch v := obj.(type) {
	case *mtproto.TLUpdates:
		out = append(out, messagesFromUpdates(v.To_Updates())...)
	case *mtproto.TLUpdatesCombined:
		out = append(out, messagesFromUpdates(v.To_Updates())...)
	case *mtproto.TLUpdatesDifference:
		out = append(out, v.GetNewMessages()...)
	case *mtproto.TLUpdatesDifferenceSlice:
		out = append(out, v.GetNewMessages()...)
	}
	return out
}

func messagesFromUpdates(u *mtproto.Updates) []*mtproto.Message {
	if u == nil {
		return nil
	}
	var out []*mtproto.Message
	for _, up := range u.GetUpdates() {
		if up == nil {
			continue
		}
		if m := up.GetMessage_MESSAGE(); m != nil {
			out = append(out, m)
		}
	}
	return out
}

func chatsOf(obj mtproto.TLObject) []*mtproto.Chat {
	switch v := obj.(type) {
	case *mtproto.TLUpdates:
		return v.To_Updates().GetChats()
	case *mtproto.TLUpdatesCombined:
		return v.To_Updates().GetChats()
	case *mtproto.TLMessagesInvitedUsers:
		if v.To_Messages_InvitedUsers().GetUpdates() != nil {
			return chatsOf(updatesObject(v.To_Messages_InvitedUsers().GetUpdates()))
		}
	}
	return nil
}

func updatesObject(u *mtproto.Updates) mtproto.TLObject {
	if u == nil {
		return nil
	}
	return mtproto.MakeTLUpdates(u)
}

func hasText(msgs []*mtproto.Message, text string) bool {
	for _, m := range msgs {
		if m != nil && m.GetMessage() == text {
			return true
		}
	}
	return false
}

func hasReaction(msgs []*mtproto.Message, msgID int32, emoticon string) bool {
	for _, m := range msgs {
		if m == nil || m.GetId() != msgID || m.GetReactions() == nil {
			continue
		}
		for _, rc := range m.GetReactions().GetResults() {
			if rc == nil || rc.GetCount() < 1 {
				continue
			}
			if rc.GetReaction() == emoticon || rc.GetReaction_STRING() == emoticon {
				return true
			}
			if r := rc.GetReaction_REACTION(); r != nil && r.GetEmoticon() == emoticon {
				return true
			}
		}
	}
	return false
}

func reactionIn(obj mtproto.TLObject, msgID int32, emoticon string) bool {
	return hasReaction(allMessages(obj), msgID, emoticon)
}

func waitUntil(ctx context.Context, every time.Duration, fn func() (bool, error)) error {
	t := time.NewTimer(0)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			ok, err := fn()
			if err != nil || ok {
				return err
			}
			t.Reset(every)
		}
	}
}

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}
