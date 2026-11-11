package core

import (
	"context"
	"database/sql"
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
	media := msg.GetMedia()
	withPendingFetch(t, func(context.Context, string) (*mtproto.WebPage, error) {
		return nil, errors.New("no such host")
	})
	job := PendingWebpageJob{UserID: 7, PeerType: mtproto.PEER_USER, PeerID: 8, MsgID: 9}
	c.resolvePendingWebpage(context.Background(), job, raw)
	if len(edits.edits) != 1 {
		t.Fatalf("edits %d", len(edits.edits))
	}
	wantEmptyWebpage(t, edits.edits[0].GetNewMessage().GetMessage(), media)
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
	store := withPendingJobStore(t)
	job := PendingWebpageJob{UserID: 7, PeerType: mtproto.PEER_USER, PeerID: 8, MsgID: 11, Created: time.Now()}
	store.Put(job)
	c.resolvePendingWebpage(context.Background(), job, raw)
	if len(edits.edits) != 0 {
		t.Fatalf("still-pending should retry, not edit: %#v", edits.edits)
	}
	due := store.Due(time.Now().Add(pendingSweepEvery+time.Second), 0, 10)
	if len(due) != 1 || due[0].Attempts != 1 {
		t.Fatalf("expected one retryable job, got %+v", due)
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
	wantEmptyWebpage(t, edits.edits[0].GetNewMessage().GetMessage(), msg.GetMedia())
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
	pendingID := msg.GetMedia().GetWebpage().GetId()
	webpage.ApplyFetchedPage(msg, nil)
	if !msg.GetInvertMedia() {
		t.Fatal("clearing the card changed invert_media")
	}
	got := msg.GetMedia().GetWebpage()
	if got == nil || got.GetPredicateName() != mtproto.Predicate_webPageEmpty || got.GetId() != pendingID {
		t.Fatalf("clearing should write webPageEmpty, got %#v", msg.GetMedia())
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

func wantEmptyWebpage(t *testing.T, msg *mtproto.Message, pending *mtproto.MessageMedia) {
	t.Helper()
	if msg == nil {
		t.Fatal("nil message")
	}
	page := msg.GetMedia().GetWebpage()
	if page == nil || page.GetPredicateName() != mtproto.Predicate_webPageEmpty {
		t.Fatalf("want webPageEmpty, got %#v", msg.GetMedia())
	}
	if pending != nil && pending.GetWebpage() != nil && page.GetId() != pending.GetWebpage().GetId() {
		t.Fatalf("empty id %d != pending id %d", page.GetId(), pending.GetWebpage().GetId())
	}
	if !msg.GetEditHide() {
		t.Fatal("edit_hide not set")
	}
}

type failingEdits struct {
	msgclient.MsgClient
	err   error
	calls int
}

func (f *failingEdits) MsgEditMessageV2(_ context.Context, in *msgpb.TLMsgEditMessageV2) (*mtproto.Updates, error) {
	f.calls++
	return nil, f.err
}

func withPendingJobStore(t *testing.T) *memoryPendingJobs {
	t.Helper()
	prev := pendingJobs
	store := newMemoryPendingJobs()
	pendingJobs = store
	t.Cleanup(func() { pendingJobs = prev })
	return store
}

func withGiveUpLog(t *testing.T) *[]PendingWebpageJob {
	t.Helper()
	prev := pendingJobGiveUpLog
	var got []PendingWebpageJob
	pendingJobGiveUpLog = func(job PendingWebpageJob, _ time.Time, _ string) {
		got = append(got, job)
	}
	t.Cleanup(func() { pendingJobGiveUpLog = prev })
	return &got
}

func stalePendingJob(msgID int32, created time.Time) PendingWebpageJob {
	return PendingWebpageJob{UserID: 7, PeerType: mtproto.PEER_USER, PeerID: 8, MsgID: msgID, Created: created}
}

func TestSweepDeletesJobWhenMessageGone(t *testing.T) {
	store := withPendingJobStore(t)
	edits := &pendingEdits{}
	c := New(context.Background(), &svc.ServiceContext{Dao: &dao.Dao{MessageClient: &pendingBoxes{}, MsgClient: edits}})
	store.Put(stalePendingJob(21, time.Now().Add(-61*time.Second)))

	if n := c.sweepStalePendingWebpages(time.Now()); n != 0 {
		t.Fatalf("cleared %d, want 0 (message gone is not an edit)", n)
	}
	if len(edits.edits) != 0 {
		t.Fatalf("edited a missing message: %#v", edits.edits)
	}
	if due := store.Due(time.Now(), 0, 10); len(due) != 0 {
		t.Fatalf("gone message left a job: %+v", due)
	}
}

func TestSweepDeletesJobWhenNoLongerPending(t *testing.T) {
	store := withPendingJobStore(t)
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Id:      22,
		Message: "https://already-fetched.example/",
	}).To_Message()
	boxes := &pendingBoxes{box: &mtproto.MessageBox{UserId: 7, MessageId: 22, SenderUserId: 7, Message: msg}}
	edits := &pendingEdits{}
	c := New(context.Background(), &svc.ServiceContext{Dao: &dao.Dao{MessageClient: boxes, MsgClient: edits}})
	store.Put(stalePendingJob(22, time.Now().Add(-61*time.Second)))

	if n := c.sweepStalePendingWebpages(time.Now()); n != 1 {
		t.Fatalf("cleared %d, want 1", n)
	}
	if len(edits.edits) != 0 {
		t.Fatalf("edited a non-pending message: %#v", edits.edits)
	}
	if due := store.Due(time.Now(), 0, 10); len(due) != 0 {
		t.Fatalf("non-pending message left a job: %+v", due)
	}
}

func TestSweepRetriesOnEditFailureWithBackoff(t *testing.T) {
	store := withPendingJobStore(t)
	const raw = "https://edit-fails.example/"
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Id:      23,
		Message: raw,
		Media:   chatboxPending(t, raw),
	}).To_Message()
	boxes := &pendingBoxes{box: &mtproto.MessageBox{UserId: 7, MessageId: 23, SenderUserId: 7, Message: msg}}
	edits := &failingEdits{err: errors.New("permission denied")}
	c := New(context.Background(), &svc.ServiceContext{Dao: &dao.Dao{MessageClient: boxes, MsgClient: edits}})
	created := time.Now().Add(-61 * time.Second)
	store.Put(stalePendingJob(23, created))

	now := time.Now()
	if n := c.sweepStalePendingWebpages(now); n != 0 {
		t.Fatalf("cleared %d after a failed edit", n)
	}
	if edits.calls != 1 {
		t.Fatalf("edit calls %d, want 1", edits.calls)
	}
	if due := store.Due(now, pendingWebpageMaxAge, 10); len(due) != 0 {
		t.Fatalf("failed job was immediately due again: %+v", due)
	}
	later := store.Due(now.Add(pendingSweepEvery+time.Second), pendingWebpageMaxAge, 10)
	if len(later) != 1 || later[0].MsgID != 23 || later[0].Attempts != 1 {
		t.Fatalf("backoff job: %+v", later)
	}
}

