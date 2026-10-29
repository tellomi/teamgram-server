package service

import (
	"context"
	"crypto/rand"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/dh"
)

func (s *Service) MessagesGetDhConfig(ctx context.Context, in *mtproto.TLMessagesGetDhConfig) (*mtproto.Messages_DhConfig, error) {
	if _, err := s.md(ctx); err != nil {
		return nil, err
	}
	n := in.GetRandomLength()
	if n < 0 || n > 256 {
		return nil, mtproto.ErrRandomLengthInvalid
	}
	buf := make([]byte, n)
	if n > 0 {
		if _, err := rand.Read(buf); err != nil {
			return nil, err
		}
	}
	if in.GetVersion() == dh.Version {
		return mtproto.MakeTLMessagesDhConfigNotModified(&mtproto.Messages_DhConfig{Random: buf}).To_Messages_DhConfig(), nil
	}
	return mtproto.MakeTLMessagesDhConfig(&mtproto.Messages_DhConfig{
		G: dh.G, P: dh.PrimeBytes(), Version: dh.Version, Random: buf,
	}).To_Messages_DhConfig(), nil
}
