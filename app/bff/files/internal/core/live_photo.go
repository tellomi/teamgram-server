package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
	mediapb "github.com/teamgram/teamgram-server/app/service/media/media"
)

// resolveInputDocument resolves a live photo video for messages.uploadMedia
// with the same access check as inputMediaDocument.
func (c *FilesCore) resolveInputDocument(in *mtproto.InputDocument) (*mtproto.Document, error) {
	return catalog.ResolveInputDocument(in, func() (*mtproto.Document, error) {
		if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MediaClient == nil || in.GetId() == 0 {
			return nil, mtproto.ErrMediaInvalid
		}
		return c.svcCtx.Dao.MediaClient.MediaGetDocument(c.ctx, &mediapb.TLMediaGetDocument{Id: in.GetId()})
	})
}
