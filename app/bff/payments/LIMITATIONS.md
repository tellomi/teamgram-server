# Stars, giveaways, boosts, and Star gifts

Batch 1 is the balance, subscription, giveaway, gift-code, and boost reads.
Batch 2 is the Star gift catalog, saved-gift pages, collections, auctions,
offers, and `fragment.getCollectibleInfo`. Batch 3 is the payment form,
saved checkout info, bank-card title, currency rates, on-ramp reads, StarRef
affiliate links, and store-receipt assignment.

`RPCStars`, `RPCStarSubscriptions`, `RPCGiveaways`, and `RPCGiftCodes` are
routed to the bff process. `premium.getBoostsList`, `premium.getMyBoosts`,
`premium.applyBoost`, `premium.getBoostsStatus`, and `premium.getUserBoosts`
are the premium methods on `RPCBoosts`, which was already routed.
`channels.setBoostsToUnblockRestrictions` is unchanged. Story boost methods on
that service are still unimplemented.

There is no Stars ledger and no store. Balances are zero. Boost slots are
kept in this process and dropped on restart. `access_hash` is not checked.
Per-user `users.premium` rows are not read: boost slots exist only while
`TELLOMI_PREMIUM_EVERYONE` is on (the default).

## Schema-valid results

- `payments.getStarsStatus` returns `payments.starsStatus` with balance 0.
  `ton:true` uses `starsTonAmount`; otherwise `starsAmount` with nanos 0.
  History and subscriptions are omitted. Any user, chat, or channel peer is
  accepted. Admin rights are not checked.
- `payments.getStarsTransactions` returns the same status with an empty
  `history`. `inbound`, `outbound`, and `ascending` are accepted and do not
  change the empty page. A non-empty `offset` is `OFFSET_INVALID` because no
  page token is issued. `limit` 0 means 20. A `subscription_id` that is
  present must be a non-empty id; the page is still empty.
- `payments.getStarsTransactionsByID` returns an empty `history` after each
  id is checked. Unknown ids are omitted. There are no stored transactions.
- `payments.getStarsSubscriptions` returns balance 0 and an empty
  `subscriptions` vector. `missing_balance` sets
  `subscriptions_missing_balance` to 0. A non-empty `offset` is
  `OFFSET_INVALID`.
- `payments.getStarsTopupOptions`, `payments.getStarsGiftOptions`,
  `payments.getPremiumGiftCodeOptions`, and `payments.getStarsGiveawayOptions`
  return empty vectors. Nothing is offered for sale.
- `payments.getStarsRevenueStats` returns `payments.starsRevenueStats` with
  `statsGraphError` (`GRAPH_UNAVAILABLE`), zero balances, and `usd_rate` 0.
  `dark` is accepted and does not change the graph.
- `premium.getMyBoosts` returns four `myBoost` slots while the global Premium
  switch is on. Slots are 0..3, last 30 days, and have no peer until applied.
  Chats and users are empty. When the switch is off the vector is empty.
- `premium.applyBoost` assigns slots and returns `premium.myBoosts`. With no
  `slots` vector, or an empty one, the first free slot is used. Moving a slot
  to another peer waits 24 hours (`BOOST_COOLDOWN`).
- `premium.getBoostsStatus` returns `premium.boostsStatus`. Level equals the
  live boost count, capped at 100. The next level needs one more boost.
  `boost_url` is empty. `my_boost` and `my_boost_slots` are set together when
  this user has a live slot on the peer and the Premium switch is on. Gift
  boosts, prepaid giveaways, and premium audience are omitted. Boosts already
  applied stay counted after the switch is turned off; `my_boost` does not.
- `premium.getBoostsList` returns `premium.boostsList`. `gifts:true` is an
  empty page. `offset` is a decimal index this method would return in
  `next_offset`. Users are not filled in; each `boost.user_id` is set.
- `premium.getUserBoosts` is the same list filtered to one user.

## Documented 400s

