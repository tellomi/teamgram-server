package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/accountassets"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
)

func userID(ctx context.Context) int64 {
	md := metadata.RpcMetadataFromIncoming(ctx)
	if md == nil {
		return 0
	}
	return md.GetUserId()
}

func cat() *catalog.Service { return catalog.Must() }

func (s *Service) MessagesGetAllStickers(ctx context.Context, in *mtproto.TLMessagesGetAllStickers) (*mtproto.Messages_AllStickers, error) {
	return cat().AllStickers(userID(ctx), in.GetHash()), nil
}

func (s *Service) MessagesGetMaskStickers(ctx context.Context, in *mtproto.TLMessagesGetMaskStickers) (*mtproto.Messages_AllStickers, error) {
	return cat().MaskStickers(userID(ctx), in.GetHash()), nil
}

func (s *Service) MessagesGetStickerSet(ctx context.Context, in *mtproto.TLMessagesGetStickerSet) (*mtproto.Messages_StickerSet, error) {
	return cat().GetSet(userID(ctx), in.GetStickerset(), in.GetHash())
}

func (s *Service) MessagesInstallStickerSet(ctx context.Context, in *mtproto.TLMessagesInstallStickerSet) (*mtproto.Messages_StickerSetInstallResult, error) {
	return cat().Install(userID(ctx), in.GetStickerset(), mtproto.FromBool(in.GetArchived()))
}

