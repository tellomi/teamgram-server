# Stars, giveaways, and boosts

Batch 1 of the premium/payments methods. These are the balance, subscription,
giveaway, gift-code, and boost reads. Star-gift inventory, payment forms, and
on-ramp stay for later batches.

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