func TestSweepGivesUpAfterMaxAttempts(t *testing.T) {
	store := withPendingJobStore(t)
	gaveUp := withGiveUpLog(t)
	const raw = "https://give-up-attempts.example/"
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Id:      24,
		Message: raw,
		Media:   chatboxPending(t, raw),
	}).To_Message()
	boxes := &pendingBoxes{box: &mtproto.MessageBox{UserId: 7, MessageId: 24, SenderUserId: 7, Message: msg}}
	edits := &failingEdits{err: errors.New("session gone")}
	c := New(context.Background(), &svc.ServiceContext{Dao: &dao.Dao{MessageClient: boxes, MsgClient: edits}})
	job := stalePendingJob(24, time.Now().Add(-61*time.Second))
	store.Put(job)
	// Put resets attempts; write 9 failures directly.
	store.mu.Lock()
	job = store.jobs[store.key(job.UserID, job.MsgID)]
	job.Attempts = pendingJobMaxAttempts - 1
	store.jobs[store.key(job.UserID, job.MsgID)] = job
	store.mu.Unlock()

	if n := c.sweepStalePendingWebpages(time.Now()); n != 0 {
		t.Fatalf("cleared %d, want 0", n)
	}
	if edits.calls != 1 {
		t.Fatalf("edit calls %d, want 1 (the last attempt)", edits.calls)
	}
	if due := store.Due(time.Now(), 0, 10); len(due) != 0 {
		t.Fatalf("job not dropped after max attempts: %+v", due)
	}
	if len(*gaveUp) != 1 || (*gaveUp)[0].Attempts != pendingJobMaxAttempts {
		t.Fatalf("give-up log: %+v", *gaveUp)
	}
}

