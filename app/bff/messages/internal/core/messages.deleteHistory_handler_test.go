package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	msgclient "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	dialogclient "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestDeleteHistoryPassesDateRangeAndKeepsTheDialog(t *testing.T) {
	msgFake := &deleteHistoryMsg{offset: 7}
	dlg := &deleteHistoryDialog{}
	c := deleteHistoryCore(msgFake, dlg)
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 9}).To_InputPeer()
	got, err := c.MessagesDeleteHistory(&mtproto.TLMessagesDeleteHistory{
		Peer:    peer,
		Revoke:  true,
		MaxId:   40,
		MinDate: &wrapperspb.Int32Value{Value: 10},
		MaxDate: &wrapperspb.Int32Value{Value: 30},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetOffset() != 7 || got.GetPtsCount() != 2 {
		t.Fatalf("%#v", got)
	}
	if msgFake.got == nil || msgFake.got.MinDate != 10 || msgFake.got.MaxDate != 30 || msgFake.got.MaxId != 40 || !msgFake.got.Revoke {
		t.Fatalf("forwarded %#v", msgFake.got)
	}
	if dlg.calls != 0 {
		t.Fatalf("date range removed the dialog %d times", dlg.calls)
	}
}

func TestDeleteHistoryRemovesDialogOnlyWhenTheLastSliceFinishes(t *testing.T) {
	msgFake := &deleteHistoryMsg{offset: 4}
	dlg := &deleteHistoryDialog{}
	c := deleteHistoryCore(msgFake, dlg)
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 9}).To_InputPeer()
	if _, err := c.MessagesDeleteHistory(&mtproto.TLMessagesDeleteHistory{Peer: peer, Revoke: true}); err != nil {
		t.Fatal(err)
	}
	if dlg.calls != 0 {
		t.Fatal("dialog removed before offset reached 0")
	}
	msgFake.offset = 0
	if _, err := c.MessagesDeleteHistory(&mtproto.TLMessagesDeleteHistory{Peer: peer, Revoke: true}); err != nil {
		t.Fatal(err)
	}
	if dlg.calls != 2 {
		t.Fatalf("dialog deletes %d, want both sides", dlg.calls)
	}
}

func TestDeleteHistoryDateOnChatIsUnsupported(t *testing.T) {
	c := deleteHistoryCore(&deleteHistoryMsg{}, &deleteHistoryDialog{})
	peer := mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 3}).To_InputPeer()
	_, err := c.MessagesDeleteHistory(&mtproto.TLMessagesDeleteHistory{
		Peer:    peer,
		MinDate: &wrapperspb.Int32Value{Value: 1},
	})
	if err != mtproto.ErrChatRevokeDateUnsupported {
		t.Fatalf("%v", err)
	}
}

func TestDeleteHistoryRejectsInvertedDates(t *testing.T) {
	_, _, _, err := deleteHistoryDates(&mtproto.TLMessagesDeleteHistory{
		MinDate: &wrapperspb.Int32Value{Value: 20},
		MaxDate: &wrapperspb.Int32Value{Value: 10},
	})
	if err != mtproto.ErrMaxDateInvalid {
		t.Fatalf("%v", err)
	}
	_, _, _, err = deleteHistoryDates(&mtproto.TLMessagesDeleteHistory{
		MinDate: &wrapperspb.Int32Value{Value: -1},
	})
	if err != mtproto.ErrMinDateInvalid {
		t.Fatalf("%v", err)
	}
}

func deleteHistoryCore(msgs *deleteHistoryMsg, dlg *deleteHistoryDialog) *MessagesCore {
	return &MessagesCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			MsgClient:    msgs,
			DialogClient: dlg,
		}},
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 1, PermAuthKeyId: 8},
	}
}

type deleteHistoryMsg struct {
	msgclient.MsgClient
	got    *msg.TLMsgDeleteHistory
	offset int32
}

func (f *deleteHistoryMsg) MsgDeleteHistory(_ context.Context, in *msg.TLMsgDeleteHistory) (*mtproto.Messages_AffectedHistory, error) {
	f.got = in
	return mtproto.MakeTLMessagesAffectedHistory(&mtproto.Messages_AffectedHistory{
		Pts:      10,
		PtsCount: 2,
		Offset:   f.offset,
	}).To_Messages_AffectedHistory(), nil
}

type deleteHistoryDialog struct {
	dialogclient.DialogClient
	calls int
}

func (f *deleteHistoryDialog) DialogDeleteDialog(context.Context, *dialog.TLDialogDeleteDialog) (*mtproto.Bool, error) {
	f.calls++
	return mtproto.BoolTrue, nil
}
