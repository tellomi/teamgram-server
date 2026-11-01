package core

import (
	"context"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
)

// ChatEditChatCreator
// messages.editChatCreator for a basic group. The operator must already be
// the creator. The target stays in the group and becomes creator; the old
// creator stays as an admin.
// https://core.telegram.org/method/messages.editChatCreator
func (c *ChatCore) ChatEditChatCreator(in *chat.TLChatEditChatAdmin) (*mtproto.MutableChat, error) {
	if in == nil || in.ChatId == 0 || in.OperatorId == 0 || in.EditChatAdminId == 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if in.OperatorId == in.EditChatAdminId {
		return nil, mtproto.ErrUserIdInvalid
	}
	chat2, err := c.svcCtx.Dao.GetMutableChat(c.ctx, in.ChatId, in.OperatorId, in.EditChatAdminId)
	if err != nil {
		c.Logger.Errorf("chat.editChatCreator - error: %v", err)
		return nil, mtproto.ErrPeerIdInvalid
	}
	if chat2.Creator() != in.OperatorId {
		return nil, mtproto.ErrChatAdminRequired
	}
	me, _ := chat2.GetImmutableChatParticipant(in.OperatorId)
	target, _ := chat2.GetImmutableChatParticipant(in.EditChatAdminId)
	if me == nil || !me.IsChatMemberStateNormal() || !me.IsChatMemberCreator() {
		return nil, mtproto.ErrChatAdminRequired
	}
	if target == nil || !target.IsChatMemberStateNormal() {
		return nil, mtproto.ErrUserNotParticipant
	}
	_, _, err = c.svcCtx.Dao.CachedConn.Exec(
		c.ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			tR := sqlx.TxWrapper(c.ctx, c.svcCtx.Dao.DB, func(tx *sqlx.Tx, result *sqlx.StoreResult) {
				_, result.Err = c.svcCtx.Dao.ChatsDAO.UpdateCreatorTx(tx, in.EditChatAdminId, in.ChatId)
				if result.Err != nil {
					return
				}
				_, result.Err = c.svcCtx.Dao.ChatParticipantsDAO.UpdateParticipantTypeTx(tx, mtproto.ChatMemberAdmin, me.Id)
				if result.Err != nil {
					return
				}
				_, result.Err = c.svcCtx.Dao.ChatParticipantsDAO.UpdateParticipantTypeTx(tx, mtproto.ChatMemberCreator, target.Id)
			})
			return 0, 0, tR.Err
		},
		c.svcCtx.Dao.GetChatCacheKey(in.ChatId),
		c.svcCtx.Dao.GetChatParticipantCacheKey(in.ChatId, in.OperatorId),
		c.svcCtx.Dao.GetChatParticipantCacheKey(in.ChatId, in.EditChatAdminId),
	)
	if err != nil {
		c.Logger.Errorf("chat.editChatCreator - error: %v", err)
		return nil, err
	}
	return c.svcCtx.Dao.GetMutableChat(c.ctx, in.ChatId, in.OperatorId, in.EditChatAdminId)
}
