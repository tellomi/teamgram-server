# Stickers and emoji

Official Telegram clients ask the server for sticker sets, animated emoji, custom emoji documents, emoji keywords, and the reaction list before they draw anything in the sticker panel. Those RPCs are implemented here. The client source is unchanged.

The packs are openly licensed. Telegram's own sticker sets are not downloaded or redistributed.

## What the client can call

| RPC | Behavior |
|---|---|
| `messages.getAllStickers` | Installed regular packs. A pack with `installed_default` is in this list for every user until that user uninstalls it. Twemoji and Fluent Emoji are not in the list. The reply is set metadata, not sticker bytes. A matching hash is `messages.allStickersNotModified` and does not rebuild those sets. |
| `messages.getStickerSet` | By id, short name, or the system constructors `inputStickerSetAnimatedEmoji`, `AnimatedEmojiAnimations`, `EmojiGenericAnimations`, `PremiumGifts`, `EmojiDefaultStatuses`, `EmojiDefaultTopicIcons`, `EmojiChannelDefaultStatuses`. |
| `messages.getFeaturedStickers` / `messages.getFeaturedEmojiStickers` | The first 100 featured packs (`count` is the full list). Imported packs are featured. Twemoji, Fluent Emoji, and Noto Emoji are not. A matching hash is `messages.featuredStickersNotModified`. |
| `messages.getEmojiStickers` | Installed custom-emoji packs, including imported packs with `emojis` set, for every user until that user uninstalls one. |
| `messages.getOldFeaturedStickers` | The same featured list, paged. `offset` is the index and `limit` defaults to 100. |
| `messages.installStickerSet` / `messages.uninstallStickerSet` | Per-user install state. Uninstalling Twemoji sticks; it is not forced back. |
| `messages.searchStickerSets` / `messages.searchEmojiStickerSets` | Title, short name, sticker keyword, and emoticon search. The match is a UTF-8 substring after Unicode case fold and fullwidth fold, so `猫猫1` matches `猫猫１`, `开心` matches a sticker keyword, and 🙂 matches that sticker's emoticon. Twemoji, Fluent Emoji, and Noto Emoji are left out. |
| `messages.getStickers` / `messages.searchStickers` | Documents for an emoji or a keyword. Emoji match ignores `U+FE0E` and `U+FE0F`. Keyword match is a folded substring of that sticker's keywords. `limit` defaults to 100 and `next_offset` continues the page. |
| `messages.getRecentStickers` / `messages.saveRecentSticker` / `messages.clearRecentStickers` | The last 50 stickers the user sent. |
| `messages.getFavedStickers` / `messages.faveSticker` | Up to 200 favorites. |
| `messages.getEmojiKeywords` / `Difference` / `Languages` | Keywords aggregated per language from sticker keywords. English stays version `1` until that list changes. `zh-hans`, `zh-CN`, and `zh` all read simplified Chinese. The reply `lang_code` is the code the client sent. |
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

There is no `.webm` video-sticker pack in this seed. `stickerSet.videos` is false for the seed. Prepared video packs imported with `importstickers -dir` set it.

## Sticker panel search

Official clients do not send one language code.

Android opens the sticker search field and calls `messages.getEmojiKeywords` (or `Difference`, once a version is cached) for `AndroidUtilities.getCurrentKeyboardLanguage()`. That is the IME language tag with `_` rewritten to `-`: a Chinese keyboard sends `zh-CN`, `zh-Hans`, or `zh`. The sticker results themselves are `messages.searchStickers` with `q` set to the typed text, `lang_code` set to that first language, `limit` 100, and `emoticon` set to the emoji concatenation of an exact local keyword hit. `messages.searchStickerSets` is the pack-title search and has no language argument.

iOS syncs keywords from `ManagedSynchronizeEmojiKeywordsOperations` with the app language (`zh-hans` for simplified Chinese). A later difference update is kept only when the reply `lang_code` equals the code iOS asked for. iOS does not call `messages.getEmojiKeywordsLanguages`.

Desktop calls `messages.getEmojiKeywordsLanguages` with its keyboard languages and then fetches only the codes in the reply. It discards a keyword list whose `lang_code` is not the code it requested.

The server therefore echoes the requested code, and treats `zh`, `zh-CN`, `zh-Hans`, and `zh-hans` as one simplified list. `zh-TW`, `zh-HK`, and `zh-hant` are traditional. English is unchanged. A Han query also searches the Chinese lists when the IME language is English. Untagged keywords that contain Han, kana, or hangul are simplified Chinese. Any other untagged keyword is English. Words tagged in the import manifest stay on that language.

