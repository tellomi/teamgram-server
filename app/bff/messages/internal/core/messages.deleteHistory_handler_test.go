package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	"github.com/teamgram/teamgram-server/app/messenger/msg/deletehistory"
	msgclient "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	dialogclient "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestDeleteHistoryByDateReachesTheMessageService(t *testing.T) {
	msgFake := &historyMsgFake{offset: 40}
	dialogs := &historyDialogFake{}
	c := historyCore(msgFake, dialogs)
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 9}).To_InputPeer()
	got, err := c.MessagesDeleteHistory(&mtproto.TLMessagesDeleteHistory{
		Peer:    peer,
		MaxId:   80,
		Revoke:  true,
		MinDate: &wrapperspb.Int32Value{Value: 100},
		MaxDate: &wrapperspb.Int32Value{Value: 200},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetOffset() != 40 || got.GetPtsCount() != 3 {
		t.Fatalf("affected %+v", got)
	}
	if msgFake.got == nil || msgFake.got.MaxId != 80 || !msgFake.got.Revoke || msgFake.got.PeerId != 9 {
		t.Fatalf("downstream %#v", msgFake.got)
	}
	dates, ok := deletehistory.RangeFrom(msgFake.ctx)
	if !ok || dates.MinDate != 100 || dates.MaxDate != 200 || !dates.MinSet || !dates.MaxSet {
		t.Fatalf("dates were not forwarded: %+v ok %v", dates, ok)
	}
	if dialogs.calls != 0 {
		t.Fatal("a date-window delete must not drop the dialog")
	}
}

func TestDeleteHistoryOffsetKeepsTheDialog(t *testing.T) {
	msgFake := &historyMsgFake{offset: 15}
	dialogs := &historyDialogFake{}
	c := historyCore(msgFake, dialogs)
	_, err := c.MessagesDeleteHistory(&mtproto.TLMessagesDeleteHistory{
		Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 9}).To_InputPeer(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if dialogs.calls != 0 {
		t.Fatal("offset > 0 means the client will call again; the dialog stays")
	}
}

func TestDeleteHistoryFinishedClearsTheDialog(t *testing.T) {
	msgFake := &historyMsgFake{offset: 0}
	dialogs := &historyDialogFake{}
	c := historyCore(msgFake, dialogs)
	_, err := c.MessagesDeleteHistory(&mtproto.TLMessagesDeleteHistory{
		Peer:   mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 9}).To_InputPeer(),
		Revoke: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if dialogs.calls != 2 {
		t.Fatalf("revoke of a finished history deletes both dialogs, calls %d", dialogs.calls)
	}
}

func TestDeleteHistoryDateOnAChatIsRejected(t *testing.T) {
	msgFake := &historyMsgFake{}
	c := historyCore(msgFake, &historyDialogFake{})
	for _, peer := range []*mtproto.InputPeer{
		mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 4}).To_InputPeer(),
		mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 8}).To_InputPeer(),
	} {
		_, err := c.MessagesDeleteHistory(&mtproto.TLMessagesDeleteHistory{
			Peer:    peer,
			MinDate: &wrapperspb.Int32Value{Value: 1},
		})
		if err != mtproto.ErrChatRevokeDateUnsupported {
			t.Fatalf("%v: %v", peer.GetPredicateName(), err)
		}
	}
	if msgFake.got != nil {
		t.Fatal("rejected date delete must not call the message service")
	}
}

func TestDeleteHistoryNegativeDateIsInvalid(t *testing.T) {
	c := historyCore(&historyMsgFake{}, &historyDialogFake{})
	_, err := c.MessagesDeleteHistory(&mtproto.TLMessagesDeleteHistory{
		Peer:    mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 9}).To_InputPeer(),
		MinDate: &wrapperspb.Int32Value{Value: -1},
	})
	if err != mtproto.ErrMinDateInvalid {
		t.Fatalf("%v", err)
	}
}

func historyCore(messages msgclient.MsgClient, dialogs dialogclient.DialogClient) *MessagesCore {
	ctx := context.Background()
	return &MessagesCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			MsgClient:    messages,
			DialogClient: dialogs,
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 7, PermAuthKeyId: 11},
	}
}

type historyMsgFake struct {
	msgclient.MsgClient
	got    *msg.TLMsgDeleteHistory
	ctx    context.Context
	offset int32
}

func (f *historyMsgFake) MsgDeleteHistory(ctx context.Context, in *msg.TLMsgDeleteHistory) (*mtproto.Messages_AffectedHistory, error) {
	f.got = in
	f.ctx = ctx
	return mtproto.MakeTLMessagesAffectedHistory(&mtproto.Messages_AffectedHistory{
		Pts:      9,
		PtsCount: 3,
		Offset:   f.offset,
	}).To_Messages_AffectedHistory(), nil
}

type historyDialogFake struct {
	dialogclient.DialogClient
	calls int
}

func (f *historyDialogFake) DialogDeleteDialog(context.Context, *dialog.TLDialogDeleteDialog) (*mtproto.Bool, error) {
	f.calls++
	return mtproto.BoolTrue, nil
}
