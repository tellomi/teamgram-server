package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
)

func (s *Service) PaymentsGetStarGifts(ctx context.Context, in *mtproto.TLPaymentsGetStarGifts) (*mtproto.Payments_StarGifts, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var hash int32
	if in != nil {
		hash = in.GetHash()
	}
	return s.svcCtx.Book.StarGifts(id, hash)
}

func (s *Service) PaymentsCheckCanSendGift(ctx context.Context, in *mtproto.TLPaymentsCheckCanSendGift) (*mtproto.Payments_CheckCanSendGiftResult, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var giftID int64
	if in != nil {
		giftID = in.GetGiftId()
	}
	return s.svcCtx.Book.CheckCanSendGift(id, giftID)
}

func (s *Service) PaymentsGetSavedStarGifts(ctx context.Context, in *mtproto.TLPaymentsGetSavedStarGifts) (*mtproto.Payments_SavedStarGifts, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.SavedStarGifts(id, in)
}

func (s *Service) PaymentsGetSavedStarGift(ctx context.Context, in *mtproto.TLPaymentsGetSavedStarGift) (*mtproto.Payments_SavedStarGifts, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var gifts []*mtproto.InputSavedStarGift
	if in != nil {
		gifts = in.GetStargift()
	}
	return s.svcCtx.Book.SavedStarGift(id, gifts)
}

func (s *Service) PaymentsGetCraftStarGifts(ctx context.Context, in *mtproto.TLPaymentsGetCraftStarGifts) (*mtproto.Payments_SavedStarGifts, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var giftID int64
	offset := ""
	var limit int32
	if in != nil {
		giftID = in.GetGiftId()
		offset = in.GetOffset()
		limit = in.GetLimit()
	}
	return s.svcCtx.Book.CraftStarGifts(id, giftID, offset, limit)
}

func (s *Service) PaymentsGetResaleStarGifts(ctx context.Context, in *mtproto.TLPaymentsGetResaleStarGifts) (*mtproto.Payments_ResaleStarGifts, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.ResaleStarGifts(id, in)
}

func (s *Service) PaymentsGetStarGiftUpgradePreview(ctx context.Context, in *mtproto.TLPaymentsGetStarGiftUpgradePreview) (*mtproto.Payments_StarGiftUpgradePreview, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var giftID int64
	if in != nil {
		giftID = in.GetGiftId()
	}
	return s.svcCtx.Book.UpgradePreview(id, giftID)
}

func (s *Service) PaymentsGetStarGiftUpgradeAttributes(ctx context.Context, in *mtproto.TLPaymentsGetStarGiftUpgradeAttributes) (*mtproto.Payments_StarGiftUpgradeAttributes, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var giftID int64
	if in != nil {
		giftID = in.GetGiftId()
	}
	return s.svcCtx.Book.UpgradeAttributes(id, giftID)
}

func (s *Service) PaymentsGetStarGiftActiveAuctions(ctx context.Context, in *mtproto.TLPaymentsGetStarGiftActiveAuctions) (*mtproto.Payments_StarGiftActiveAuctions, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var hash int64
	if in != nil {
		hash = in.GetHash()
	}
	return s.svcCtx.Book.ActiveAuctions(id, hash)
}

func (s *Service) PaymentsGetStarGiftAuctionAcquiredGifts(ctx context.Context, in *mtproto.TLPaymentsGetStarGiftAuctionAcquiredGifts) (*mtproto.Payments_StarGiftAuctionAcquiredGifts, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var giftID int64
	if in != nil {
		giftID = in.GetGiftId()
	}
	return s.svcCtx.Book.AuctionAcquired(id, giftID)
}

func (s *Service) PaymentsGetStarGiftAuctionState(ctx context.Context, in *mtproto.TLPaymentsGetStarGiftAuctionState) (*mtproto.Payments_StarGiftAuctionState, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var auction *mtproto.InputStarGiftAuction
	if in != nil {
		auction = in.GetAuction()
	}
	return nil, s.svcCtx.Book.AuctionState(id, auction)
}

func (s *Service) PaymentsGetUniqueStarGift(ctx context.Context, in *mtproto.TLPaymentsGetUniqueStarGift) (*mtproto.Payments_UniqueStarGift, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	slug := ""
	if in != nil {
		slug = in.GetSlug()
	}
	return nil, s.svcCtx.Book.UniqueStarGift(id, slug)
}

func (s *Service) PaymentsGetUniqueStarGiftValueInfo(ctx context.Context, in *mtproto.TLPaymentsGetUniqueStarGiftValueInfo) (*mtproto.Payments_UniqueStarGiftValueInfo, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	slug := ""
	if in != nil {
		slug = in.GetSlug()
	}
	return nil, s.svcCtx.Book.UniqueStarGiftValue(id, slug)
}

