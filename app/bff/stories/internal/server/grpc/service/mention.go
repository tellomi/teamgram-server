package service

import (
	"context"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/teamgram/proto/mtproto"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

// notifyStoryMentions writes a private messageMediaStory for each user named
// in the caption. MentionName carries a user id. A plain @username is
// resolved through the user service. The author is not notified.
func (s *Service) notifyStoryMentions(ctx context.Context, from, auth int64, storyPeer *mtproto.Peer, storyID int32, caption string, entities []*mtproto.MessageEntity) {
	if s == nil || s.svcCtx == nil || s.svcCtx.Msg == nil || storyID == 0 || storyPeer == nil {
		return
	}
	ids := mentionUserIDs(entities)
	for _, name := range mentionUsernames(caption, entities) {
		if id := s.userIDByUsername(ctx, name); id != 0 {
			ids = append(ids, id)
		}
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		if id == 0 || id == from || seen[id] {
			continue
		}
		seen[id] = true
		body := mtproto.MakeTLMessage(&mtproto.Message{
			Out:     true,
			FromId:  mtproto.MakePeerUser(from),
			PeerId:  mtproto.MakePeerUser(id),
			Date:    int32(time.Now().Unix()),
			Message: "",
			Media: mtproto.MakeTLMessageMediaStory(&mtproto.MessageMedia{
				ViaMention: true,
				Peer:       storyPeer,
				Id:         storyID,
			}).To_MessageMedia(),
		}).To_Message()
		random := from ^ (id << 1) ^ (int64(storyID) << 17)
		if random == 0 {
			random = 1
		}
		if _, err := s.svcCtx.Msg.MsgSendMessageV2(ctx, &msgpb.TLMsgSendMessageV2{
			UserId:    from,
			AuthKeyId: auth,
			PeerType:  mtproto.PEER_USER,
			PeerId:    id,
			Message: []*msgpb.OutboxMessage{
				msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
					RandomId: random,
					Message:  body,
				}).To_OutboxMessage(),
			},
		}); err != nil {
			logx.WithContext(ctx).Errorf("story mention to %d: %v", id, err)
		}
	}
}

func storyIDOf(up *mtproto.Updates) int32 {
	for _, u := range up.GetUpdates() {
		if u.GetPredicateName() == mtproto.Predicate_updateStoryID && u.GetId_INT32() != 0 {
			return u.GetId_INT32()
		}
		if id := u.GetStory().GetId(); id != 0 {
			return id
		}
	}
	return 0
}

func mentionUserIDs(entities []*mtproto.MessageEntity) []int64 {
	var ids []int64
	for _, e := range entities {
		if e == nil {
			continue
		}
		switch e.GetPredicateName() {
		case mtproto.Predicate_messageEntityMentionName, mtproto.Predicate_inputMessageEntityMentionName:
			id := e.GetUserId_INT64()
			if id == 0 {
				id = e.GetUserId_INPUTUSER().GetUserId()
			}
			if id != 0 {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

func mentionUsernames(caption string, entities []*mtproto.MessageEntity) []string {
	var names []string
	for _, e := range entities {
		if e == nil || e.GetPredicateName() != mtproto.Predicate_messageEntityMention {
			continue
		}
		text := utf16Slice(caption, e.GetOffset(), e.GetLength())
		text = strings.TrimPrefix(strings.TrimSpace(text), "@")
		if text != "" {
			names = append(names, text)
		}
	}
	return names
}

func utf16Slice(s string, offset, length int32) string {
	if offset < 0 || length <= 0 {
		return ""
	}
	units := utf16.Encode([]rune(s))
	end := int(offset + length)
	if int(offset) > len(units) || end > len(units) {
		return ""
	}
	return string(utf16.Decode(units[offset:end]))
}

func (s *Service) userIDByUsername(ctx context.Context, name string) int64 {
	if s.svcCtx == nil || s.svcCtx.UserClient == nil || name == "" {
		return 0
	}
	res, err := s.svcCtx.UserClient.UserGetListByUsernameList(ctx, &user.TLUserGetListByUsernameList{
		Names: []string{name},
	})
	if err != nil || res == nil {
		return 0
	}
	for _, row := range res.GetDatas() {
		if row.GetPeer().GetUserId() != 0 {
			return row.GetPeer().GetUserId()
		}
	}
	return 0
}
