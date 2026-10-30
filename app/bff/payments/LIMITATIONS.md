# Stars, giveaways, boosts, and Star gifts

Batch 1 is the balance, subscription, giveaway, gift-code, and boost reads.
Batch 2 is the Star gift catalog, saved-gift pages, collections, auctions,
offers, and `fragment.getCollectibleInfo`. Payment forms, on-ramp, StarRef
bots, and store assign stay for later batches.

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
schema, so `docs/api-coverage.md` lists them as separate rows. There is no
`stars` RPC service. `stars.updatePaidMessagesPrice` is constructor
`0x4b12327b`, which is `channels.updatePaidMessagesPrice` and is already
handled. The other `stars.*` rows in the premium/payments section are the
gift constructors implemented here.

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
