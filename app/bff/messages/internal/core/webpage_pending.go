package core

import (
	"context"
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
	// A fetch error, a still-pending result, or an empty page must not
	// leave webPagePending on the message. Clear the card with EditHide.
	if err != nil || page.GetPredicateName() != mtproto.Predicate_webPage {
		if err != nil {
			c.Logger.Errorf("pending webpage fetch: %v", err)
		}
		page = nil
	}
	if c.commitPendingWebpage(ctx, job, page) {
		untrackPendingJob(job.UserID, job.MsgID)
	}
}

func (c *MessagesCore) commitPendingWebpage(ctx context.Context, job PendingWebpageJob, page *mtproto.WebPage) bool {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.MessageClient == nil || c.svcCtx.MsgClient == nil {
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	boxes, err := c.svcCtx.MessageClient.MessageGetUserMessageList(ctx, &message.TLMessageGetUserMessageList{
		UserId: job.UserID,
		IdList: []int32{job.MsgID},
	})
	if err != nil || boxes == nil || len(boxes.GetDatas()) != 1 {
		return false
	}
	dst, _ := proto.Clone(boxes.Datas[0]).(*mtproto.MessageBox)
	if dst == nil || dst.Message == nil {
		return false
	}
	if !webpage.IsPending(dst.Message.GetMedia()) {
		return true
	}
	out := dst.Message
	webpage.ApplyFetchedPage(out, page)
	out.EditHide = true
	out.EditDate = &wrapperspb.Int32Value{Value: int32(time.Now().Unix())}
	if _, err = c.svcCtx.MsgClient.MsgEditMessageV2(ctx, &msgpb.TLMsgEditMessageV2{
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
		return false
	}
	return true
}

func (c *MessagesCore) sweepStalePendingWebpages(now time.Time) int {
	jobs := duePendingJobs(now)
	cleared := 0
	for _, job := range jobs {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		ok := c.commitPendingWebpage(ctx, job, nil)
		cancel()
		if ok {
			untrackPendingJob(job.UserID, job.MsgID)
			cleared++
		}
	}
	return cleared
}

// StartPendingWebpageSweep runs once at startup and then on a ticker.
// Jobs older than pendingWebpageMaxAge (60s) are cleared with EditHide,
// so a restart that drops an in-flight goroutine cannot leave
// webPagePending on a message forever. Each tick is limited to
// pendingSweepLimit rows, using the created_unix index.
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
	StartPendingWebpageSweep(ctx, func(now time.Time) int {
		n := c.sweepStalePendingWebpages(now)
		if n > 0 {
			logx.Infof("pending webpage sweep cleared %d stale card(s)", n)
		}
		return n
	})
}

// StartPendingWebpageSweeper loads the job store and starts the 60s safety net.
func StartPendingWebpageSweeper(svcCtx *svc.ServiceContext) {
	if svcCtx == nil {
		return
	}
	c := New(context.Background(), svcCtx)
	c.startPendingWebpageSweep(context.Background())
}

// shouldAutoAttachWebpage is the sendMessage policy: a client that already
// sent media (including inputMediaWebPage via sendMedia) or set no_webpage
// is left alone. invert_media lives on the message and is not used here.
func shouldAutoAttachWebpage(noWebpage bool, media *mtproto.MessageMedia) bool {
	return !noWebpage && media == nil
}
