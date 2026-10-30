package service

import (
	"context"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/wallet/internal/ledger"
)

func emptyState() *mtproto.WalletState {
	return mtproto.MakeTLWalletStateEmpty(&mtproto.WalletState{}).To_WalletState()
}

func emptyTransactions() *mtproto.Wallet_Transactions {
	return mtproto.MakeTLWalletTransactions(&mtproto.Wallet_Transactions{
		Transactions: []*mtproto.WalletTransaction{},
		Chats:        []*mtproto.Chat{},
		Users:        []*mtproto.User{},
	}).To_Wallet_Transactions()
}

func emptyNfts() *mtproto.Wallet_NftItems {
	return mtproto.MakeTLWalletNftItems(&mtproto.Wallet_NftItems{
		Items: []*mtproto.Wallet_NftItem{},
	}).To_Wallet_NftItems()
}

func emptyAddresses() *mtproto.Wallet_UserAddresses {
	return mtproto.MakeTLWalletUserAddresses(&mtproto.Wallet_UserAddresses{
		Addresses: []*mtproto.WalletUserAddress{},
		Users:     []*mtproto.User{},
	}).To_Wallet_UserAddresses()
}

func pageLimit(limit int32) (int32, error) {
	if limit == 0 {
		return 20, nil
	}
	if limit < 0 || limit > 100 {
		return 0, rpcErr("LIMIT_INVALID")
	}
	return limit, nil
}

func (s *Service) WalletGetState(ctx context.Context, _ *mtproto.TLWalletGetState) (*mtproto.WalletState, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	_ = s.svcCtx.Ledger.ReadState(id)
	return emptyState(), nil
}

func (s *Service) WalletGetUserAddresses(ctx context.Context, in *mtproto.TLWalletGetUserAddresses) (*mtproto.Wallet_UserAddresses, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	for _, user := range in.GetId() {
		if _, err := inputUserID(user, id); err != nil {
			return nil, err
		}
	}
	for _, addr := range in.GetAddresses() {
		if strings.TrimSpace(addr) == "" || len(addr) > 128 {
			return nil, rpcErr("ADDRESS_INVALID")
		}
	}
	s.svcCtx.Ledger.Touch(id)
	return emptyAddresses(), nil
}

func (s *Service) WalletGetTransactions(ctx context.Context, in *mtproto.TLWalletGetTransactions) (*mtproto.Wallet_Transactions, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := pageLimit(in.GetLimit()); err != nil {
		return nil, err
	}
	if code := s.svcCtx.Ledger.Transactions(id, in.GetOffset()); code != "" {
		return nil, fail(code)
	}
	return emptyTransactions(), nil
}

func (s *Service) WalletGetTransactionsByIDs(ctx context.Context, in *mtproto.TLWalletGetTransactionsByIDs) (*mtproto.Wallet_Transactions, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	for _, tx := range in.GetId() {
		if strings.TrimSpace(tx) == "" {
			return nil, rpcErr("TRANSACTION_ID_INVALID")
		}
	}
	s.svcCtx.Ledger.Touch(id)
	return emptyTransactions(), nil
}

func (s *Service) WalletGetTransactionsByMsgHash(ctx context.Context, in *mtproto.TLWalletGetTransactionsByMsgHash) (*mtproto.Wallet_Transactions, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	for _, hash := range in.GetMsgHash() {
		if strings.TrimSpace(hash) == "" {
			return nil, rpcErr("MSG_HASH_INVALID")
		}
	}
	s.svcCtx.Ledger.Touch(id)
	return emptyTransactions(), nil
}

func (s *Service) WalletExportSecretPhrase(ctx context.Context, _ *mtproto.TLWalletExportSecretPhrase) (*mtproto.Wallet_SecretPhraseParts, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	s.svcCtx.Ledger.Touch(id)
	return nil, rpcErr("WALLET_NOT_CREATED")
}

func (s *Service) WalletFetchEncryptedSecretPhrasePart(ctx context.Context, in *mtproto.TLWalletFetchEncryptedSecretPhrasePart) (*mtproto.Wallet_EncryptedSecretPhrasePart, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	s.svcCtx.Ledger.Touch(id)
	if strings.TrimSpace(in.GetToken()) == "" {
		return nil, rpcErr("TOKEN_EMPTY")
	}
	if len(in.GetPublicKey()) == 0 {
		return nil, rpcErr("PUBLIC_KEY_REQUIRED")
	}
	return nil, rpcErr("WALLET_BACKUP_MISSING")
}

