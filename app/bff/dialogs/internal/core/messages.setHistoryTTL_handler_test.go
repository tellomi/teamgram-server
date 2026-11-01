package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/dialogs/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/dialogs/internal/svc"
	msg_client "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	chat_client "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	dialog_client "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestMessagesSetHistoryTTLServiceMessage(t *testing.T) {
	for _, period := range []int32{86400, 0} {
		msg := &ttlMsg{}
		dlg := &ttlDialog{}
		syn := &ttlSync{}
		c := newTTLCore(dlg, msg, syn, nil)

		ups, err := c.MessagesSetHistoryTTL(&mtproto.TLMessagesSetHistoryTTL{
			Peer:   mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 20, AccessHash: 1}).To_InputPeer(),
			Period: period,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(dlg.calls) != 1 || dlg.calls[0].UserId != 10 || dlg.calls[0].PeerType != mtproto.PEER_USER || dlg.calls[0].PeerId != 20 || dlg.calls[0].TtlPeriod != period {
			t.Fatalf("dialog ttl %+v", dlg.calls)
		}
		if msg.last == nil || msg.last.UserId != 10 || msg.last.AuthKeyId != 99 || msg.last.PeerType != mtproto.PEER_USER || msg.last.PeerId != 20 {
			t.Fatalf("send %+v", msg.last)
		}
		sent := msg.last.GetMessage()[0].GetMessage()
		act := sent.GetAction()
		if sent.GetPredicateName() != mtproto.Predicate_messageService || act.GetPredicateName() != mtproto.Predicate_messageActionSetMessagesTTL || act.GetPeriod() != period || act.GetAutoSettingFrom() != nil {
			t.Fatalf("service message period %d action %#v", period, act)
		}
		if sent.GetFromId().GetUserId() != 10 || sent.GetPeerId().GetUserId() != 20 {
			t.Fatalf("message peers from %#v peer %#v", sent.GetFromId(), sent.GetPeerId())
		}
		newUp := findTTLUpdate(ups, mtproto.Predicate_updateNewMessage)
		if newUp == nil || newUp.GetPts_INT32() != 15 || newUp.GetPtsCount() != 1 {
			t.Fatalf("pts update %#v", newUp)
		}
		ttlUp := findTTLUpdate(ups, mtproto.Predicate_updatePeerHistoryTTL)
		if ttlUp == nil || ttlUp.GetPeer_PEER().GetUserId() != 20 {
			t.Fatalf("ttl update %#v", ttlUp)
		}
		if period > 0 && ttlUp.GetTtlPeriod().GetValue() != period {
			t.Fatalf("ttl flag %#v", ttlUp.GetTtlPeriod())
		}
		if period == 0 && ttlUp.GetTtlPeriod() != nil {
			t.Fatalf("disable flag should be absent, got %#v", ttlUp.GetTtlPeriod())
		}
		if len(syn.notMe) != 1 || syn.notMe[0].UserId != 10 || syn.notMe[0].PermAuthKeyId != 99 {
			t.Fatalf("not me %#v", syn.notMe)
		}
		if len(syn.push) != 1 || syn.push[0].UserId != 20 {
			t.Fatalf("push %#v", syn.push)
		}
		peerTTL := findTTLUpdate(syn.push[0].Updates, mtproto.Predicate_updatePeerHistoryTTL)
		if peerTTL == nil || peerTTL.GetPeer_PEER().GetUserId() != 10 {
			t.Fatalf("peer ttl update %#v", peerTTL)
		}
	}
}

func TestMessagesSetHistoryTTLSavedAndGroup(t *testing.T) {
	msg := &ttlMsg{}
	dlg := &ttlDialog{}
	syn := &ttlSync{}
	c := newTTLCore(dlg, msg, syn, nil)
	if _, err := c.MessagesSetHistoryTTL(&mtproto.TLMessagesSetHistoryTTL{
		Peer:   mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Period: 60,
	}); err != nil {
		t.Fatal(err)
	}
	if dlg.calls[0].PeerType != mtproto.PEER_SELF || dlg.calls[0].PeerId != 10 {
		t.Fatalf("saved dialog %+v", dlg.calls[0])
	}
	if msg.last.PeerType != mtproto.PEER_USER || msg.last.PeerId != 10 {
		t.Fatalf("saved send %+v", msg.last)
	}
	sent := msg.last.GetMessage()[0].GetMessage()
	if sent.GetSavedPeerId().GetUserId() != 10 || sent.GetAction().GetPeriod() != 60 {
		t.Fatalf("saved message %#v", sent)
	}
	if len(syn.push) != 0 || len(syn.notMe) != 1 {
		t.Fatalf("saved sync push %d notMe %d", len(syn.push), len(syn.notMe))
	}

	msg = &ttlMsg{}
	dlg = &ttlDialog{}
	syn = &ttlSync{}
	ch := &ttlChat{chat: &mtproto.MutableChat{ChatParticipants: []*mtproto.ImmutableChatParticipant{
		{UserId: 10, State: mtproto.ChatMemberStateNormal},
		{UserId: 11, State: mtproto.ChatMemberStateNormal},
		{UserId: 12, State: mtproto.ChatMemberStateLeft},
	}}}
	c = newTTLCore(dlg, msg, syn, ch)
	if _, err := c.MessagesSetHistoryTTL(&mtproto.TLMessagesSetHistoryTTL{
		Peer:   mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 30}).To_InputPeer(),
		Period: 0,
	}); err != nil {
		t.Fatal(err)
	}
	if dlg.calls[0].PeerType != mtproto.PEER_CHAT || dlg.calls[0].PeerId != 30 || dlg.calls[0].TtlPeriod != 0 {
		t.Fatalf("chat dialog %+v", dlg.calls[0])
	}
	if msg.last.PeerType != mtproto.PEER_CHAT || msg.last.PeerId != 30 || msg.last.GetMessage()[0].GetMessage().GetAction().GetPeriod() != 0 {
		t.Fatalf("chat send %+v", msg.last)
	}
	if len(syn.push) != 1 || syn.push[0].UserId != 11 {
		t.Fatalf("chat push %#v", syn.push)
	}
	if got := findTTLUpdate(syn.push[0].Updates, mtproto.Predicate_updatePeerHistoryTTL); got.GetPeer_PEER().GetChatId() != 30 || got.GetTtlPeriod() != nil {
		t.Fatalf("chat ttl update %#v", got)
	}
}

