# Wallet

Layer 232 `wallet.*` methods are routed on `RPCWallet`. `wallet.setCardStickers`
is routed on `RPCWalletCard` because `wallet.Card` is not in the upstream
generated service (`proto/mtproto/schema.tl.walletcard.proto`).

This process does not speak to a TON node and does not create keys. It will
not return a receive address. A client that showed one could send real TON
to a key this server does not hold.

## Schema-valid results

- `wallet.getState` returns `walletStateEmpty` (`creating` unset).
- `wallet.getTransactions`, `wallet.getTransactionsByIDs`, and
  `wallet.getTransactionsByMsgHash` return `wallet.transactions` with balance
  0 and no rows. A non-empty `offset` is `OFFSET_INVALID` because there is no
  next page. `limit` outside 1..100 is `LIMIT_INVALID` (`0` means 20).
- `wallet.getUserAddresses` returns an empty `wallet.userAddresses` after
  checking each `InputUser` and address. No address is on file, so the user
  vector stays empty.
- `wallet.getNfts` returns `wallet.nftItems` with no rows. Offset and limit
  use the same rules as transactions.
- `wallet.getExistingWaltBalance` returns `wallet.existingBalance` with
  `has_balance` unset and an empty url. There is no legacy balance to open.
- `wallet.getBackupHolderDcs` returns an empty vector. This deployment does
  not publish a backup-holder public key.
- `wallet.getProofChallenge` returns a random payload, expiry ten minutes
  out, and domain `wallet.tellomi.local`. The challenge is not checked later,
  because there is no wallet key to bind it to.
- `wallet.getGaslessInfo` and `wallet.sendTransfer` return `Updates` in the
  schema. An empty `updates` box would be applied to the client's sequence,
  so both return `400 WALLET_CHAIN_UNAVAILABLE` instead.
- Ton Connect sessions, challenges, connect results, and response bodies are
  stored in memory for the life of the process. `wallet.tonConnectGetPending`
  returns the session and an empty request vector. Nothing is forwarded to a
  dapp bridge, and connect `challenge_answer` bytes are stored without a
  signature check.
- `wallet.setCardStickers` stores the placement and returns `wallet.card`
  with address `""` and balance 0. NFT items in that card carry the address
  the client sent, owner `""`, and index `"0"`. The card is not written onto
  `userFull.wallet_card` (that flag is still rejected by the userFull codec).

Sessions and cards are process-local. A restart drops them. At most 64 Ton
Connect sessions are kept per user.

## Documented 400s

| Code | When |
| --- | --- |
| `ADDRESS_INVALID` | `wallet.getUserAddresses` address is empty or longer than 128 bytes |
| `USER_ID_INVALID` | `InputUser` is missing, empty, or has user id 0 |
| `LIMIT_INVALID` | transaction or NFT `limit` is negative or greater than 100 |
| `OFFSET_INVALID` | transaction or NFT `offset` is not empty |
| `TRANSACTION_ID_INVALID` | a transaction id is empty |
| `MSG_HASH_INVALID` | a message hash is empty |
| `WALLET_NOT_CREATED` | export, enable, or disable backup. No wallet exists to back up |
| `WALLET_BACKUP_PARTS_EMPTY` | `wallet.enableBackup` has no parts, or a part is empty |
| `TOKEN_EMPTY` | `wallet.fetchEncryptedSecretPhrasePart` token is empty |
| `PUBLIC_KEY_REQUIRED` | fetch part has no public key, or an import has no public key |
| `WALLET_BACKUP_MISSING` | the token does not name a stored share |
| `WALLET_REPLACEMENT_INVALID` | `wallet.replaceWallet` wallet is missing or not `inputWalletNew` / `inputWalletImported` |
| `WALLET_PROOF_REQUIRED` | an import has no ownership-proof signature |
| `WALLET_CHAIN_UNAVAILABLE` | create, import, gasless info, or send. No chain broadcast |
| `TRANSFER_EMPTY` | `wallet.sendTransfer` `data_normal` is empty |
| `RANDOM_ID_EMPTY` | `wallet.sendTransfer` `random_id` is 0 |
| `DAPP_CLIENT_ID_INVALID` | Ton Connect dapp client id is empty or longer than 256 bytes |
| `MANIFEST_URL_INVALID` | manifest URL is not http(s) with a host, or is longer than 2048 bytes |
| `CLIENT_ID_INVALID` | `wallet.tonConnectRegisterKey` client id is empty or longer than 256 bytes |
| `SESSION_ID_INVALID` | the session is not owned by this user |
| `SESSION_CLOSED` | the session is already closed |
| `SESSION_PENDING` | a response was submitted before connect finished |
| `SESSION_ALREADY_CONNECTED` | `registerKey` after a successful connect |
| `CHALLENGE_INVALID` | connect result before `registerKey` |
| `CHALLENGE_ANSWER_EMPTY` | connect result without `challenge_answer` |
| `RESPONSE_BODY_EMPTY` | connect result or submit response with an empty body |
| `APP_REQUEST_ID_EMPTY` | `wallet.tonConnectClaimRequest` without an app request id |
| `TONCONNECT_REQUEST_NOT_FOUND` | no dapp request is waiting. There is no bridge |
| `STICKER_INVALID` | a card sticker has no coordinates, a non-positive size, or an empty NFT address |
| `STICKERS_TOO_MUCH` | more than 32 card stickers |
| `BG_NFT_INVALID` | background NFT address is present but empty or longer than 128 bytes |
| `ENDPOINT_EMPTY` | `toncenter.performApiRequest` endpoint is empty |
| `TONCENTER_UNAVAILABLE` | `toncenter.performApiRequest` and `toncenter.getStreamingUrl`. They share `RPCWallet` and are not proxied |

`AUTH_KEY_UNREGISTERED` (401) is returned when the RPC has no user id.

Android `TL_wallet.getCurrencyRates` is constructor `0xd35983e8`, which is
`payments.getCurrencyRates` in layer 232. It is not a separate wallet method.
Payments stay out of this change.
