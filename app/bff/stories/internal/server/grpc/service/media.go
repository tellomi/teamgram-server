package service

import (
	"context"
	"time"

	"github.com/teamgram/proto/mtproto"
	mediapb "github.com/teamgram/teamgram-server/app/service/media/media"
)

// MediaAPI is the slice of the media service stories use for photos and videos.
type MediaAPI interface {
	MediaUploadPhotoFile(ctx context.Context, in *mediapb.TLMediaUploadPhotoFile) (*mtproto.Photo, error)
	MediaGetPhotoSizeList(ctx context.Context, in *mediapb.TLMediaGetPhotoSizeList) (*mediapb.PhotoSizeList, error)
	MediaUploadedDocumentMedia(ctx context.Context, in *mediapb.TLMediaUploadedDocumentMedia) (*mtproto.MessageMedia, error)
	MediaGetDocument(ctx context.Context, in *mediapb.TLMediaGetDocument) (*mtproto.Document, error)
}

// ResolveMedia turns an InputMedia into the MessageMedia stored on a story.
// Uploaded photos and videos go through the same media service messages use.
func ResolveMedia(ctx context.Context, owner int64, in *mtproto.InputMedia, api MediaAPI) (*mtproto.MessageMedia, error) {
	if in == nil {
		return nil, mtproto.ErrMediaInvalid
	}
	now := int32(time.Now().Unix())
	switch in.PredicateName {
	case mtproto.Predicate_inputMediaEmpty:
		return mtproto.MakeTLMessageMediaEmpty(nil).To_MessageMedia(), nil
	case mtproto.Predicate_inputMediaUploadedPhoto:
		if api == nil {
			return nil, mtproto.ErrMediaInvalid
		}
		photo, err := api.MediaUploadPhotoFile(ctx, &mediapb.TLMediaUploadPhotoFile{
			OwnerId:    owner,
			File:       in.File,
			Stickers:   in.Stickers,
			TtlSeconds: in.TtlSeconds,
		})
		if err != nil {
			return nil, err
		}
		return mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{
			Spoiler:         in.Spoiler,
			Photo_FLAGPHOTO: photo,
			TtlSeconds:      in.TtlSeconds,
		}).To_MessageMedia(), nil
	case mtproto.Predicate_inputMediaPhoto:
		photoIn := in.To_InputMediaPhoto().GetId_INPUTPHOTO()
		if photoIn == nil {
			return nil, mtproto.ErrMediaInvalid
		}
		photo := mtproto.MakeTLPhoto(&mtproto.Photo{
			Id:         photoIn.GetId(),
			AccessHash: photoIn.GetAccessHash(),
			Date:       now,
			DcId:       1,
		}).To_Photo()
		if api != nil {
			if sizes, err := api.MediaGetPhotoSizeList(ctx, &mediapb.TLMediaGetPhotoSizeList{SizeId: photoIn.GetId()}); err == nil && sizes != nil {
				photo.Sizes = sizes.Sizes
				photo.DcId = sizes.DcId
			}
		}
		return mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{
			Spoiler:         in.GetSpoiler(),
			Photo_FLAGPHOTO: photo,
			TtlSeconds:      in.GetTtlSeconds(),
		}).To_MessageMedia(), nil
	case mtproto.Predicate_inputMediaUploadedDocument:
		if api == nil {
			return nil, mtproto.ErrMediaInvalid
		}
		media, err := api.MediaUploadedDocumentMedia(ctx, &mediapb.TLMediaUploadedDocumentMedia{
			OwnerId: owner,
			Media:   in,
		})
		if err != nil {
			return nil, mtproto.ErrMediaInvalid
		}
		return media, nil
	case mtproto.Predicate_inputMediaDocument:
		docIn := in.To_InputMediaDocument().GetId_INPUTDOCUMENT()
		if docIn == nil {
			return nil, mtproto.ErrMediaInvalid
		}
		var doc *mtproto.Document
		if api != nil {
			doc, _ = api.MediaGetDocument(ctx, &mediapb.TLMediaGetDocument{Id: docIn.GetId()})
		}
		if doc == nil {
			doc = stubDocument(docIn.GetId(), docIn.GetAccessHash(), "video/mp4", now)
		}
		return mtproto.MakeTLMessageMediaDocument(&mtproto.MessageMedia{
			Spoiler:    in.GetSpoiler(),
			Document:   doc,
			TtlSeconds: in.GetTtlSeconds(),
		}).To_MessageMedia(), nil
	default:
		return nil, mtproto.ErrMediaInvalid
	}
}

func stubDocument(id, access int64, mime string, date int32) *mtproto.Document {
	return mtproto.MakeTLDocument(&mtproto.Document{
		Id:         id,
		AccessHash: access,
		Date:       date,
		MimeType:   mime,
		DcId:       1,
		Attributes: []*mtproto.DocumentAttribute{},
	}).To_Document()
}

func (s *Service) mediaOf(ctx context.Context, owner int64, in *mtproto.InputMedia) (*mtproto.MessageMedia, error) {
	var api MediaAPI
	if s.svcCtx != nil {
		api = s.svcCtx.MediaClient
	}
	return ResolveMedia(ctx, owner, in, api)
}

func (s *Service) musicOf(ctx context.Context, in *mtproto.InputDocument) *mtproto.Document {
	if in == nil || in.GetId() == 0 {
		return nil
	}
	if s.svcCtx != nil && s.svcCtx.MediaClient != nil {
		if doc, err := s.svcCtx.MediaClient.MediaGetDocument(ctx, &mediapb.TLMediaGetDocument{Id: in.GetId()}); err == nil && doc != nil {
			return doc
		}
	}
	return stubDocument(in.GetId(), in.GetAccessHash(), "audio/mpeg", int32(time.Now().Unix()))
}
