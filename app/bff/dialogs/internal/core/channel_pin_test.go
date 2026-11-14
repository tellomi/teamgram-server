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
