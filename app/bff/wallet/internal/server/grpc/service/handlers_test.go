package service

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	rpcmd "github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/wallet/internal/ledger"

	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func withUser(userID int64) context.Context {
	md := &rpcmd.RpcMetadata{UserId: userID, PermAuthKeyId: userID + 10}
	buf, _ := proto.Marshal(md)
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"rpc_metadata", base64.StdEncoding.EncodeToString(buf),
	))
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return status.Convert(err).Message()
}

func TestWalletRequiresAuth(t *testing.T) {
	svc := New(nil)
	if _, err := svc.WalletGetState(context.Background(), &mtproto.TLWalletGetState{}); errText(err) != "AUTH_KEY_UNREGISTERED" {
		t.Fatal(err)
	}
}

func TestWalletReads(t *testing.T) {
	svc := New(nil)
	ctx := withUser(7)
	st, err := svc.WalletGetState(ctx, &mtproto.TLWalletGetState{})
	if err != nil || st.GetPredicateName() != mtproto.Predicate_walletStateEmpty || st.GetCreating() {
		t.Fatalf("state %#v %v", st, err)
	}
	tx, err := svc.WalletGetTransactions(ctx, &mtproto.TLWalletGetTransactions{Limit: 10, Inbound: true})
	if err != nil || len(tx.GetTransactions()) != 0 || tx.GetNextOffset() != nil || tx.GetBalance() != 0 {
		t.Fatalf("tx %#v %v", tx, err)
	}
	if _, err := svc.WalletGetTransactions(ctx, &mtproto.TLWalletGetTransactions{Limit: 101}); errText(err) != "LIMIT_INVALID" {
		t.Fatal(err)
	}
	if _, err := svc.WalletGetTransactions(ctx, &mtproto.TLWalletGetTransactions{Offset: "next"}); errText(err) != "OFFSET_INVALID" {
		t.Fatal(err)
	}
	if _, err := svc.WalletGetTransactionsByIDs(ctx, &mtproto.TLWalletGetTransactionsByIDs{Id: []string{" "}}); errText(err) != "TRANSACTION_ID_INVALID" {
		t.Fatal(err)
	}
	got, err := svc.WalletGetTransactionsByIDs(ctx, &mtproto.TLWalletGetTransactionsByIDs{Id: []string{"abc"}})
	if err != nil || len(got.GetTransactions()) != 0 {
		t.Fatal(err)
	}
	if _, err := svc.WalletGetTransactionsByMsgHash(ctx, &mtproto.TLWalletGetTransactionsByMsgHash{MsgHash: []string{""}}); errText(err) != "MSG_HASH_INVALID" {
		t.Fatal(err)
	}
	addrs, err := svc.WalletGetUserAddresses(ctx, &mtproto.TLWalletGetUserAddresses{
		Id: []*mtproto.InputUser{mtproto.MakeTLInputUserSelf(nil).To_InputUser()},
	})
	if err != nil || len(addrs.GetAddresses()) != 0 {
		t.Fatalf("addrs %#v %v", addrs, err)
	}
	if _, err := svc.WalletGetUserAddresses(ctx, &mtproto.TLWalletGetUserAddresses{Addresses: []string{" "}}); errText(err) != "ADDRESS_INVALID" {
		t.Fatal(err)
	}
	nfts, err := svc.WalletGetNfts(ctx, &mtproto.TLWalletGetNfts{Limit: 5})
	if err != nil || len(nfts.GetItems()) != 0 {
		t.Fatal(err)
	}
	bal, err := svc.WalletGetExistingWaltBalance(ctx, &mtproto.TLWalletGetExistingWaltBalance{})
	if err != nil || bal.GetHasBalance() || bal.GetPredicateName() != mtproto.Predicate_wallet_existingBalance {
		t.Fatalf("balance %#v %v", bal, err)
	}
	holders, err := svc.WalletGetBackupHolderDcs(ctx, &mtproto.TLWalletGetBackupHolderDcs{})
	if err != nil || len(holders.GetDatas()) != 0 {
		t.Fatal(err)
	}
	a, err := svc.WalletGetProofChallenge(ctx, &mtproto.TLWalletGetProofChallenge{})
	b, err2 := svc.WalletGetProofChallenge(ctx, &mtproto.TLWalletGetProofChallenge{})
	if err != nil || err2 != nil || a.GetPayload() == "" || a.GetPayload() == b.GetPayload() || a.GetDomain() != ledger.ProofDomain {
		t.Fatalf("proof %q %q %v %v", a.GetPayload(), b.GetPayload(), err, err2)
	}
	if a.GetExpires() <= int32(time.Now().Unix()) {
		t.Fatal(a.GetExpires())
	}
}

