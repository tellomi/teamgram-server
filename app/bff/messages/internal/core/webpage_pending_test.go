package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	"github.com/teamgram/teamgram-server/app/bff/messages/webpage"
	msgclient "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	messageclient "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type pendingBoxes struct {
	messageclient.MessageClient
	box *mtproto.MessageBox
}

func (f *pendingBoxes) MessageGetUserMessageList(_ context.Context, in *message.TLMessageGetUserMessageList) (*message.Vector_MessageBox, error) {
	if f.box == nil {
		return &message.Vector_MessageBox{}, nil
	}
	return &message.Vector_MessageBox{Datas: []*mtproto.MessageBox{f.box}}, nil
}

type pendingEdits struct {
	msgclient.MsgClient
	edits []*msgpb.TLMsgEditMessageV2
}

func (f *pendingEdits) MsgEditMessageV2(_ context.Context, in *msgpb.TLMsgEditMessageV2) (*mtproto.Updates, error) {
	f.edits = append(f.edits, in)
	return mtproto.MakeTLUpdates(&mtproto.Updates{}).To_Updates(), nil
}

func withPendingFetch(t *testing.T, fn func(context.Context, string) (*mtproto.WebPage, error)) {
	t.Helper()
	prev := fetchPendingPage
	fetchPendingPage = fn
	t.Cleanup(func() { fetchPendingPage = prev })
}

func TestResolvePendingWebpageClearsOnError(t *testing.T) {
	t.Setenv("TELLOMI_SERVER_LINK_FETCH", "1")
	const raw = "https://clear-on-error.example/"
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Id:      9,
		Message: raw,
		Media:   chatboxPending(t, raw),
	}).To_Message()
	boxes := &pendingBoxes{box: &mtproto.MessageBox{UserId: 7, MessageId: 9, SenderUserId: 7, Message: msg}}
	edits := &pendingEdits{}
	c := New(context.Background(), &svc.ServiceContext{Dao: &dao.Dao{MessageClient: boxes, MsgClient: edits}})
	withPendingFetch(t, func(context.Context, string) (*mtproto.WebPage, error) {
		return nil, errors.New("timeout")
	})
	job := PendingWebpageJob{UserID: 7, PeerType: mtproto.PEER_USER, PeerID: 8, MsgID: 9}
	c.resolvePendingWebpage(context.Background(), job, raw)
	if len(edits.edits) != 1 {
		t.Fatalf("edits %d", len(edits.edits))
	}
	out := edits.edits[0].GetNewMessage().GetMessage()
	if out.GetMedia() != nil {
		t.Fatalf("card left after error: %#v", out.GetMedia())
	}
	if !out.GetEditHide() {
		t.Fatal("edit_hide not set")
	}
}

func TestResolvePendingWebpageClearsWhenStillPending(t *testing.T) {
	t.Setenv("TELLOMI_SERVER_LINK_FETCH", "1")
	const raw = "https://still-pending.example/"
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Id:      11,
		Message: raw,
		Media:   chatboxPending(t, raw),
	}).To_Message()
	boxes := &pendingBoxes{box: &mtproto.MessageBox{UserId: 7, MessageId: 11, SenderUserId: 7, Message: msg}}
	edits := &pendingEdits{}
	c := New(context.Background(), &svc.ServiceContext{Dao: &dao.Dao{MessageClient: boxes, MsgClient: edits}})
	withPendingFetch(t, func(context.Context, string) (*mtproto.WebPage, error) {
		return chatboxPending(t, raw).GetWebpage(), nil
	})
	c.resolvePendingWebpage(context.Background(), PendingWebpageJob{UserID: 7, PeerType: mtproto.PEER_USER, PeerID: 8, MsgID: 11}, raw)
	if len(edits.edits) != 1 {
		t.Fatalf("edits %d", len(edits.edits))
	}
	if edits.edits[0].GetNewMessage().GetMessage().GetMedia() != nil {
		t.Fatal("still-pending result left a card")
	}
	if !edits.edits[0].GetNewMessage().GetMessage().GetEditHide() {
		t.Fatal("edit_hide not set")
	}
}

