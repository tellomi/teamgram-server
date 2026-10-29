# Stickers and emoji

Official Telegram clients ask the server for sticker sets, animated emoji, custom emoji documents, emoji keywords, and the reaction list before they draw anything in the sticker panel. Those RPCs are implemented here. The client source is unchanged.

The packs are openly licensed. Telegram's own sticker sets are not downloaded or redistributed.

## What the client can call

| RPC | Behavior |
|---|---|
| `messages.getAllStickers` | Installed regular packs. Twemoji is installed for every user until they uninstall it. |
| `messages.getStickerSet` | By id, short name, or the system constructors `inputStickerSetAnimatedEmoji`, `AnimatedEmojiAnimations`, `EmojiGenericAnimations`, `PremiumGifts`, `EmojiDefaultStatuses`, `EmojiDefaultTopicIcons`, `EmojiChannelDefaultStatuses`. |
| `messages.getFeaturedStickers` / `messages.getFeaturedEmojiStickers` | Featured regular packs and the Noto custom-emoji pack. |
| `messages.installStickerSet` / `messages.uninstallStickerSet` | Per-user install state. Uninstalling Twemoji sticks; it is not forced back. |
| `messages.searchStickerSets` / `messages.searchEmojiStickerSets` | Title and short-name search. |
| `messages.getStickers` / `messages.searchStickers` | Documents for an emoji or a keyword. |
| `messages.getRecentStickers` / `messages.saveRecentSticker` / `messages.clearRecentStickers` | The last 50 stickers the user sent. |
| `messages.getFavedStickers` / `messages.faveSticker` | Up to 200 favorites. |
| `messages.getEmojiKeywords` / `Difference` / `Languages` | English keywords for the seeded emoji. Version is `1`. |
| `messages.getCustomEmojiDocuments` / `messages.searchCustomEmoji` | Noto documents, marked free so they are not premium-gated. |
| `messages.getEmojiStickers` | The installed Noto custom-emoji pack. |
| `messages.getAvailableReactions` | Reaction buttons whose static icon is Twemoji and whose animation is the Noto TGS, when one exists. |

A matching hash returns the `notModified` variant. `messages.getMaskStickers` returns an empty list; there is no mask pack.

`inputStickerSetDice` is not seeded. Dice messages still show the emoji character.

## Packs in the seed

| Short name | Source | License | Format |
|---|---|---|---|
| `Twemoji` | [jdecked/twemoji](https://github.com/jdecked/twemoji) 15.1.0, 72×72 PNG | CC-BY 4.0. Twemoji by Twitter, Inc. and other contributors. | lossless `.webp` |
| `FluentEmoji` | [microsoft/fluentui-emoji](https://github.com/microsoft/fluentui-emoji) 3D PNG (grinning face, fire, red heart) | MIT. Fluent Emoji by Microsoft. | lossless `.webp` |
| `NotoEmoji` and the system animated sets | [googlefonts/noto-emoji](https://github.com/googlefonts/noto-emoji) animated Lottie, via `fonts.gstatic.com` | OFL-1.1 for the Noto color-emoji artwork and Apache-2.0 for the animation data. Noto Emoji Animation by Google. | `.tgs` (gzip of the Lottie JSON) |

The seed is a few dozen common emoji, not the full Unicode set. Files live in `app/bff/stickers/catalog/assets/`.

Noto's Lottie files are 1024×1024 at 60 fps. Telegram's sticker upload rules ask for 512×512. The client player may still play them; if a particular animation does not render, the Twemoji WebP for the same character is the static fallback in the sticker panel and on reaction buttons.

There is no `.webm` video-sticker pack in this seed. `stickerSet.videos` is false.

## Storage

Document bytes are written to the existing dfs/S3 layout:

| Class | Key |
|---|---|
| Twemoji and Fluent | `stickers/<document id>.dat` in `TELLOMI_S3_BUCKET_PUBLIC_MEDIA` |
| Noto TGS | `emoji/<document id>.dat` in the same bucket |

`upload.getFile` reads `documents/<id>.dat` first, then `stickers/<id>.dat` and `emoji/<id>.dat`. A set thumbnail (`inputStickerSetThumb`) is rewritten to the cover document.

The bff uploads those objects in the background when `TELLOMI_S3_ENDPOINT` and `TELLOMI_S3_ACCESS_KEY_ID` are set. Buckets must already exist. See `docs/storage.md`.

## Database

`teamgramd/deploy/sql/migrate-20261029-stickers.sql` creates `tellomi_sticker_packs`, `tellomi_sticker_items`, `tellomi_sticker_installs`, `tellomi_sticker_recent`, and `tellomi_sticker_faved`. The bff runs the same statements on startup when `ChannelMySQL` or `TEAMGRAM_MYSQL_DSN` is set, then upserts the seed. Running it again updates the same rows.

The link-preview table `sticker_sets` is separate. A successful upsert also writes a public row there so `/addstickers/<short name>` can resolve. That write is skipped if the preview table is absent.

Without MySQL the catalog still serves from memory. Installs, recents, and favorites are then lost when the process exits.

## Import

```
TEAMGRAM_MYSQL_DSN='user:pass@tcp(127.0.0.1:3306)/teamgram?charset=utf8mb4&parseTime=true' \
TELLOMI_S3_ENDPOINT=http://127.0.0.1:9000 \
TELLOMI_S3_ACCESS_KEY_ID=minio \
TELLOMI_S3_SECRET_ACCESS_KEY=miniostorage \
TELLOMI_S3_BUCKET_ATTACHMENTS=tellomi-attachments \
TELLOMI_S3_BUCKET_PUBLIC_MEDIA=tellomi-public-media \
go run ./app/bff/stickers/cmd/importstickers
```

The command is idempotent. It does not fetch anything from the network; it uploads the files already converted into the repo.

## Still missing

- The rest of Twemoji, Fluent, and Noto beyond the seeded characters.
- `.webm` video stickers.
- Masks, dice sets, and user-created packs (`stickers.createStickerSet` stays unimplemented).
- Animated emoji that a client refuses because the Lottie is larger than 512×512.
- Per-language keyword lists other than English.
