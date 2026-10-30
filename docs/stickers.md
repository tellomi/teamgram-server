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
| `messages.getAvailableReactions` | 👍❤️🔥🥰👏😁🤔😢🎉🙏👎😱 plus the other seeded reactions. Animations are 512×512 `.tgs`, with a small fallback Lottie when Noto has no file. |
| `messages.getTopReactions` | Those reactions sorted by how often they have been sent. Unused counts keep the catalog order. A matching hash returns `messages.reactionsNotModified`. |
| `messages.getRecentReactions` | Emoji this user has sent, newest first, with the same hash. `messages.clearRecentReactions` empties the list. |

A matching hash returns the `notModified` variant. `messages.getMaskStickers` returns an empty list; there is no mask pack.

Sticker documents, custom-emoji documents, and reaction lists omit emoji on the blocklist below. A set that still has other emoji keeps those emoji. Its count, cover, and set hash follow what was returned. A set whose emoji are all blocked is left out of installed, featured, and search lists. `messages.getStickerSet` for that set returns an empty document list. `messages.getCustomEmojiDocuments` returns `documentEmpty` for a blocked id. `messages.getStickers`, `messages.searchStickers`, `messages.searchCustomEmoji`, recent stickers, and faved stickers drop blocked items. `messages.getAvailableReactions`, `messages.getTopReactions`, and `messages.getRecentReactions` do the same. Emoji keywords do too. A reaction already stored on a message is not rewritten.

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

Document bytes are written through the dfs S3 client (`MinioConfig.ApplyEnv`, then the same `resolveLocation` path as `PutStickerBytes` / `GetStickerFile`). `importstickers` and the bff background upload do not open a second client. `upload.getFile` reads those objects back with `GetStickerFile` / `GetEmojiFile`. TLS, region, path style, bucket, and key all come from the `TELLOMI_S3_*` variables in `docs/storage.md`. The link-preview `TELLOMI_MINIO_*` settings are not used.

| Class | Bucket | Key |
|---|---|---|
| Twemoji and Fluent | `TELLOMI_S3_BUCKET_PUBLIC_MEDIA` | `stickers/<document id>.dat` |
| Noto TGS | the same bucket | `emoji/<document id>.dat` |

`upload.getFile` serves a seeded document from the binary when the id matches, then dfs reads `stickers/<id>.dat` and `emoji/<id>.dat` in `TELLOMI_S3_BUCKET_PUBLIC_MEDIA`. A set thumbnail (`inputStickerSetThumb`) is rewritten to the cover document.

`messages.getAvailableReactions` includes 👍❤️🔥🥰👏😁🤔😢🎉🙏👎😱, with 👍 first. An entry on `emoji_blocklist` is left out. `static_icon` and `center_icon` are WebP. Every animation field is a 512×512 `.tgs` (`DocumentAttributeSticker` and `DocumentAttributeImageSize`, not a custom emoji). A missing Noto file is replaced with a small valid Lottie instead of dropping the reaction. `help.getConfig` `reactions_default` and app config `reactions_default` stay 👍 even if that emoji is blocked. `MessageReactions` is that emoji plus a count. A chat's saved available-reactions list is not rewritten.

The bff uploads those objects in the background when `TELLOMI_S3_ENDPOINT`, the access keys, `TELLOMI_S3_BUCKET_ATTACHMENTS`, and `TELLOMI_S3_BUCKET_PUBLIC_MEDIA` are set. Buckets must already exist. Leave `TELLOMI_S3_USE_SSL` unset for an `https://` R2 endpoint so dfs turns TLS on. An explicit `false` forces plain HTTP, and the read never reaches objects written over TLS. Set `TELLOMI_S3_REGION=auto` for R2.

## Database

`teamgramd/deploy/sql/migrate-20261029-stickers.sql` creates `tellomi_sticker_packs`, `tellomi_sticker_items`, `tellomi_sticker_installs`, `tellomi_sticker_recent`, and `tellomi_sticker_faved`. The bff runs the same statements on startup when `ChannelMySQL` or `TEAMGRAM_MYSQL_DSN` is set, then upserts the seed. Running it again updates the same rows.

The link-preview table `sticker_sets` is separate. A successful upsert also writes a public row there so `/addstickers/<short name>` can resolve. That write is skipped if the preview table is absent.

