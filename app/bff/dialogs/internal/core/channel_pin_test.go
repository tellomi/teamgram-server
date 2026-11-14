package core

import (
	"sort"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

func TestPinnedChannelDialogSortsFirst(t *testing.T) {
	channelID := int64(50)
	userID := int64(7)
	channelRow := dialog.MakeTLDialogExt(&dialog.DialogExt{
		Order: int64(100),
		Date:  100,
		Dialog: mtproto.MakeTLDialog(&mtproto.Dialog{
			Peer:       mtproto.MakePeerChannel(channelID),
			TopMessage: 4,
		}).To_Dialog(),
	}).To_DialogExt()
	userRow := dialog.MakeTLDialogExt(&dialog.DialogExt{
		Order: int64(500),
		Date:  500,
		Dialog: mtproto.MakeTLDialog(&mtproto.Dialog{
			Peer:       mtproto.MakePeerUser(userID),
			TopMessage: 9,
		}).To_Dialog(),
	}).To_DialogExt()
	pinned := dialog.MakeTLDialogExt(&dialog.DialogExt{
		Order: int64(9 << 32),
		Dialog: mtproto.MakeTLDialog(&mtproto.Dialog{
			Pinned: true,
			Peer:   mtproto.MakePeerChannel(channelID),
		}).To_Dialog(),
	}).To_DialogExt()

	applyChannelDialogRows(dialog.DialogExtList{channelRow}, dialog.DialogExtList{pinned})
	if !channelRow.GetDialog().GetPinned() || channelRow.GetOrder() != pinned.GetOrder() {
		t.Fatalf("pin was not copied: pinned=%v order=%d", channelRow.GetDialog().GetPinned(), channelRow.GetOrder())
	}

	list := dialog.DialogExtList{userRow, channelRow}
	sort.Sort(sort.Reverse(list))
	if list[0].GetDialog().GetPeer().GetChannelId() != channelID {
		t.Fatalf("first dialog peer = %#v", list[0].GetDialog().GetPeer())
	}
}

func TestPinnedChannelOrderFollowsPinRank(t *testing.T) {
	older := channelDialog(1, 1, false, 0)
	newer := channelDialog(2, 1, false, 0)
	userRow := channelDialog(0, 900, false, 0)
	userRow.Dialog.Peer = mtproto.MakePeerUser(7)
	applyChannelDialogRows(dialog.DialogExtList{older, newer}, dialog.DialogExtList{
		channelDialog(1, 0, true, 100),
		channelDialog(2, 0, true, 200),
	})
	list := dialog.DialogExtList{userRow, older, newer}
	sort.Sort(sort.Reverse(list))
	if list[0].GetDialog().GetPeer().GetChannelId() != 2 || list[1].GetDialog().GetPeer().GetChannelId() != 1 {
		t.Fatalf("order %#v %#v", list[0].GetDialog().GetPeer(), list[1].GetDialog().GetPeer())
	}
	kept := withoutPinnedDialogs(dialog.DialogExtList{older, newer, userRow})
	if len(kept) != 1 || kept[0].GetDialog().GetPeer().GetUserId() != 7 {
		t.Fatalf("exclude_pinned kept %#v", kept)
	}
}

func TestChannelLiveTopReplacesStalePinRow(t *testing.T) {
	stored := channelDialog(50, 9, true, 9<<32)
	stored.Dialog.TopMessage = 9
	live := channelDialog(50, 10, false, 10)
	live.Dialog.TopMessage = 4
	applyChannelLiveRows(dialog.DialogExtList{stored}, dialog.DialogExtList{live})
	if stored.GetDialog().GetTopMessage() != 4 || !stored.GetDialog().GetPinned() || stored.GetOrder() != 9<<32 {
		t.Fatalf("top %d pinned %v order %d", stored.GetDialog().GetTopMessage(), stored.GetDialog().GetPinned(), stored.GetOrder())
	}
	if stored.GetDate() != 10 {
		t.Fatalf("date %d", stored.GetDate())
	}
}

func channelDialog(channelID, date int64, pinned bool, order int64) *dialog.DialogExt {
	if !pinned {
		order = date
	}
	return dialog.MakeTLDialogExt(&dialog.DialogExt{
		Order: order,
		Date:  date,
		Dialog: mtproto.MakeTLDialog(&mtproto.Dialog{
			Pinned:     pinned,
			Peer:       mtproto.MakePeerChannel(channelID),
			TopMessage: int32(date),
		}).To_Dialog(),
	}).To_DialogExt()
}