func TestSweepGivesUpAfterOneHour(t *testing.T) {
	store := withPendingJobStore(t)
	gaveUp := withGiveUpLog(t)
	const raw = "https://give-up-age.example/"
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Id:      25,
		Message: raw,
		Media:   chatboxPending(t, raw),
	}).To_Message()
	boxes := &pendingBoxes{box: &mtproto.MessageBox{UserId: 7, MessageId: 25, SenderUserId: 7, Message: msg}}
	edits := &failingEdits{err: errors.New("should not be called")}
	c := New(context.Background(), &svc.ServiceContext{Dao: &dao.Dao{MessageClient: boxes, MsgClient: edits}})
	store.Put(stalePendingJob(25, time.Now().Add(-pendingJobGiveUpAfter-time.Second)))

	if n := c.sweepStalePendingWebpages(time.Now()); n != 0 {
		t.Fatalf("cleared %d, want 0", n)
	}
	if edits.calls != 1 {
		t.Fatalf("1h give-up should try one last EditHide, calls=%d", edits.calls)
	}
	if due := store.Due(time.Now(), 0, 10); len(due) != 0 {
		t.Fatalf("1h job was not dropped: %+v", due)
	}
	if len(*gaveUp) != 1 {
		t.Fatalf("give-up log: %+v", *gaveUp)
	}
}

func TestConfigurePendingJobsDBCapsPool(t *testing.T) {
	db, err := sql.Open("mysql", "pending_jobs:pending_jobs@tcp(127.0.0.1:1)/pending_jobs")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	configurePendingJobsDB(db)
	if got := db.Stats().MaxOpenConnections; got != pendingJobsMaxOpenConns {
		t.Fatalf("MaxOpenConns=%d, want %d", got, pendingJobsMaxOpenConns)
	}
}

func TestPendingJobShouldGiveUp(t *testing.T) {
	now := time.Now()
	if pendingJobShouldGiveUp(PendingWebpageJob{Attempts: 9, Created: now}, now) {
		t.Fatal("9 attempts should still retry")
	}
	if !pendingJobShouldGiveUp(PendingWebpageJob{Attempts: 10, Created: now}, now) {
		t.Fatal("10 attempts should give up")
	}
	if !pendingJobShouldGiveUp(PendingWebpageJob{Attempts: 1, Created: now.Add(-pendingJobGiveUpAfter)}, now) {
		t.Fatal("1h-old job should give up")
	}
}

type ctxAwareBoxes struct {
	messageclient.MessageClient
	box          *mtproto.MessageBox
	canceledGets int
	liveGets     int
}

func (f *ctxAwareBoxes) MessageGetUserMessageList(ctx context.Context, in *message.TLMessageGetUserMessageList) (*message.Vector_MessageBox, error) {
	if ctx != nil && ctx.Err() != nil {
		f.canceledGets++
		// Production footgun: a canceled ctx can yield an empty success.
		return &message.Vector_MessageBox{}, nil
	}
	f.liveGets++
	if f.box == nil {
		return &message.Vector_MessageBox{}, nil
	}
	return &message.Vector_MessageBox{Datas: []*mtproto.MessageBox{f.box}}, nil
}

