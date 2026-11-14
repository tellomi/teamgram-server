package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestChannelSendCarriesMessageID is ISSUE-S-CHAN-MSGID-001 (row 110).
// A supergroup and a channel send must return updateMessageID before
// updateNewChannelMessage. Private chats already do this.
func TestChannelSendCarriesMessageID(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c, _ := dialPhone(t, ctx, cfg, "9996610431", "ChanID")

	megaID, megaHash, err := c.CreateMegagroup(ctx, fmt.Sprintf("msgid-mega-%d", time.Now().Unix()))
	if err != nil {
		t.Fatal(err)
	}
	assertSendMessageID(t, ctx, c, inputPeerChannel(megaID, megaHash), 43101)

	broadID, broadHash := createBroadcast(t, ctx, c, fmt.Sprintf("msgid-ch-%d", time.Now().Unix()))
	assertSendMessageID(t, ctx, c, inputPeerChannel(broadID, broadHash), 43102)
}

// TestAdminLogAfterPhotoReturns is ISSUE-S-ADMINLOG-PHOTO-001 (row 159).
// Changing a channel photo used to leave channels.getAdminLog unable to
// encode its result, so the recent-actions page never opened.
func TestAdminLogAfterPhotoReturns(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c, _ := dialPhone(t, ctx, cfg, "9996610432", "ChanLog")
	id, hash := createBroadcast(t, ctx, c, fmt.Sprintf("log-photo-%d", time.Now().Unix()))
	channel := inputChannel(id, hash)
	if _, err := c.Invoke(ctx, &mtproto.TLChannelsEditPhoto{
		Constructor: mtproto.CRC32_channels_editPhoto,
		Channel:     channel,
		Photo:       mtproto.MakeTLInputChatPhotoEmpty(nil).To_InputChatPhoto(),
	}); err != nil {
		t.Fatal(err)
	}
	obj, err := c.Invoke(ctx, &mtproto.TLChannelsGetAdminLog{
		Constructor: mtproto.CRC32_channels_getAdminLog,
		Channel:     channel,
		Limit:       20,
	})
	if err != nil {
		t.Fatal(err)
	}
	events := adminLogEvents(t, obj)
	var saw bool
	for _, ev := range events {
		if ev.GetAction().GetPredicateName() != mtproto.Predicate_channelAdminLogEventActionChangePhoto {
			continue
		}
		saw = true
		if ev.GetAction().GetPrevPhoto() == nil || ev.GetAction().GetNewPhoto() == nil {
			t.Fatalf("photo event %#v", ev.GetAction())
		}
	}
	if !saw {
		t.Fatalf("no changePhoto event in %d", len(events))
	}
}

func assertSendMessageID(t *testing.T, ctx context.Context, c *Client, peer *mtproto.InputPeer, random int64) {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLMessagesSendMessage{
		Constructor: mtproto.CRC32_messages_sendMessage_fef48f62,
		Peer:        peer,
		Message:     fmt.Sprintf("msgid-%d", random),
		RandomId:    random,
	})
	if err != nil {
		t.Fatal(err)
	}
	ups := updatesOf(obj)
	var idUp, msgUp *mtproto.Update
	for _, u := range ups {
		switch u.GetPredicateName() {
		case mtproto.Predicate_updateMessageID:
			if idUp == nil {
				idUp = u
			}
		case mtproto.Predicate_updateNewChannelMessage:
			if msgUp == nil {
				msgUp = u
			}
		}
	}
	if idUp == nil || msgUp == nil || msgUp.GetMessage_MESSAGE() == nil {
		t.Fatalf("updates missing message id mapping: %d updates", len(ups))
	}
	if idUp.GetRandomId() != random || idUp.GetId_INT32() == 0 || idUp.GetId_INT32() != msgUp.GetMessage_MESSAGE().GetId() {
		t.Fatalf("updateMessageID random=%d id=%d message=%d", idUp.GetRandomId(), idUp.GetId_INT32(), msgUp.GetMessage_MESSAGE().GetId())
	}
	idAt, msgAt := -1, -1
	for i, u := range ups {
		if u == idUp {
			idAt = i
		}
		if u == msgUp {
			msgAt = i
		}
	}
	if idAt < 0 || msgAt < 0 || idAt > msgAt {
		t.Fatalf("updateMessageID at %d, updateNewChannelMessage at %d", idAt, msgAt)
	}
}

func adminLogEvents(t *testing.T, obj mtproto.TLObject) []*mtproto.ChannelAdminLogEvent {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.TLChannelsAdminLogResults:
		return v.GetEvents()
	case *mtproto.Channels_AdminLogResults:
		return v.GetEvents()
	default:
		t.Fatalf("getAdminLog %T", obj)
		return nil
	}
}