func (s *Service) PaymentsGetUserStarGifts(ctx context.Context, in *mtproto.TLPaymentsGetUserStarGifts) (*mtproto.Payments_UserStarGifts, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var user *mtproto.InputUser
	offset := ""
	var limit int32
	if in != nil {
		user = in.GetUserId()
		offset = in.GetOffset()
		limit = in.GetLimit()
	}
	return s.svcCtx.Book.UserStarGifts(id, user, offset, limit)
}

func (s *Service) PaymentsGetUserStarGift(ctx context.Context, in *mtproto.TLPaymentsGetUserStarGift) (*mtproto.Payments_UserStarGifts, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var ids []int32
	if in != nil {
		ids = in.GetMsgId()
	}
	return s.svcCtx.Book.UserStarGift(id, ids)
}

func (s *Service) PaymentsSaveStarGift(ctx context.Context, in *mtproto.TLPaymentsSaveStarGift) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var gift *mtproto.InputSavedStarGift
	if in != nil {
		gift = in.GetStargift()
	}
	return nil, s.svcCtx.Book.SaveStarGift(id, gift)
}

func (s *Service) PaymentsConvertStarGift(ctx context.Context, in *mtproto.TLPaymentsConvertStarGift) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var gift *mtproto.InputSavedStarGift
	if in != nil {
		gift = in.GetStargift()
	}
	return nil, s.svcCtx.Book.ConvertStarGift(id, gift)
}

func (s *Service) PaymentsUpgradeStarGift(ctx context.Context, in *mtproto.TLPaymentsUpgradeStarGift) (*mtproto.Updates, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var gift *mtproto.InputSavedStarGift
	if in != nil {
		gift = in.GetStargift()
	}
	return nil, s.svcCtx.Book.UpgradeStarGift(id, gift)
}

func (s *Service) PaymentsTransferStarGift(ctx context.Context, in *mtproto.TLPaymentsTransferStarGift) (*mtproto.Updates, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var gift *mtproto.InputSavedStarGift
	var to *mtproto.InputPeer
	var toUser *mtproto.InputUser
	if in != nil {
		gift = in.GetStargift()
		to = in.GetToId_INPUTPEER()
		toUser = in.GetToId_INPUTUSER()
	}
	return nil, s.svcCtx.Book.TransferStarGift(id, gift, to, toUser)
}

func (s *Service) PaymentsUpdateStarGiftPrice(ctx context.Context, in *mtproto.TLPaymentsUpdateStarGiftPrice) (*mtproto.Updates, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var gift *mtproto.InputSavedStarGift
	var amount *mtproto.StarsAmount
	var legacy int64
	if in != nil {
		gift = in.GetStargift()
		amount = in.GetResellAmount()
		legacy = in.GetResellStars()
	}
	return nil, s.svcCtx.Book.UpdateStarGiftPrice(id, gift, amount, legacy)
}

func (s *Service) PaymentsGetStarGiftWithdrawalUrl(ctx context.Context, in *mtproto.TLPaymentsGetStarGiftWithdrawalUrl) (*mtproto.Payments_StarGiftWithdrawalUrl, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var gift *mtproto.InputSavedStarGift
	var tmp []byte
	var password *mtproto.InputCheckPasswordSRP
	if in != nil {
		gift = in.GetStargift()
		tmp = in.GetTmpPassword()
		password = in.GetPassword()
	}
	return nil, s.svcCtx.Book.WithdrawalURL(id, gift, tmp, password)
}

func (s *Service) PaymentsToggleChatStarGiftNotifications(ctx context.Context, in *mtproto.TLPaymentsToggleChatStarGiftNotifications) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var peer *mtproto.InputPeer
	enabled := false
	if in != nil {
		peer = in.GetPeer()
		enabled = in.GetEnabled()
	}
	return s.svcCtx.Book.ToggleChatStarGiftNotifications(id, peer, enabled)
}

func (s *Service) PaymentsToggleStarGiftsPinnedToTop(ctx context.Context, in *mtproto.TLPaymentsToggleStarGiftsPinnedToTop) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var peer *mtproto.InputPeer
	var gifts []*mtproto.InputSavedStarGift
	if in != nil {
		peer = in.GetPeer()
		gifts = in.GetStargift()
	}
	if err := s.svcCtx.Book.ToggleStarGiftsPinned(id, peer, gifts); err != nil {
		return nil, err
	}
	return mtproto.ToBool(true), nil
}

func (s *Service) PaymentsCraftStarGift(ctx context.Context, in *mtproto.TLPaymentsCraftStarGift) (*mtproto.Updates, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var gifts []*mtproto.InputSavedStarGift
	if in != nil {
		gifts = in.GetStargift()
	}
	return nil, s.svcCtx.Book.CraftStarGift(id, gifts)
}