func TestResolvePendingWebpageRetriesOnFetchTimeoutThenEmptiesAtCap(t *testing.T) {
	t.Setenv("TELLOMI_SERVER_LINK_FETCH", "1")
	const raw = "https://this-domain-does-not-exist-tellomi-305-test.invalid/"
	media := chatboxPending(t, raw)
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Id:      68,
		Message: raw,
		Media:   media,
	}).To_Message()
	boxes := &ctxAwareBoxes{box: &mtproto.MessageBox{UserId: 7, MessageId: 68, SenderUserId: 7, Message: msg}}
	edits := &pendingEdits{}
	c := New(context.Background(), &svc.ServiceContext{Dao: &dao.Dao{MessageClient: boxes, MsgClient: edits}})
	store := withPendingJobStore(t)
	job := PendingWebpageJob{UserID: 7, PeerType: mtproto.PEER_USER, PeerID: 8, MsgID: 68, Created: time.Now()}
	store.Put(job)
	withPendingFetch(t, func(ctx context.Context, _ string) (*mtproto.WebPage, error) {
		// Production uses pendingWebpageTimeout (20s). A short parent
		// deadline stands in so the test does not wait the full 20s.
		// fetchRemote currently ignores the deadline and can return
		// after ctx is already done; treat that as a retryable timeout.
		if ctx != nil {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return nil, context.DeadlineExceeded
	})
	parent, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	c.resolvePendingWebpage(parent, job, raw)
	cancel()
	if len(edits.edits) != 0 {
		t.Fatalf("timeout should retry, not edit yet: %#v", edits.edits)
	}
	due := store.Due(time.Now().Add(pendingSweepEvery+time.Second), 0, 10)
	if len(due) != 1 || due[0].Attempts != 1 {
		t.Fatalf("timeout should bump attempts, got %+v", due)
	}

	store.mu.Lock()
	held := store.jobs[store.key(job.UserID, job.MsgID)]
	held.Attempts = pendingJobMaxAttempts - 1
	held.NextRetry = time.Time{}
	store.jobs[store.key(job.UserID, job.MsgID)] = held
	store.mu.Unlock()

	parent, cancel = context.WithTimeout(context.Background(), 40*time.Millisecond)
	c.resolvePendingWebpage(parent, held, raw)
	cancel()
	if boxes.canceledGets != 0 {
		t.Fatalf("Get used a canceled context %d time(s)", boxes.canceledGets)
	}
	if boxes.liveGets == 0 {
		t.Fatal("Get was not called with a live context after the cap")
	}
	if len(edits.edits) != 1 {
		t.Fatalf("edits %d after retry cap, want 1", len(edits.edits))
	}
	wantEmptyWebpage(t, edits.edits[0].GetNewMessage().GetMessage(), media)
	if due := store.Due(time.Now(), 0, 10); len(due) != 0 {
		t.Fatalf("capped timeout job was not untracked: %+v", due)
	}
}

func TestResolvePendingWebpageClearsOnDNSEmptyPage(t *testing.T) {
	t.Setenv("TELLOMI_SERVER_LINK_FETCH", "1")
	const raw = "https://nxdomain-empty.example/"
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Id:      37,
		Message: raw,
		Media:   chatboxPending(t, raw),
	}).To_Message()
	boxes := &pendingBoxes{box: &mtproto.MessageBox{UserId: 7, MessageId: 37, SenderUserId: 7, Message: msg}}
	edits := &pendingEdits{}
	c := New(context.Background(), &svc.ServiceContext{Dao: &dao.Dao{MessageClient: boxes, MsgClient: edits}})
	// WebPageContext returns (webPageEmpty, nil) on NXDOMAIN, not an error.
	withPendingFetch(t, func(context.Context, string) (*mtproto.WebPage, error) {
		return mtproto.MakeTLWebPageEmpty(&mtproto.WebPage{}).To_WebPage(), nil
	})
	c.resolvePendingWebpage(context.Background(), PendingWebpageJob{UserID: 7, PeerType: mtproto.PEER_USER, PeerID: 8, MsgID: 37}, raw)
	if len(edits.edits) != 1 {
		t.Fatalf("edits %d, want 1", len(edits.edits))
	}
	wantEmptyWebpage(t, edits.edits[0].GetNewMessage().GetMessage(), msg.GetMedia())
}

func TestPendingLoadStatusCanceledOrErroredGetIsRetry(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if got := pendingLoadStatus(nil, canceled, 0); got != pendingCommitRetry {
		t.Fatalf("canceled empty Get: %v, want Retry", got)
	}
	if got := pendingLoadStatus(errors.New("rpc failed"), context.Background(), 0); got != pendingCommitRetry {
		t.Fatalf("errored Get: %v, want Retry", got)
	}
	if got := pendingLoadStatus(nil, context.Background(), 0); got != pendingCommitGone {
		t.Fatalf("live empty Get: %v, want Gone", got)
	}
	if got := pendingLoadStatus(nil, context.Background(), 1); got != pendingCommitDone {
		t.Fatalf("live non-empty Get: %v, want load-ok", got)
	}
}

