package push

import (
	"context"
	"errors"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/logx"
)

// DiscoverNotifyUserID is @faxian (user 777005, username faxian / discover).
// account.updateNotifySettings on that user is the discover notification switch.
const DiscoverNotifyUserID int64 = 777005

// ErrMuted means the recipient muted discover notifications.
var ErrMuted = errors.New("discover notifications are muted")

// Activity is one content-engine interaction to push.
// Kind is like, comment, reply, mention, follow, or repost.
type Activity struct {
	UserID    int64
	Kind      string
	ActorName string
	PostID    string
	CommentID string
	Text      string
}

// DeliverActivity pushes one discover event through the same device loop as
// a chat message (APNs, FCM, Huawei, and any later sender installed on
// Service). Muting user 777005 suppresses it. Muting all private chats does
// not. A mention does not bypass that mute.
func (s *Service) DeliverActivity(ctx context.Context, a Activity) (int, error) {
	if s == nil || s.store == nil || a.UserID == 0 {
		return 0, nil
	}
	devices, err := s.store.ListByUser(ctx, a.UserID)
	if err != nil {
		return 0, err
	}
	decision := s.decideActivity(ctx, a.UserID)
	if !decision.Send {
		return 0, ErrMuted
	}
	n := 0
	for _, dev := range devices {
		if s.delivers(dev.TokenType) {
			n++
		}
	}
	if n == 0 {
		return 0, nil
	}
	s.send(ctx, devices, a.incoming(), decision)
	return n, nil
}

func (s *Service) decideActivity(ctx context.Context, userID int64) Decision {
	var peer NotifySettings
	if reader, ok := s.store.(SettingsReader); ok {
		var err error
		peer, err = reader.NotifySettings(ctx, userID, mtproto.PEER_USER, DiscoverNotifyUserID)
		if err != nil {
			logx.WithContext(ctx).Errorf("push: discover notify settings user=%d: %v", userID, err)
		}
	}
	return Decide(s.now().Unix(), peer, NotifySettings{}, false, false)
}

func (a Activity) incoming() Incoming {
	kind := strings.ToLower(strings.TrimSpace(a.Kind))
	key := "DISCOVER_" + strings.ToUpper(kind)
	text := clip(strings.TrimSpace(a.Text), 120)
	if (kind == "comment" || kind == "reply") && text != "" {
		key += "_TEXT"
	}
	name := strings.TrimSpace(a.ActorName)
	if name == "" {
		name = "发现"
	}
	extra := map[string]string{
		"discover": "1",
		"kind":     kind,
		"post_id":  a.PostID,
	}
	if a.CommentID != "" {
		extra["comment_id"] = a.CommentID
	}
	return Incoming{
		UserID:     a.UserID,
		SenderID:   DiscoverNotifyUserID,
		SenderName: name,
		Text:       text,
		LocKey:     key,
		PeerType:   mtproto.PEER_USER,
		PeerID:     DiscoverNotifyUserID,
		Extra:      extra,
	}
}

// delivers reports whether this process will actually hand the token to a sender.
// An unconfigured channel is skipped so the engine hears no_devices instead of sent.
func (s *Service) delivers(tokenType int32) bool {
	switch tokenType {
	case TokenAPNS:
		return s.apns != nil
	case TokenFCM:
		_, off := s.fcm.(fcmUnconfigured)
		return !off
	case TokenHuawei:
		_, off := s.huawei.(huaweiUnconfigured)
		return !off
	case TokenXiaomi:
		_, off := s.xiaomi.(xiaomiUnconfigured)
		return !off
	default:
		return false
	}
}

func discoverPhrase(lang, locKey string) (string, bool) {
	table, ok := discoverPhrases[locKey]
	if !ok {
		return "", false
	}
	if s, ok := table[lang]; ok {
		return s, true
	}
	s, ok := table["en"]
	return s, ok
}

var discoverPhrases = map[string]map[string]string{
	"DISCOVER_LIKE": {
		"zh-hans": "赞了你的帖子",
		"zh-hant": "對你的帖子按了讚",
		"en":      "liked your post",
	},
	"DISCOVER_COMMENT": {
		"zh-hans": "评论了你的帖子",
		"zh-hant": "評論了你的帖子",
		"en":      "commented on your post",
	},
	"DISCOVER_COMMENT_TEXT": {
		"zh-hans": "评论了你的帖子：%2$@",
		"zh-hant": "評論了你的帖子：%2$@",
		"en":      "commented on your post: %2$@",
	},
	"DISCOVER_REPLY": {
		"zh-hans": "回复了你的评论",
		"zh-hant": "回覆了你的評論",
		"en":      "replied to your comment",
	},
	"DISCOVER_REPLY_TEXT": {
		"zh-hans": "回复了你的评论：%2$@",
		"zh-hant": "回覆了你的評論：%2$@",
		"en":      "replied to your comment: %2$@",
	},
	"DISCOVER_MENTION": {
		"zh-hans": "在帖子里提到了你",
		"zh-hant": "在帖子裡提到了你",
		"en":      "mentioned you in a post",
	},
	"DISCOVER_FOLLOW": {
		"zh-hans": "关注了你",
		"zh-hant": "關注了你",
		"en":      "followed you",
	},
	"DISCOVER_REPOST": {
		"zh-hans": "转发了你的帖子",
		"zh-hant": "轉發了你的帖子",
		"en":      "reposted your post",
	},
}