func (s *Service) PaymentsResolveStarGiftOffer(ctx context.Context, in *mtproto.TLPaymentsResolveStarGiftOffer) (*mtproto.Updates, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var msgID int32
	if in != nil {
		msgID = in.GetOfferMsgId()
	}
	return nil, s.svcCtx.Book.ResolveStarGiftOffer(id, msgID)
}

func (s *Service) PaymentsSendStarGiftOffer(ctx context.Context, in *mtproto.TLPaymentsSendStarGiftOffer) (*mtproto.Updates, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return nil, s.svcCtx.Book.SendStarGiftOffer(id, in)
}

func (s *Service) PaymentsCreateStarGiftCollection(ctx context.Context, in *mtproto.TLPaymentsCreateStarGiftCollection) (*mtproto.StarGiftCollection, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var peer *mtproto.InputPeer
	title := ""
	var gifts []*mtproto.InputSavedStarGift
	if in != nil {
		peer = in.GetPeer()
		title = in.GetTitle()
		gifts = in.GetStargift()
	}
	return s.svcCtx.Book.CreateStarGiftCollection(id, peer, title, gifts)
}

func (s *Service) PaymentsUpdateStarGiftCollection(ctx context.Context, in *mtproto.TLPaymentsUpdateStarGiftCollection) (*mtproto.StarGiftCollection, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.UpdateStarGiftCollection(id, in)
}

func (s *Service) PaymentsReorderStarGiftCollections(ctx context.Context, in *mtproto.TLPaymentsReorderStarGiftCollections) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var peer *mtproto.InputPeer
	var order []int32
	if in != nil {
		peer = in.GetPeer()
		order = in.GetOrder()
	}
	if err := s.svcCtx.Book.ReorderStarGiftCollections(id, peer, order); err != nil {
		return nil, err
	}
	return mtproto.ToBool(true), nil
}

func (s *Service) PaymentsDeleteStarGiftCollection(ctx context.Context, in *mtproto.TLPaymentsDeleteStarGiftCollection) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var peer *mtproto.InputPeer
	var collectionID int32
	if in != nil {
		peer = in.GetPeer()
		collectionID = in.GetCollectionId()
	}
	if err := s.svcCtx.Book.DeleteStarGiftCollection(id, peer, collectionID); err != nil {
		return nil, err
	}
	return mtproto.ToBool(true), nil
}

func (s *Service) PaymentsGetStarGiftCollections(ctx context.Context, in *mtproto.TLPaymentsGetStarGiftCollections) (*mtproto.Payments_StarGiftCollections, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var peer *mtproto.InputPeer
	var hash int64
	if in != nil {
		peer = in.GetPeer()
		hash = in.GetHash()
	}
	return s.svcCtx.Book.GetStarGiftCollections(id, peer, hash)
}

func (s *Service) PaymentsDeleteStarGift(ctx context.Context, in *mtproto.TLPaymentsDeleteStarGift) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var gift *mtproto.InputSavedStarGift
	if in != nil {
		gift = in.GetStargift()
	}
	return nil, s.svcCtx.Book.DeleteStarGift(id, gift)
}

func (s *Service) PaymentsWithdrawStarGift(ctx context.Context, in *mtproto.TLPaymentsWithdrawStarGift) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var gift *mtproto.InputSavedStarGift
	var tmp []byte
	if in != nil {
		gift = in.GetStargift()
		tmp = in.GetTmpPassword()
	}
	return nil, s.svcCtx.Book.WithdrawStarGift(id, gift, tmp)
}

func (s *Service) PaymentsDepositStarGift(ctx context.Context, in *mtproto.TLPaymentsDepositStarGift) (*mtproto.Payments_StarGiftDepositPayload, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var peer *mtproto.InputPeer
	slug := ""
	if in != nil {
		peer = in.GetPeer()
		slug = in.GetSlug()
	}
	return nil, s.svcCtx.Book.DepositStarGift(id, peer, slug)
}

func (s *Service) PaymentsDisplayStarGift(ctx context.Context, in *mtproto.TLPaymentsDisplayStarGift) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var peer *mtproto.InputPeer
	hasPeer := false
	slug := ""
	if in != nil {
		peer = in.GetPeer()
		hasPeer = in.GetHasPeer()
		slug = in.GetSlug()
	}
	return nil, s.svcCtx.Book.DisplayStarGift(id, peer, hasPeer, slug)
}

func (s *Service) FragmentGetCollectibleInfo(ctx context.Context, in *mtproto.TLFragmentGetCollectibleInfo) (*mtproto.Fragment_CollectibleInfo, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var item *mtproto.InputCollectible
	if in != nil {
		item = in.GetCollectible()
	}
	return nil, s.svcCtx.Book.CollectibleInfo(id, item)
}
