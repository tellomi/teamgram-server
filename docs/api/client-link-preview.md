# 聊天链接卡片

给 iOS 和安卓实现。服务端已经按这份契约处理。

官方气泡的样子不变：`messageMediaWebPage` 里的 `webPage`，客户端按 `type` 画标题、说明、缩略图和按钮。贴纸集链接里的 short name（`erelpack`）留在网址里，标题用贴纸集自己的名字（`erel`）。「查看贴纸」是客户端根据 `type=telegram_stickerset` 画的，不是服务端下发的按钮文案。

## 内部链接

`messages.getWebPage`、`messages.getWebPagePreview`，以及 `messages.sendMessage`（没有 `no_webpage`）会给下面这些链接直接生成卡片。数据来自本进程里的贴纸目录、频道和用户名，不访问外网。

主机：`tell.cc`、`www.tell.cc`、`tellomi.cn`、`www.tellomi.cn`，以及 `TELLOMI_LINK_BASE_URL` 指到的 Tellomi 主机。`t.me`、`telegram.me`、`telegram.dog` 是 Telegram 的，不当成内部卡片；抓取关闭时没有卡片，打开 `TELLOMI_SERVER_LINK_FETCH` 后按外站处理。`TELLOMI_LINK_BASE_URL` 如果仍写成这几个 Telegram 主机，也不会生成 Tellomi 卡片。

| 路径 | `webPage.type` |
| --- | --- |
| `/addstickers/<short_name>`、`/addemoji/<short_name>` | `telegram_stickerset` |
| `/<username>` 是频道 | `telegram_channel`；需要管理员批准加入时是 `telegram_channel_request` |
| `/<username>` 是超级群 | `telegram_megagroup`；需要管理员批准加入时是 `telegram_megagroup_request` |
| `/<username>` 是用户 | `telegram_user` |
| `/<username>` 是机器人 | `telegram_bot` |
| `/<username>/<msg_id>`、`/s/<username>/<msg_id>`、`/c/<channel_id>/<msg_id>` | `telegram_message` |
| `/+<hash>`、`/joinchat/<hash>` 是频道或超级群 | `telegram_channel`、`telegram_megagroup`，需要批准时带 `_request` |
| `/+<hash>`、`/joinchat/<hash>` 是普通群 | `telegram_chat` |

`site_name` 是 `Tellomi`。能看到头像时，卡片的 `photo` 是已经入库的 `Photo`（带签发的 `access_hash`），对方用 `upload.getFile` 下载。看不到头像（隐私）时不放 `photo`。

用户没有简介时，说明是 `You can contact @用户名 right away.`。频道和超级群有简介时用简介，否则用订阅人数或成员数。人数按调用者的语言包复数来写（iOS 的 `Conversation.StatusSubscribers` / `Conversation.StatusMembers`，安卓的 `Subscribers` / `Members`）。没有语言时用英文。

贴纸集和表情集都是 `telegram_stickerset`。表情集的 `webPageAttributeStickerSet` 带 `emojis`。说明文字由服务端写成 `Free stickerpack with N stickers.` 或 `Free emoji pack with N emoji.`。属性里最多带 5 张贴纸，`document` 是第一张，给缩略图用。`messages.getWebPage` 的 `chats` / `users` 里会带上对应的频道或用户。

私密频道的消息不会放进卡片。找不到的 short name、用户名，返回 `webPageEmpty`，不会改去抓这个网址。

`messages.sendMessage` 只自动挂上 `telegram_*` 卡片。外站网址在抓取关闭时仍然只是文字，等发送方用下面的方式提交卡片。`no_webpage` 不给纯文字再挂卡片；编辑时如果没有换上新的 `inputMediaWebPage`，也会拿掉已经挂上的内部卡片。`messages.sendMedia` 里的 `inputMediaWebPage` 本身就是卡片，会留下来。

## 外站抓取默认关闭

聊天里的外站抓取默认关闭。`TELLOMI_SERVER_LINK_FETCH` 不是 `1` 时，`messages.getWebPage("https://www.apple.com")` 返回 `webPageEmpty`，不会发出 HTTP 请求。部署之后再打开这个开关。

打开之后，同一条规范化 URL（sha256）在 Redis 里共享：成功缓存约 24 小时，失败缓存 20 分钟。同一个 URL 同时只有一次抓取。每个域名大约每秒 1 次、突发 5 次；全局同时大约 8 个连接。超过限额直接返回 `webPageEmpty`，不发请求，也不写入失败缓存。`messages.getWebPagePreview` 另外走现有的按用户洪水桶（`web_preview`）。

连接超时 3 秒，整次请求 5 秒。HTML 最多 1MB，图片最多 2MB，最多 3 次跳转，只允许 80 和 443 端口。内网、回环、链路本地地址在拨号前拒绝，DNS 结果在拨号时再查一次，跳转同样检查。不读取 `HTTP_PROXY`。`TELLOMI_LINK_FETCH_PROXY` 只给这个抓取器用；目标先通过上面的检查，才交给代理。

`User-Agent` 是 `Mozilla/5.0 (compatible; TellomiBot/1.0; +https://tellomi.cn/bot)`。卡片图片按内容哈希写入对象存储（ADR-0082）：相同字节只留一份对象，`access_hash` 等于照片 id，对方可以下载。

缓存 Redis 用 `TELLOMI_LINK_CACHE_REDIS`，空则用 `TELLOMI_FLOOD_REDIS`。都没有时，缓存和限额只在本进程里。图片去重用 `TEAMGRAM_MYSQL_DSN` 里的 `media_blobs`；没配数据库时仍然把对象写到照片桶。

失败是 `webPageEmpty`，可能重试的失败是 `webPagePending`。`TELLOMI_LINK_PREVIEW` 只控制发现页的 `POST /internal/v1/link-preview`（默认关闭，见 `docs/api/link-preview-card.md`），不控制 `messages.getWebPage`。

`messages.sendMessage` 只自动挂上内部 `telegram_*` 卡片。外站卡片在抓取打开后出现在 `messages.getWebPage` / `messages.getWebPagePreview`；发到对话里仍由发送方用 `messages.sendMedia` 和 `inputMediaWebPage` 提交。官方 iOS 在已经有预览时就是这样发的。服务端保留这条 `messageMediaWebPage`，不会因为 `sendMedia` 的出站 `no_webpage` 把它拿掉。`editMessage` 带上 `inputMediaWebPage` 时同样保留。

8801 上的 `GET /linkpreview/v1/resolve` 是给 tell.cc 网页用的，不产生聊天里的 `webPage`。

发现页那个内部接口保持原样：默认关闭，只有 `TELLOMI_LINK_PREVIEW=1` 才由服务器抓。

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
