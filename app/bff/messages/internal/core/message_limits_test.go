package core

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
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
}

func TestCaptionFollowsAdvertisedPolicy(t *testing.T) {
	t.Cleanup(func() { premium.Reload() })
	t.Setenv(premium.EnvEveryone, "true")
	premium.Reload()

	def, prem := premium.Advertised(premium.FeatureCaption)
	if def != premium.CaptionLength || prem != premium.CaptionLength {
		t.Fatalf("everyone advertised default %d premium %d", def, prem)
	}
	everyone := captionCore(false, 0)
	assertCaptionCap(t, everyone, def)

	t.Setenv(premium.EnvEveryone, "false")
	premium.Reload()
	def, prem = premium.Advertised(premium.FeatureCaption)
	if def != premium.CaptionLengthFree || prem != premium.CaptionLength {
		t.Fatalf("member advertised default %d premium %d", def, prem)
	}
	free := captionCore(false, 0)
	if free.senderMember() {
		t.Fatal("stored premium=0 is a member while everyone is off")
	}
	assertCaptionCap(t, free, def)
	paid := captionCore(true, 0)
	if !paid.senderMember() {
		t.Fatal("unexpired subscription is not a member while everyone is off")
	}
	assertCaptionCap(t, paid, prem)
	expired := captionCore(true, time.Now().Unix()-10)
	if expired.senderMember() || expired.captionLimit() != def {
		t.Fatalf("expired cap %d member %v", expired.captionLimit(), expired.senderMember())
	}
	if status.Convert(free.rejectOverlong(repeat("c", def+1), true)).Message() != "MEDIA_CAPTION_TOO_LONG" {
		t.Fatal("caption error")
	}
	if status.Convert(free.rejectOverlong(repeat("a", maxMessageUTF16+1), false)).Message() != "MESSAGE_TOO_LONG" {
		t.Fatal("message error")
	}
}

func assertCaptionCap(t *testing.T, c *MessagesCore, cap int) {
	t.Helper()
	if cap < 2 || c.captionLimit() != cap {
		t.Fatalf("cap %d sender %d", cap, c.captionLimit())
	}
	if c.captionLimitExceeded(repeat("c", cap)) || !c.captionLimitExceeded(repeat("c", cap+1)) {
		t.Fatal("ascii caption")
	}
	// 🙂 is two UTF-16 units. cap/2 of them fill the advertised cap.
	if cap%2 != 0 {
		t.Fatalf("caption cap %d is odd", cap)
	}
	if c.captionLimitExceeded(repeat("🙂", cap/2)) || !c.captionLimitExceeded(repeat("🙂", cap/2+1)) {
		t.Fatal("emoji caption")
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
	if !storedEditIsCaption(nil, nil, false) {
		t.Fatal("an unresolved stored message must use the caption cap")
	}
	if storedEditIsCaption(photo, empty, false) {
		t.Fatal("clearing media is a message even when the lookup missed")
	}
	if !storedEditIsCaption(photo, nil, true) || storedEditIsCaption(nil, nil, true) {
		t.Fatal("resolved stored media")
	}
}

func TestScheduledEditUsesStoredMedia(t *testing.T) {
	chatbox.ResetShared()
	t.Cleanup(chatbox.ResetShared)
	ctx := context.Background()
	peer := mtproto.MakePeerUtil(mtproto.PEER_USER, 9)
	photo := mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{}).To_MessageMedia()
	when := int32(time.Now().Add(time.Hour).Unix())
	ups, err := chatbox.Shared().Schedule(ctx, 7, peer, when, []*mtproto.Message{{
		Message: "cap",
		Media:   photo,
	}})
	if err != nil {
		t.Fatal(err)
	}
	id := ups.Updates[0].GetMessage_MESSAGE().GetId()
	c := &MessagesCore{ctx: ctx, MD: &metadata.RpcMetadata{UserId: 7}}
	media, ok := c.scheduledStoredMedia(peer, id)
	if !ok || !storedEditIsCaption(media, nil, ok) {
		t.Fatalf("stored scheduled media ok=%v %#v", ok, media)
	}
	_, ok = c.scheduledStoredMedia(peer, id+1)
	if ok || !storedEditIsCaption(nil, nil, ok) {
		t.Fatal("missing scheduled row must use the caption cap")
	}
}