| Code | When |
| --- | --- |
| `PEER_ID_INVALID` | peer is missing, empty, from a message, or has id 0 |
| `USER_ID_INVALID` | `InputUser` is missing, empty, from a message, or has id 0 |
| `LIMIT_INVALID` | `limit` is negative or greater than 100 |
| `OFFSET_INVALID` | Stars or subscription `offset` is not empty, or a boost offset is not a decimal index in range |
| `SUBSCRIPTION_ID_INVALID` | subscription id is empty, longer than 128 bytes, or contains a control character |
| `SUBSCRIPTION_NOT_FOUND` | `payments.changeStarsSubscription` or `payments.fulfillStarsSubscription`. No subscription is stored |
| `TRANSACTION_ID_INVALID` | a transaction id is missing or empty |
| `TRANSACTIONS_TOO_MUCH` | more than 100 transaction ids |
| `FORM_ID_INVALID` | `payments.sendStarsForm` `form_id` is not positive |
| `INVOICE_INVALID` | invoice is missing |
| `STARS_PAYMENT_UNAVAILABLE` | `payments.sendStarsForm`. Stars are not charged |
| `CHARGE_ID_INVALID` | charge id is empty |
| `CHARGE_NOT_FOUND` | `payments.refundStarsCharge` or `payments.botCancelStarsSubscription`. No charge is stored |
| `PASSWORD_HASH_INVALID` | withdrawal password is missing or not `inputCheckPasswordSRP` with `a` and `m1` |
| `AMOUNT_INVALID` | withdrawal `amount` is present and not positive |
| `BALANCE_TOO_LOW` | `payments.getStarsRevenueWithdrawalUrl`. Available balance is 0. The password hash is not checked against 2FA |
| `ADS_REVENUE_UNAVAILABLE` | `payments.getStarsRevenueAdsAccountUrl`. No ads account URL is issued |
| `SLUG_INVALID` | gift-code slug is empty, longer than 128 bytes, or not letters, digits, `_`, or `-` |
| `GIFT_CODE_NOT_FOUND` | `payments.checkGiftCode` and `payments.applyGiftCode`. No codes are issued |
| `MSG_ID_INVALID` | giveaway `msg_id` is not positive |
| `GIVEAWAY_NOT_FOUND` | `payments.getGiveawayInfo`. Giveaway messages are not stored |
| `GIVEAWAY_ID_INVALID` | `giveaway_id` is 0 |
| `PURPOSE_INVALID` | prepaid giveaway purpose is missing |
| `PREPAID_GIVEAWAY_NOT_FOUND` | `payments.launchPrepaidGiveaway`. No prepaid giveaway is stored |
| `SLOT_INVALID` | a boost slot is outside 0..3 |
| `BOOSTS_EMPTY` | `premium.applyBoost` with no `slots` vector and every slot is already in use |
| `BOOST_COOLDOWN` | a slot was moved less than 24 hours ago |
| `PREMIUM_ACCOUNT_REQUIRED` | `premium.applyBoost` while `TELLOMI_PREMIUM_EVERYONE` is off |

`AUTH_KEY_UNREGISTERED` (401) is returned when the RPC has no user id.

`payments.sendStarsForm`, `payments.refundStarsCharge`, `payments.applyGiftCode`,
and `payments.launchPrepaidGiveaway` do not return `Updates` or
`payments.paymentResult`. An empty updates box would be applied to the
client's sequence.

## Star gifts (batch 2)

`RPCGifts`, `RPCGiftCollections`, `RPCFragmentCollectibles`, and
`RPCStarGiftHost` are routed to this process. There is no gift catalog, no
saved-gift row, and no Fragment purchase record. Collection titles and the
per-peer gift-notification flag are kept in memory and dropped on restart.
`access_hash` and admin rights are not checked. Balances stay at zero, so
nothing is withdrawn, deposited, crafted, or sold.

`payments.deleteStarGift#3cdaf8aa`, `payments.withdrawStarGift#ec2030ae`,
`payments.depositStarGift#c0a2ea9f`, `payments.displayStarGift#4aa5f0fe`, and
`payments.starGiftDepositPayload#725bb4eb` are layer 232. The vendored codegen
did not emit them, so they live on `RPCStarGiftHost` (`stargift_host_codec.go`).

Android `org.telegram.tgnet.tl.TL_stars` (DrKLO `3de37c8`) sends the same
constructor ids under `stars.*` names. Those names are not in the layer-232
schema. `docs/api-coverage.md` counts the call sites on the payments and
channels methods, the same way it counts `TL_account` cross-namespace calls.
There is no `stars` RPC service. `stars.updatePaidMessagesPrice` is
constructor `0x4b12327b`, which is `channels.updatePaidMessagesPrice`. The
other names are the gift constructors implemented here.

### Schema-valid results

- `payments.getStarGifts` returns `payments.starGifts` with hash 0 and empty
  vectors. The hash of an empty catalog is 0, which is also what a client
  sends when it has no cache, so this method does not return
  `starGiftsNotModified`.
