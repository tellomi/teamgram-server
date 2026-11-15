package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestMegagroupMentionFlag is ISSUE-S-MENTION-MEGAGROUP-001 (row 76).
// Voice and ttl media_unread (row 296) is covered by
// TestMegagroupMentionFlagAndVoiceUnread, which sends the stored media
// through channel.Send and channels.readMessageContents.
// Phones 9996610451–452, code 11111, layer 232.
func TestMegagroupMentionFlag(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	owner, _ := dialPhone(t, ctx, cfg, "9996610451", "Own451")
	member, _ := dialPhone(t, ctx, cfg, "9996610452", "Mem452")
	ownerID, ownerHash, err := member.ImportPhone(ctx, "9996610451", "Own451")
	if err != nil {
		t.Fatal(err)
	}
	groupID, groupHash, err := owner.CreateMegagroup(ctx, "MentionRoom")
	if err != nil {
		t.Fatal(err)
	}
	memberID, memberHash, err := owner.ImportPhone(ctx, "9996610452", "Mem452")
	if err != nil {
		t.Fatal(err)
	}
	if err = owner.InviteToChannel(ctx, inputChannel(groupID, groupHash), inputUser(memberID, memberHash)); err != nil {
		t.Fatal(err)
	}
	memberCh := waitChannelAccess(t, ctx, member, groupID)
	id, err := member.SendText(ctx, inputPeerChannel(memberCh.GetChannelId(), memberCh.GetAccessHash()), "hi", []*mtproto.MessageEntity{
		mtproto.MakeTLMessageEntityMentionName(&mtproto.MessageEntity{
			Offset: 0, Length: 2, UserId_INT64: ownerID,
		}).To_MessageEntity(),
	})
	if err != nil || id == 0 {
		t.Fatal(err)
	}
	_ = ownerHash
	ownerCh := waitChannelAccess(t, ctx, owner, groupID)
	msg := historyMessage(t, ctx, owner, ownerCh, id)
	if !msg.GetMentioned() {
		t.Fatal("mentioned is false")
	}
	// ISSUE-S-PEERDIALOGS-MENTIONS-001: opening one chat calls
	// getPeerDialogs. That count has to match getDialogs, or the @ badge
	// disappears. messages.readMentions then clears both.
	var listN, peerN int32
	if err = waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		var listOK, peerOK bool
		var waitErr error
		listN, listOK, peerN, peerOK, waitErr = channelMentionCounts(ctx, owner, groupID, ownerCh.GetAccessHash())
		if waitErr != nil || !listOK || !peerOK {
			return false, waitErr
		}
		return listN > 0 && listN == peerN, nil
	}); err != nil {
		t.Fatalf("mentions getDialogs=%d getPeerDialogs=%d: %v", listN, peerN, err)
	}
	if _, err = owner.Invoke(ctx, &mtproto.TLMessagesReadMentions{
		Constructor: mtproto.CRC32_messages_readMentions_36e5bf4d,
		Peer:        inputPeerChannel(ownerCh.GetChannelId(), ownerCh.GetAccessHash()),
	}); err != nil {
		t.Fatal(err)
	}
	if err = waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		var listOK, peerOK bool
		var waitErr error
		listN, listOK, peerN, peerOK, waitErr = channelMentionCounts(ctx, owner, groupID, ownerCh.GetAccessHash())
		if waitErr != nil || !listOK || !peerOK {
			return false, waitErr
		}
		return listN == 0 && peerN == 0, nil
	}); err != nil {
		t.Fatalf("after readMentions getDialogs=%d getPeerDialogs=%d: %v", listN, peerN, err)
	}
}

func channelMentionCounts(ctx context.Context, c *Client, channelID, accessHash int64) (listN int32, listOK bool, peerN int32, peerOK bool, err error) {
	dialogs, err := c.Dialogs(ctx)
	if err != nil {
		return 0, false, 0, false, err
	}
	for _, d := range dialogs {
		if d.GetPeer().GetChannelId() == channelID {
			listN, listOK = d.GetUnreadMentionsCount(), true
			break
		}
	}
	obj, err := c.Invoke(ctx, &mtproto.TLMessagesGetPeerDialogs{
		Constructor: mtproto.CRC32_messages_getPeerDialogs,
		Peers: []*mtproto.InputDialogPeer{
			mtproto.MakeTLInputDialogPeer(&mtproto.InputDialogPeer{
				Peer: inputPeerChannel(channelID, accessHash),
			}).To_InputDialogPeer(),
		},
	})
	if err != nil {
		return listN, listOK, 0, false, err
	}
	for _, d := range dialogsOf(obj) {
		if d.GetPeer().GetChannelId() == channelID {
			peerN, peerOK = d.GetUnreadMentionsCount(), true
			break
		}
	}
	return listN, listOK, peerN, peerOK, nil
}

func waitChannelAccess(t *testing.T, ctx context.Context, c *Client, channelID int64) *mtproto.InputChannel {
	t.Helper()
	var hash int64
	if err := waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		got, ok, err := c.ChatAccess(ctx, channelID)
		if err != nil || !ok || got == 0 {
			return false, err
		}
		hash = got
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	return inputChannel(channelID, hash)
}

func historyMessage(t *testing.T, ctx context.Context, c *Client, channel *mtproto.InputChannel, id int32) *mtproto.Message {
	t.Helper()
	var found *mtproto.Message
	err := waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		msgs, err := c.History(ctx, inputPeerChannel(channel.GetChannelId(), channel.GetAccessHash()), 20)
		if err != nil {
			return false, err
		}
		for _, m := range msgs {
			if m.GetId() == id {
				found = m
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil || found == nil {
		t.Fatalf("history %d: %v", id, err)
	}
	return found
}