func (s *Service) MessagesUninstallStickerSet(ctx context.Context, in *mtproto.TLMessagesUninstallStickerSet) (*mtproto.Bool, error) {
	if err := cat().Uninstall(userID(ctx), in.GetStickerset()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) MessagesGetFeaturedStickers(ctx context.Context, in *mtproto.TLMessagesGetFeaturedStickers) (*mtproto.Messages_FeaturedStickers, error) {
	return cat().Featured(userID(ctx), in.GetHash(), false), nil
}

func (s *Service) MessagesGetOldFeaturedStickers(ctx context.Context, in *mtproto.TLMessagesGetOldFeaturedStickers) (*mtproto.Messages_FeaturedStickers, error) {
	return cat().Featured(userID(ctx), in.GetHash(), false), nil
}

func (s *Service) MessagesReadFeaturedStickers(ctx context.Context, in *mtproto.TLMessagesReadFeaturedStickers) (*mtproto.Bool, error) {
	if err := cat().ReadFeatured(userID(ctx), in.GetId()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) MessagesGetRecentStickers(ctx context.Context, in *mtproto.TLMessagesGetRecentStickers) (*mtproto.Messages_RecentStickers, error) {
	return cat().Recent(userID(ctx), in.GetHash(), in.GetAttached()), nil
}

func (s *Service) MessagesSaveRecentSticker(ctx context.Context, in *mtproto.TLMessagesSaveRecentSticker) (*mtproto.Bool, error) {
	if err := cat().SaveRecent(userID(ctx), in.GetId().GetId(), in.GetAttached(), mtproto.FromBool(in.GetUnsave())); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) MessagesClearRecentStickers(ctx context.Context, in *mtproto.TLMessagesClearRecentStickers) (*mtproto.Bool, error) {
	if err := cat().ClearRecent(userID(ctx), in.GetAttached()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) MessagesGetFavedStickers(ctx context.Context, in *mtproto.TLMessagesGetFavedStickers) (*mtproto.Messages_FavedStickers, error) {
	return cat().Faved(userID(ctx), in.GetHash()), nil
}

func (s *Service) MessagesFaveSticker(ctx context.Context, in *mtproto.TLMessagesFaveSticker) (*mtproto.Bool, error) {
	if err := cat().Fave(userID(ctx), in.GetId().GetId(), mtproto.FromBool(in.GetUnfave())); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) MessagesSearchStickerSets(ctx context.Context, in *mtproto.TLMessagesSearchStickerSets) (*mtproto.Messages_FoundStickerSets, error) {
	return cat().SearchSets(in.GetQ(), in.GetHash(), false), nil
}

func (s *Service) MessagesSearchEmojiStickerSets(ctx context.Context, in *mtproto.TLMessagesSearchEmojiStickerSets) (*mtproto.Messages_FoundStickerSets, error) {
	return cat().SearchSets(in.GetQ(), in.GetHash(), true), nil
}

func (s *Service) MessagesGetStickers(ctx context.Context, in *mtproto.TLMessagesGetStickers) (*mtproto.Messages_Stickers, error) {
	return cat().StickersByEmoji(in.GetEmoticon(), in.GetHash(), false), nil
}

func (s *Service) MessagesSearchStickers(ctx context.Context, in *mtproto.TLMessagesSearchStickers) (*mtproto.Messages_FoundStickers, error) {
	q := in.GetQ()
	if q == "" {
		q = in.GetEmoticon()
	}
	return cat().SearchDocuments(q, in.GetEmojis(), in.GetHash()), nil
}

func (s *Service) MessagesGetEmojiKeywords(ctx context.Context, in *mtproto.TLMessagesGetEmojiKeywords) (*mtproto.EmojiKeywordsDifference, error) {
	return cat().Keywords(in.GetLangCode(), 0), nil
}

func (s *Service) MessagesGetEmojiKeywordsDifference(ctx context.Context, in *mtproto.TLMessagesGetEmojiKeywordsDifference) (*mtproto.EmojiKeywordsDifference, error) {
	return cat().Keywords(in.GetLangCode(), in.GetFromVersion()), nil
}

func (s *Service) MessagesGetEmojiKeywordsLanguages(ctx context.Context, in *mtproto.TLMessagesGetEmojiKeywordsLanguages) (*mtproto.Vector_EmojiLanguage, error) {
	return cat().Languages(), nil
}

func (s *Service) MessagesGetEmojiURL(ctx context.Context, in *mtproto.TLMessagesGetEmojiURL) (*mtproto.EmojiURL, error) {
	return cat().EmojiURL(in.GetLangCode())
}

func (s *Service) MessagesGetAvailableEffects(ctx context.Context, in *mtproto.TLMessagesGetAvailableEffects) (*mtproto.Messages_AvailableEffects, error) {
	return cat().AvailableEffects(in.GetHash()), nil
}

func (s *Service) MessagesGetCustomEmojiDocuments(ctx context.Context, in *mtproto.TLMessagesGetCustomEmojiDocuments) (*mtproto.Vector_Document, error) {
	return cat().Documents(in.GetDocumentId()), nil
}

func (s *Service) MessagesGetEmojiStickers(ctx context.Context, in *mtproto.TLMessagesGetEmojiStickers) (*mtproto.Messages_AllStickers, error) {
	return cat().EmojiStickers(userID(ctx), in.GetHash()), nil
}

func (s *Service) MessagesGetFeaturedEmojiStickers(ctx context.Context, in *mtproto.TLMessagesGetFeaturedEmojiStickers) (*mtproto.Messages_FeaturedStickers, error) {
	return cat().Featured(userID(ctx), in.GetHash(), true), nil
}

func (s *Service) MessagesSearchCustomEmoji(ctx context.Context, in *mtproto.TLMessagesSearchCustomEmoji) (*mtproto.EmojiList, error) {
	ids := cat().CustomEmojiIDs(in.GetEmoticon())
	if ids == nil {
		ids = []int64{}
	}
	return mtproto.MakeTLEmojiList(&mtproto.EmojiList{Hash: 1, DocumentId: ids}).To_EmojiList(), nil
}

func (s *Service) AccountGetDefaultProfilePhotoEmojis(_ context.Context, in *mtproto.TLAccountGetDefaultProfilePhotoEmojis) (*mtproto.EmojiList, error) {
	return accountassets.EmojiList(in.GetHash()), nil
}

func (s *Service) AccountGetDefaultGroupPhotoEmojis(_ context.Context, in *mtproto.TLAccountGetDefaultGroupPhotoEmojis) (*mtproto.EmojiList, error) {
	return accountassets.EmojiList(in.GetHash()), nil
}
