package core

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/premium"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestTextAndCaptionLimits(t *testing.T) {
	if textTooLong(repeat("a", 4096)) || !textTooLong(repeat("a", 4097)) {
		t.Fatal("message length")
	}
	// One emoji is two UTF-16 units. 2049 of them is past 4096.
	if !textTooLong(repeat("🙂", 2049)) {
		t.Fatal("emoji message length")
	}
	if captionTooLong(repeat("a", 1024), false) || !captionTooLong(repeat("a", 1025), false) {
		t.Fatal("caption")
	}
	if captionTooLong(repeat("a", 2048), true) || !captionTooLong(repeat("a", 2049), true) {
		t.Fatal("premium caption")
	}
	if captionTooLong(repeat("🙂", 512), false) || !captionTooLong(repeat("🙂", 513), false) {
		t.Fatal("emoji caption")
	}
}

func TestCaptionIgnoresEveryonePremiumSwitch(t *testing.T) {
	t.Cleanup(func() { premium.Reload() })
	t.Setenv(premium.EnvEveryone, "true")
	premium.Reload()

	plain := &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 7}}
	if !plain.Premium() {
		t.Fatal("TELLOMI_PREMIUM_EVERYONE should mark the account premium")
	}

	free := captionCore(false, 0)
	if free.senderPremium() {
		t.Fatal("stored premium=0 counted as a caption subscription")
	}
	if free.captionLimitExceeded(repeat("c", 1024)) || !free.captionLimitExceeded(repeat("c", 1025)) {
		t.Fatal("non-subscriber caption")
	}
	if free.captionLimitExceeded(repeat("🙂", 512)) || !free.captionLimitExceeded(repeat("🙂", 513)) {
		t.Fatal("non-subscriber emoji caption")
	}
	if status.Convert(free.rejectOverlong(repeat("c", 1025), true)).Message() != "MEDIA_CAPTION_TOO_LONG" {
		t.Fatal("caption error")
	}
	if status.Convert(free.rejectOverlong(repeat("a", 4097), false)).Message() != "MESSAGE_TOO_LONG" {
		t.Fatal("message error")
	}

	paid := captionCore(true, 0)
	if !paid.senderPremium() || paid.captionLimitExceeded(repeat("c", 2048)) || !paid.captionLimitExceeded(repeat("c", 2049)) {
		t.Fatal("paid caption")
	}
	expired := captionCore(true, time.Now().Unix()-10)
	if expired.senderPremium() || !expired.captionLimitExceeded(repeat("c", 1025)) {
		t.Fatal("expired caption")
	}
}

func TestEditCaptionFollowsMedia(t *testing.T) {
	photo := mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{}).To_MessageMedia()
	page := mtproto.MakeTLMessageMediaWebPage(&mtproto.MessageMedia{}).To_MessageMedia()
	uploaded := mtproto.MakeTLInputMediaUploadedPhoto(&mtproto.InputMedia{}).To_InputMedia()
	empty := mtproto.MakeTLInputMediaEmpty(&mtproto.InputMedia{}).To_InputMedia()
	web := mtproto.MakeTLInputMediaWebPage(&mtproto.InputMedia{}).To_InputMedia()
	if editIsCaption(nil, nil) || editIsCaption(page, nil) || editIsCaption(photo, empty) || editIsCaption(photo, web) {
		t.Fatal("text edits")
	}
	if !editIsCaption(photo, nil) || !editIsCaption(nil, uploaded) || !editIsCaption(page, uploaded) {
		t.Fatal("caption edits")
	}
}

func captionCore(paid bool, expire int64) *MessagesCore {
	return &MessagesCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			UserClient: captionUsers{premium: paid, expire: expire},
		}},
		MD: &metadata.RpcMetadata{UserId: 7},
	}
}

type captionUsers struct {
	userclient.UserClient
	premium bool
	expire  int64
}

func (u captionUsers) UserGetMutableUsers(context.Context, *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	row := &mtproto.UserData{Id: 7, Premium: u.premium}
	if u.expire != 0 {
		row.PremiumExpireDate = wrapperspb.Int64(u.expire)
	}
	return &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{{User: row}}}, nil
}

func repeat(s string, n int) string {
	out := make([]rune, 0, n*len([]rune(s)))
	one := []rune(s)
	for i := 0; i < n; i++ {
		out = append(out, one...)
	}
	return string(out)
}
