package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestNotifyExceptionsFollowThePeerScope mutes one private chat, one
// supergroup and one broadcast channel, then reads account.getNotifyExceptions
// for each category. inputNotifyUsers is only the user. inputNotifyChats is
// only the supergroup. inputNotifyBroadcasts is only the broadcast channel.
// Phones 9996610801 and 9996610802, code 11111.
func TestNotifyExceptionsFollowThePeerScope(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(2 * time.Minute)
	defer cancel()
	owner, _ := dialPhone(t, ctx, cfg, "9996610801", "ExcOwn")
	_, _ = dialPhone(t, ctx, cfg, "9996610802", "ExcPeer")
	peerID, peerHash, err := owner.ImportPhone(ctx, "9996610802", "ExcPeer")
	if err != nil {
		t.Fatal(err)
	}
	megaID, megaHash, err := owner.CreateMegagroup(ctx, "ExcMega")
	if err != nil {
		t.Fatal(err)
	}
	created := invokeOK(t, owner, &mtproto.TLChannelsCreateChannel{
		Constructor: mtproto.CRC32_channels_createChannel_91006707,
		Broadcast:   true,
		Title:       "ExcFeed",
		About:       "",
	})
	var feedID, feedHash int64
	for _, chat := range chatsOf(created) {
		if chat != nil && chat.GetBroadcast() && !chat.GetMegagroup() && chat.GetTitle() == "ExcFeed" {
			feedID, feedHash = chat.GetId(), chatAccessHash(chat)
		}
	}
	if peerID == 0 || megaID == 0 || feedID == 0 {
		t.Fatalf("peers user=%d mega=%d feed=%d", peerID, megaID, feedID)
	}

	muteNotify(t, owner, mtproto.MakeTLInputNotifyPeer(&mtproto.InputNotifyPeer{
		Peer: inputPeerUser(peerID, peerHash),
	}).To_InputNotifyPeer())
	muteNotify(t, owner, mtproto.MakeTLInputNotifyPeer(&mtproto.InputNotifyPeer{
		Peer: inputPeerChannel(megaID, megaHash),
	}).To_InputNotifyPeer())
	muteNotify(t, owner, mtproto.MakeTLInputNotifyPeer(&mtproto.InputNotifyPeer{
		Peer: inputPeerChannel(feedID, feedHash),
	}).To_InputNotifyPeer())

	userKey := fmt.Sprintf("user:%d", peerID)
	megaKey := fmt.Sprintf("channel:%d", megaID)
	feedKey := fmt.Sprintf("channel:%d", feedID)
	all := notifyExceptionPeers(t, owner, nil)
	if !samePeerSet(all, userKey, megaKey, feedKey) {
		t.Fatalf("unset peer %v", all)
	}
	users := notifyExceptionPeers(t, owner, mtproto.MakeTLInputNotifyUsers(&mtproto.InputNotifyPeer{}).To_InputNotifyPeer())
	if !samePeerSet(users, userKey) {
		t.Fatalf("inputNotifyUsers %v", users)
	}
	chats := notifyExceptionPeers(t, owner, mtproto.MakeTLInputNotifyChats(&mtproto.InputNotifyPeer{}).To_InputNotifyPeer())
	if !samePeerSet(chats, megaKey) {
		t.Fatalf("inputNotifyChats %v", chats)
	}
	feeds := notifyExceptionPeers(t, owner, mtproto.MakeTLInputNotifyBroadcasts(&mtproto.InputNotifyPeer{}).To_InputNotifyPeer())
	if !samePeerSet(feeds, feedKey) {
		t.Fatalf("inputNotifyBroadcasts %v", feeds)
	}
}

func muteNotify(t *testing.T, c *Client, peer *mtproto.InputNotifyPeer) {
	t.Helper()
	invokeOK(t, c, &mtproto.TLAccountUpdateNotifySettings{
		Constructor: mtproto.CRC32_account_updateNotifySettings,
		Peer:        peer,
		Settings: mtproto.MakeTLInputPeerNotifySettings(&mtproto.InputPeerNotifySettings{
			MuteUntil: wrapperspb.Int32(2147483647),
		}).To_InputPeerNotifySettings(),
	})
}

func notifyExceptionPeers(t *testing.T, c *Client, peer *mtproto.InputNotifyPeer) map[string]bool {
	t.Helper()
	obj := invokeOK(t, c, &mtproto.TLAccountGetNotifyExceptions{
		Constructor: mtproto.CRC32_account_getNotifyExceptions,
		Peer:        peer,
	})
	got := map[string]bool{}
	ups := updatesOf(obj)
	if len(ups) == 0 && obj != nil {
		if _, empty := obj.(*mtproto.TLUpdates); !empty {
			if _, bare := obj.(*mtproto.Updates); !bare {
				t.Fatalf("getNotifyExceptions %T", obj)
			}
		}
	}
	for _, upd := range ups {
		if upd.GetPredicateName() != mtproto.Predicate_updateNotifySettings {
			t.Fatalf("update %s", upd.GetPredicateName())
		}
		if upd.GetNotifySettings().GetMuteUntil().GetValue() != 2147483647 {
			t.Fatalf("settings %+v", upd.GetNotifySettings())
		}
		p := upd.GetPeer_NOTIFYPEER().GetPeer()
		switch p.GetPredicateName() {
		case mtproto.Predicate_peerUser:
			got[fmt.Sprintf("user:%d", p.GetUserId())] = true
		case mtproto.Predicate_peerChat:
			got[fmt.Sprintf("chat:%d", p.GetChatId())] = true
		case mtproto.Predicate_peerChannel:
			got[fmt.Sprintf("channel:%d", p.GetChannelId())] = true
		default:
			t.Fatalf("notify peer %s", upd.GetPeer_NOTIFYPEER().GetPredicateName())
		}
	}
	return got
}

func samePeerSet(got map[string]bool, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	for _, key := range want {
		if !got[key] {
			return false
		}
	}
	return true
}
