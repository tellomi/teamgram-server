# Premium membership

Official clients are unchanged. Tellomi keeps a real Premium membership: a per-user flag and expiry in MySQL, the limits clients read from `help.getAppConfig`, and a gate on each Premium feature. Nothing in the user object hard-codes `premium=true`.

Who counts as a member, and which features a non-member may use, is server config. Turning a feature from free to paid, or turning off the global switch, is enough to start requiring a subscription. New features can use the same gate.

## Database

`users.premium` and `users.premium_expire_date` are the subscription.

- `premium=0` is not a subscription.
- `premium=1` and `premium_expire_date=0` is a subscription with no end date.
- `premium=1` and `premium_expire_date` in the future is a subscription until that Unix time.
- A past `premium_expire_date` is expired even if the flag is still set.

`user.updatePremium` still writes those columns. The row is not rewritten just because the global switch is on.

## Config

Environment variables. Unset values use the defaults. A process reads them at startup; tests can call `premium.Reload()`.

| Variable | Default | Meaning |
|---|---|---|
| `TELLOMI_PREMIUM_EVERYONE` | `true` | When true, every non-deleted account is a Premium member. The database row is unchanged. When false, only an unexpired subscription is a member. |
| `TELLOMI_PREMIUM_FREE_FEATURES` | empty | Comma-separated features that non-members may use. `*` marks every feature free. An unknown token is ignored. |
| `TELLOMI_PREMIUM_PURCHASE_BLOCKED` | `true` | Sets `premium_purchase_blocked` in `help.getAppConfig`. |

`user.premium` on every `User` the server returns (self and others) is the effective membership: the global switch, or an active subscription. Deleted users are never Premium. Link-preview cards for users and bots use the same rule. Channels are not marked Premium by the global switch.

A paid feature is allowed when the account is an effective member. A free feature is allowed for everyone. The global switch makes every account a member, so paid features are included until that switch is turned off.

Feature tokens:

| Token | What it gates |
|---|---|
| `emoji_status` | `account.updateEmojiStatus` when setting a document. Clearing a status is always allowed. |
| `about` | Biography length. Free cap 70 characters, Premium cap 140. |
| `pinned_chats` | Main-list pins kept when chats leave the archive. Free cap 5, Premium cap 100. |
| `folder_pins` | Archive pin cap advertised to clients. Free 100, Premium 200. |
| `voice_to_text` | Not used by `messages.transcribeAudio`. Voice notes are recognized for every account by the self-hosted SenseVoice `asr` service. `TELLOMI_TRANSCRIBE_ENABLED` (default false) gates the client button: unset, `transcribe_audio_trial_weekly_number`, `transcribe_audio_trial_duration_max`, and `transcribe_audio_trial_cooldown_until` are 0 and clients hide it; `true` advertises 100000 / 300 / 0. Production compose sets it next to the `asr` service. |
| `custom_emoji` | `stickers_premium_by_emoji_num` in app config. Free or global membership advertises 100. Paid and not a member advertises 0. The server still stores `messageEntityCustomEmoji`. |
| `profile_color` | `account.updateColor` with `for_profile`. Clearing the color is always allowed. |
| `name_color` | `account.updateColor` for the name color. |
| `stories` | Daily story cap. Free 3, Premium 100. A non-member who hits the cap gets `PREMIUM_ACCOUNT_REQUIRED`. A member who hits it gets `STORIES_TOO_MUCH`. |
| `saved_gifs` | Saved GIF cap advertised to clients. Free 200, Premium 400. |
| `faved_stickers` | Favorite sticker cap advertised to clients. Free 5, Premium 200. |
| `dialog_filters` | Chat-folder count and chats per folder. Free 10 folders / 100 chats, Premium 20 / 200. |
| `upload` | `upload_max_fileparts`. Free 4000, Premium 8000. |
| `caption` | Caption length advertised to clients. Free 1024, Premium 4096. |
| `reactions` | Reactions one person may add. Free 1, Premium 3. |
| `channels` | Channels and megagroups an account may join. Free 500, Premium 1000. |
| `public_channels` | Public usernames. Free 10, Premium 20. |

Example: keep the badge off, give everyone emoji status, and charge for a longer biography.

```
TELLOMI_PREMIUM_EVERYONE=false
TELLOMI_PREMIUM_FREE_FEATURES=emoji_status
```