func TestSweepRecoversOrphanPendingWithoutJobRow(t *testing.T) {
	store := withPendingJobStore(t)
	const raw = "https://orphan-pending.example/"
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Id:      68,
		Message: raw,
		Media:   chatboxPending(t, raw),
	}).To_Message()
	boxes := &pendingBoxes{box: &mtproto.MessageBox{UserId: 7, MessageId: 68, SenderUserId: 7, Message: msg}}
	edits := &pendingEdits{}
	c := New(context.Background(), &svc.ServiceContext{Dao: &dao.Dao{MessageClient: boxes, MsgClient: edits}})
	prev := scanOrphanPendingJobs
	scanOrphanPendingJobs = func(time.Time, time.Duration, int) []PendingWebpageJob {
		return []PendingWebpageJob{stalePendingJob(68, time.Now().Add(-2*time.Minute))}
	}
	t.Cleanup(func() { scanOrphanPendingJobs = prev })

	if due := store.Due(time.Now(), 0, 10); len(due) != 0 {
		t.Fatalf("precondition: store should be empty, got %+v", due)
	}
	if n := c.sweepStalePendingWebpages(time.Now()); n != 1 {
		t.Fatalf("cleared %d, want 1 orphan", n)
	}
	if len(edits.edits) != 1 || edits.edits[0].GetNewMessage().GetMessage().GetId() != 68 {
		t.Fatalf("orphan edit: %#v", edits.edits)
	}
	wantEmptyWebpage(t, edits.edits[0].GetNewMessage().GetMessage(), msg.GetMedia())
}

func TestEnsureDoesNotResetAttemptsAndTombstoneIsNotRequeued(t *testing.T) {
	store := withPendingJobStore(t)
	const raw = "https://tombstone-orphan.example/"
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Id:      70,
		Message: raw,
		Media:   chatboxPending(t, raw),
	}).To_Message()
	boxes := &pendingBoxes{box: &mtproto.MessageBox{UserId: 7, MessageId: 70, SenderUserId: 7, Message: msg}}
	edits := &pendingEdits{}
	c := New(context.Background(), &svc.ServiceContext{Dao: &dao.Dao{MessageClient: boxes, MsgClient: edits}})

	job := stalePendingJob(70, time.Now().Add(-2*time.Minute))
	store.Put(job)
	store.mu.Lock()
	held := store.jobs[store.key(job.UserID, job.MsgID)]
	held.Attempts = 4
	held.NextRetry = time.Now().Add(time.Hour)
	store.jobs[store.key(job.UserID, job.MsgID)] = held
	store.mu.Unlock()
	store.Ensure(stalePendingJob(70, time.Now().Add(-2*time.Minute)))
	store.mu.Lock()
	got := store.jobs[store.key(job.UserID, job.MsgID)]
	store.mu.Unlock()
	if got.Attempts != 4 {
		t.Fatalf("Ensure reset attempts to %d", got.Attempts)
	}

	store.Tombstone(got)
	if due := store.Due(time.Now(), 0, 10); len(due) != 0 {
		t.Fatalf("tombstone was due: %+v", due)
	}
	prev := scanOrphanPendingJobs
	scanOrphanPendingJobs = func(time.Time, time.Duration, int) []PendingWebpageJob {
		return []PendingWebpageJob{stalePendingJob(70, time.Now().Add(-2*time.Minute))}
	}
	t.Cleanup(func() { scanOrphanPendingJobs = prev })
	if n := c.sweepStalePendingWebpages(time.Now()); n != 0 {
		t.Fatalf("tombstone sweep cleared %d", n)
	}
	if len(edits.edits) != 0 {
		t.Fatalf("tombstone was requeued and edited: %#v", edits.edits)
	}
	if due := store.Due(time.Now(), 0, 10); len(due) != 0 {
		t.Fatalf("tombstone became due after orphan Ensure: %+v", due)
	}
}

