package channels_helper

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Bridge satisfies the channel-shaped plugin interfaces on dialogs, usernames,
// contacts, drafts, and notification.
type Bridge struct{}

func (Bridge) GetChannelListByIdList(ctx context.Context, selfId int64, id ...int64) []*mtproto.Chat {
	return channel.Shared().ChatsFor(ctx, selfId, id...)
}

func (Bridge) GetChannelById(ctx context.Context, selfId, id int64) (*mtproto.Chat, error) {
	chats := channel.Shared().ChatsFor(ctx, selfId, id)
	if len(chats) == 0 {
		return nil, mtproto.ErrChannelInvalid
	}
	return chats[0], nil
}

func (Bridge) GetChannelMessage(ctx context.Context, selfId, channelId int64, id int32) (*mtproto.MessageBox, error) {
	return channel.Shared().MessageBox(ctx, selfId, channelId, id)
}

func (Bridge) GetChannelDialogById(ctx context.Context, selfId, id int64) (*dialog.DialogExt, error) {
	rows, err := channel.Shared().DialogsFor(ctx, selfId)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.ChannelID == id {
			return dialogExt(row), nil
		}
	}
	return nil, mtproto.ErrChannelPrivate
}

func (Bridge) ListChannelDialogs(ctx context.Context, selfId int64) (dialog.DialogExtList, error) {
	rows, err := channel.Shared().DialogsFor(ctx, selfId)
	if err != nil {
		return nil, err
	}
	out := make(dialog.DialogExtList, 0, len(rows))
	for _, row := range rows {
		out = append(out, dialogExt(row))
	}
	return out, nil
}

func dialogExt(row channel.DialogRow) *dialog.DialogExt {
	return dialog.MakeTLDialogExt(&dialog.DialogExt{
		Order: int64(row.Date),
		Date:  int64(row.Date),
		Dialog: mtproto.MakeTLDialog(&mtproto.Dialog{
			Peer:                mtproto.MakePeerChannel(row.ChannelID),
			TopMessage:          row.TopMessage,
			ReadInboxMaxId:      row.ReadInbox,
			ReadOutboxMaxId:     row.ReadOutbox,
			UnreadCount:         row.Unread,
			NotifySettings:      mtproto.MakeTLPeerNotifySettings(&mtproto.PeerNotifySettings{ShowPreviews: mtproto.BoolTrue, Silent: mtproto.BoolFalse}).To_PeerNotifySettings(),
			Pts:                 &wrapperspb.Int32Value{Value: row.Pts},
			ViewForumAsMessages: row.ViewForumAsMessages,
		}).To_Dialog(),
	}).To_DialogExt()
}
