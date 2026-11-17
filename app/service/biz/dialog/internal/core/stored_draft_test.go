package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"
	"github.com/zeromicro/go-zero/core/jsonx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// draftJSON is the column value dialog.saveDraftMessage writes.
func draftJSON(t *testing.T, draft *mtproto.DraftMessage) string {
	t.Helper()
	raw, err := jsonx.Marshal(draft)
	if err != nil {
		t.Fatalf("marshal draft: %v", err)
	}
	return string(raw)
}

func TestAppendStoredDraftRestoresSavedPrivateDraft(t *testing.T) {
	saved := mtproto.MakeTLDraftMessage(&mtproto.DraftMessage{
		NoWebpage:    true,
		Message:      "hello draft",
		Date_INT32:   1_700_000_000,
		ReplyToMsgId: &wrapperspb.Int32Value{Value: 42},
		Entities: []*mtproto.MessageEntity{
			mtproto.MakeTLMessageEntityBold(&mtproto.MessageEntity{
				Offset: 0,
				Length: 5,
			}).To_MessageEntity(),
		},
	}).To_DraftMessage()

	row := &dataobject.DialogsDO{
		UserId:           10,
		PeerType:         mtproto.PEER_USER,
		PeerId:           20,
		DraftMessageData: draftJSON(t, saved),
	}

	got, err := appendStoredDraft(nil, row)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("drafts %d, want the saved private draft", len(got))
	}
	item := got[0]
	if item.GetPeer().GetUserId() != 20 || item.GetPeer().GetPredicateName() != mtproto.Predicate_peerUser {
		t.Fatalf("peer %+v", item.GetPeer())
	}
	draft := item.GetDraft()
	if draft.GetPredicateName() != mtproto.Predicate_draftMessage {
		t.Fatalf("predicate %q", draft.GetPredicateName())
	}
	if draft.GetMessage() != "hello draft" || !draft.GetNoWebpage() || draft.GetDate_INT32() != 1_700_000_000 {
		t.Fatalf("draft %+v", draft)
	}
	if draft.GetReplyToMsgId().GetValue() != 42 {
		t.Fatalf("reply_to_msg_id %v", draft.GetReplyToMsgId())
	}
	if len(draft.GetEntities()) != 1 || draft.GetEntities()[0].GetLength() != 5 {
		t.Fatalf("entities %+v", draft.GetEntities())
	}
}

func TestAppendStoredDraftKeepsRealDraftsAndSkipsEmptyColumns(t *testing.T) {
	text := mtproto.MakeTLDraftMessage(&mtproto.DraftMessage{
		Message:    "keep me",
		Date_INT32: 11,
	}).To_DraftMessage()
	empty := mtproto.MakeTLDraftMessageEmpty(&mtproto.DraftMessage{
		Date_FLAGINT32: mtproto.MakeFlagsInt32(22),
	}).To_DraftMessage()
	media := mtproto.MakeTLDraftMessage(&mtproto.DraftMessage{
		Date_INT32: 33,
		Media:      mtproto.MakeTLInputMediaEmpty(&mtproto.InputMedia{}).To_InputMedia(),
	}).To_DraftMessage()

	rows := []*dataobject.DialogsDO{
		{PeerType: mtproto.PEER_USER, PeerId: 1, DraftMessageData: ""},
		{PeerType: mtproto.PEER_USER, PeerId: 2, DraftMessageData: "null"},
		{PeerType: mtproto.PEER_CHAT, PeerId: 3, DraftMessageData: draftJSON(t, text)},
		{PeerType: mtproto.PEER_USER, PeerId: 4, DraftMessageData: "  null  "},
		{PeerType: mtproto.PEER_CHANNEL, PeerId: 5, DraftMessageData: draftJSON(t, empty)},
		nil,
		{PeerType: mtproto.PEER_USER, PeerId: 6, DraftMessageData: "{}"},
		{PeerType: mtproto.PEER_USER, PeerId: 7, DraftMessageData: draftJSON(t, media)},
	}

	var peers []*mtproto.Peer
	var drafts []*mtproto.DraftMessage
	for _, row := range rows {
		next, err := appendStoredDraft(nil, row)
		if err != nil {
			t.Fatal(err)
		}
		if len(next) == 0 {
			continue
		}
		peers = append(peers, next[0].GetPeer())
		drafts = append(drafts, next[0].GetDraft())
	}

	if len(peers) != 3 {
		t.Fatalf("kept %d drafts, want the text, the cleared draft, and the media draft", len(peers))
	}
	if peers[0].GetChatId() != 3 || peers[0].GetPredicateName() != mtproto.Predicate_peerChat {
		t.Fatalf("text peer %+v", peers[0])
	}
	if drafts[0].GetMessage() != "keep me" || drafts[0].GetPredicateName() != mtproto.Predicate_draftMessage {
		t.Fatalf("text draft %+v", drafts[0])
	}
	if peers[1].GetChannelId() != 5 || peers[1].GetPredicateName() != mtproto.Predicate_peerChannel {
		t.Fatalf("empty peer %+v", peers[1])
	}
	if drafts[1].GetPredicateName() != mtproto.Predicate_draftMessageEmpty || drafts[1].GetDate_FLAGINT32().GetValue() != 22 {
		t.Fatalf("empty draft %+v", drafts[1])
	}
	if peers[2].GetUserId() != 7 || drafts[2].GetMedia() == nil || drafts[2].GetMessage() != "" {
		t.Fatalf("media peer %+v draft %+v", peers[2], drafts[2])
	}
}

func TestAppendStoredDraftReportsCorruptJSON(t *testing.T) {
	_, err := appendStoredDraft(nil, &dataobject.DialogsDO{
		PeerType:         mtproto.PEER_USER,
		PeerId:           9,
		DraftMessageData: "{",
	})
	if err == nil {
		t.Fatal("corrupt draft JSON was treated as an empty column")
	}
}