- `payments.getSavedStarGifts` returns `payments.savedStarGifts` with count 0.
  Filter flags are accepted and do not change the empty page. `limit` 0 means
  20. A non-empty `offset` is `OFFSET_INVALID`. `collection_id`, when set,
  must be a collection this process created for that peer. The notification
  flag is set only after `payments.toggleChatStarGiftNotifications`.
- `payments.getSavedStarGift` and `payments.getUserStarGift` validate each
  reference and return an empty page. Unknown gifts are omitted.
- `payments.getUserStarGifts`, `payments.getCraftStarGifts`, and
  `payments.getResaleStarGifts` return an empty page. Resale attribute ids
  are checked. `attributes_hash` is accepted and no attributes are returned.
- `payments.getStarGiftUpgradePreview` and
  `payments.getStarGiftUpgradeAttributes` return empty vectors for a positive
  `gift_id`.
- `payments.getStarGiftActiveAuctions` returns
  `payments.starGiftActiveAuctions` with no auctions. The empty-list hash is
  0, so `starGiftActiveAuctionsNotModified` is not used.
- `payments.getStarGiftAuctionAcquiredGifts` returns an empty acquired list.
- `payments.checkCanSendGift` returns `payments.checkCanSendGiftResultFail`
  for any positive `gift_id`. The reason text is "This gift is not available."
- `payments.getStarGiftCollections` returns the in-memory titles for the
  peer. The list hash mixes each collection id with a hash of its title
  (`hash ^= id>>21; hash ^= id<<35; hash ^= id>>4; hash += id`). Hash 0, and
  any hash that does not match, returns the list. A matching non-zero hash
  returns `payments.starGiftCollectionsNotModified`.
- `payments.createStarGiftCollection` stores an empty collection and returns
  `starGiftCollection`. A non-empty gift vector is `STARGIFT_NOT_FOUND` and
  nothing is stored.
- `payments.updateStarGiftCollection` renames a stored collection. Adding,
  removing, or reordering gifts is `STARGIFT_NOT_FOUND` and the title is left
  unchanged.
- `payments.reorderStarGiftCollections` accepts a permutation of the stored
  ids and returns `boolTrue`.
- `payments.deleteStarGiftCollection` removes one stored collection and
  returns `boolTrue`.
- `payments.toggleChatStarGiftNotifications` stores the flag and returns
  `boolTrue`.
- `payments.toggleStarGiftsPinnedToTop` with an empty vector returns
  `boolTrue`. A non-empty vector is `STARGIFT_NOT_FOUND`.

### Documented 400s

| Code | When |
| --- | --- |
| `STARGIFT_INVALID` | `gift_id` is not positive; an auction, attribute id, or saved-gift reference has the wrong shape; a gift vector is empty where one is required, or longer than 100; an offer `duration` is not positive |
| `STARGIFT_NOT_FOUND` | the reference is well formed and no gift, auction, offer, or unique slug is stored. Also `payments.getUniqueStarGift`, `payments.getUniqueStarGiftValueInfo`, `payments.getStarGiftAuctionState`, save, convert, upgrade, transfer, delete, withdraw, deposit, display, craft, price, offer, and withdrawal URL |
| `STARGIFT_SLUG_INVALID` | a gift slug is empty, longer than 128 bytes, or not letters, digits, `_`, or `-` |
| `SAVED_ID_EMPTY` | `inputSavedStarGiftChat.saved_id` is 0 |
| `COLLECTION_ID_INVALID` | `collection_id` is 0, unknown, or the reorder vector is not a permutation of the stored ids |
| `TITLE_INVALID` | a collection title is empty, longer than 64 characters, or contains a control character |
| `COLLECTIONS_TOO_MUCH` | the peer already has 100 collections in this process |
| `COLLECTIBLE_INVALID` | `fragment.getCollectibleInfo` collectible is missing, not a username or phone, the username is not 4–32 characters starting with a letter, or the phone is not 7–15 digits |
| `COLLECTIBLE_NOT_FOUND` | the username or phone is well formed and no Fragment purchase is stored |
| `RANDOM_ID_EMPTY` | `payments.sendStarGiftOffer` `random_id` is 0 |
| `PASSWORD_HASH_INVALID` | withdrawal `tmp_password` is empty. A non-empty value is not checked against 2FA |
| `AMOUNT_INVALID` | a resale amount or offer price is missing, not positive, or has `nanos` outside 0..999999999. `allow_paid_stars` when set must be ≥ 0 |

