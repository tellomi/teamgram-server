package core

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	"github.com/teamgram/teamgram-server/app/bff/messages/webpage"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"

	"github.com/zeromicro/go-zero/core/contextx"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/threading"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const pendingWebpageTimeout = 20 * time.Second
const pendingCommitTimeout = 10 * time.Second

// pendingOrphanLookback bounds the no-job-row recovery scan. Cards older
// than pendingWebpageMaxAge inside this window can be retried with EditHide.
// Kept to one day: production EXPLAIN on messages is a table scan (no
// date2-leading index; the only date2 key is messages_ttl_due (ttl_period,
// date2)). After jobs persist in MySQL this scan is a startup-only
// fallback, not a periodic full-table LIKE.
const pendingOrphanLookback = 24 * time.Hour

type pendingCommitStatus int

const (
	pendingCommitDone  pendingCommitStatus = iota // edited, or no longer pending
	pendingCommitGone                             // message no longer exists
	pendingCommitRetry                            // transient get/edit failure
)

// fetchPendingPage is the background fetch. Tests replace it.
var fetchPendingPage = func(ctx context.Context, raw string) (*mtproto.WebPage, error) {
	page, _, _, err := chatbox.WebPageContext(ctx, raw, 0)
	return page, err
}

// schedulePendingWebpage fetches an external page after sendMessage (or
// editMessage) returned a webPagePending card, then edits the stored
// message for both sides with EditHide set so the transition is not shown
// as an ordinary edit. Scheduled messages are left alone until they send.
//
// The edit goes through MsgEditMessageV2: PEER_USER updates both sides,
// PEER_CHAT walks every normal member (editChatOutgoingMessageV2 →
// InboxEditMessageToInboxV2), and PEER_CHANNEL calls channel.Edit, which
// emit()s updateEditChannelMessage to every normal participant.
func (c *MessagesCore) schedulePendingWebpage(peer *mtproto.PeerUtil, media *mtproto.MessageMedia, text string, entities []*mtproto.MessageEntity, sent *mtproto.Updates, scheduled bool) {
	if c == nil || scheduled || !webpage.IsPending(media) || peer == nil {
		return
	}
	id := sentMessageID(sent)
	raw := chatbox.FirstURL(text, entities)
	if id == 0 || raw == "" || c.svcCtx == nil || c.svcCtx.Dao == nil {
		return
	}
	userId := int64(0)
	authKey := int64(0)
	if c.MD != nil {
		userId = c.MD.UserId
		authKey = c.MD.PermAuthKeyId
	}
	if userId == 0 {
		return
	}
	peerType := peer.PeerType
	if peerType == mtproto.PEER_SELF {
		peerType = mtproto.PEER_USER
	}
	job := PendingWebpageJob{
		UserID:    userId,
		AuthKeyID: authKey,
		PeerType:  peerType,
		PeerID:    peer.PeerId,
		MsgID:     id,
		Created:   time.Now(),
	}
	trackPendingJob(job)
	ctx := contextx.ValueOnlyFrom(c.ctx)
	threading.GoSafe(func() {
		c.resolvePendingWebpage(ctx, job, raw)
	})
}

func (c *MessagesCore) resolvePendingWebpage(parent context.Context, job PendingWebpageJob, raw string) {
	ctx, cancel := context.WithTimeout(parent, pendingWebpageTimeout)
	defer cancel()
	page, err := fetchPendingPage(ctx, raw)
	if err != nil {
		c.Logger.Errorf("pending webpage fetch: %v", err)
	}
	// The fetch context is not reused for Get/Edit: fetchRemote and
	// httpGetContext currently use context.Background(), so a 20s fetch
	// deadline can expire long before the HTTP/DNS call returns.
	apply, retry := pendingFetchOutcome(page, err)
	if retry {
		c.retryOrFinishPending(job, time.Now())
		return
	}
	switch c.commitPendingWebpage(nil, job, apply) {
	case pendingCommitDone, pendingCommitGone:
		untrackPendingJob(job.UserID, job.MsgID)
	case pendingCommitRetry:
		c.retryOrFinishPending(job, time.Now())
	}
}

// pendingFetchOutcome follows Telegram's pending → webPage / webPageEmpty
// split: a definitive miss (NXDOMAIN, webPageEmpty) is applied immediately
// as webPageEmpty; a timeout or a still-pending result is retried until the cap.
func pendingFetchOutcome(page *mtproto.WebPage, err error) (apply *mtproto.WebPage, retry bool) {
	if err != nil {
		if pendingFetchRetryable(err) {
			return nil, true
		}
		return nil, false
	}
	if page != nil && page.GetPredicateName() == mtproto.Predicate_webPage {
		return page, false
	}
	if page != nil && page.GetPredicateName() == mtproto.Predicate_webPagePending {
		return nil, true
	}
	return nil, false
}

func pendingFetchRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "timeout") || strings.Contains(msg, "Timeout") ||
		strings.Contains(msg, "deadline") || strings.Contains(msg, "connection refused")
}

func (c *MessagesCore) retryOrFinishPending(job PendingWebpageJob, now time.Time) {
	updated, giveUp := failPendingJob(job, now)
	if !giveUp {
		return
	}
	// Cap reached: finish as a definitive miss (webPageEmpty + EditHide).
	switch c.commitPendingWebpage(nil, updated, nil) {
	case pendingCommitDone, pendingCommitGone:
		untrackPendingJob(job.UserID, job.MsgID)
	default:
		tombstonePendingJob(updated)
	}
}

func contextCanceled(ctx context.Context) bool {
	return ctx != nil && ctx.Err() != nil
}

// pendingLoadStatus classifies MessageGetUserMessageList. An error or a
// canceled/deadline-exceeded context is always Retry: an empty result in
// that state is not proof that the message is gone. Gone is only returned
// after a successful Get with a live context and zero rows.
func pendingLoadStatus(err error, ctx context.Context, n int) pendingCommitStatus {
	if err != nil || contextCanceled(ctx) {
		return pendingCommitRetry
	}
	if n == 0 {
		return pendingCommitGone
	}
	return pendingCommitDone // meaning "load ok, continue" — caller must not untrack yet
}

func (c *MessagesCore) commitPendingWebpage(ctx context.Context, job PendingWebpageJob, page *mtproto.WebPage) pendingCommitStatus {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.MessageClient == nil || c.svcCtx.MsgClient == nil {
		return pendingCommitRetry
	}
	// Never inherit the fetch context. A canceled parent makes gRPC return
	// an empty success or a deadline error, which used to be treated as Gone
	// and untracked the job without EditHide.
	commitCtx, cancel := context.WithTimeout(context.Background(), pendingCommitTimeout)
	defer cancel()
	_ = ctx // caller may pass a live or canceled ctx; it is not used for RPCs
	boxes, err := c.svcCtx.MessageClient.MessageGetUserMessageList(commitCtx, &message.TLMessageGetUserMessageList{
		UserId: job.UserID,
		IdList: []int32{job.MsgID},
	})
	n := 0
	if boxes != nil {
		n = len(boxes.GetDatas())
	}
	switch pendingLoadStatus(err, commitCtx, n) {
	case pendingCommitRetry:
		if err != nil {
			c.Logger.Errorf("pending webpage load: %v", err)
		} else if contextCanceled(commitCtx) {
			c.Logger.Errorf("pending webpage load: context %v", commitCtx.Err())
		}
		return pendingCommitRetry
	case pendingCommitGone:
		return pendingCommitGone
	}
	if len(boxes.GetDatas()) != 1 {
		c.Logger.Errorf("pending webpage load: unexpected box count %d", len(boxes.GetDatas()))
		return pendingCommitRetry
	}
	dst, _ := proto.Clone(boxes.Datas[0]).(*mtproto.MessageBox)
	if dst == nil || dst.Message == nil {
		return pendingCommitGone
	}
	if !webpage.IsPending(dst.Message.GetMedia()) {
		return pendingCommitDone
	}
	out := dst.Message
	webpage.ApplyFetchedPage(out, page)
	out.EditHide = true
	out.EditDate = &wrapperspb.Int32Value{Value: int32(time.Now().Unix())}
	if _, err = c.svcCtx.MsgClient.MsgEditMessageV2(commitCtx, &msgpb.TLMsgEditMessageV2{
		UserId:    job.UserID,
		AuthKeyId: job.AuthKeyID,
		PeerType:  job.PeerType,
		PeerId:    job.PeerID,
		EditType:  msgpb.EditTypeNormal,
		NewMessage: msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
			Message: out,
		}).To_OutboxMessage(),
		DstMessage: dst,
	}); err != nil {
		c.Logger.Errorf("pending webpage edit: %v", err)
		return pendingCommitRetry
	}
	return pendingCommitDone
}

func (c *MessagesCore) sweepStalePendingWebpages(now time.Time) int {
	return c.sweepPendingWebpages(now, true)
}

