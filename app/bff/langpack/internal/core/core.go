package core

import (
	"context"
	"strings"

	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/langpack/internal/svc"
	"github.com/teamgram/teamgram-server/pkg/langpack"
	"github.com/zeromicro/go-zero/core/logx"
)

type LangpackCore struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
	MD *metadata.RpcMetadata
}

func New(ctx context.Context, svcCtx *svc.ServiceContext) *LangpackCore {
	return &LangpackCore{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
		MD:     metadata.RpcMetadataFromIncoming(ctx),
	}
}

func (c *LangpackCore) catalog() *langpack.Catalog {
	if c.svcCtx == nil {
		return langpack.DefaultCatalog()
	}
	return c.svcCtx.Catalog
}

func (c *LangpackCore) resolvePack(requested string) string {
	if strings.TrimSpace(requested) != "" {
		return langpack.NormalizePlatform(requested)
	}
	if c.MD != nil && strings.TrimSpace(c.MD.GetLangpack()) != "" {
		return langpack.NormalizePlatform(c.MD.GetLangpack())
	}
	return ""
}

func (c *LangpackCore) resolveLang(requested string) string {
	if strings.TrimSpace(requested) != "" {
		if code, ok := langpack.CanonicalLang(requested); ok {
			return code
		}
		return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(requested), "_", "-"))
	}
	if c.MD != nil {
		if code, ok := langpack.CanonicalLang(c.MD.GetLangCode()); ok {
			return code
		}
	}
	return "en"
}