func TestWalletChainRefused(t *testing.T) {
	svc := New(nil)
	ctx := withUser(3)
	if _, err := svc.WalletExportSecretPhrase(ctx, &mtproto.TLWalletExportSecretPhrase{}); errText(err) != "WALLET_NOT_CREATED" {
		t.Fatal(err)
	}
	if _, err := svc.WalletFetchEncryptedSecretPhrasePart(ctx, &mtproto.TLWalletFetchEncryptedSecretPhrasePart{}); errText(err) != "TOKEN_EMPTY" {
		t.Fatal(err)
	}
	if _, err := svc.WalletFetchEncryptedSecretPhrasePart(ctx, &mtproto.TLWalletFetchEncryptedSecretPhrasePart{Token: "tok"}); errText(err) != "PUBLIC_KEY_REQUIRED" {
		t.Fatal(err)
	}
	if _, err := svc.WalletFetchEncryptedSecretPhrasePart(ctx, &mtproto.TLWalletFetchEncryptedSecretPhrasePart{Token: "tok", PublicKey: []byte{1}}); errText(err) != "WALLET_BACKUP_MISSING" {
		t.Fatal(err)
	}
	if _, err := svc.WalletReplaceWallet(ctx, &mtproto.TLWalletReplaceWallet{}); errText(err) != "WALLET_REPLACEMENT_INVALID" {
		t.Fatal(err)
	}
	if _, err := svc.WalletReplaceWallet(ctx, &mtproto.TLWalletReplaceWallet{
		Wallet: mtproto.MakeTLInputWalletNew(nil).To_InputWalletReplacement(),
	}); errText(err) != "WALLET_CHAIN_UNAVAILABLE" {
		t.Fatal(err)
	}
	if _, err := svc.WalletReplaceWallet(ctx, &mtproto.TLWalletReplaceWallet{
		Wallet: mtproto.MakeTLInputWalletImported(&mtproto.InputWalletReplacement{}).To_InputWalletReplacement(),
	}); errText(err) != "PUBLIC_KEY_REQUIRED" {
		t.Fatal(err)
	}
	imported := mtproto.MakeTLInputWalletImported(&mtproto.InputWalletReplacement{PublicKey: []byte{9}}).To_InputWalletReplacement()
	if _, err := svc.WalletReplaceWallet(ctx, &mtproto.TLWalletReplaceWallet{Wallet: imported}); errText(err) != "WALLET_PROOF_REQUIRED" {
		t.Fatal(err)
	}
	imported.Proof = &mtproto.WalletOwnershipProof{Timestamp: 1, Signature: []byte{2}}
	if _, err := svc.WalletReplaceWallet(ctx, &mtproto.TLWalletReplaceWallet{Wallet: imported}); errText(err) != "WALLET_CHAIN_UNAVAILABLE" {
		t.Fatal(err)
	}
	if _, err := svc.WalletEnableBackup(ctx, &mtproto.TLWalletEnableBackup{}); errText(err) != "WALLET_BACKUP_PARTS_EMPTY" {
		t.Fatal(err)
	}
	if _, err := svc.WalletEnableBackup(ctx, &mtproto.TLWalletEnableBackup{Parts: [][]byte{{1}}}); errText(err) != "WALLET_NOT_CREATED" {
		t.Fatal(err)
	}
	if _, err := svc.WalletDisableBackup(ctx, &mtproto.TLWalletDisableBackup{}); errText(err) != "WALLET_NOT_CREATED" {
		t.Fatal(err)
	}
	if _, err := svc.WalletGetGaslessInfo(ctx, &mtproto.TLWalletGetGaslessInfo{}); errText(err) != "WALLET_CHAIN_UNAVAILABLE" {
		t.Fatal(err)
	}
	if _, err := svc.WalletSendTransfer(ctx, &mtproto.TLWalletSendTransfer{}); errText(err) != "TRANSFER_EMPTY" {
		t.Fatal(err)
	}
	if _, err := svc.WalletSendTransfer(ctx, &mtproto.TLWalletSendTransfer{
		DataNormal: []byte{1},
		UserId:     mtproto.MakeTLInputUserSelf(nil).To_InputUser(),
	}); errText(err) != "RANDOM_ID_EMPTY" {
		t.Fatal(err)
	}
	if _, err := svc.WalletSendTransfer(ctx, &mtproto.TLWalletSendTransfer{
		DataNormal: []byte{1},
		UserId:     mtproto.MakeTLInputUserSelf(nil).To_InputUser(),
		RandomId:   4,
	}); errText(err) != "WALLET_CHAIN_UNAVAILABLE" {
		t.Fatal(err)
	}
}