Accounts with `premium=1` and an unexpired `premium_expire_date` still receive the paid features. Everyone else can set an emoji status and is limited to a 70-character biography.

## What clients see

`help.getAppConfig` (both constructors) publishes each limit twice. The `_premium` value is always the raised cap. The `_default` value is the raised cap when the global switch is on or that feature is free, and the stock cap otherwise. The hash changes with the policy. A client that sends the current hash receives `help.appConfigNotModified`.

`emoji_blocklist` in that document is a JSON array of emoji to hide. The default is the Taiwan flag 🇹🇼 (`U+1F1F9` `U+1F1FC`). The hash also changes when the list changes. See [Stickers and emoji](stickers.md#emoji-blocklist).

`premium_purchase_blocked` follows `TELLOMI_PREMIUM_PURCHASE_BLOCKED`. While it is true, `help.getPremiumPromo` has no subscription periods, purchase checks return `boolFalse`, store receipts return `PAYMENT_UNSUPPORTED`, and `users.getFullUser` does not include gift prices.

The same document hides entry points official clients already know how to read. Android is `MessagesController` / `AppGlobalConfig` (DrKLO, 2026), iOS is `PremiumConfiguration` / `TranslationConfiguration` / `WalletConfiguration` (Telegram-iOS), Desktop is `Main::AppConfig`.

| Key | Value | What clients do |
|---|---|---|
| `premium_purchase_blocked` | `true` | Hides Premium purchase. Android's "Send a Gift" settings row uses `premiumPurchaseBlocked()`. The Premium and Business settings rows use `premiumFeaturesBlocked()`, which is `premium_purchase_blocked && !user.premium`. With `TELLOMI_PREMIUM_EVERYONE=true` those two rows stay, because every account is Premium. There is no separate Business hide key; `business_promo_order` only orders the promo. |
| `stars_purchase_blocked` | `true` | Hides Stars UI (`starsPurchaseAvailable` / `areStarsDisabled`). |
| `stars_gifts_enabled` | `false` | Star gifting. Android defaults this to true when the key is absent. |
| `stargifts_blocked` | `true` | Disables gifts. |
| `enable_gifts_in_profile` | `false` | Android profile gifts. Defaults to true when absent. |
| `premium_gift_attach_menu_icon` | `false` | Gift icon in the attachment menu. |
| `premium_gift_text_field_icon` | `false` | Gift icon in the text field. |
| `stars_paid_messages_available` | `false` | Paid messages. Android defaults this to true when absent. |
| `giveaway_gifts_purchase_available` | `false` | Starting a giveaway, which is the boosts purchase gate. |
| `channel_revenue_withdrawal_enabled` | `false` | Hides ad-revenue withdrawal. Sponsored-message slots themselves are not gated by an appConfig key. |
| `starref_program_allowed` | `false` | Creating a bot referral program (Desktop). |
| `starref_connect_allowed` | `false` | Joining a referral program (Desktop). |
| `wallet_available` | `false` | Wallet row. Desktop defaults this to true when absent. Android's Gram earnings row on standalone builds is not gated by this key. |
| `translations_manual_enabled` | `disabled` | Manual translation. Android's stored default is `enabled`. |
| `translations_auto_enabled` | `disabled` | Automatic translation. Same Android default. |
| `tellomi_ai_compose_enabled` | `false` | Tellomi key. Official clients ignore it. Compose-with-AI stays off. |
| `tellomi_ai_enabled` | `false` | Tellomi key. True only when `TELLOMI_AI=1` and `TELLOMI_AI_API_KEY` is set. Official clients ignore it. See `docs/api/tellomi-ai.md`. |
| `tellomi_translate_enabled` | `false` | Tellomi key. Official clients ignore it. |

`fragment_prefixes` is a list of anonymous Fragment phone prefixes, not a switch that hides fragment.com. `ton_topup_url` and `ton_proxy_address` are URLs, not hide switches, so they are left unset. `stories_posting` stays `enabled`.

`help.getPeerColors` and `help.getPeerProfileColors` still return the palette. Saving a color or an emoji status checks the feature gate and then stores the value. Color id 0 is stored as `1` in `users.color` / `users.profile_color` because `0` in that column means "no color".

## Not stored yet

Channel accent colors (`channels.updateColor`) and channel emoji status are not persisted. User name colors, user profile colors, and user emoji status are stored.