Restart bff after an import. The process loads imported packs from MySQL at startup, and clients refetch keywords when that language's version changes.

## Storage

Document bytes are written through the dfs S3 client (`MinioConfig.ApplyEnv`, then the same `resolveLocation` path as `PutStickerBytes` / `GetStickerFile`). `importstickers` and the bff background upload do not open a second client. `upload.getFile` reads those objects back with `GetStickerFile` / `GetEmojiFile`. TLS, region, path style, bucket, and key all come from the `TELLOMI_S3_*` variables in `docs/storage.md`. The link-preview `TELLOMI_MINIO_*` settings are not used.

| Class | Bucket | Key |
|---|---|---|
| Twemoji, Fluent, and imported WebP or WebM | `TELLOMI_S3_BUCKET_PUBLIC_MEDIA` | `stickers/<document id>.dat` |
| Noto TGS | the same bucket | `emoji/<document id>.dat` |

`upload.getFile` serves a seeded document from the binary when the id matches, then dfs reads `stickers/<id>.dat` and `emoji/<id>.dat` in `TELLOMI_S3_BUCKET_PUBLIC_MEDIA`. A set thumbnail (`inputStickerSetThumb`) is rewritten to the cover document.

`messages.getAvailableReactions` includes 👍❤️🔥🥰👏😁🤔😢🎉🙏👎😱, with 👍 first. An entry on `emoji_blocklist` is left out. `static_icon` and `center_icon` are WebP. Every animation field is a 512×512 `.tgs` (`DocumentAttributeSticker` and `DocumentAttributeImageSize`, not a custom emoji). A missing Noto file is replaced with a small valid Lottie instead of dropping the reaction. `help.getConfig` `reactions_default` and app config `reactions_default` stay 👍 even if that emoji is blocked. `MessageReactions` is that emoji plus a count. A chat's saved available-reactions list is not rewritten.

The bff uploads those objects in the background when `TELLOMI_S3_ENDPOINT`, the access keys, `TELLOMI_S3_BUCKET_ATTACHMENTS`, and `TELLOMI_S3_BUCKET_PUBLIC_MEDIA` are set. Buckets must already exist. Leave `TELLOMI_S3_USE_SSL` unset for an `https://` R2 endpoint so dfs turns TLS on. An explicit `false` forces plain HTTP, and the read never reaches objects written over TLS. Set `TELLOMI_S3_REGION=auto` for R2.

## Database

`teamgramd/deploy/sql/migrate-20261029-stickers.sql` creates `tellomi_sticker_packs`, `tellomi_sticker_items`, `tellomi_sticker_installs`, `tellomi_sticker_recent`, and `tellomi_sticker_faved`. The bff runs the same statements on startup when `ChannelMySQL` or `TEAMGRAM_MYSQL_DSN` is set, then upserts the seed. Running it again updates the same rows. `migrate-20261121-sticker-keywords.sql` widens `tellomi_sticker_items.keywords` to `TEXT`. The bff applies that `ALTER` on startup. A sticker with more than one keyword language is stored as JSON. An English-only list stays newline-separated.

The link-preview table `sticker_sets` is separate. A successful upsert also writes a public row there so `/addstickers/<short name>` can resolve. That write is skipped if the preview table is absent.

Without MySQL the catalog still serves from memory. Installs, recents, and favorites are then lost when the process exits.

## Import

The image builds `importstickers` in the Go stage and copies it to `/app/bin/importstickers`. The converted emoji are embedded in that binary. It does not fetch packs from the network. Running it again updates the same rows and objects.

### Prepared packs

`-dir` reads a directory of packs that are already converted. The directory is one pack when it contains `manifest.json`. Otherwise each immediate child directory with a `manifest.json` is a pack. A real run also upserts the built-in emoji packs. `-dry-run` validates and prints the plan, and does not need MySQL or S3.

```
go run ./app/bff/stickers/cmd/importstickers -dir ./chinese-bqb -featured -dry-run
```