func TestTonConnectAndCard(t *testing.T) {
	svc := New(nil)
	ctx := withUser(9)
	if _, err := svc.WalletTonConnectCreateSession(ctx, &mtproto.TLWalletTonConnectCreateSession{}); errText(err) != "DAPP_CLIENT_ID_INVALID" {
		t.Fatal(err)
	}
	if _, err := svc.WalletTonConnectCreateSession(ctx, &mtproto.TLWalletTonConnectCreateSession{
		DappClientId: "dapp", ManifestUrl: "not a url",
	}); errText(err) != "MANIFEST_URL_INVALID" {
		t.Fatal(err)
	}
	sess, err := svc.WalletTonConnectCreateSession(ctx, &mtproto.TLWalletTonConnectCreateSession{
		DappClientId: "dapp", ManifestUrl: "https://dapp.example/manifest.json",
	})
	if err != nil || !sess.GetPending() || sess.GetId() == 0 || len(sess.GetNonce()) == 0 || sess.GetManifest().GetName() != "dapp.example" {
		t.Fatalf("session %#v manifest %#v %v", sess, sess.GetManifest(), err)
	}
	again, err := svc.WalletTonConnectCreateSession(ctx, &mtproto.TLWalletTonConnectCreateSession{
		DappClientId: "dapp", ManifestUrl: "https://dapp.example/manifest.json",
	})
	if err != nil || again.GetId() != sess.GetId() {
		t.Fatalf("reuse %#v %v", again, err)
	}
	ch, err := svc.WalletTonConnectRegisterKey(ctx, &mtproto.TLWalletTonConnectRegisterKey{SessionId: sess.GetId(), ClientId: "client"})
	if err != nil || len(ch.GetChallenge()) != 32 || ch.GetEventId() == 0 {
		t.Fatalf("challenge %#v %v", ch, err)
	}
	if _, err := svc.WalletTonConnectSubmitResponse(ctx, &mtproto.TLWalletTonConnectSubmitResponse{
		SessionId: sess.GetId(), Body: []byte("resp"),
	}); errText(err) != "SESSION_PENDING" {
		t.Fatal(err)
	}
	if _, err := svc.WalletTonConnectSubmitConnectResult(ctx, &mtproto.TLWalletTonConnectSubmitConnectResult{
		SessionId: sess.GetId(), ChallengeAnswer: []byte{1}, Body: []byte("connect"),
	}); err != nil {
		t.Fatal(err)
	}
	pending, err := svc.WalletTonConnectGetPending(ctx, &mtproto.TLWalletTonConnectGetPending{
		SessionId: wrapperspb.Int64(sess.GetId()),
	})
	if err != nil || pending.GetSession().GetPending() || len(pending.GetRequests()) != 0 {
		t.Fatalf("pending %#v %v", pending, err)
	}
	if _, err := svc.WalletTonConnectSubmitResponse(ctx, &mtproto.TLWalletTonConnectSubmitResponse{
		SessionId: sess.GetId(), MsgId: 3, Body: []byte("resp"),
	}); err != nil {
		t.Fatal(err)
	}
	body, ok := svc.svcCtx.Ledger.StoredResponse(9, sess.GetId(), 3)
	if !ok || string(body) != "resp" {
		t.Fatalf("stored %q %v", body, ok)
	}
	ev1, err := svc.WalletTonConnectNextEventId(ctx, &mtproto.TLWalletTonConnectNextEventId{SessionId: sess.GetId()})
	ev2, err2 := svc.WalletTonConnectNextEventId(ctx, &mtproto.TLWalletTonConnectNextEventId{SessionId: sess.GetId()})
	if err != nil || err2 != nil || ev2.GetEventId() != ev1.GetEventId()+1 {
		t.Fatalf("events %d %d %v %v", ev1.GetEventId(), ev2.GetEventId(), err, err2)
	}
	if _, err := svc.WalletTonConnectClaimRequest(ctx, &mtproto.TLWalletTonConnectClaimRequest{
		SessionId: sess.GetId(), AppRequestId: "r",
	}); errText(err) != "TONCONNECT_REQUEST_NOT_FOUND" {
		t.Fatal(err)
	}
	list, err := svc.WalletTonConnectGetSessions(ctx, &mtproto.TLWalletTonConnectGetSessions{})
	if err != nil || len(list.GetSessions()) != 1 || list.GetSessions()[0].GetId() != sess.GetId() {
		t.Fatal(err)
	}
	other := withUser(8)
	if _, err := svc.WalletTonConnectGetPending(other, &mtproto.TLWalletTonConnectGetPending{
		SessionId: wrapperspb.Int64(sess.GetId()),
	}); errText(err) != "SESSION_ID_INVALID" {
		t.Fatal(err)
	}
	if _, err := svc.WalletTonConnectCloseSession(ctx, &mtproto.TLWalletTonConnectCloseSession{SessionId: sess.GetId()}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.WalletTonConnectCloseSession(ctx, &mtproto.TLWalletTonConnectCloseSession{SessionId: sess.GetId()}); err != nil {
		t.Fatal(err)
	}
	closed, err := svc.WalletTonConnectGetPending(ctx, &mtproto.TLWalletTonConnectGetPending{
		SessionId: wrapperspb.Int64(sess.GetId()),
	})
	if err != nil || !closed.GetSession().GetClosed() {
		t.Fatalf("closed %#v %v", closed.GetSession(), err)
	}

	if _, err := svc.WalletSetCardStickers(ctx, &mtproto.TLWalletSetCardStickers{}); err != nil {
		t.Fatal(err)
	}
	card, err := svc.WalletSetCardStickers(ctx, &mtproto.TLWalletSetCardStickers{
		Stickers: []*mtproto.InputWalletCardSticker{{
			Coordinates: mtproto.MakeTLMediaAreaCoordinates(&mtproto.MediaAreaCoordinates{
				X: 1, Y: 2, W: 3, H: 4,
			}).To_MediaAreaCoordinates(),
			NftAddress: "nft-1",
		}},
		BgNftAddress: wrapperspb.String("bg-1"),
	})
	if err != nil || card.GetAddress() != "" || card.GetBalance() != 0 || !card.GetHasBg() || len(card.GetStickers()) != 1 {
		t.Fatalf("card %#v %v", card, err)
	}
	if card.GetStickers()[0].GetNft().GetAddress() != "nft-1" || card.GetBgNftItem().GetAddress() != "bg-1" {
		t.Fatalf("nfts %#v", card)
	}
	if _, err := svc.WalletSetCardStickers(ctx, &mtproto.TLWalletSetCardStickers{
		Stickers: []*mtproto.InputWalletCardSticker{{NftAddress: "x"}},
	}); errText(err) != "STICKER_INVALID" {
		t.Fatal(err)
	}
	if _, err := svc.WalletSetCardStickers(ctx, &mtproto.TLWalletSetCardStickers{
		BgNftAddress: wrapperspb.String("  "),
	}); errText(err) != "BG_NFT_INVALID" {
		t.Fatal(err)
	}
}

func TestToncenterRefused(t *testing.T) {
	svc := New(nil)
	ctx := withUser(1)
	if _, err := svc.ToncenterPerformApiRequest(ctx, &mtproto.TLToncenterPerformApiRequest{}); errText(err) != "ENDPOINT_EMPTY" {
		t.Fatal(err)
	}
	if _, err := svc.ToncenterPerformApiRequest(ctx, &mtproto.TLToncenterPerformApiRequest{Endpoint: "/v2/get"}); errText(err) != "TONCENTER_UNAVAILABLE" {
		t.Fatal(err)
	}
	if _, err := svc.ToncenterGetStreamingUrl(ctx, &mtproto.TLToncenterGetStreamingUrl{}); errText(err) != "TONCENTER_UNAVAILABLE" {
		t.Fatal(err)
	}
}