`payments.upgradeStarGift`, `payments.transferStarGift`,
`payments.updateStarGiftPrice`, `payments.craftStarGift`,
`payments.resolveStarGiftOffer`, and `payments.sendStarGiftOffer` do not
return `Updates`. `payments.depositStarGift` does not return
`payments.starGiftDepositPayload`. `payments.getUniqueStarGift` and
`payments.getUniqueStarGiftValueInfo` do not return an empty gift object.
`payments.getStarGiftWithdrawalUrl` does not return a URL.

## Payment forms, on-ramp, and store receipts (batch 3)

`RPCPayments` is registered on this process. `account.getTmpPassword` stays
on that service and is still built by the two-factor core. There is no card
processor, no invoice row, no receipt, and no on-ramp provider. Saved
checkout info is kept in memory and dropped on restart. Credentials are not
stored. Affiliate links live next to the in-memory bot star-ref program and
are also dropped on restart. App Store and Play receipts are not verified.

Android `stars.*` gift call sites are counted on the payments methods above.
There is still no `stars` RPC service.
`toncenter.createOnrampSession` and `toncenter.getOnrampProviders` are not in
the layer-232 schema and are unchanged.

### Schema-valid results

- `payments.getSavedInfo` returns `payments.savedInfo`. `saved_info` is set
  only after `payments.validateRequestedInfo` with `save:true`.
  `has_saved_credentials` stays unset.
- `payments.clearSavedInfo` returns `boolTrue`. `info:true` drops the saved
  info. `credentials:true` is accepted and there is nothing to drop. Neither
  flag leaves the saved info in place.
- `payments.validateRequestedInfo` returns `payments.validatedRequestedInfo`
  with an id (`ri{user}-{n}`) and no shipping options. The id is remembered
  for `payments.sendPaymentForm` even when `save` is not set. A name, phone,
  email, or address that is present is checked. At least one field is
  required.
- `payments.getBankCardData` returns `payments.bankCardData` for a 13–19
  digit number that passes the Luhn check. Spaces and dashes are ignored.
  The title is Visa, Mastercard, American Express, UnionPay, or Card.
  `open_urls` is empty.
- `payments.getCurrencyRates` returns `payments.currencyRates` with an empty
  `rates` vector. No rate feed is configured.
- `payments.getOnrampProviders` returns an empty vector. `crypto_currency`,
  when set, must be 2–16 letters or digits.
- `payments.getOnrampBaseCurrencies` returns an empty vector after the
  provider id and crypto currency are checked.
- `payments.getOnrampAvailability` returns `onrampAvailability` with
  `allowed` and `buy_allowed` unset, `country_code` `ZZ`, and no methods.
  Every well-formed provider is not available.
- `payments.getOnrampLimits` returns `onrampLimits` with every amount `"0"`
  and the requested base currency. `payment_method`, when set, is copied
  through.
- `payments.getConnectedStarRefBots` returns `payments.connectedStarRefBots`.
  `count` is the total. The vector is one page, newest date first, then url.
  `limit` 0 means 20. `offset_date` and `offset_link` must both be set and
  must match one connection; the next page starts after that row. Users are
  filled for the bots on the page.
- `payments.getConnectedStarRefBot` returns that one connection, or count 0
  and empty vectors when the bot is not connected.
- `payments.getSuggestedStarRefBots` returns bots in this process that have
  an open affiliate program, ordered by bot id. `order_by_revenue` and
  `order_by_date` are accepted and do not change the order. `offset` empty
  starts at 0. A non-empty offset must be a decimal index this method would
  return in `next_offset`. `count` is the total. `next_offset` is omitted on
  the last page.
- `payments.connectStarRefBot` stores a connection and returns the peer's
  full list. The url is `https://t.me/{username}?start=starref_{kind}_{id}`,
  or `id{bot}` when the bot has no username. Commission is copied from the
  program. Connecting again clears `revoked` and refreshes the commission.
- `payments.editConnectedStarRefBot` sets or clears `revoked` and returns the
  full list. Restoring a link requires the program to still be open.

Peer, user, message, slug, and saved-gift errors use the codes in the tables above.

### Documented 400s

