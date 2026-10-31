package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
)

func (s *Service) MessagesGetArchivedStickers(ctx context.Context, in *mtproto.TLMessagesGetArchivedStickers) (*mtproto.Messages_ArchivedStickers, error) {
	return cat().ArchivedStickers(userID(ctx), in.GetMasks(), in.GetEmojis(), in.GetOffsetId(), in.GetLimit()), nil
}

func (s *Service) MessagesGetAttachedStickers(ctx context.Context, in *mtproto.TLMessagesGetAttachedStickers) (*mtproto.Vector_StickerSetCovered, error) {
	return cat().AttachedStickers(in.GetMedia())
}

func (s *Service) MessagesGetMyStickers(ctx context.Context, in *mtproto.TLMessagesGetMyStickers) (*mtproto.Messages_MyStickers, error) {
	return cat().MyStickers(userID(ctx), in.GetOffsetId(), in.GetLimit()), nil
}

func (s *Service) MessagesReorderStickerSets(ctx context.Context, in *mtproto.TLMessagesReorderStickerSets) (*mtproto.Bool, error) {
	if err := cat().Reorder(userID(ctx), in.GetMasks(), in.GetEmojis(), in.GetOrder()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) MessagesToggleStickerSets(ctx context.Context, in *mtproto.TLMessagesToggleStickerSets) (*mtproto.Bool, error) {
	if err := cat().Toggle(userID(ctx), in.GetUninstall(), in.GetArchive(), in.GetUnarchive(), in.GetStickersets()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) MessagesGetSavedGifs(ctx context.Context, in *mtproto.TLMessagesGetSavedGifs) (*mtproto.Messages_SavedGifs, error) {
	return cat().SavedGifs(userID(ctx), in.GetHash()), nil
}

func (s *Service) MessagesSaveGif(ctx context.Context, in *mtproto.TLMessagesSaveGif) (*mtproto.Bool, error) {
	if err := cat().SaveGif(userID(ctx), in.GetId(), mtproto.FromBool(in.GetUnsave())); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) StickersCreateStickerSet(ctx context.Context, in *mtproto.TLStickersCreateStickerSet) (*mtproto.Messages_StickerSet, error) {
	return cat().CreateSet(catalog.CreateInput{
		Self:      userID(ctx),
		Owner:     in.GetUserId(),
		Title:     in.GetTitle(),
		ShortName: in.GetShortName(),
		Masks:     in.GetMasks(),
		Emojis:    in.GetEmojis(),
		TextColor: in.GetTextColor(),
		Animated:  in.GetAnimated(),
		Videos:    in.GetVideos(),
		Thumb:     in.GetThumb(),
		Stickers:  in.GetStickers(),
	})
}

func (s *Service) StickersAddStickerToSet(ctx context.Context, in *mtproto.TLStickersAddStickerToSet) (*mtproto.Messages_StickerSet, error) {
	return cat().AddSticker(userID(ctx), in.GetStickerset(), in.GetSticker())
}

func (s *Service) StickersRemoveStickerFromSet(ctx context.Context, in *mtproto.TLStickersRemoveStickerFromSet) (*mtproto.Messages_StickerSet, error) {
	return cat().RemoveSticker(userID(ctx), in.GetSticker())
}

func (s *Service) StickersChangeStickerPosition(ctx context.Context, in *mtproto.TLStickersChangeStickerPosition) (*mtproto.Messages_StickerSet, error) {
	return cat().ChangePosition(userID(ctx), in.GetSticker(), in.GetPosition())
}

func (s *Service) StickersSetStickerSetThumb(ctx context.Context, in *mtproto.TLStickersSetStickerSetThumb) (*mtproto.Messages_StickerSet, error) {
	if in == nil {
		return nil, mtproto.ErrStickersetInvalid
	}
	return cat().SetThumb(userID(ctx), in.GetStickerset(), in.GetThumb(), in.GetThumbDocumentId())
}

func (s *Service) StickersChangeSticker(ctx context.Context, in *mtproto.TLStickersChangeSticker) (*mtproto.Messages_StickerSet, error) {
	if in == nil {
		return nil, mtproto.ErrStickerDocumentInvalid
	}
	return cat().ChangeSticker(userID(ctx), in.GetSticker(), in.GetEmoji(), in.GetMaskCoords(), in.GetKeywords())
}

func (s *Service) StickersReplaceSticker(ctx context.Context, in *mtproto.TLStickersReplaceSticker) (*mtproto.Messages_StickerSet, error) {
	return cat().ReplaceSticker(userID(ctx), in.GetSticker(), in.GetNewSticker())
}

func (s *Service) StickersRenameStickerSet(ctx context.Context, in *mtproto.TLStickersRenameStickerSet) (*mtproto.Messages_StickerSet, error) {
	return cat().RenameSet(userID(ctx), in.GetStickerset(), in.GetTitle())
}

func (s *Service) StickersDeleteStickerSet(ctx context.Context, in *mtproto.TLStickersDeleteStickerSet) (*mtproto.Bool, error) {
	if err := cat().DeleteSet(userID(ctx), in.GetStickerset()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) StickersCheckShortName(ctx context.Context, in *mtproto.TLStickersCheckShortName) (*mtproto.Bool, error) {
	if err := cat().CheckShortName(in.GetShortName()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) StickersSuggestShortName(ctx context.Context, in *mtproto.TLStickersSuggestShortName) (*mtproto.Stickers_SuggestedShortName, error) {
	return cat().SuggestShortName(in.GetTitle()), nil
}
