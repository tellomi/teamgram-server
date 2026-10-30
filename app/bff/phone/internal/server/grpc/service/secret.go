package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
)

// MessagesReceivedQueue is messages.receivedQueue. iOS calls it from
// AccountStateManager.reset while applying state, so it has to return
// immediately. There is no secret-chat queue on this server. The empty
// vector is the same payload the session used when the method was unrouted,
// and this handler does not touch Redis or MySQL.
func (s *Service) MessagesReceivedQueue(ctx context.Context, _ *mtproto.TLMessagesReceivedQueue) (*mtproto.Vector_Long, error) {
	if _, err := s.md(ctx); err != nil {
		return nil, err
	}
	return &mtproto.Vector_Long{Datas: []int64{}}, nil
}
