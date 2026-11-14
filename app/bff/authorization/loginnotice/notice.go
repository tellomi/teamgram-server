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
	"github.com/teamgram/teamgram-server/pkg/usercopy"
)

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
// TextLang follows the client's language and falls back to English.
func Text(device string, when time.Time) (string, []*mtproto.MessageEntity) {
	return TextLang("zh-hans", device, when)
}

// TextLang is Text in the client's language. Empty lang is English.
func TextLang(lang, device string, when time.Time) (string, []*mtproto.MessageEntity) {
	return usercopy.LoginAlert(lang, device, when)
}

// Pusher is the msg service call that writes the 777000 outbox.
type Pusher interface {
	MsgPushUserMessage(ctx context.Context, in *msgpb.TLMsgPushUserMessage) (*mtproto.Bool, error)
}

// Push schedules one notice. A nil pusher, a failed claim, or the service
// user sends nothing. The body is built before the delay so a later clock
// change cannot move the Beijing minute.
func Push(p Pusher, userID, authKey int64, lang, device string, when time.Time) {
	if p == nil || !Claim(userID, authKey, when) {
		return
	}
	time.AfterFunc(2*time.Second, func() {
		Send(context.Background(), p, userID, lang, device, when)
	})
}

// Send writes the notice immediately. Push is the production caller.
// lang is the client's initConnection language. Empty is English.
func Send(ctx context.Context, p Pusher, userID int64, lang, device string, when time.Time) {
	if p == nil || userID == 0 || userID == 777000 {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	text, entities := TextLang(lang, device, when)
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