func TestPendingFetchOutcomeClassifiesTimeoutAndNXDOMAIN(t *testing.T) {
	if apply, retry := pendingFetchOutcome(nil, context.DeadlineExceeded); apply != nil || !retry {
		t.Fatalf("20s timeout / deadline: apply=%v retry=%v", apply, retry)
	}
	if apply, retry := pendingFetchOutcome(nil, context.Canceled); apply != nil || !retry {
		t.Fatalf("canceled fetch: apply=%v retry=%v", apply, retry)
	}
	empty := mtproto.MakeTLWebPageEmpty(&mtproto.WebPage{Id: 1}).To_WebPage()
	if apply, retry := pendingFetchOutcome(empty, nil); retry || apply != nil {
		// apply is nil; commitPendingWebpage(nil page) writes webPageEmpty from the pending card.
		t.Fatalf("NXDOMAIN empty page should finish now: apply=%v retry=%v", apply, retry)
	}
	if apply, retry := pendingFetchOutcome(nil, errors.New("no such host")); retry || apply != nil {
		t.Fatalf("NXDOMAIN error should finish now: apply=%v retry=%v", apply, retry)
	}
}

func TestPendingMessageTableOK(t *testing.T) {
	if !pendingMessageTableOK("messages") || !pendingMessageTableOK("messages_3") {
		t.Fatal("expected messages / messages_N to be allowed")
	}
	if pendingMessageTableOK("messages_pending") || pendingMessageTableOK("users") || pendingMessageTableOK("messages_") {
		t.Fatal("rejected names were allowed")
	}
}

func TestPendingOrphanScanBounds(t *testing.T) {
	if pendingOrphanLookback < time.Hour || pendingOrphanLookback > 24*time.Hour {
		t.Fatalf("lookback %s: want 1h–1d", pendingOrphanLookback)
	}
	if pendingOrphanLookback != 24*time.Hour {
		t.Fatalf("lookback %s, want 24h", pendingOrphanLookback)
	}
	if pendingOrphanScanEvery < 10*time.Minute || pendingOrphanScanEvery > 15*time.Minute {
		t.Fatalf("orphan interval %s: want 10–15min", pendingOrphanScanEvery)
	}
	if pendingSweepEvery != 15*time.Second {
		t.Fatalf("due-job tick %s, want 15s", pendingSweepEvery)
	}
	if pendingSweepLimit != 100 {
		t.Fatalf("batch limit %d, want 100", pendingSweepLimit)
	}
}

func TestOrphanScanDueOnlyAtStartupAndInterval(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	if !orphanScanDue(time.Time{}, now) {
		t.Fatal("startup must scan orphans")
	}
	if orphanScanDue(now, now.Add(pendingSweepEvery)) {
		t.Fatal("15s due-job tick must not rescan messages")
	}
	if orphanScanDue(now, now.Add(pendingOrphanScanEvery-time.Second)) {
		t.Fatal("scan fired before the 15m interval")
	}
	if !orphanScanDue(now, now.Add(pendingOrphanScanEvery)) {
		t.Fatal("scan should fire at the 15m interval")
	}
}

func TestSweepSkipsOrphanScanWhenNotDue(t *testing.T) {
	store := withPendingJobStore(t)
	const raw = "https://orphan-skip.example/"
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Id:      71,
		Message: raw,
		Media:   chatboxPending(t, raw),
	}).To_Message()
	boxes := &pendingBoxes{box: &mtproto.MessageBox{UserId: 7, MessageId: 71, SenderUserId: 7, Message: msg}}
	edits := &pendingEdits{}
	c := New(context.Background(), &svc.ServiceContext{Dao: &dao.Dao{MessageClient: boxes, MsgClient: edits}})
	calls := 0
	prev := scanOrphanPendingJobs
	scanOrphanPendingJobs = func(time.Time, time.Duration, int) []PendingWebpageJob {
		calls++
		return []PendingWebpageJob{stalePendingJob(71, time.Now().Add(-2*time.Minute))}
	}
	t.Cleanup(func() { scanOrphanPendingJobs = prev })

	if n := c.sweepPendingWebpages(time.Now(), false); n != 0 {
		t.Fatalf("cleared %d without an orphan scan", n)
	}
	if calls != 0 {
		t.Fatalf("orphan hook called %d time(s) on a due-job-only tick", calls)
	}
	if len(edits.edits) != 0 {
		t.Fatalf("edited without a job row: %#v", edits.edits)
	}
	if n := c.sweepPendingWebpages(time.Now(), true); n != 1 {
		t.Fatalf("cleared %d, want 1 on an orphan tick", n)
	}
	if calls != 1 {
		t.Fatalf("orphan hook calls %d, want 1", calls)
	}
	wantEmptyWebpage(t, edits.edits[0].GetNewMessage().GetMessage(), msg.GetMedia())
	_ = store
}