Without MySQL the catalog still serves from memory. Installs, recents, and favorites are then lost when the process exits.

## Import

The image builds `importstickers` in the Go stage and copies it to `/app/bin/importstickers`. The converted emoji are embedded in that binary. It does not fetch packs from the network. Running it again updates the same rows and objects.

From a checkout, with MySQL and S3 credentials in the environment:

```
TEAMGRAM_MYSQL_DSN='user:pass@tcp(127.0.0.1:3306)/teamgram?charset=utf8mb4&parseTime=true' \
TELLOMI_S3_ENDPOINT=http://127.0.0.1:9000 \
TELLOMI_S3_ACCESS_KEY_ID=minio \
TELLOMI_S3_SECRET_ACCESS_KEY=miniostorage \
TELLOMI_S3_BUCKET_ATTACHMENTS=tellomi-attachments \
TELLOMI_S3_BUCKET_PUBLIC_MEDIA=tellomi-public-media \
go run ./app/bff/stickers/cmd/importstickers
```

Inside the Compose stack, `docker compose exec` runs in the already-started `teamgram` container and inherits its `TELLOMI_S3_*` variables. Pass the same DSN the bff uses (`teamgramd/etc2/bff.yaml`):

```
docker compose exec teamgram /app/bin/importstickers \
  -dsn 'teamgram:teamgram@tcp(mysql:3306)/teamgram?charset=utf8mb4&parseTime=true'
```

`docker compose run` starts a new container and would otherwise run the process entrypoint. Override it:

```
docker compose run --rm --no-deps --entrypoint /app/bin/importstickers teamgram \
  -dsn 'teamgram:teamgram@tcp(mysql:3306)/teamgram?charset=utf8mb4&parseTime=true'
```

Session must route `/mtproto.RPCStickers`, `/mtproto.RPCEmoji`, and `/mtproto.RPCCustomEmojis` to `bff.bff` (`teamgramd/etc2/session.yaml` in this container). Without those routes the client never reaches this catalog.

## Emoji blocklist

`help.getAppConfig` (both constructors) includes `emoji_blocklist`, a JSON array of emoji strings. Clients hide those emoji in the sticker, custom-emoji, and reaction panels. The server applies the same list to the payloads in the section above, so a client that skips its own filter still does not receive them.

The default array is one entry: the Taiwan flag 🇹🇼, regional indicators T + W (`U+1F1F9` `U+1F1FC`). That entry is always present. The app-config hash changes when the list changes. A client that sends the current hash receives `help.appConfigNotModified`.

The bff reads the list at startup. Restart bff after changing it. Unset variables use the default.

| Variable | Default | Meaning |
|---|---|---|
| `TELLOMI_EMOJI_BLOCKLIST` | empty | Extra entries, separated by commas or newlines. Appended after the Taiwan flag. Duplicates are ignored. |
| `TELLOMI_EMOJI_BLOCKLIST_FILE` | empty | Optional UTF-8 file. Same syntax, one group of entries per line. Blank lines and lines starting with `#` are ignored. Appended after the environment variable. A missing file is logged and skipped. |

An entry is either the emoji itself or a code-point sequence. Commas separate entries. Spaces, hyphens, and underscores separate code points inside one entry. `U+` is optional. `U+FE0E` and `U+FE0F` are ignored when matching, so `❤` and `❤️` are the same emoji. The published string does not include those selectors.

```
TELLOMI_EMOJI_BLOCKLIST=1F44D, U+1F525
```

That blocks 👍 and 🔥 in addition to 🇹🇼. A code-point pair has to stay in one entry: `1F1E8-1F1F3` is one emoji, while `1F1E8,1F1F3` is two entries.

With only the default, sticker and reaction hashes stay the same, because the seeded packs do not contain 🇹🇼. Adding an entry that removes an item changes that set's hash and the list hash, so clients refetch. The emoji-keyword version stays `1` until an extra entry is configured.

## Still missing

- The rest of Twemoji, Fluent, and Noto beyond the seeded characters.
- `.webm` video stickers.
- Masks, dice sets, and user-created packs (`stickers.createStickerSet` stays unimplemented).
- Animated emoji that a client refuses because the Lottie is larger than 512×512.
- Per-language keyword lists other than English.
