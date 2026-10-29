package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/premium"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func (c *PremiumCore) AccountUpdateEmojiStatus(in *mtproto.TLAccountUpdateEmojiStatus) (*mtproto.Bool, error) {
	if c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil || c.MD == nil {
		return nil, mtproto.ErrInternalServerError
	}
	documentID, until := emojiStatusParts(in.GetEmojiStatus())
	ok, err := c.svcCtx.Dao.UserClient.UserUpdateEmojiStatus(c.ctx, &userpb.TLUserUpdateEmojiStatus{
		UserId:                c.MD.UserId,
		EmojiStatusDocumentId: documentID,
		EmojiStatusUntil:      until,
	})
	if err != nil {
		c.Logger.Errorf("account.updateEmojiStatus - error: %v", err)
		return nil, err
	}
	if !mtproto.FromBool(ok) {
		return nil, mtproto.ErrUserIdInvalid
	}
	c.pushEmojiStatus(documentID, until)
	return mtproto.BoolTrue, nil
}

func (c *PremiumCore) pushEmojiStatus(documentID int64, until int32) {
	if c.svcCtx.Dao.SyncClient == nil || c.MD == nil {
		return
	}
	var status *mtproto.EmojiStatus
	if documentID == 0 {
		status = mtproto.MakeTLEmojiStatusEmpty(&mtproto.EmojiStatus{}).To_EmojiStatus()
	} else {
		status = mtproto.MakeTLEmojiStatus(&mtproto.EmojiStatus{
			DocumentId:      documentID,
			Until_INT32:     until,
			Until_FLAGINT32: wrapperspb.Int32(until),
		}).To_EmojiStatus()
		if until == 0 {
			status.Until_FLAGINT32 = nil
		}
	}
	_, _ = c.svcCtx.Dao.SyncClient.SyncUpdatesNotMe(c.ctx, &sync.TLSyncUpdatesNotMe{
		UserId:        c.MD.UserId,
		PermAuthKeyId: c.MD.PermAuthKeyId,
		Updates: mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateUserEmojiStatus(&mtproto.Update{
			UserId:      c.MD.UserId,
			EmojiStatus: status,
		}).To_Update()),
	})
}

func (c *PremiumCore) AccountGetDefaultEmojiStatuses(in *mtproto.TLAccountGetDefaultEmojiStatuses) (*mtproto.Account_EmojiStatuses, error) {
	return emojiStatusList(in.GetHash()), nil
}

func (c *PremiumCore) AccountGetRecentEmojiStatuses(in *mtproto.TLAccountGetRecentEmojiStatuses) (*mtproto.Account_EmojiStatuses, error) {
	return emojiStatusList(in.GetHash()), nil
}

func (c *PremiumCore) AccountGetChannelDefaultEmojiStatuses(in *mtproto.TLAccountGetChannelDefaultEmojiStatuses) (*mtproto.Account_EmojiStatuses, error) {
	return emojiStatusList(in.GetHash()), nil
}

func (c *PremiumCore) AccountClearRecentEmojiStatuses(in *mtproto.TLAccountClearRecentEmojiStatuses) (*mtproto.Bool, error) {
	return mtproto.BoolTrue, nil
}

func emojiStatusList(hash int64) *mtproto.Account_EmojiStatuses {
	if hash == premium.EmojiStatusListHash {
		return mtproto.MakeTLAccountEmojiStatusesNotModified(&mtproto.Account_EmojiStatuses{}).To_Account_EmojiStatuses()
	}
	// Document ids come from the sticker import. An empty catalog still lets
	// account.updateEmojiStatus accept a document the client already has.
	return mtproto.MakeTLAccountEmojiStatuses(&mtproto.Account_EmojiStatuses{
		Hash:     premium.EmojiStatusListHash,
		Statuses: []*mtproto.EmojiStatus{},
	}).To_Account_EmojiStatuses()
}

func (c *PremiumCore) AccountUpdateColor(in *mtproto.TLAccountUpdateColor) (*mtproto.Bool, error) {
	if c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil || c.MD == nil {
		return nil, mtproto.ErrInternalServerError
	}
	colorID, emoji, _ := selectedColor(in)
	ok, err := c.svcCtx.Dao.UserClient.UserSetColor(c.ctx, &userpb.TLUserSetColor{
		UserId:            c.MD.UserId,
		ForProfile:        in.GetForProfile(),
		Color:             colorID,
		BackgroundEmojiId: emoji,
	})
	if err != nil {
		c.Logger.Errorf("account.updateColor - error: %v", err)
		return nil, err
	}
	if !mtproto.FromBool(ok) {
		return nil, mtproto.ErrUserIdInvalid
	}
	c.pushUser(c.MD.UserId)
	return mtproto.BoolTrue, nil
}

func (c *PremiumCore) pushUser(userID int64) {
	if c.svcCtx.Dao.SyncClient == nil || c.MD == nil {
		return
	}
	me, err := c.svcCtx.Dao.UserClient.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: userID})
	if err != nil || me == nil {
		c.Logger.Errorf("account.updateColor - reload user: %v", err)
		return
	}
	_, _ = c.svcCtx.Dao.SyncClient.SyncUpdatesNotMe(c.ctx, &sync.TLSyncUpdatesNotMe{
		UserId:        c.MD.UserId,
		PermAuthKeyId: c.MD.PermAuthKeyId,
		Updates: mtproto.MakeUpdatesByUpdatesUsers(
			[]*mtproto.User{me.ToSelfUser()},
			mtproto.MakeTLUpdateUser(&mtproto.Update{UserId: userID}).To_Update(),
		),
	})
}

func (c *PremiumCore) HelpGetPeerColors(in *mtproto.TLHelpGetPeerColors) (*mtproto.Help_PeerColors, error) {
	return peerColorsReply(in.GetHash(), false), nil
}

func (c *PremiumCore) HelpGetPeerProfileColors(in *mtproto.TLHelpGetPeerProfileColors) (*mtproto.Help_PeerColors, error) {
	return peerColorsReply(in.GetHash(), true), nil
}

func (c *PremiumCore) AccountGetDefaultBackgroundEmojis(in *mtproto.TLAccountGetDefaultBackgroundEmojis) (*mtproto.EmojiList, error) {
	return emptyEmojiList(in.GetHash()), nil
}

func emptyEmojiList(hash int64) *mtproto.EmojiList {
	if hash == premium.EmojiStatusListHash {
		return mtproto.MakeTLEmojiListNotModified(&mtproto.EmojiList{}).To_EmojiList()
	}
	return mtproto.MakeTLEmojiList(&mtproto.EmojiList{
		Hash:       premium.EmojiStatusListHash,
		DocumentId: []int64{},
	}).To_EmojiList()
}

// MessagesTranscribeAudio acknowledges a voice-to-text request without a
// Premium check. Tellomi has no speech engine, so the transcription fails
// instead of opening a purchase screen.
func (c *PremiumCore) MessagesTranscribeAudio(in *mtproto.TLMessagesTranscribeAudio) (*mtproto.Messages_TranscribedAudio, error) {
	if in.GetPeer() == nil || in.GetMsgId() == 0 {
		return nil, mtproto.ErrMsgIdInvalid
	}
	return nil, mtproto.ErrTranscriptionFailed
}

func (c *PremiumCore) MessagesRateTranscribedAudio(in *mtproto.TLMessagesRateTranscribedAudio) (*mtproto.Bool, error) {
	return mtproto.BoolTrue, nil
}
