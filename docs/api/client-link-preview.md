# 聊天链接卡片

给 iOS 和安卓实现。服务端已经按这份契约处理。

官方气泡的样子不变：`messageMediaWebPage` 里的 `webPage`，客户端按 `type` 画标题、说明、缩略图和按钮。贴纸集链接里的 short name（`erelpack`）留在网址里，标题用贴纸集自己的名字（`erel`）。「查看贴纸」是客户端根据 `type=telegram_stickerset` 画的，不是服务端下发的按钮文案。

## 内部链接

`messages.getWebPage`、`messages.getWebPagePreview`，以及 `messages.sendMessage`（没有 `no_webpage`）会给下面这些链接直接生成卡片。数据来自本进程里的贴纸目录、频道和用户名，不访问外网。

主机：`tell.cc`、`www.tell.cc`、`t.me`、`telegram.me`、`telegram.dog`，以及 `TELLOMI_LINK_BASE_URL` 指到的主机。

| 路径 | `webPage.type` |
| --- | --- |
| `/addstickers/<short_name>`、`/addemoji/<short_name>` | `telegram_stickerset` |
| `/<username>` 是频道 | `telegram_channel` |
| `/<username>` 是超级群 | `telegram_megagroup` |
| `/<username>` 是用户 | `telegram_user` |
| `/<username>/<msg_id>`、`/s/<username>/<msg_id>`、`/c/<channel_id>/<msg_id>` | `telegram_message` |
| `/+<hash>`、`/joinchat/<hash>` | `telegram_channel` 或 `telegram_megagroup` |

贴纸集和表情集都是 `telegram_stickerset`。表情集的 `webPageAttributeStickerSet` 带 `emojis`。说明文字由服务端写成 `Free stickerpack with N stickers.` 或 `Free emoji pack with N emoji.`。属性里最多带 5 张贴纸，`document` 是第一张，给缩略图用。`messages.getWebPage` 的 `chats` / `users` 里会带上对应的频道或用户。

私密频道的消息不会放进卡片。找不到的 short name、用户名，返回 `webPageEmpty`，不会改去抓这个网址。

`messages.sendMessage` 只自动挂上 `telegram_*` 卡片。外站网址仍然只是文字，等发送方用下面的方式提交卡片。`no_webpage` 不挂卡片，编辑时也会拿掉已经挂上的内部卡片。

## 外站不要由服务器抓

聊天里的外站抓取默认关闭。`TELLOMI_SERVER_LINK_FETCH` 不是 `1` 时，`messages.getWebPage("https://www.apple.com")` 返回 `webPageEmpty`，不会发出 HTTP 请求。超时不再是这条路径的结果，因为根本没有请求。

这和已经合并的 PR 195 不同。PR 195 把聊天抓取做成了始终开启：`User-Agent: TellomiBot/1.0`，8 秒超时，拦截内网、回环和链路本地地址。失败是 `webPageEmpty`，可能重试的失败是 `webPagePending`。没有任何环境变量能关掉它。`TELLOMI_LINK_PREVIEW` 只控制发现页的 `POST /internal/v1/link-preview`（默认关闭，见 `docs/api/link-preview-card.md`），不控制 `messages.getWebPage`。

`messages.sendMessage` 以前也不会把抓到的卡片挂到消息上，所以即使抓取成功，只发文字的客户端气泡里仍然没有卡片。

8801 上的 `GET /linkpreview/v1/resolve` 是给 tell.cc 网页用的，不产生聊天里的 `webPage`。

发现页那个内部接口保持原样：默认关闭，只有 `TELLOMI_LINK_PREVIEW=1` 才由服务器抓。聊天不要打开 `TELLOMI_SERVER_LINK_FETCH`。

## 发送方本机抓取

显示仍用 Telegram 的 `webPage`。抓取由发送方用本机网络完成，和发现页本机抓取同一份 Open Graph 结果（标题、说明、站点名、图片）。图片不要把外链交给服务器，先走现有的 `messages.uploadMedia`，拿到 `Photo` 之后再提交。

提交用已有的 `inputMediaWebPage#c21b8849`，后面多四个 Tellomi 标志。官方 layer 232 只用到 flags.0–2。官方客户端不设这几位，编解码和现在一样。

```
inputMediaWebPage#c21b8849 flags:#
  force_large_media:flags.0?true
  force_small_media:flags.1?true
  optional:flags.2?true
  url:string
  title:flags.3?string
  description:flags.4?string
  site_name:flags.5?string
  photo:flags.6?InputPhoto
  = InputMedia;
```

`site_name` 在这个构造器里放在 `InputMedia` 的 `provider` 字符串上。`photo` 放在 `InputPhoto`（`id`、`access_hash`、`file_reference`）。只设了 `url`、没有 flags.3–6 时，服务端按内部链接或关闭的外站抓取处理，不会把空卡片当成客户端提交。

然后 `messages.sendMedia`。服务端生成 `messageMediaWebPage`，随消息发给对方，对方不需要再抓。

限制：

| 字段 | 上限 |
| --- | --- |
| `url` | 2048 字节，必须是 http 或 https，内网和回环地址拒绝 |
| `title` | 存 128 个字符；线上超过 16KB 拒绝 |
| `description` | 存 400 个字符；线上超过 16KB 拒绝 |
| `site_name` | 存 64 个字符 |
| 图片 | 必须是已经上传的 `InputPhoto`。媒体库里没有这张图时，卡片仍然保存，只是没有图 |

`optional` 仍然表示网址不合法时只发文字、不报错。

未经验证：服务端不打开这个网站，也不核对 Open Graph 是否属实。这样生成的 `webPage.author` 固定为 `unverified`。客户端不要把这个作者画出来。`type` 仍是 `article`，有图时是 `photo`，气泡布局和官方外站卡片一样。

如果以后官方 layer 给 `inputMediaWebPage#c21b8849` 的 flags.3–6 派了别的含义，这份扩展要换掉。改动在 `proto/mtproto/codec_schema.tl.pb.go` 的 `TLInputMediaWebPage`，重新生成这份文件时要把这段补回去。
