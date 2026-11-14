package usercopy

import (
	"context"
	"strings"

	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/pkg/langpack"
)

// LangFromIncoming reads initConnection.lang_code, then system_lang_code,
// then lang_pack, from the RPC metadata on ctx. An empty result means the
// request did not say which language it is, and callers keep their previous
// text instead of guessing English.
func LangFromIncoming(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	md := metadata.RpcMetadataFromIncoming(ctx)
	if md == nil {
		return ""
	}
	if strings.TrimSpace(md.GetLangCode()) == "" && strings.TrimSpace(md.GetSystemLangCode()) == "" {
		if code, ok := langpack.CanonicalLang(md.GetLangpack()); ok && code != "" {
			return code
		}
		return ""
	}
	return langpack.ClientLang(md.GetLangCode(), md.GetSystemLangCode(), md.GetLangpack())
}
