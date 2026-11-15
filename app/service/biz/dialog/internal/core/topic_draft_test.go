package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/jsonx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestPeerWithTopicDraftKeepsTopicAndSkipsDialogShape(t *testing.T) {
	saved := mtproto.MakeTLDraftMessage(&mtproto.DraftMessage{
		Message: "in topic",
		ReplyTo: mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{
			TopMsgId: wrapperspb.Int32(15),
		}).To_InputReplyTo(),
	}).To_DraftMessage()
	raw, err := jsonx.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	item, err := peerWithTopicDraft(&dataobject.DialogTopicDraftDO{
		UserId:           10,
		PeerType:         mtproto.PEER_CHANNEL,
		PeerId:           77,
		TopMsgId:         15,
		DraftType:        2,
		DraftMessageData: string(raw),
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.GetPeer().GetChannelId() != 77 {
		t.Fatalf("peer %+v", item.GetPeer())
	}
	if item.GetDraft().GetMessage() != "in topic" || dialog.DraftTopicID(item.GetDraft()) != 15 {
		t.Fatalf("draft %+v", item.GetDraft())
	}

	none, err := peerWithTopicDraft(&dataobject.DialogTopicDraftDO{TopMsgId: 0, DraftType: 2, DraftMessageData: string(raw)})
	if err != nil || none != nil {
		t.Fatalf("top 0 row leaked: %+v %v", none, err)
	}
}

func TestPeerWithTopicDraftStampsTopWhenJSONHasNone(t *testing.T) {
	saved := mtproto.MakeTLDraftMessage(&mtproto.DraftMessage{Message: "legacy"}).To_DraftMessage()
	raw, err := jsonx.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	item, err := peerWithTopicDraft(&dataobject.DialogTopicDraftDO{
		PeerType:         mtproto.PEER_CHANNEL,
		PeerId:           3,
		TopMsgId:         8,
		DraftType:        2,
		DraftMessageData: string(raw),
	})
	if err != nil {
		t.Fatal(err)
	}
	if dialog.DraftTopicID(item.GetDraft()) != 8 || item.GetDraft().GetMessage() != "legacy" {
		t.Fatalf("%+v", item.GetDraft())
	}
}

func TestPeerWithTopicDraftReportsCorruptJSON(t *testing.T) {
	_, err := peerWithTopicDraft(&dataobject.DialogTopicDraftDO{
		PeerType:         mtproto.PEER_CHANNEL,
		PeerId:           3,
		TopMsgId:         8,
		DraftType:        2,
		DraftMessageData: "{",
	})
	if err == nil {
		t.Fatal("corrupt topic JSON was ignored")
	}
}

func TestEmptyTopicDraftCarriesOnlyThatTopic(t *testing.T) {
	draft := emptyTopicDraft(15)
	if !dialog.DraftIsEmpty(draft) || dialog.DraftTopicID(draft) != 15 {
		t.Fatalf("%+v", draft)
	}
	upd := dialog.DraftUpdate(mtproto.MakePeerChannel(1), draft)
	if upd.GetTopMsgId_FLAGINT32().GetValue() != 15 {
		t.Fatal(upd.GetTopMsgId_FLAGINT32())
	}
}