func TestSweepClearsStalePendingAfter60s(t *testing.T) {
	prev := pendingJobs
	store := newMemoryPendingJobs()
	pendingJobs = store
	t.Cleanup(func() { pendingJobs = prev })

	t.Setenv("TELLOMI_SERVER_LINK_FETCH", "1")
	const raw = "https://stale-pending.example/"
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Id:      13,
		Message: raw,
		Media:   chatboxPending(t, raw),
	}).To_Message()
	boxes := &pendingBoxes{box: &mtproto.MessageBox{UserId: 7, MessageId: 13, SenderUserId: 7, Message: msg}}
	edits := &pendingEdits{}
	c := New(context.Background(), &svc.ServiceContext{Dao: &dao.Dao{MessageClient: boxes, MsgClient: edits}})

	fresh := PendingWebpageJob{UserID: 7, PeerType: mtproto.PEER_USER, PeerID: 8, MsgID: 14, Created: time.Now()}
	stale := PendingWebpageJob{UserID: 7, PeerType: mtproto.PEER_USER, PeerID: 8, MsgID: 13, Created: time.Now().Add(-61 * time.Second)}
	store.Put(fresh)
	store.Put(stale)

	if n := c.sweepStalePendingWebpages(time.Now()); n != 1 {
		t.Fatalf("cleared %d, want 1", n)
	}
	if len(edits.edits) != 1 || edits.edits[0].GetNewMessage().GetMessage().GetId() != 13 {
		t.Fatalf("swept the wrong edit: %#v", edits.edits)
	}
	if edits.edits[0].GetNewMessage().GetMessage().GetMedia() != nil || !edits.edits[0].GetNewMessage().GetMessage().GetEditHide() {
		t.Fatal("stale pending was not cleared with edit_hide")
	}
	if due := store.Due(time.Now(), pendingWebpageMaxAge, 10); len(due) != 0 {
		t.Fatalf("stale job was not untracked: %+v", due)
	}
	if due := store.Due(time.Now(), 0, 10); len(due) != 1 || due[0].MsgID != 14 {
		t.Fatalf("fresh job was swept: %+v", due)
	}
}

func TestShouldAutoAttachWebpagePolicy(t *testing.T) {
	card := chatboxPending(t, "https://policy.example/")
	if shouldAutoAttachWebpage(true, nil) {
		t.Fatal("no_webpage still attached a card")
	}
	if shouldAutoAttachWebpage(false, card) {
		t.Fatal("existing media (sendMedia / inputMediaWebPage) was replaced")
	}
	if !shouldAutoAttachWebpage(false, nil) {
		t.Fatal("plain sendMessage should attach")
	}
}

func TestApplyFetchedPageKeepsInvertMedia(t *testing.T) {
	const raw = "https://invert.example/"
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Message:     raw,
		InvertMedia: true,
		Media:       chatboxPending(t, raw),
	}).To_Message()
	page := mtproto.MakeTLWebPage(&mtproto.WebPage{
		Id:         msg.GetMedia().GetWebpage().GetId(),
		Url_STRING: raw,
		Title:      wrapperspb.String("Hi"),
	}).To_WebPage()
	webpage.ApplyFetchedPage(msg, page)
	if !msg.GetInvertMedia() {
		t.Fatal("invert_media was cleared")
	}
	if msg.GetMedia().GetWebpage().GetTitle().GetValue() != "Hi" {
		t.Fatal("page not applied")
	}
	webpage.ApplyFetchedPage(msg, nil)
	if !msg.GetInvertMedia() || msg.GetMedia() != nil {
		t.Fatal("clearing the card changed invert_media")
	}
}

func TestPendingWebpageEditFanoutInSource(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	root := file
	for i := 0; i < 12; i++ {
		root = filepath.Dir(root)
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
	}
	chat, err := os.ReadFile(filepath.Join(root, "app/messenger/msg/msg/internal/core/msg.editMessageV2_handler.go"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(chat)
	if !strings.Contains(src, "case mtproto.PEER_CHAT:") || !strings.Contains(src, "chat.Walk") || !strings.Contains(src, "InboxEditMessageToInboxV2") {
		t.Fatal("chat edit no longer fans updateEditMessage out to every member")
	}
	if !strings.Contains(src, "case mtproto.PEER_CHANNEL:") || !strings.Contains(src, "channelpb.Shared().Edit") {
		t.Fatal("channel pending edits no longer go through channel.Edit")
	}
	ch, err := os.ReadFile(filepath.Join(root, "app/service/biz/channel/service_ops.go"))
	if err != nil {
		t.Fatal(err)
	}
	cs := string(ch)
	if !strings.Contains(cs, "func (s *Service) Edit(") || !strings.Contains(cs, "s.emit(") || !strings.Contains(cs, "ListParticipants") {
		t.Fatal("channel.Edit no longer emits to participants")
	}
	conv := string(mustRead(t, filepath.Join(root, "app/service/biz/channel/convert.go")))
	if !strings.Contains(conv, "MakeTLUpdateEditChannelMessage") {
		t.Fatal("channel edit update is no longer updateEditChannelMessage")
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func chatboxPending(t *testing.T, raw string) *mtproto.MessageMedia {
	t.Helper()
	t.Setenv("TELLOMI_SERVER_LINK_FETCH", "1")
	media := webpage.PreviewMedia(context.Background(), raw, nil)
	if !webpage.IsPending(media) {
		t.Fatalf("want pending, got %#v", media)
	}
	return media
}
