package reaction

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestChipPrivateGroupAndChannel(t *testing.T) {
	ctx := context.Background()
	st := NewMemory()

	cases := []struct {
		name      string
		key       Key
		boxes     []Box
		actor     int64
		viewerBox Box
	}{
		{
			name: "private",
			key:  Key{DialogID1: 1, DialogID2: 2, DialogMessageID: 100},
			boxes: []Box{
				{UserID: 1, PeerType: mtproto.PEER_USER, PeerID: 2, MsgID: 5},
				{UserID: 2, PeerType: mtproto.PEER_USER, PeerID: 1, MsgID: 8},
			},
			actor:     2,
			viewerBox: Box{UserID: 1, PeerType: mtproto.PEER_USER, PeerID: 2, MsgID: 5},
		},
		{
			name: "group",
			key:  Key{DialogID1: -int64(mtproto.PEER_CHAT), DialogID2: 40, DialogMessageID: 77},
			boxes: []Box{
				{UserID: 1, PeerType: mtproto.PEER_CHAT, PeerID: 40, MsgID: 3},
				{UserID: 2, PeerType: mtproto.PEER_CHAT, PeerID: 40, MsgID: 9},
			},
			actor:     1,
			viewerBox: Box{UserID: 2, PeerType: mtproto.PEER_CHAT, PeerID: 40, MsgID: 9},
		},
		{
			name: "channel",
			key:  ChannelKey(50, 4),
			boxes: []Box{
				{UserID: 1, PeerType: mtproto.PEER_CHANNEL, PeerID: 50, MsgID: 4},
				{UserID: 2, PeerType: mtproto.PEER_CHANNEL, PeerID: 50, MsgID: 4},
			},
			actor:     2,
			viewerBox: Box{UserID: 1, PeerType: mtproto.PEER_CHANNEL, PeerID: 50, MsgID: 4},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := st.Replace(ctx, tc.key, tc.actor, []string{"👍"}, false, tc.boxes)
			if err != nil || len(rows) != 1 {
				t.Fatalf("replace %#v %v", rows, err)
			}
			up := Update(Peer(tc.boxes[0].PeerType, tc.boxes[0].PeerID), tc.boxes[0].MsgID, Summary(rows, tc.actor))
			if err := roundTripUpdate(t, up, tc.boxes[0].MsgID); err != nil {
				t.Fatal(err)
			}
			msgs := []*mtproto.Message{{
				Id:     tc.viewerBox.MsgID,
				PeerId: Peer(tc.viewerBox.PeerType, tc.viewerBox.PeerID),
			}}
			boxes := []*mtproto.MessageBox{{
				UserId:          tc.viewerBox.UserID,
				PeerType:        tc.viewerBox.PeerType,
				PeerId:          tc.viewerBox.PeerID,
				MessageId:       tc.viewerBox.MsgID,
				DialogId1:       tc.key.DialogID1,
				DialogId2:       tc.key.DialogID2,
				DialogMessageId: tc.key.DialogMessageID,
			}}
			AttachBoxes(ctx, st, tc.viewerBox.UserID, boxes, msgs)
			if !chip(msgs[0], "👍") {
				t.Fatalf("history chip missing: %#v", msgs[0].GetReactions())
			}
			AttachMessages(ctx, st, tc.viewerBox.UserID, msgs)
			if !chip(msgs[0], "👍") {
				t.Fatal("difference chip missing")
			}
			rows, err = st.Replace(ctx, tc.key, tc.actor, nil, false, tc.boxes)
			if err != nil || len(rows) != 0 {
				t.Fatalf("remove %#v %v", rows, err)
			}
			cleared := Summary(rows, tc.actor)
			if len(cleared.GetResults()) != 0 || cleared.GetRecentReactions() == nil {
				t.Fatalf("cleared %#v", cleared)
			}
			msgs[0].Reactions = nil
			AttachBoxes(ctx, st, tc.viewerBox.UserID, boxes, msgs)
			if msgs[0].GetReactions() != nil {
				t.Fatal("chip remained after remove")
			}
		})
	}
}

func roundTripUpdate(t *testing.T, up *mtproto.Update, msgID int32) error {
	t.Helper()
	buf := mtproto.NewEncodeBuf(1 << 16)
	if err := up.Encode(buf, 232); err != nil {
		return err
	}
	dbuf := mtproto.NewDecodeBuf(buf.GetBuf())
	obj := dbuf.Object()
	if err := dbuf.GetError(); err != nil {
		return err
	}
	got, ok := obj.(*mtproto.TLUpdateMessageReactions)
	if !ok || got.GetMsgId_INT32() != msgID {
		t.Fatalf("decoded %T id %d", obj, msgID)
	}
	sum := got.GetReactions_MESSAGEREACTIONS()
	if sum == nil || len(sum.GetResults()) != 1 || sum.GetResults()[0].GetCount() != 1 {
		t.Fatalf("results %#v", sum)
	}
	rc := sum.GetResults()[0]
	if rc.GetChosenOrder() == nil || rc.GetReaction_REACTION().GetEmoticon() != "👍" {
		t.Fatalf("chosen %#v", rc)
	}
	if len(sum.GetRecentReactions()) != 1 || sum.GetRecentReactions()[0].GetReaction_REACTION().GetEmoticon() != "👍" {
		t.Fatalf("recent %#v", sum.GetRecentReactions())
	}
	return nil
}

func chip(m *mtproto.Message, emo string) bool {
	if m == nil || m.GetReactions() == nil {
		return false
	}
	for _, rc := range m.GetReactions().GetResults() {
		if rc.GetCount() < 1 {
			continue
		}
		if rc.GetReaction_STRING() == emo || rc.GetReaction_REACTION().GetEmoticon() == emo {
			return true
		}
	}
	return false
}
