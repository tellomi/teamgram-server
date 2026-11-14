package service

import (
	"context"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	msg_client "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	status_client "github.com/teamgram/teamgram-server/app/service/status/client"
	"github.com/teamgram/teamgram-server/app/service/status/status"
)

// scheduleSync pushes updates to one user's sessions.
type scheduleSync interface {
	SyncPushUpdates(ctx context.Context, in *syncpb.TLSyncPushUpdates) (*mtproto.Void, error)
}

// OnlineFunc reports which of the given user ids currently have a session.
type OnlineFunc func(ctx context.Context, ids []int64) map[int64]bool

type scheduleSink struct {
	msg  msg_client.MsgClient
	sync scheduleSync
}

func (s scheduleSink) Send(ctx context.Context, owner int64, peer *mtproto.PeerUtil, msg *mtproto.Message) (int32, error) {
	if s.msg == nil || peer == nil || msg == nil {
		return 0, mtproto.ErrInternalServerError
	}
	ups, err := s.msg.MsgSendMessageV2(ctx, &msgpb.TLMsgSendMessageV2{
		UserId:   owner,
		PeerType: peer.PeerType,
		PeerId:   peer.PeerId,
		Message: []*msgpb.OutboxMessage{
			msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
				RandomId: chatboxRandom(),
				Message:  msg,
			}).To_OutboxMessage(),
		},
	})
	if err != nil {
		return 0, err
	}
	if ups == nil {
		return 0, mtproto.ErrInternalServerError
	}
	for _, u := range ups.Updates {
		if m := u.GetMessage_MESSAGE(); m != nil && m.GetId() != 0 {
			return m.GetId(), nil
		}
	}
	return 0, mtproto.ErrInternalServerError
}

func (s scheduleSink) Push(ctx context.Context, userID int64, ups *mtproto.Updates) error {
	if s.sync == nil || userID == 0 || ups == nil {
		return nil
	}
	_, err := s.sync.SyncPushUpdates(ctx, &syncpb.TLSyncPushUpdates{
		UserId:  userID,
		Updates: ups,
	})
	return err
}

func chatboxRandom() int64 {
	return time.Now().UnixNano()
}

var scheduleOnce sync.Once

// StartScheduleDispatch runs the due-message sender when
// TELLOMI_SCHEDULED_DISPATCH is enabled. The loop is process-wide.
// Messages stay in the chatbox doc store until it runs, including across
// restarts. online may be nil; send-when-online then waits.
func StartScheduleDispatch(msg msg_client.MsgClient, syncer scheduleSync, online OnlineFunc) {
	if !chatbox.DispatchEnabled() || msg == nil {
		return
	}
	scheduleOnce.Do(func() {
		sink := scheduleSink{msg: msg, sync: syncer}
		go func() {
			tick := time.NewTicker(2 * time.Second)
			defer tick.Stop()
			for range tick.C {
				ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
				now := int32(time.Now().Unix())
				var who map[int64]bool
				if online != nil {
					ids, err := chatbox.Shared().OnlinePeers(ctx)
					if err == nil && len(ids) > 0 {
						who = online(ctx, ids)
					}
				}
				_, _ = chatbox.DispatchDue(ctx, chatbox.Shared(), now, who, sink)
				cancel()
			}
		}()
	})
}

// StatusOnline reads status.getUserOnlineSessions. A session whose expiry
// is still in the future counts as online.
func StatusOnline(st status_client.StatusClient) OnlineFunc {
	if st == nil {
		return nil
	}
	return func(ctx context.Context, ids []int64) map[int64]bool {
		now := time.Now().Unix()
		out := map[int64]bool{}
		for _, id := range ids {
			list, err := st.StatusGetUserOnlineSessions(ctx, &status.TLStatusGetUserOnlineSessions{UserId: id})
			if err != nil || list == nil {
				continue
			}
			for _, sess := range list.GetUserSessions() {
				if sess.GetExpired() > now {
					out[id] = true
					break
				}
			}
		}
		return out
	}
}
