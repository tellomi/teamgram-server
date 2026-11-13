package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestInviteLinkSheet is the iOS sheet that spun forever, and the 新链接
// screen that saved a name, 新成员审核, an expiry, and a usage limit and
// then showed 发生错误，请稍后再试. Both a supergroup and a basic group have
// to return a tell.cc link.
func TestInviteLinkSheet(t *testing.T) {
	a, b, _, bSelf := sharedPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	chID, chHash, err := a.CreateMegagroup(ctx, "e2e-invite-mega")
	if err != nil {
		t.Fatal(err)
	}
	channelPeer := inputPeerChannel(chID, chHash)
	primary := exportInvite(t, ctx, a, &mtproto.TLMessagesExportChatInvite{
		Constructor:           mtproto.CRC32_messages_exportChatInvite_a455de90,
		LegacyRevokePermanent: true,
		Peer:                  channelPeer,
	})
	if !strings.HasPrefix(primary.GetLink(), "https://tell.cc/+") || !primary.GetPermanent() {
		t.Fatalf("megagroup primary %+v", primary)
	}
	fullObj, err := a.Invoke(ctx, &mtproto.TLChannelsGetFullChannel{
		Constructor: mtproto.CRC32_channels_getFullChannel,
		Channel:     inputChannel(chID, chHash),
	})
	if err != nil {
		t.Fatal(err)
	}
	full := asChatFull(t, fullObj)
	if full.GetExportedInvite().GetLink() != primary.GetLink() {
		t.Fatalf("channels.getFullChannel exported_invite %+v", full.GetExportedInvite())
	}

	expire := int32(time.Now().Add(2 * time.Hour).Unix())
	created := exportInvite(t, ctx, a, &mtproto.TLMessagesExportChatInvite{
		Constructor:   mtproto.CRC32_messages_exportChatInvite_a455de90,
		RequestNeeded: true,
		Peer:          channelPeer,
		ExpireDate:    &wrapperspb.Int32Value{Value: expire},
		UsageLimit:    &wrapperspb.Int32Value{Value: 10},
		Title:         &wrapperspb.StringValue{Value: "周末局"},
	})
	if created.GetTitle().GetValue() != "周末局" || !created.GetRequestNeeded() || created.GetPermanent() ||
		created.GetExpireDate().GetValue() != expire || created.GetUsageLimit().GetValue() != 10 {
		t.Fatalf("new link %+v", created)
	}
	edited := editInvite(t, ctx, a, &mtproto.TLMessagesEditExportedChatInvite{
		Constructor:   mtproto.CRC32_messages_editExportedChatInvite,
		Peer:          channelPeer,
		Link:          created.GetLink(),
		ExpireDate:    &wrapperspb.Int32Value{Value: expire + 60},
		UsageLimit:    &wrapperspb.Int32Value{Value: 3},
		RequestNeeded: mtproto.BoolFalse,
		Title:         &wrapperspb.StringValue{Value: "改名"},
	})
	if edited.GetInvite().GetTitle().GetValue() != "改名" || edited.GetInvite().GetRequestNeeded() ||
		edited.GetInvite().GetUsageLimit().GetValue() != 3 {
		t.Fatalf("edited %+v", edited.GetInvite())
	}
	assertUnlimitedUsage(t, ctx, a, channelPeer)

	listObj, err := a.Invoke(ctx, &mtproto.TLMessagesGetExportedChatInvites{
		Constructor: mtproto.CRC32_messages_getExportedChatInvites,
		Peer:        channelPeer,
		AdminId:     inputUserSelf(),
		Limit:       20,
	})
	if err != nil {
		t.Fatal(err)
	}
	list := asInviteList(t, listObj)
	if list.GetCount() < 2 {
		t.Fatalf("list %+v", list)
	}

	hash := strings.TrimPrefix(primary.GetLink(), "https://tell.cc/+")
	previewObj, err := b.Invoke(ctx, &mtproto.TLMessagesCheckChatInvite{
		Constructor: mtproto.CRC32_messages_checkChatInvite,
		Hash:        hash,
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, ok := previewObj.(*mtproto.ChatInvite)
	if !ok {
		if tl, ok2 := previewObj.(*mtproto.TLChatInvite); ok2 {
			preview = tl.To_ChatInvite()
		}
	}
	if preview == nil || preview.GetTitle() == "" {
		t.Fatalf("check %T %+v", previewObj, previewObj)
	}
	if _, err := b.Invoke(ctx, &mtproto.TLMessagesImportChatInviteDE91436E{
		Constructor: mtproto.CRC32_messages_importChatInviteDE91436E,
		Hash:        hash,
	}); err != nil {
		t.Fatal(err)
	}
	_ = bSelf

	chatID, err := a.CreateChat(ctx, "e2e-invite-basic", inputUser(bSelf.ID, bSelf.AccessHash))
	if err != nil {
		t.Fatal(err)
	}
	basic := exportInvite(t, ctx, a, &mtproto.TLMessagesExportChatInvite{
		Constructor:           mtproto.CRC32_messages_exportChatInvite_a455de90,
		LegacyRevokePermanent: true,
		Peer:                  inputPeerChat(chatID),
	})
	if !strings.HasPrefix(basic.GetLink(), "https://tell.cc/+") || !basic.GetPermanent() {
		t.Fatalf("basic primary %+v", basic)
	}
	basicExtra := exportInvite(t, ctx, a, &mtproto.TLMessagesExportChatInvite{
		Constructor:   mtproto.CRC32_messages_exportChatInvite_a455de90,
		RequestNeeded: true,
		Peer:          inputPeerChat(chatID),
		ExpireDate:    &wrapperspb.Int32Value{Value: expire},
		UsageLimit:    &wrapperspb.Int32Value{Value: 10},
		Title:         &wrapperspb.StringValue{Value: "周末局"},
	})
	if basicExtra.GetTitle().GetValue() != "周末局" || !basicExtra.GetRequestNeeded() || basicExtra.GetUsageLimit().GetValue() != 10 {
		t.Fatalf("basic new link %+v", basicExtra)
	}
	assertUnlimitedUsage(t, ctx, a, inputPeerChat(chatID))
	basicFullObj, err := a.Invoke(ctx, &mtproto.TLMessagesGetFullChat{
		Constructor: mtproto.CRC32_messages_getFullChat,
		ChatId:      chatID,
	})
	if err != nil {
		t.Fatal(err)
	}
	basicFull := asChatFull(t, basicFullObj)
	if basicFull.GetExportedInvite().GetLink() != basic.GetLink() {
		t.Fatalf("messages.getFullChat exported_invite %+v", basicFull.GetExportedInvite())
	}
}

// assertUnlimitedUsage is the Android LinkEditActivity payload: the usage_limit
// flag is set and the value is 0. The reply must omit usage_limit. Editing a
// limited link to 0 does the same. Negative values and values above 99999
// are USAGE_LIMIT_INVALID.
func assertUnlimitedUsage(t *testing.T, ctx context.Context, c *Client, peer *mtproto.InputPeer) {
	t.Helper()
	open := exportInvite(t, ctx, c, &mtproto.TLMessagesExportChatInvite{
		Constructor: mtproto.CRC32_messages_exportChatInvite_a455de90,
		Peer:        peer,
		UsageLimit:  &wrapperspb.Int32Value{Value: 0},
		Title:       &wrapperspb.StringValue{Value: "不限"},
	})
	if open.GetUsageLimit() != nil || open.GetPermanent() || open.GetTitle().GetValue() != "不限" {
		t.Fatalf("unlimited export %+v", open)
	}
	limited := exportInvite(t, ctx, c, &mtproto.TLMessagesExportChatInvite{
		Constructor: mtproto.CRC32_messages_exportChatInvite_a455de90,
		Peer:        peer,
		UsageLimit:  &wrapperspb.Int32Value{Value: 99999},
	})
	if limited.GetUsageLimit().GetValue() != 99999 {
		t.Fatalf("max %+v", limited)
	}
	cleared := editInvite(t, ctx, c, &mtproto.TLMessagesEditExportedChatInvite{
		Constructor: mtproto.CRC32_messages_editExportedChatInvite,
		Peer:        peer,
		Link:        limited.GetLink(),
		UsageLimit:  &wrapperspb.Int32Value{Value: 0},
	})
	if cleared.GetInvite().GetUsageLimit() != nil {
		t.Fatalf("edit to unlimited %+v", cleared.GetInvite())
	}
	for _, n := range []int32{-1, 100000} {
		_, err := c.Invoke(ctx, &mtproto.TLMessagesExportChatInvite{
			Constructor: mtproto.CRC32_messages_exportChatInvite_a455de90,
			Peer:        peer,
			UsageLimit:  &wrapperspb.Int32Value{Value: n},
		})
		rpc, ok := asRPCError(err)
		if !ok || rpc.Code != 400 || rpc.Message != "USAGE_LIMIT_INVALID" {
			t.Fatalf("export usage %d: %v", n, err)
		}
		_, err = c.Invoke(ctx, &mtproto.TLMessagesEditExportedChatInvite{
			Constructor: mtproto.CRC32_messages_editExportedChatInvite,
			Peer:        peer,
			Link:        open.GetLink(),
			UsageLimit:  &wrapperspb.Int32Value{Value: n},
		})
		rpc, ok = asRPCError(err)
		if !ok || rpc.Code != 400 || rpc.Message != "USAGE_LIMIT_INVALID" {
			t.Fatalf("edit usage %d: %v", n, err)
		}
	}
}

func exportInvite(t *testing.T, ctx context.Context, c *Client, req *mtproto.TLMessagesExportChatInvite) *mtproto.ExportedChatInvite {
	t.Helper()
	obj, err := c.Invoke(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	switch v := obj.(type) {
	case *mtproto.TLChatInviteExported:
		return v.To_ExportedChatInvite()
	case *mtproto.ExportedChatInvite:
		return v
	default:
		t.Fatalf("export %T", obj)
		return nil
	}
}

func editInvite(t *testing.T, ctx context.Context, c *Client, req *mtproto.TLMessagesEditExportedChatInvite) *mtproto.Messages_ExportedChatInvite {
	t.Helper()
	obj, err := c.Invoke(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	switch v := obj.(type) {
	case *mtproto.TLMessagesExportedChatInvite:
		return v.To_Messages_ExportedChatInvite()
	case *mtproto.TLMessagesExportedChatInviteReplaced:
		return v.To_Messages_ExportedChatInvite()
	case *mtproto.Messages_ExportedChatInvite:
		return v
	default:
		t.Fatalf("edit %T", obj)
		return nil
	}
}

func asChatFull(t *testing.T, obj mtproto.TLObject) *mtproto.ChatFull {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.TLMessagesChatFull:
		return v.To_Messages_ChatFull().GetFullChat()
	case *mtproto.Messages_ChatFull:
		return v.GetFullChat()
	default:
		t.Fatalf("full %T", obj)
		return nil
	}
}

func asInviteList(t *testing.T, obj mtproto.TLObject) *mtproto.Messages_ExportedChatInvites {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.TLMessagesExportedChatInvites:
		return v.To_Messages_ExportedChatInvites()
	case *mtproto.Messages_ExportedChatInvites:
		return v
	default:
		t.Fatalf("list %T", obj)
		return nil
	}
}