func TestNoopEditKeepsResentEntities(t *testing.T) {
	ent := mtproto.MakeTLMessageEntityBold(&mtproto.MessageEntity{Offset: 0, Length: 4}).To_MessageEntity()
	before := mtproto.MakeTLMessage(&mtproto.Message{
		Message:  "text",
		Entities: []*mtproto.MessageEntity{ent},
	}).To_Message()
	same := &mtproto.TLMessagesEditMessage{
		Message:  wrapperspb.String("text"),
		Entities: []*mtproto.MessageEntity{ent},
	}
	out := stagePrivateEdit(before, same, nil)
	if !mtproto.MessageContentUnchanged(before, out) {
		t.Fatalf("resent entities were stripped: %#v", out.GetEntities())
	}
	omitted := &mtproto.TLMessagesEditMessage{Message: wrapperspb.String("text")}
	out = stagePrivateEdit(before, omitted, nil)
	if len(out.GetEntities()) != 0 || mtproto.MessageContentUnchanged(before, out) {
		t.Fatal("omitted entities should clear, matching a channel edit")
	}
	changed := &mtproto.TLMessagesEditMessage{
		Message:  wrapperspb.String("text2"),
		Entities: []*mtproto.MessageEntity{ent},
	}
	out = stagePrivateEdit(before, changed, nil)
	if out.GetMessage() != "text2" || len(out.GetEntities()) != 1 {
		t.Fatalf("changed text dropped resent entities: %#v", out)
	}
}

func TestBotKeyboardKeptWhenUserRecordIsBot(t *testing.T) {
	markup := mtproto.MakeTLReplyKeyboardMarkup(&mtproto.ReplyMarkup{
		Rows_VECTORKEYBOARDBUTTONROW: []*mtproto.KeyboardButtonRow{
			mtproto.MakeTLKeyboardButtonRow(&mtproto.KeyboardButtonRow{
				Buttons: []*mtproto.KeyboardButton{
					mtproto.MakeTLKeyboardButton(&mtproto.KeyboardButton{Text: "go"}).To_KeyboardButton(),
				},
			}).To_KeyboardButtonRow(),
		},
	}).To_ReplyMarkup()
	bot := &MessagesCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			UserClient: botFlagUsers{bot: true},
		}},
		MD: &metadata.RpcMetadata{UserId: 7},
	}
	if bot.MD.IsBot {
		t.Fatal("session metadata must not be the bot flag")
	}
	if got := bot.botReplyMarkup(markup); got == nil || got.GetPredicateName() != mtproto.Predicate_replyKeyboardMarkup {
		t.Fatal("bot keyboard dropped")
	}
	person := &MessagesCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			UserClient: botFlagUsers{bot: false},
		}},
		MD: &metadata.RpcMetadata{UserId: 8},
	}
	if person.botReplyMarkup(markup) != nil {
		t.Fatal("user keyboard kept")
	}
	edit := &mtproto.TLMessagesEditMessage{ReplyMarkup: markup}
	edit.ReplyMarkup = bot.botReplyMarkup(edit.ReplyMarkup)
	if edit.ReplyMarkup == nil {
		t.Fatal("bot edit dropped the keyboard")
	}
	edit.ReplyMarkup = person.botReplyMarkup(markup)
	if edit.ReplyMarkup != nil {
		t.Fatal("user edit kept the keyboard")
	}
}

type botFlagUsers struct {
	userclient.UserClient
	bot bool
}

func (u botFlagUsers) UserIsBot(context.Context, *userpb.TLUserIsBot) (*mtproto.Bool, error) {
	return mtproto.ToBool(u.bot), nil
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
