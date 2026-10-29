# Premium for everyone

Tellomi grants Telegram Premium to every non-deleted account. Official iOS and Android clients are unchanged. The server sets `user.premium` on every user object it returns, raises the limits those clients read from `help.getAppConfig`, and accepts the Premium-gated RPCs below.

Purchases stay off. `premium_purchase_blocked` is true, `help.getPremiumPromo` has no subscription periods, and `payments.canPurchasePremium` / `payments.canPurchaseStore` return `boolFalse`. App Store and Play billing receipts return `PAYMENT_UNSUPPORTED`. `users.getFullUser` does not include gift prices.

## Who is Premium

`ImmutableUser.Premium()` is true for every user that is not deleted, including bots. `user.premium` on the MTProto `User` for self and for other users follows that. Deleted users stay `premium=false`.

The `users.premium` and `users.premium_expire_date` columns are not consulted when building a user object. A row left at the old default still shows as Premium. Link-preview cards for users and bots do the same.

## App config

`help.getAppConfig` (both constructors) returns one JSON object. The hash is `0x544c5032`. A client that sends that hash back receives `help.appConfigNotModified`.

`premium_purchase_blocked` is true. Each limit is advertised twice, under the `_default` and `_premium` keys, and both values are the raised one. A client that still treats the account as non-Premium therefore applies the same cap.

| Key prefix | Value |
|---|---|
| `about_length_limit` | 140 characters |
| `caption_length_limit` | 4096 |
| `dialogs_pinned_limit` | 100 |
| `dialogs_folder_pinned_limit` | 200 |
| `dialog_filters_limit` | 20 |
| `dialog_filters_chats_limit` | 200 |
| `saved_gifs_limit` | 400 |
| `stickers_faved_limit` | 200 |
| `channels_limit` | 1000 |
| `channels_public_limit` | 20 |
| `reactions_user_max` | 3 |
| `upload_max_fileparts` | 8000 |
| `stickers_premium_by_emoji_num` | 100 |

`reactions_uniq_max` is 11. `transcribe_audio_trial_weekly_number` is 100000 so a client does not open a trial paywall before calling transcription.

## What the server accepts

| Feature | Behavior |
|---|---|
| Biography | `account.updateProfile` allows 140 Unicode code points. Longer text returns `ABOUT_TOO_LONG`. |
| Pinned chats | Moving chats out of the archive keeps up to 100 pins. The previous cap was 5. |
| Emoji status | `account.updateEmojiStatus` stores any document id (or clears it) and pushes `updateUserEmojiStatus`. It does not check Premium. The default status catalog is empty until emoji documents are imported. |
| Name and profile colors | `help.getPeerColors` and `help.getPeerProfileColors` return a palette. `account.updateColor` saves the color id and optional background emoji. Color id 0 is stored as `1` in `users.color` / `users.profile_color` because `0` in that column means "no color". |
| Custom emoji in messages | `messageEntityCustomEmoji` is kept when a message is saved. The server does not reject it. Rendering still needs `messages.getCustomEmojiDocuments` from the sticker service. |
| Stories | Hitting the story cap returns `STORIES_TOO_MUCH`, not `PREMIUM_ACCOUNT_REQUIRED`. |
| Voice to text | `messages.transcribeAudio` is registered and does not return `PREMIUM_ACCOUNT_REQUIRED`. There is no speech engine in this server, so the call returns `TRANSCRIPTION_FAILED`. |

`messages.rateTranscribedAudio` returns `boolTrue`.

## Not stored yet

Channel accent colors (`channels.updateColor`) and channel emoji status are not persisted. Those RPCs are still unimplemented. User name colors, user profile colors, and user emoji status are stored.
