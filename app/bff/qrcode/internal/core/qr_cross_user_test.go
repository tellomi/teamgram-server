package core

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	mkv "github.com/teamgram/marmota/pkg/stores/kv"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/qrcode/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/qrcode/internal/model"
	"github.com/teamgram/teamgram-server/app/bff/qrcode/internal/svc"
	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

func TestIndependentOtherUserCannotResumeAcceptedQR(t *testing.T) {
	mr := miniredis.RunT(t)
	store := mkv.NewStore(cache.ClusterConf{{
		RedisConf: redis.RedisConf{Host: mr.Addr(), Type: redis.NodeType},
		Weight:    100,
	}})
	const keyID int64 = 9001
	exp := time.Now().Unix() + 3600
	code := &model.QRCodeTransaction{
		PermAuthKeyId: keyID,
		AuthKeyId:     100,
		CodeHash:      "hash",
		ExpireAt:      exp,
		UserId:        42,
		State:         model.QRCodeStateAccepted,
	}
	ctx := context.Background()
	if err := store.HmsetCtx(ctx, "qr_codes_"+strconv.FormatInt(keyID, 10), map[string]string{
		"perm_auth_key_id": strconv.FormatInt(keyID, 10),
		"auth_key_id":      "100",
		"code_hash":        "hash",
		"expire_at":        strconv.FormatInt(exp, 10),
		"user_id":          "42",
		"state":            strconv.Itoa(model.QRCodeStateAccepted),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetCtx(ctx, "qr_claim_"+strconv.FormatInt(keyID, 10), "42:1"); err != nil {
		t.Fatal(err)
	}
	c := New(ctx, &svc.ServiceContext{Dao: dao.NewForTest(store)})
	c.MD = &metadata.RpcMetadata{UserId: 43}
	_, err := c.AuthAcceptLoginToken(&mtproto.TLAuthAcceptLoginToken{Token: code.Token()})
	if err == nil {
		t.Fatal("user 43 resumed user 42's accepted token")
	}
	if got, _ := store.GetCtx(ctx, "qr_claim_"+strconv.FormatInt(keyID, 10)); got != "42:1" {
		t.Fatalf("claim changed to %q", got)
	}
}