```
TEAMGRAM_MYSQL_DSN='user:pass@tcp(127.0.0.1:3306)/teamgram?charset=utf8mb4&parseTime=true' \
TELLOMI_S3_ENDPOINT=http://127.0.0.1:9000 \
TELLOMI_S3_ACCESS_KEY_ID=minio \
TELLOMI_S3_SECRET_ACCESS_KEY=miniostorage \
TELLOMI_S3_BUCKET_ATTACHMENTS=tellomi-attachments \
TELLOMI_S3_BUCKET_PUBLIC_MEDIA=tellomi-public-media \
go run ./app/bff/stickers/cmd/importstickers -dir ./chinese-bqb -featured
```

Packs from `-dir` are featured and installed for every user. `messages.getAllStickers` (or `messages.getEmojiStickers` when the manifest sets `emojis`) returns them until that user uninstalls one. `-featured=false` or `-installed-default=false` leaves the manifest flag. `-drop-foreign-seeds` unfeatures and uninstalls Twemoji, Fluent Emoji, and Noto Emoji. It is safe to run again. Deploy this build before restarting bff, then run it so a previous process does not mark those packs featured on startup:

```
go run ./app/bff/stickers/cmd/importstickers -drop-foreign-seeds
```

`type` `custom_emoji` (or `"emojis": true` with `static` or `video`) is a custom-emoji set. A static one holds at most 200 stickers. Documents are marked free custom emoji, and the set is served by `messages.getEmojiStickers` and `messages.getFeaturedEmojiStickers`. Inside Compose, pass the flags after the entrypoint:

```
docker compose exec teamgram /app/bin/importstickers \
  -dsn 'teamgram:teamgram@tcp(mysql:3306)/teamgram?charset=utf8mb4&parseTime=true' \
  -dir /packs/chinese-bqb -featured
```

The command is safe to run again. Pack and document ids come from the short name and file name, so the same manifest updates the same rows. An object already stored at the same byte size is not uploaded again. A crash mid-upload continues on the next run. Stickers removed from the manifest are removed from the pack. Restart bff after the import so the running process loads the new packs.

A static pack holds at most 120 stickers. A video pack holds at most 50. A larger manifest is split, in order, into `标题 1/2`, `标题 2/2`, and so on. The first part keeps `short_name`. Later parts append the part number (`PandaHead`, `PandaHead2`). A manifest with more than 20000 stickers is rejected.

`type` is `static`, `video`, or `custom_emoji` (`webp`, `webm`, `emoji`, and `custom_emoji_video` are accepted aliases). `"emojis": true` with `static` or `video` is the same as a custom-emoji set. A static file is a 512×512 WebP, at most 1MB. A video file is a 512×512 VP9 WebM, at most 3 seconds and 256KB. A static pack holds at most 120 stickers, a video pack 50, and a custom-emoji pack 200. `short_name` matches `^[A-Za-z][A-Za-z0-9_]{4,31}$`. `title` is 1 to 64 characters. Each sticker needs an `emoticon`. `keywords` is a string, an array, or a language map. An untagged array is stored as written: Han, kana, and hangul words are simplified Chinese and other words are English. A language map may use `zh-hans`, `zh-hant`, `zh-CN`, `zh`, or `en`. At most 32 keywords, each at most 64 characters.

```json
{
  "title": "熊猫头",
  "short_name": "PandaHead",
  "type": "static",
  "emojis": false,
  "license": "",
  "attribution": "ChineseBQB",
  "source_url": "https://github.com/zhaoolee/ChineseBQB",
  "stickers": [
    {
      "file": "001.webp",
      "emoticon": "😂",
      "keywords": {
        "zh-hans": ["熊猫头", "笑哭"],
        "en": ["panda", "lol"]
      }
    }
  ]
}
```

`file` is relative to the manifest directory. `license` is at most 64 characters, `attribution` 512, and `source_url` 255.

### Built-in emoji

Without `-dir`, the command only upserts the embedded emoji packs.

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

With only the default, sticker and reaction hashes stay the same, because the seeded packs do not contain 🇹🇼. Adding an entry that removes an item changes that set's hash and the list hash, so clients refetch. The English emoji-keyword version stays `1` until an extra entry removes a seeded emoji, or an imported sticker adds an English keyword. Simplified Chinese and traditional Chinese have their own versions.

## Still missing

- The rest of Twemoji, Fluent, and Noto beyond the seeded characters.
- Masks and dice sets. User-created packs are `stickers.createStickerSet`: an empty `short_name` is assigned on the server, and each sticker must already be a `messages.uploadMedia` document (`image/webp`, `image/png`, `application/x-tgsticker`, or `video/webm`). Chat file upload is unchanged.
- Animated emoji that a client refuses because the Lottie is larger than 512×512.