func TestMessagesSetHistoryTTLRejectsBadInput(t *testing.T) {
	msg := &ttlMsg{}
	dlg := &ttlDialog{}
	c := newTTLCore(dlg, msg, &ttlSync{}, nil)
	if _, err := c.MessagesSetHistoryTTL(&mtproto.TLMessagesSetHistoryTTL{
		Peer:   mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 20}).To_InputPeer(),
		Period: -1,
	}); err != mtproto.ErrTtlPeriodInvalid {
		t.Fatalf("negative: %v", err)
	}
	if _, err := c.MessagesSetHistoryTTL(&mtproto.TLMessagesSetHistoryTTL{
		Peer:   mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 20}).To_InputPeer(),
		Period: 366*86400 + 1,
	}); err != mtproto.ErrTtlPeriodInvalid {
		t.Fatalf("too long: %v", err)
	}
	if _, err := c.MessagesSetHistoryTTL(&mtproto.TLMessagesSetHistoryTTL{
		Peer:   mtproto.MakeTLInputPeerEmpty(nil).To_InputPeer(),
		Period: 60,
	}); err != mtproto.ErrPeerIdInvalid {
		t.Fatalf("empty peer: %v", err)
	}
	if len(dlg.calls) != 0 || msg.last != nil {
		t.Fatal("rejected call still wrote state")
	}
}

func newTTLCore(d dialog_client.DialogClient, m msg_client.MsgClient, s sync_client.SyncClient, ch chat_client.ChatClient) *DialogsCore {
	ctx := context.Background()
	return &DialogsCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			DialogClient: d,
			MsgClient:    m,
			SyncClient:   s,
			ChatClient:   ch,
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 10, PermAuthKeyId: 99},
	}
}

func findTTLUpdate(ups *mtproto.Updates, pred string) *mtproto.Update {
	if ups == nil {
		return nil
	}
	for _, u := range ups.GetUpdates() {
		if u.GetPredicateName() == pred {
			return u
		}
	}
	return nil
}

type ttlDialog struct {
	dialog_client.DialogClient
	calls []*dialog.TLDialogSetHistoryTTL
}

func (d *ttlDialog) DialogSetHistoryTTL(_ context.Context, in *dialog.TLDialogSetHistoryTTL) (*mtproto.Bool, error) {
	d.calls = append(d.calls, in)
	return mtproto.BoolTrue, nil
}

type ttlMsg struct {
	msg_client.MsgClient
	last *msgpb.TLMsgSendMessageV2
}

func (m *ttlMsg) MsgSendMessageV2(_ context.Context, in *msgpb.TLMsgSendMessageV2) (*mtproto.Updates, error) {
	m.last = in
	box := in.GetMessage()[0]
	return mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateNewMessage(&mtproto.Update{
		Pts_INT32:       15,
		PtsCount:        1,
		RandomId:        box.GetRandomId(),
		Message_MESSAGE: box.GetMessage(),
	}).To_Update()), nil
}

type ttlSync struct {
	sync_client.SyncClient
	notMe []*sync.TLSyncUpdatesNotMe
	push  []*sync.TLSyncPushUpdates
}

func (s *ttlSync) SyncUpdatesNotMe(_ context.Context, in *sync.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	s.notMe = append(s.notMe, in)
	return nil, nil
}

func (s *ttlSync) SyncPushUpdates(_ context.Context, in *sync.TLSyncPushUpdates) (*mtproto.Void, error) {
	s.push = append(s.push, in)
	return nil, nil
}

type ttlChat struct {
	chat_client.ChatClient
	chat *mtproto.MutableChat
}

func (c *ttlChat) ChatGetMutableChat(_ context.Context, _ *chatpb.TLChatGetMutableChat) (*mtproto.MutableChat, error) {
	return c.chat, nil
}
