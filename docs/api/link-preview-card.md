# 发现帖子的链接卡片

内容引擎在 `ENGINE_LINK_PREVIEW=1` 时，给发现帖子做链接卡片会调用：

`POST /internal/v1/link-preview`

默认关闭。`TELLOMI_LINK_PREVIEW` 不是 `1` 时不监听。这不是公网的 tell.cc 解析（那个在 8801，见 `docs/link-preview.md`）。

默认 `127.0.0.1:8817`。不要映射到公网。

鉴权和 `POST /internal/v1/ai/complete` 相同。HTTP 时限也相同：读头 5 秒，读请求 15 秒，写响应 30 秒，空闲 60 秒。页面抓取单独限 8 秒，和聊天里的链接预览一样。

```
Authorization: Bearer <TELLOMI_LINK_PREVIEW_KEY 的原文，标准 base64>
Content-Type: application/json
```

```json
{ "url": "https://example.com/post" }
```

`url` 只能是 http 或 https。内网、回环、链路本地地址在解析前就拒绝，DNS 和跳转之后再查一次。

`200`

```json
{
  "url": "https://example.com/post",
  "title": "Hello",
  "description": "From the page",
  "image": "https://cdn.example/pic.jpg",
  "site_name": "Example"
}
```

`title` 和 `site_name` 缺了标签时用主机名。`description` 和 `image` 没有就不出现。`image` 是图片地址，不是图片内容。图片地址如果指向内网，这一项会丢掉，标题还在。

| HTTP | `error` | 含义 |
| --- | --- | --- |
| 401 | `unauthorized` | 密钥不对 |
| 400 | `bad_request` | 字段不合法，或地址不能抓 |
| 504 | `timeout` | 8 秒内没有抓完。可以重试 |
| 502 | `fetch` | 页面没有抓到 |

## 环境变量

| 名字 | 默认 | 作用 |
| --- | --- | --- |
| `TELLOMI_LINK_PREVIEW` | 关 | `1` 才监听 |
| `TELLOMI_LINK_PREVIEW_KEY` | 空 | 标准 base64，解码后至少 32 字节。缺了不监听 |
| `TELLOMI_LINK_PREVIEW_ADDR` | `127.0.0.1:8817` | `off` 不监听 |