| Code | When |
| --- | --- |
| `INVOICE_INVALID` | the invoice is missing, the constructor is unknown, or a well-formed plain invoice is not stored. `payments.getPaymentForm` does not return a form |
| `STARGIFT_NOT_FOUND` | `payments.getPaymentForm` for a well-formed star-gift invoice. No gift is stored |
| `THEME_PARAMS_INVALID` | `theme_params` is present and is not `dataJSON` |
| `PREMIUM_PURPOSE_INVALID` | an invoice purpose is missing, the constructor is unknown, or a required field is missing. Currency must be 1–8 letters and the amount must be positive |
| `MONTH_INVALID` | `inputInvoicePremiumGiftStars.months` is outside 1..36 |
| `GIFT_STARS_INVALID` | `inputInvoiceBusinessBotTransferStars.stars` is not positive |
| `STARGIFT_MESSAGE_INVALID` | a gift message is empty, longer than 256 characters, or contains a control character other than newline or tab |
| `MESSAGE_ID_INVALID` | `payments.getPaymentReceipt`. No receipt message is stored, including when `msg_id` is 0 |
| `PAYMENT_INFO_INVALID` | requested info is missing, not `paymentRequestedInfo`, or has no name, phone, email, or address |
| `NAME_INVALID` | a name is empty, longer than 128 characters, or contains a control character |
| `PHONE_NUMBER_INVALID` | a phone is not 7–15 digits |
| `EMAIL_INVALID` | an email has no single `@`, or the domain has no dot |
| `SHIPPING_ADDRESS_INVALID` | street line 1, city, or post code is empty or too long, or `country_iso2` is not two letters |
| `FORM_ID_INVALID` | `payments.sendPaymentForm` `form_id` is not positive |
| `PAYMENT_CREDENTIALS_INVALID` | credentials are missing, the wrong constructor, or `dataJSON` is empty. A saved-credential id that is empty uses this code too |
| `TMP_PASSWORD_INVALID` | `inputPaymentCredentialsSaved.tmp_password` is empty. A non-empty value is not checked against 2FA |
| `SHIPPING_OPTION_INVALID` | `shipping_option_id` is set. No shipping option is issued |
| `REQUESTED_INFO_INVALID` | `requested_info_id` is set and is not the last id this process issued for the user |
| `PAYMENT_FAILED` | `payments.sendPaymentForm` after the invoice and credentials are well formed. Nothing is charged |
| `BANK_CARD_NUMBER_INVALID` | the card number is not 13–19 digits or fails the Luhn check |
| `ONRAMP_PROVIDER_INVALID` | a provider id is empty, longer than 64 characters, or not letters, digits, `_`, or `-` |
| `ONRAMP_CURRENCY_INVALID` | a currency code is not 2–16 letters or digits, or an on-ramp address is not 8–128 characters of letters, digits, `:`, `_`, or `-` |
| `ONRAMP_METHOD_INVALID` | `payment_method` is set and is not a provider-style id |
| `ONRAMP_AMOUNT_INVALID` | a quote sets both amounts or neither, or an amount is not a positive decimal. A leading or trailing dot is invalid |
| `ONRAMP_UNAVAILABLE` | `payments.getOnrampQuote` and `payments.createOnrampSession` after the arguments are well formed. No quote or session URL is returned |
| `URL_INVALID` | a return URL is set and is not `http://` or `https://`, or is longer than 512 characters |
| `THEME_INVALID` | an on-ramp theme is set and is empty, longer than 128 characters, or contains a control character |
| `MEMO_INVALID` | an on-ramp memo is set and fails the same text check |
| `STARREF_NOT_FOUND` | the bot has no open affiliate program |
| `STARREF_AWAITING_END` | the program was ended and the cooldown has not passed |
| `STARREF_LINK_INVALID` | `payments.editConnectedStarRefBot` link is empty or not a connection on that peer |
| `BOT_INVALID` | the affiliate bot is missing, not a bot this process created, or the access hash does not match |
| `LIMIT_INVALID` | a StarRef `limit` is negative or greater than 100 |
| `OFFSET_INVALID` | a StarRef offset is not a pair this method issued, or only one of `offset_date` and `offset_link` is set |
| `RECEIPT_EMPTY` | an App Store receipt has length 0, or a Play receipt is missing, not `dataJSON`, or has empty `data` |
| `INPUT_PURPOSE_INVALID` | a store purpose is missing or not a known constructor with currency, a positive amount, and its other required fields |
| `RECEIPT_INVALID` | `payments.assignAppStoreTransaction` and `payments.assignPlayMarketTransaction` after the receipt and purpose are well formed. The receipt is not verified and `Updates` is not returned |

`payments.sendPaymentForm` does not return `payments.paymentResult`.
`payments.getPaymentForm` does not return a form.
`payments.getPaymentReceipt` does not return a receipt.
`payments.assignAppStoreTransaction` and `payments.assignPlayMarketTransaction`
do not return `Updates`. An empty updates box would be applied to the
client's sequence. `payments.getOnrampQuote` does not return `onrampQuote`.
`payments.createOnrampSession` does not return `onrampSession`.