func (s *Service) WalletReplaceWallet(ctx context.Context, in *mtproto.TLWalletReplaceWallet) (*mtproto.WalletState, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	s.svcCtx.Ledger.Touch(id)
	w := in.GetWallet()
	if w == nil {
		return nil, rpcErr("WALLET_REPLACEMENT_INVALID")
	}
	switch w.GetPredicateName() {
	case mtproto.Predicate_inputWalletNew:
		return nil, rpcErr("WALLET_CHAIN_UNAVAILABLE")
	case mtproto.Predicate_inputWalletImported:
		if len(w.GetPublicKey()) == 0 {
			return nil, rpcErr("PUBLIC_KEY_REQUIRED")
		}
		if w.GetProof() == nil || len(w.GetProof().GetSignature()) == 0 {
			return nil, rpcErr("WALLET_PROOF_REQUIRED")
		}
		return nil, rpcErr("WALLET_CHAIN_UNAVAILABLE")
	default:
		return nil, rpcErr("WALLET_REPLACEMENT_INVALID")
	}
}

func (s *Service) WalletDisableBackup(ctx context.Context, _ *mtproto.TLWalletDisableBackup) (*mtproto.WalletState, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	s.svcCtx.Ledger.Touch(id)
	return nil, rpcErr("WALLET_NOT_CREATED")
}

func (s *Service) WalletGetBackupHolderDcs(ctx context.Context, _ *mtproto.TLWalletGetBackupHolderDcs) (*mtproto.Vector_Wallet_HolderDc, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	s.svcCtx.Ledger.Touch(id)
	return &mtproto.Vector_Wallet_HolderDc{Datas: []*mtproto.Wallet_HolderDc{}}, nil
}

func (s *Service) WalletEnableBackup(ctx context.Context, in *mtproto.TLWalletEnableBackup) (*mtproto.WalletState, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	s.svcCtx.Ledger.Touch(id)
	if len(in.GetParts()) == 0 {
		return nil, rpcErr("WALLET_BACKUP_PARTS_EMPTY")
	}
	for _, part := range in.GetParts() {
		if len(part) == 0 {
			return nil, rpcErr("WALLET_BACKUP_PARTS_EMPTY")
		}
	}
	return nil, rpcErr("WALLET_NOT_CREATED")
}

func (s *Service) WalletGetProofChallenge(ctx context.Context, _ *mtproto.TLWalletGetProofChallenge) (*mtproto.Wallet_ProofChallenge, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	payload, expires := s.svcCtx.Ledger.IssueProof(id, time.Now())
	return mtproto.MakeTLWalletProofChallenge(&mtproto.Wallet_ProofChallenge{
		Payload: payload,
		Expires: expires,
		Domain:  ledger.ProofDomain,
	}).To_Wallet_ProofChallenge(), nil
}

func (s *Service) WalletSendTransfer(ctx context.Context, in *mtproto.TLWalletSendTransfer) (*mtproto.Updates, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	s.svcCtx.Ledger.Touch(id)
	if len(in.GetDataNormal()) == 0 {
		return nil, rpcErr("TRANSFER_EMPTY")
	}
	if _, err := inputUserID(in.GetUserId(), id); err != nil {
		return nil, err
	}
	if in.GetRandomId() == 0 {
		return nil, rpcErr("RANDOM_ID_EMPTY")
	}
	return nil, rpcErr("WALLET_CHAIN_UNAVAILABLE")
}

func (s *Service) WalletGetGaslessInfo(ctx context.Context, _ *mtproto.TLWalletGetGaslessInfo) (*mtproto.Updates, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	s.svcCtx.Ledger.Touch(id)
	// An Updates box would be applied to the client's seq. There is no chain
	// update to deliver, so this stays a 400 instead of an empty updates.
	return nil, rpcErr("WALLET_CHAIN_UNAVAILABLE")
}

func (s *Service) WalletGetExistingWaltBalance(ctx context.Context, _ *mtproto.TLWalletGetExistingWaltBalance) (*mtproto.Wallet_ExistingBalance, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	s.svcCtx.Ledger.Touch(id)
	return mtproto.MakeTLWalletExistingBalance(&mtproto.Wallet_ExistingBalance{}).To_Wallet_ExistingBalance(), nil
}

func (s *Service) WalletGetNfts(ctx context.Context, in *mtproto.TLWalletGetNfts) (*mtproto.Wallet_NftItems, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := pageLimit(in.GetLimit()); err != nil {
		return nil, err
	}
	if code := s.svcCtx.Ledger.Transactions(id, in.GetOffset()); code != "" {
		return nil, fail(code)
	}
	return emptyNfts(), nil
}

func inputUserID(in *mtproto.InputUser, self int64) (int64, error) {
	if in == nil {
		return 0, rpcErr("USER_ID_INVALID")
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputUserSelf:
		return self, nil
	case mtproto.Predicate_inputUser, mtproto.Predicate_inputUserFromMessage:
		if in.GetUserId() == 0 {
			return 0, rpcErr("USER_ID_INVALID")
		}
		return in.GetUserId(), nil
	default:
		return 0, rpcErr("USER_ID_INVALID")
	}
}
