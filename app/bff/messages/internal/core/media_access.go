package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
	mediapb "github.com/teamgram/teamgram-server/app/service/media/media"
	"github.com/teamgram/teamgram-server/pkg/livephoto"
)

// authorizeReferencedMedia checks inputMediaPhoto and inputMediaDocument
// before random_id dedupe. A resend joins an in-flight send only after the
// access_hash on this request has been accepted. A wrong hash is
// MEDIA_INVALID and does not receive another call's cached updates.
func (c *MessagesCore) authorizeReferencedMedia(media *mtproto.InputMedia) error {
	if media == nil {
		return nil
	}
	switch media.GetPredicateName() {
	case mtproto.Predicate_inputMediaPhoto:
		_, err := catalog.ResolveInputPhoto(media.GetId_INPUTPHOTO(), func() (*mtproto.Photo, error) {
			return c.fetchPhoto(media.GetId_INPUTPHOTO().GetId())
		})
		if err != nil {
			return err
		}
		// A live photo also references its video document.
		_, err = livephoto.Video(media, c.resolveInputDocument)
		return err
	case mtproto.Predicate_inputMediaUploadedPhoto:
		_, err := livephoto.Video(media, c.resolveInputDocument)
		return err
	case mtproto.Predicate_inputMediaDocument:
		_, err := catalog.ResolveInputDocument(media.GetId_INPUTDOCUMENT(), func() (*mtproto.Document, error) {
			return c.fetchDocument(media.GetId_INPUTDOCUMENT().GetId())
		})
		return err
	default:
		return nil
	}
}

// resolveInputDocument checks access_hash and file_reference like
// inputMediaDocument and returns the stored document.
func (c *MessagesCore) resolveInputDocument(in *mtproto.InputDocument) (*mtproto.Document, error) {
	return catalog.ResolveInputDocument(in, func() (*mtproto.Document, error) {
		return c.fetchDocument(in.GetId())
	})
}

func (c *MessagesCore) fetchPhoto(id int64) (*mtproto.Photo, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MediaClient == nil || id == 0 {
		return nil, mtproto.ErrMediaInvalid
	}
	return c.svcCtx.Dao.MediaGetPhoto(c.ctx, &mediapb.TLMediaGetPhoto{PhotoId: id})
}

func (c *MessagesCore) fetchDocument(id int64) (*mtproto.Document, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MediaClient == nil || id == 0 {
		return nil, mtproto.ErrMediaInvalid
	}
	return c.svcCtx.Dao.MediaGetDocument(c.ctx, &mediapb.TLMediaGetDocument{Id: id})
}
