package core

import (
	"time"

	"github.com/teamgram/proto/mtproto"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
)

// MessagesMigrateChat
// messages.migrateChat#a2875319 chat_id:long = Updates;
func (c *ChatsCore) MessagesMigrateChat(in *mtproto.TLMessagesMigrateChat) (*mtproto.Updates, error) {
	chat, err := c.svcCtx.Dao.ChatClient.Client().ChatGetMutableChat(c.ctx, &chatpb.TLChatGetMutableChat{
		ChatId: in.ChatId,
	})
	if err != nil || chat == nil {
		c.Logger.Errorf("messages.migrateChat - error: %v", err)
		return nil, mtproto.ErrChatIdInvalid
	}
	if chat.Creator() != c.MD.UserId {
		return nil, mtproto.ErrChatAdminRequired
	}
	if migrated := chat.MigratedTo(); migrated != nil && migrated.GetChannelId() != 0 {
		chats, err := channel.Shared().GetChannels(c.ctx, c.MD.UserId, []*mtproto.InputChannel{migrated})
		if err != nil {
			return nil, err
		}
		return mtproto.MakeUpdatesByUpdatesChats(
			chats.Chats,
			mtproto.MakeTLUpdateChannel(&mtproto.Update{ChannelId: migrated.GetChannelId()}).To_Update(),
		), nil
	}

	var members []channel.LegacyMember
	chat.Walk(func(userId int64, participant *mtproto.ImmutableChatParticipant) error {
		if participant == nil || !participant.IsChatMemberStateNormal() {
			return nil
		}
		members = append(members, channel.LegacyMember{UserID: participant.UserId, IsBot: participant.IsBot})
		return nil
	})
	result, err := channel.Shared().Migrate(c.ctx, channel.MigrateRequest{
		Actor:     c.MD.UserId,
		ActorAuth: c.MD.PermAuthKeyId,
		ChatID:    chat.Id(),
		Title:     chat.Title(),
		About:     chat.About(),
		CreatorID: chat.Creator(),
		Members:   members,
	})
	if err != nil {
		c.Logger.Errorf("messages.migrateChat - error: %v", err)
		return nil, err
	}
	_, _ = c.svcCtx.Dao.ChatClient.Client().ChatMigratedToChannel(c.ctx, &chatpb.TLChatMigratedToChannel{
		Chat:       chat,
		Id:         result.ChannelID,
		AccessHash: result.AccessHash,
	})

	chatUpdates, err := c.svcCtx.Dao.MsgClient.MsgSendMessageV2(c.ctx, &msgpb.TLMsgSendMessageV2{
		UserId:    c.MD.UserId,
		AuthKeyId: c.MD.PermAuthKeyId,
		PeerType:  mtproto.PEER_CHAT,
		PeerId:    chat.Id(),
		Message: []*msgpb.OutboxMessage{
			msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
				NoWebpage: true,
				RandomId:  time.Now().UnixNano(),
				Message: mtproto.MakeTLMessageService(&mtproto.Message{
					Out:    true,
					FromId: mtproto.MakePeerUser(c.MD.UserId),
					PeerId: mtproto.MakePeerChat(chat.Id()),
					Date:   int32(time.Now().Unix()),
					Action: mtproto.MakeMessageActionChatMigrateTo(result.ChannelID),
				}).To_Message(),
			}).To_OutboxMessage(),
		},
	})
	if err != nil {
		c.Logger.Errorf("messages.migrateChat - legacy service message: %v", err)
		return result.Updates, nil
	}
	return mergeUpdates(result.Updates, chatUpdates), nil
}

func mergeUpdates(parts ...*mtproto.Updates) *mtproto.Updates {
	out := mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: []*mtproto.Update{},
		Users:   []*mtproto.User{},
		Chats:   []*mtproto.Chat{},
		Date:    int32(time.Now().Unix()),
	}).To_Updates()
	for _, part := range parts {
		if part == nil {
			continue
		}
		out.Updates = append(out.Updates, part.Updates...)
		out.Users = append(out.Users, part.Users...)
		out.Chats = append(out.Chats, part.Chats...)
		if part.Date != 0 {
			out.Date = part.Date
		}
	}
	if out.Updates == nil {
		out.Updates = []*mtproto.Update{}
	}
	if out.Users == nil {
		out.Users = []*mtproto.User{}
	}
	if out.Chats == nil {
		out.Chats = []*mtproto.Chat{}
	}
	return out
}
