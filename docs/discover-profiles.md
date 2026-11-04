# 发现：按查看者过滤的资料（给 content-engine）

引擎不再保存昵称和头像。卡片上的名字、用户名、简介和头像，每次展示时向 teamgram 要一份**已经按作者当前隐私设置过滤过的**结果（D-17）。联系人和密友用的是作者现在的名单，不在发帖时做快照（D-43）。

请求和响应都不出现 teamgram 的 user id，也不出现手机号。昵称用账号上的 `first_name` / `last_name`，不用联系人备注。

## 开关

| 名字 | 默认 | 作用 |
| --- | --- | --- |
| `TELLOMI_DISCOVER_PROFILES` | 关 | `1` 才监听 |
| `TELLOMI_DISCOVER_PROFILE_KEY` | 空 | 标准 base64，解码后至少 32 字节。和关系接口、推送接口不是同一把 |
| `TELLOMI_DISCOVER_PROFILE_ADDR` | `127.0.0.1:8816` | `off` 不监听 |

`sub` 存在 `discover_subject` 里。这张表只在 `TELLOMI_DISCOVER_RELATIONS=1` 且用户成功调用过 `discover.getEngineCredential` 或 `discover.getAudienceToken` 之后才有行。关系开关关着时，这里查出来全是 `known: false`。不改登录。

默认只绑在 `127.0.0.1`。不要把这个端口映射到公网。没有新表。

## 请求

`POST /internal/v1/profiles/resolve`

```
Authorization: Bearer <TELLOMI_DISCOVER_PROFILE_KEY 的原文，即标准 base64>
Content-Type: application/json
```

```json
{
  "viewer": "<查看者 sub>",
  "authors": ["<作者 sub>", "..."]
}
```

`authors` 1 到 100 个。`sub` 必须是 16 字节的 base64url，不带 `=`。

## 响应

`200`

```json
{
  "viewer_known": true,
  "results": [
    {
      "author": "<规范的 base64url>",
      "known": true,
      "deleted": false,
      "first_name": "林",
      "last_name": "",
      "username": "lin",
      "about": "",
      "about_visible": false,
      "photo": false,
      "photo_id": ""
    }
  ]
}
```

`results` 和请求顺序一致。

| 字段 | 含义 |
| --- | --- |
| `viewer_known` | teamgram 见过这个查看者的凭证 |
| `known` | 见过这个作者的凭证 |
| `deleted` | 账号已删除。名字、用户名、简介、头像都是空的 |
| `first_name` / `last_name` / `username` | 账号资料。未删除就返回，没有单独的隐私键 |
| `about` / `about_visible` | 简介。`about_visible` 为 false 时 `about` 是空的，表示不给这位查看者看。`about_visible` 为 true 且 `about` 为空，表示对方允许看，只是没写简介 |
| `photo` / `photo_id` | 头像。`photo` 为 false 时 `photo_id` 是空的。`photo_id` 是十进制字符串，避免超过 2^53 时丢精度。文件字节仍在 teamgram，本接口不返回图片 |

没见过的作者：`known` 为 false，其余为空。查看者没见过：按陌生人处理，只能看到「所有人」可见的头像和简介。

作者看自己：头像和简介都给，不受隐私规则限制。

## 头像和简介怎么判

读 `user_privacies` 里 `key_type` 6（头像，`privacyKeyProfilePhoto`）和 10（简介，`privacyKeyAbout`）。没有这一行时，和聊天里一样，默认所有人可见。

| 作者的设置 | 谁能看到 |
| --- | --- |
| 所有人 | 任何人，包括没见过的查看者 |
| 我的联系人 | 只看作者通讯录里 `is_deleted = 0` 的人。查看者自己的通讯录不算 |
| 密友 | 只看同一行上 `close_friend = 1` 的人。密友也算联系人 |
| 不允许任何人 | 只有作者自己 |
| 额外允许 / 禁止某些用户 | 点名的人优先。先看禁止名单，再看允许名单 |
| 所有人，但联系人除外 | 联系人看不到，其他人看得到 |

群成员、Premium、机器人这几条规则这里不生效：既不因此放行，也不因此拦住。聊天里「允许这些群的成员看头像」不会反映到发现卡片上。

删掉联系人之后，下一次查询立刻看不到仅联系人的头像和简介。不要把结果写进帖子。

## 错误

| HTTP | `error` | 引擎怎么做 |
| --- | --- | --- |
| 401 | `unauthorized` | 密钥不对。这张卡片不要显示头像和简介，名字也先空着 |
| 400 | `bad_request` | `sub` 不合法或名单为空、超过 100 |
| 503 | `unavailable` | 数据库暂时不可用。不要改成「所有人可见」 |
| 404 | `not_found` | 路径不对 |
| 405 | `method` | 不是 POST |

失败不能当成头像公开。
