// Package loginnotice is the one 777000 login message shared by phone,
// email, password, and QR login. One claim map covers every path, so a
// finished login does not send a second copy for the same auth key.
package loginnotice

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
)

// shanghaiTime is the clock shown in the notice. China has no DST.
var shanghaiTime = time.FixedZone("CST", 8*3600)

var (
	mu sync.Mutex
	at = map[string]int64{}
)

// Reset clears the idempotency map. Tests use it.
func Reset() {
	mu.Lock()
	at = map[string]int64{}
	mu.Unlock()
}

// Claim is false for the service user and for the same auth key inside two
// minutes. A repeat export or a second handler observing the same key does
// not send another message.
func Claim(userID, authKey int64, now time.Time) bool {
	if userID == 0 || userID == 777000 {
		return false
	}
	key := fmt.Sprintf("%d:%d", userID, authKey)
	nowUnix := now.Unix()
	mu.Lock()
	defer mu.Unlock()
	if prev, ok := at[key]; ok && nowUnix-prev < 120 {
		return false
	}
	at[key] = nowUnix
	return true
}

// Label is the lang pack ("android", "ios") used only when no device model
// was stored.
func Label(client, langpack string) string {
	client = strings.TrimSpace(client)
	if client != "" {
		return client
	}
	return strings.TrimSpace(langpack)
}

// Device prefers the initConnection device model over the lang pack.
func Device(model, client, langpack string) string {
	model = strings.TrimSpace(model)
	if model != "" {
		return model
	}
	return Label(client, langpack)
}

// Text is the Simplified Chinese 777000 message. It carries the device and
// Beijing minute. It does not carry a verification code, a token, or an auth key.
func Text(device string, when time.Time) (string, []*mtproto.MessageEntity) {
	device = strings.TrimSpace(device)
	if device == "" {
		device = "未知设备"
	}
	stamp := when.In(shanghaiTime).Format("2006-01-02 15:04")
	return mtproto.MakeTextAndMessageEntities([]mtproto.MessageBuildEntry{
		{
			Param:      "新设备登录提醒",
			EntityType: mtproto.Predicate_messageEntityBold,
		},
		{
			Text: fmt.Sprintf("\n你的账号于 %s（北京时间）在 %s 上登录。\n如果这是你本人的操作，可以忽略这条消息。如果不是，请到设置 > 设备中结束该会话。", stamp, device),
		},
	})
}

// Pusher is the msg service call that writes the 777000 outbox.
type Pusher interface {
	MsgPushUserMessage(ctx context.Context, in *msgpb.TLMsgPushUserMessage) (*mtproto.Bool, error)
}

// Push schedules one notice. A nil pusher, a failed claim, or the service
// user sends nothing. The body is built before the delay so a later clock
// change cannot move the Beijing minute.
func Push(p Pusher, userID, authKey int64, device string, when time.Time) {
	if p == nil || !Claim(userID, authKey, when) {
		return
	}
	time.AfterFunc(2*time.Second, func() {
		Send(context.Background(), p, userID, device, when)
	})
}

// Send writes the notice immediately. Push is the production caller.
func Send(ctx context.Context, p Pusher, userID int64, device string, when time.Time) {
	if p == nil || userID == 0 || userID == 777000 {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	text, entities := Text(device, when)
	message := mtproto.MakeTLMessage(&mtproto.Message{
		Out:      true,
		Date:     int32(when.Unix()),
		FromId:   mtproto.MakePeerUser(777000),
		PeerId:   mtproto.MakeTLPeerUser(&mtproto.Peer{UserId: userID}).To_Peer(),
		Message:  text,
		Entities: entities,
	}).To_Message()
	_, _ = p.MsgPushUserMessage(ctx, &msgpb.TLMsgPushUserMessage{
		UserId:    777000,
		AuthKeyId: 0,
		PeerType:  mtproto.PEER_USER,
		PeerId:    userID,
		PushType:  1,
		Message: msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
			NoWebpage:    false,
			Background:   false,
			RandomId:     rand.Int63(),
			Message:      message,
			ScheduleDate: nil,
		}).To_OutboxMessage(),
	})
}