func (c *MessagesCore) sweepPendingWebpages(now time.Time, scanOrphans bool) int {
	// Ensure first so Due sees stored attempts and skips tombstones.
	// Raw orphan rows are not merged: an orphan hook would otherwise
	// reintroduce a capped job with Attempts=0 and loop forever.
	if scanOrphans {
		for _, job := range orphanPendingJobs(now) {
			ensurePendingJob(job)
		}
	}
	jobs := duePendingJobs(now)
	cleared := 0
	for _, job := range jobs {
		if pendingJobIsTombstone(job, now) {
			continue
		}
		if pendingJobShouldGiveUp(job, now) {
			// Last attempt: finish as webPageEmpty. Failure keeps a tombstone
			// so the orphan scan cannot requeue this card forever.
			status := c.commitPendingWebpage(nil, job, nil)
			if status == pendingCommitDone {
				untrackPendingJob(job.UserID, job.MsgID)
				cleared++
				continue
			}
			if status == pendingCommitGone {
				untrackPendingJob(job.UserID, job.MsgID)
				continue
			}
			tombstonePendingJob(job)
			pendingJobGiveUpLog(job, now, pendingJobGiveUpReason(job))
			continue
		}
		status := c.commitPendingWebpage(nil, job, nil)
		switch status {
		case pendingCommitDone:
			untrackPendingJob(job.UserID, job.MsgID)
			cleared++
		case pendingCommitGone:
			untrackPendingJob(job.UserID, job.MsgID)
		case pendingCommitRetry:
			// Already tried EditHide this tick. Cap → tombstone, do not
			// requeue through the orphan scan.
			updated, giveUp := failPendingJob(job, now)
			if giveUp {
				tombstonePendingJob(updated)
				pendingJobGiveUpLog(updated, now, pendingJobGiveUpReason(updated))
			}
		}
	}
	return cleared
}

func mergePendingJobs(due, orphans []PendingWebpageJob) []PendingWebpageJob {
	seen := make(map[[2]int64]struct{}, len(due)+len(orphans))
	out := make([]PendingWebpageJob, 0, len(due)+len(orphans))
	add := func(job PendingWebpageJob) {
		key := [2]int64{job.UserID, int64(job.MsgID)}
		if job.UserID == 0 || job.MsgID == 0 {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, job)
	}
	for _, job := range due {
		add(job)
	}
	for _, job := range orphans {
		add(job)
	}
	return out
}

// orphanScanDue is true only for the first (startup) sweep. After jobs
// persist in MySQL, Due() on pending_webpage_jobs (created_unix index)
// is the recovery path. A periodic LIKE '%webPagePending%' on messages
// is a full table scan and would not hold at 100k-user scale. 15s job
// ticks must not scan messages.
func orphanScanDue(last, now time.Time) bool {
	_ = now
	return last.IsZero()
}

// StartPendingWebpageSweep runs once at startup and then on a ticker.
// Due jobs older than pendingWebpageMaxAge (60s) are cleared with EditHide
// every pendingSweepEvery, limited to pendingSweepLimit rows, using the
// created_unix index. The messages orphan scan (LIKE '%webPagePending%')
// runs only at startup, as a fallback for cards that never got a job row.
func StartPendingWebpageSweep(ctx context.Context, run func(now time.Time) int) {
	if run == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	initPendingJobStore()
	threading.GoSafe(func() {
		run(time.Now())
		t := time.NewTicker(pendingSweepEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-t.C:
				run(now)
			}
		}
	})
}

// StartPendingWebpageSweepFrom starts the sweeper against a messages core.
func (c *MessagesCore) startPendingWebpageSweep(ctx context.Context) {
	if c == nil {
		return
	}
	var lastOrphan time.Time
	StartPendingWebpageSweep(ctx, func(now time.Time) int {
		scanOrphans := orphanScanDue(lastOrphan, now)
		if scanOrphans {
			lastOrphan = now
		}
		n := c.sweepPendingWebpages(now, scanOrphans)
		if n > 0 {
			logx.Infof("pending webpage sweep cleared %d stale card(s)", n)
		}
		return n
	})
}

var startPendingWebpageSweeperOnce sync.Once

// StartPendingWebpageSweeper loads the job store from the service MySQL
// DSN (Config.ChannelMySQL) and starts the due-job sweeper. Safe to call
// from both the standalone messages server and the all-in-one bff helper.
func StartPendingWebpageSweeper(svcCtx *svc.ServiceContext) {
	if svcCtx == nil {
		return
	}
	setPendingJobConfigDSN(svcCtx.Config.ChannelMySQL)
	startPendingWebpageSweeperOnce.Do(func() {
		c := New(context.Background(), svcCtx)
		c.startPendingWebpageSweep(context.Background())
	})
}

// shouldAutoAttachWebpage is the sendMessage policy: a client that already
// sent media (including inputMediaWebPage via sendMedia) or set no_webpage
// is left alone. invert_media lives on the message and is not used here.
func shouldAutoAttachWebpage(noWebpage bool, media *mtproto.MessageMedia) bool {
	return !noWebpage && media == nil
}
