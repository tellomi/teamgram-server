package svc

import (
	"context"
	"fmt"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// repliesUserClient is the block list and notify settings repliesPrivacy reads.
type repliesUserClient interface {
	UserIsBlockedByUser(ctx context.Context, in *user.TLUserIsBlockedByUser) (*mtproto.Bool, error)
	UserGetNotifySettings(ctx context.Context, in *user.TLUserGetNotifySettings) (*mtproto.PeerNotifySettings, error)
}

// deliverRepliesNotice is the channel hook installed by the msg process,
// which owns the user inbox. It does not scan channels for an author.
func deliverRepliesNotice(d *dao.Dao) func(context.Context, channel.RepliesNotice) error {
	return func(ctx context.Context, n channel.RepliesNotice) error {
		var users repliesUserClient
		if d != nil && d.UserClient != nil {
			users = d.UserClient
		}
		blocked, settings, err := repliesPrivacy(ctx, users, n)
		if err != nil {
			return err
		}
		return d.DeliverRepliesNotice(ctx, n, blocked, settings, int32(time.Now().Unix()))
	}
}

// repliesPrivacy reads whether the recipient has blocked the commenter, and
// the notify settings for the discussion group and for @replies. An error
// is returned so the caller retries and does not deliver on a failed check.
// A missing user client is not "not blocked".
func repliesPrivacy(ctx context.Context, users repliesUserClient, n channel.RepliesNotice) (bool, []*mtproto.PeerNotifySettings, error) {
	if users == nil || n.Recipient == 0 || n.Commenter == 0 {
		return false, nil, fmt.Errorf("replies privacy client is missing")
	}
	flag, err := users.UserIsBlockedByUser(ctx, &user.TLUserIsBlockedByUser{
		UserId:     n.Recipient,
		PeerUserId: n.Commenter,
	})
	if err != nil {
		return false, nil, err
	}
	var settings []*mtproto.PeerNotifySettings
	peers := []struct {
		kind int32
		id   int64
	}{
		{mtproto.PEER_CHANNEL, n.GroupID},
		{mtproto.PEER_USER, channel.RepliesUserID},
	}
	for _, peer := range peers {
		if peer.id == 0 {
			continue
		}
		st, err := users.UserGetNotifySettings(ctx, &user.TLUserGetNotifySettings{
			UserId:   n.Recipient,
			PeerType: peer.kind,
			PeerId:   peer.id,
		})
		if err != nil {
			return false, nil, err
		}
		if st != nil {
			settings = append(settings, st)
		}
	}
	return mtproto.FromBool(flag), settings, nil
}
