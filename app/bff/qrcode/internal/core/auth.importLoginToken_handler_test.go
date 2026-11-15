package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
)

func TestImportLoginTokenRejectsUnknownToken(t *testing.T) {
	c := &QrCodeCore{
		ctx:    context.Background(),
		Logger: logx.WithContext(context.Background()),
	}
	cases := [][]byte{
		nil,
		{1, 2, 3, 4},
		make([]byte, 24),
	}
	for _, token := range cases {
		_, err := c.AuthImportLoginToken(&mtproto.TLAuthImportLoginToken{Token: token})
		if status.Convert(err).Message() != "AUTH_TOKEN_INVALID" {
			t.Fatalf("token len %d: %v", len(token), err)
		}
	}
}
