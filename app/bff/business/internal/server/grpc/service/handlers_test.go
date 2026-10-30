package service

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/teamgram/proto/mtproto"
	rpcmd "github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/business/internal/svc"
	"github.com/teamgram/teamgram-server/app/bff/business/profile"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"

	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func withUser(userID int64) context.Context {
	md := &rpcmd.RpcMetadata{UserId: userID, PermAuthKeyId: userID + 10}
	buf, _ := proto.Marshal(md)
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"rpc_metadata", base64.StdEncoding.EncodeToString(buf),
	))
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return status.Convert(err).Message()
}

func newSvc() *Service {
	return New(&svc.ServiceContext{Book: profile.New()})
}

func mustEncode(t *testing.T, obj interface {
	Encode(*mtproto.EncodeBuf, int32) error
}) {
	t.Helper()
	buf := mtproto.NewEncodeBuf(1024)
	if err := obj.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
}

func shortcutID(t *testing.T, user int64) int32 {
	t.Helper()
	msg := mtproto.MakeTLMessage(&mtproto.Message{Message: "hello there"}).To_Message()
	shortcut := mtproto.MakeTLInputQuickReplyShortcut(&mtproto.InputQuickReplyShortcut{Shortcut: "hello"}).To_InputQuickReplyShortcut()
	ups, err := chatbox.Shared().CaptureQuickReply(context.Background(), user, mtproto.MakeUserPeerUtil(user), shortcut, []*mtproto.Message{msg})
	if err != nil {
		t.Fatal(err)
	}
	for _, up := range ups.GetUpdates() {
		if q := up.GetQuickReply(); q != nil && q.GetShortcutId() != 0 {
			return q.GetShortcutId()
		}
	}
	t.Fatal("no shortcut id")
	return 0
}

func TestBusinessRequiresAuth(t *testing.T) {
	svc := newSvc()
	if _, err := svc.AccountGetBusinessChatLinks(context.Background(), &mtproto.TLAccountGetBusinessChatLinks{}); errText(err) != "AUTH_KEY_UNREGISTERED" {
		t.Fatal(err)
	}
}

