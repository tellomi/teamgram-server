# Reaction animation provenance

Telegram's reaction picker needs a real animated emoji for every slot on
`availableReaction`: `appear_animation`, `select_animation`,
`activate_animation`, `effect_animation`, `around_animation`, and
`center_icon` are gzipped Lottie (`.tgs`, 512×512,
`application/x-tgsticker`). `static_icon` is a 72×72 WebP used as the chip.

These bytes are **not** copied from the Telegram client. Telegram's own
reaction pack is not published under a license we can ship. The motion is
[Noto Emoji Animation](https://github.com/googlefonts/noto-emoji) by Google,
served as Lottie from `https://fonts.gstatic.com/s/e/notoemoji/latest/<code>/lottie.json`
(OFL-1.1 and Apache-2.0). Files whose canvas is larger than 512×512 are wrapped
in a 512×512 precomp scaled to fit, which is the size Telegram clients accept
for `application/x-tgsticker`. The still chip is
[Twemoji](https://github.com/jdecked/twemoji) 72×72 PNG converted to WebP
(CC-BY-4.0).

Each slot is a distinct document id. The six animated slots of one emoji share
the same Noto file, because that is the legally available animation for the
emoji. `upload.getFile` reads the bytes from this binary. On startup the same
objects are also written to Cloudflare R2 (or MinIO) under
`emoji/<document id>.dat` in `TELLOMI_S3_BUCKET_PUBLIC_MEDIA`, using the dfs
S3 client. A missing object does not stall the session.

Codes added for the default double-tap set that were previously an empty
fallback Lottie: `1f970` 🥰, `1f44f` 👏, `1f601` 😁, `1f914` 🤔, `1f631` 😱.
