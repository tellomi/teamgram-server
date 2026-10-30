# proto (Tellomi layer 232 fork)

MTProto codec used by this Teamgram server. It is a vendored fork of
[teamgram/proto](https://github.com/teamgram/proto) **v0.231.0**
(`c145a4d`, “Update API scheme on layer 231”), extended to Telegram API
**layer 232**.

The server module replaces `github.com/teamgram/proto` with this directory
(`replace github.com/teamgram/proto => ./proto` in the root `go.mod`). Import
paths are unchanged.

## Why it is vendored

`teamgram/proto` v0.230.0 (the version this server previously required) and
v0.231.0 both stop their `GetClazzID(name, layer)` tables at that layer. A
client that sends `invokeWithLayer` **232** therefore got constructor **0**
back for every object the server encoded. Upstream has no v0.232.0 tag.
Publishing a separate module was not possible from this repository, so the
fork lives here.

`v2/` is the upstream v0.231.0 tree and is not imported by teamgram-server.
The layer 232 work is in `mtproto/`.

## Schema

`schema/telegram_api_layer232.tl` is Telegram Desktop `api.tl` from commit
`f985723b552b30fe1c17d377d57f44e61a15a8c4` (dev, 2026-10-26), which ends with
`// LAYER 232`. That is the schema spoken by the stock clients this server
targets:

- Telegram iOS `TelegramMessenger/Telegram-iOS` `fb0e62ba` (2026-10-22)
- Telegram Android `DrKLO/Telegram` `3de37c8` (2026-10-25)

Layer 232 matches layer 231 except these constructors:

| TL name | Layer 231 | Layer 232 | What was added |
|---|---|---|---|
| `userFull` | `6cbe645` | `b90c2afd` | `wallet_card:flags2.19?wallet.Card`, `share_theme:flags2.27?ShareTheme` |
| `channel` | `d49f34c6` | `7ad15240` | `in_news:flags2.21?true`, `pinned_msg_id:flags2.22?int` |
| `channelFull` | `a04e8d3a` | `c4c4dd40` | `share_theme:flags2.25?ShareTheme` (`has_welcome_messages` / `guard_bot_id` were already encoded) |
| `inputReplyToMessage` | `3bd4b7c2` | `75c1ad70` | `quote_rich:flags.8?Vector<RichText>` |
| `messageReplyHeader` | `1b97dd66` | `ddb5ab75` | `quote_rich:flags.14?Vector<RichText>` |
| `wallet.nftItem` | `3441fd1c` | `173dfaf3` | `gift:flags.9?StarGift` |
| `payments.getStarGiftWithdrawalUrl` | `d06e93a8` | `c8ca817b` | `flags:#` and `tmp_password:bytes` instead of `password:InputCheckPasswordSRP` |

Every other predicate keeps its layer 231 constructor id at layer 232, which
matches this schema. `GetClazzID` still returns the historical id for layers
≤ 231.

`wallet.Card`, `wallet.cardSticker`, `inputWalletCardSticker`, and
`wallet.setCardStickers` are registered in `wallet_card_codec.go` and
`schema.tl.walletcard.proto`. The server still does not set `wallet_card` or
`share_theme` on `userFull` / `channelFull`. A payload that sets those flags
is rejected instead of being parsed out of alignment. `quote_rich` and
`channel.pinned_msg_id` are consumed so the rest of the object stays aligned;
they are not stored. The card is returned by `wallet.setCardStickers` and is
not embedded in `userFull`.
`messages.setShareTheme` and `messages.getShareThemeBackdrops` are registered
in `layer232_extra_codec.go` and `schema.tl.layer232extra.proto`.
`contacts.exportNoPaidMessagesToken` and
`contacts.importNoPaidMessagesToken` are registered in `nopaid_codec.go` and
`schema.tl.nopaid.proto` (the result type is `exportedNoPaidMessagesToken`).

`feed.*` constructors are not in layer 232. iOS and macOS at the pinned
coverage commits already send the layer-233 ids (`feed.getFeed#7e76d7d3` and
the rest of that namespace). They are registered in `feed_codec.go` and
`schema.tl.feed.proto`. Android's `TL_iv.getRichMessage` is constructor
`0x501569cf`, the same bytes as `messages.getRichMessage`.

## Regenerating

There is no published `mtprotoc`. This fork patches the generated Go from
v0.231.0 (constructor tables, encode/decode switches for the seven types
above) rather than regenerating the whole schema. Do not re-run upstream
codegen over these files without re-applying `mtproto/layer232.go` and the
layer 232 cases.

```bash
cd proto && go test ./mtproto -count=1 -run 'TestLayer232'
```

## License

Apache License 2.0, inherited from teamgram/proto. See [LICENSE](LICENSE).