func TestBusinessProfileAndLinks(t *testing.T) {
	svc := newSvc()
	ctx := withUser(880011)
	hours := mtproto.MakeTLBusinessWorkHours(&mtproto.BusinessWorkHours{
		TimezoneId: "UTC",
		WeeklyOpen: []*mtproto.BusinessWeeklyOpen{
			mtproto.MakeTLBusinessWeeklyOpen(&mtproto.BusinessWeeklyOpen{StartMinute: 9 * 60, EndMinute: 17 * 60}).To_BusinessWeeklyOpen(),
		},
	}).To_BusinessWorkHours()
	if _, err := svc.AccountUpdateBusinessWorkHours(ctx, &mtproto.TLAccountUpdateBusinessWorkHours{BusinessWorkHours: hours}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AccountUpdateBusinessWorkHours(ctx, &mtproto.TLAccountUpdateBusinessWorkHours{
		BusinessWorkHours: mtproto.MakeTLBusinessWorkHours(&mtproto.BusinessWorkHours{TimezoneId: "Not/AZone", WeeklyOpen: hours.WeeklyOpen}).To_BusinessWorkHours(),
	}); errText(err) != "TIMEZONE_INVALID" {
		t.Fatal(err)
	}
	if _, err := svc.AccountUpdateBusinessWorkHours(ctx, &mtproto.TLAccountUpdateBusinessWorkHours{
		BusinessWorkHours: mtproto.MakeTLBusinessWorkHours(&mtproto.BusinessWorkHours{
			TimezoneId: "UTC",
			WeeklyOpen: []*mtproto.BusinessWeeklyOpen{
				mtproto.MakeTLBusinessWeeklyOpen(&mtproto.BusinessWeeklyOpen{StartMinute: 10, EndMinute: 5}).To_BusinessWeeklyOpen(),
			},
		}).To_BusinessWorkHours(),
	}); errText(err) != "BUSINESS_WORK_HOURS_PERIOD_INVALID" {
		t.Fatal(err)
	}

	if _, err := svc.AccountUpdateBusinessLocation(ctx, &mtproto.TLAccountUpdateBusinessLocation{
		GeoPoint: mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{Lat: 40.7, Long: -74}).To_InputGeoPoint(),
		Address:  wrapperspb.String("1 Main"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AccountUpdateBusinessLocation(ctx, &mtproto.TLAccountUpdateBusinessLocation{
		GeoPoint: mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{Lat: 120, Long: 0}).To_InputGeoPoint(),
		Address:  wrapperspb.String("x"),
	}); errText(err) != "GEO_POINT_INVALID" {
		t.Fatal(err)
	}

	sid := shortcutID(t, 880011)
	if _, err := svc.AccountUpdateBusinessGreetingMessage(ctx, &mtproto.TLAccountUpdateBusinessGreetingMessage{
		Message: mtproto.MakeTLInputBusinessGreetingMessage(&mtproto.InputBusinessGreetingMessage{
			ShortcutId:     sid,
			NoActivityDays: 7,
			Recipients:     mtproto.MakeTLInputBusinessRecipients(&mtproto.InputBusinessRecipients{Contacts: true}).To_InputBusinessRecipients(),
		}).To_InputBusinessGreetingMessage(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AccountUpdateBusinessGreetingMessage(ctx, &mtproto.TLAccountUpdateBusinessGreetingMessage{
		Message: mtproto.MakeTLInputBusinessGreetingMessage(&mtproto.InputBusinessGreetingMessage{
			ShortcutId: 99,
			Recipients: mtproto.MakeTLInputBusinessRecipients(&mtproto.InputBusinessRecipients{}).To_InputBusinessRecipients(),
		}).To_InputBusinessGreetingMessage(),
	}); errText(err) != "SHORTCUT_INVALID" {
		t.Fatal(err)
	}
	if _, err := svc.AccountUpdateBusinessAwayMessage(ctx, &mtproto.TLAccountUpdateBusinessAwayMessage{
		Message: mtproto.MakeTLInputBusinessAwayMessage(&mtproto.InputBusinessAwayMessage{
			OfflineOnly: true,
			ShortcutId:  sid,
			Schedule:    mtproto.MakeTLBusinessAwayMessageScheduleAlways(nil).To_BusinessAwayMessageSchedule(),
			Recipients:  mtproto.MakeTLInputBusinessRecipients(&mtproto.InputBusinessRecipients{NewChats: true}).To_InputBusinessRecipients(),
		}).To_InputBusinessAwayMessage(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AccountUpdateBusinessIntro(ctx, &mtproto.TLAccountUpdateBusinessIntro{
		Intro: mtproto.MakeTLInputBusinessIntro(&mtproto.InputBusinessIntro{Title: "Shop", Description: "Open daily"}).To_InputBusinessIntro(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AccountUpdateBusinessIntro(ctx, &mtproto.TLAccountUpdateBusinessIntro{
		Intro: mtproto.MakeTLInputBusinessIntro(&mtproto.InputBusinessIntro{
			Title:   "Shop",
			Sticker: mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: 5, AccessHash: 6}).To_InputDocument(),
		}).To_InputBusinessIntro(),
	}); errText(err) != "STICKER_DOCUMENT_INVALID" {
		t.Fatal(err)
	}

	card := svc.svcCtx.Book.Card(880011)
	if card == nil || card.WorkHours.GetTimezoneId() != "UTC" || card.Location.GetAddress() != "1 Main" || card.Intro.GetTitle() != "Shop" {
		t.Fatalf("card %#v", card)
	}
	if card.Greeting.GetShortcutId() != sid || !card.Away.GetOfflineOnly() {
		t.Fatalf("messages %#v %#v", card.Greeting, card.Away)
	}

	link, err := svc.AccountCreateBusinessChatLink(ctx, &mtproto.TLAccountCreateBusinessChatLink{
		Link: mtproto.MakeTLInputBusinessChatLink(&mtproto.InputBusinessChatLink{
			Message: "ask us",
			Title:   wrapperspb.String("Desk"),
		}).To_InputBusinessChatLink(),
	})
	if err != nil {
		t.Fatal(err)
	}
	mustEncode(t, link)
	slug := link.GetLink()[len("https://t.me/m/"):]
	edited, err := svc.AccountEditBusinessChatLink(ctx, &mtproto.TLAccountEditBusinessChatLink{
		Slug: slug,
		Link: mtproto.MakeTLInputBusinessChatLink(&mtproto.InputBusinessChatLink{Message: "ask later"}).To_InputBusinessChatLink(),
	})
	if err != nil || edited.GetMessage() != "ask later" || edited.GetViews() != 0 {
		t.Fatalf("edit %#v %v", edited, err)
	}
	resolved, err := svc.AccountResolveBusinessChatLink(ctx, &mtproto.TLAccountResolveBusinessChatLink{Slug: slug})
	if err != nil || resolved.GetPeer().GetUserId() != 880011 || resolved.GetMessage() != "ask later" {
		t.Fatalf("resolve %#v %v", resolved, err)
	}
	mustEncode(t, resolved)
	links, err := svc.AccountGetBusinessChatLinks(ctx, &mtproto.TLAccountGetBusinessChatLinks{})
	if err != nil || len(links.GetLinks()) != 1 || links.GetLinks()[0].GetViews() != 1 {
		t.Fatalf("links %#v %v", links, err)
	}
	if _, err := svc.AccountDeleteBusinessChatLink(ctx, &mtproto.TLAccountDeleteBusinessChatLink{Slug: slug}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AccountResolveBusinessChatLink(ctx, &mtproto.TLAccountResolveBusinessChatLink{Slug: slug}); errText(err) != "SLUG_INVALID" {
		t.Fatal(err)
	}
	if _, err := svc.AccountUpdateBusinessWorkHours(ctx, &mtproto.TLAccountUpdateBusinessWorkHours{}); err != nil {
		t.Fatal(err)
	}
	if svc.svcCtx.Book.Card(880011).WorkHours != nil {
		t.Fatal("hours should clear")
	}
}
