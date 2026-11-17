package core

import (
	"context"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	"github.com/teamgram/teamgram-server/pkg/stagelog"
)

// inboxFanoutLimit is how many inbox publishes one send runs at once.
// Each publish waits for a Kafka ack. A group used to do that once per
// member, in order, on the sender's RPC.
const inboxFanoutLimit = 8

type inboxPublisher interface {
	InboxSendUserMessageToInboxV2(ctx context.Context, in *inbox.TLInboxSendUserMessageToInboxV2) (*mtproto.Void, error)
}

// publishInboxFanout delivers every inbox request before returning.
// One failure fails the send. Members already accepted by Kafka stay
// accepted, which is the same partial outcome as the old sequential walk.
//
// Kafka key is the inbox owner (InboxSendUserMessageToInboxV2.user_id),
// not the dialog. The sender copy and the recipient copy are different
// keys, and each group member is its own key, so one send publishes at
// most one record per partition. Parallel acks do not reorder that
// partition. The next send from this auth key publishes only after this
// call returns, and same-peer sends stay in client order on the session
// lane, so successive records for one recipient stay in that order.
func publishInboxFanout(ctx context.Context, cli inboxPublisher, reqs []*inbox.TLInboxSendUserMessageToInboxV2) error {
	if len(reqs) == 0 {
		return nil
	}
	started := time.Now()
	var err error
	if len(reqs) == 1 {
		if cli == nil {
			err = mtproto.ErrInternalServerError
		} else {
			_, err = cli.InboxSendUserMessageToInboxV2(ctx, reqs[0])
		}
	} else {
		err = publishInboxParallel(ctx, cli, reqs)
	}
	stagelog.Infof("msg kind=inbox_fanout n=%d ms=%d err=%t", len(reqs), time.Since(started).Milliseconds(), err != nil)
	return err
}

func publishInboxParallel(ctx context.Context, cli inboxPublisher, reqs []*inbox.TLInboxSendUserMessageToInboxV2) error {
	if cli == nil {
		return mtproto.ErrInternalServerError
	}
	sem := make(chan struct{}, inboxFanoutLimit)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var first error
	for _, req := range reqs {
		req := req
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			_, err := cli.InboxSendUserMessageToInboxV2(ctx, req)
			if err != nil {
				mu.Lock()
				if first == nil {
					first = err
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return first
}
