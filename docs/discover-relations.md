# 发现：联系人与密友（给 content-engine）

引擎在**读帖子**时问 teamgram：这个查看者现在是不是作者的联系人、是不是作者的密友。不在发帖时存快照。旧帖子也按最新名单（D-13、D-16、D-43）。

关注关系仍在引擎里，不走这个接口。

用户在引擎里只有 `sub`（凭证里的 16 字节假名，base64url、无填充）。请求和响应都不出现 teamgram 的 user id。

## 开关

| 名字 | 默认 | 作用 |
| --- | --- | --- |
| `TELLOMI_DISCOVER_RELATIONS` | 关 | `1` 才记录 `sub` 并提供查询 |
| `TELLOMI_DISCOVER_RELATION_KEY` | 空 | 标准 base64，解码后至少 32 字节。引擎放在 `Authorization: Bearer` 里，内容就是这串 base64 |
| `TELLOMI_DISCOVER_RELATION_ADDR` | `127.0.0.1:8812` | 监听地址。`off` 只记录 `sub`，不监听 |

还要开 `TELLOMI_DISCOVER=1` 并配好派生密钥。用户成功调用 `discover.getEngineCredential` 之后，teamgram 才记下 `sub → user_id`。没拿过凭证的 `sub` 查出来是 `known: false`，按「不是联系人、不是密友」处理。凭证 1 小时过期，客户端续期时会再次记下。

表是 `discover_subject`（`teamgramd/deploy/sql/migrate-20261104-discover-subject.sql`）。联系人和密友读现有的 `user_contacts`：`is_deleted = 0` 才算联系人，`close_friend = 1` 才算密友（`contacts.editCloseFriends` 写的那一列）。删掉联系人后下一次查询立刻看不到（D-43）。

判断方向是**作者的名单**：查看者在作者的联系人里，才看得到「我的联系人」帖子。查看者自己的通讯录不算。

默认只绑在 `127.0.0.1`，compose 不把这个端口映射到宿主机。引擎如果在别的容器里，把地址改成容器网卡上的内网地址，不要公布到公网。

## 请求

`POST /internal/v1/relations/check`

```
Authorization: Bearer <TELLOMI_DISCOVER_RELATION_KEY 的原文，即标准 base64>
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
    {"author": "<规范的 base64url>", "known": true, "contact": true, "close_friend": false}
  ]
}
```

`results` 和请求顺序一致。`author` 是同一串字节的规范 base64url。

| 字段 | 含义 |
| --- | --- |
| `viewer_known` | teamgram 见过这个查看者的凭证 |
| `known` | 见过这个作者的凭证 |
| `contact` | 作者的联系人里现在有这个查看者，且未删除 |
| `close_friend` | 同一行上 `close_friend` 为真。为真时 `contact` 也是真 |

查看者就是作者本人时，`contact` 和 `close_friend` 都是 false。作者看自己的帖子由引擎放行，不要靠这个接口。

没见过的人、已删除的联系人：`contact` 和 `close_friend` 都是 false。不要把查询结果写进帖子。同一次读可以在内存里复用，下一次读再问。

## `can_view` 怎么调用

引擎每次读帖子都在 `can_view` 里判断。联系人和密友两档走这个接口，不读 `friend_edge`，也不在发帖时把名单写进帖子。

一次只判断一篇帖时，`authors` 放一个作者：

```json
{"viewer":"<查看者 sub>","authors":["<作者 sub>"]}
```

一页很多作者时，可以放进同一次 `authors`（最多 100）。只在这一次请求里复用结果。下一次 `can_view` 再问。

| 帖子可见范围 | `can_view` |
| --- | --- |
| 所有人 | 不调用 |
| 我的联系人 | `contact == true` 才可见 |
| 密友 | `close_friend == true` 才可见 |
| 选定的用户 | 不调用。仍用发帖时的 `audience_token` 写入的 `audience_list_member` |
| 作者本人 | 引擎直接放行，不必调用 |

超时、401、400、503：这一档不可见。不要改成所有人可见。

`friend_edge`（`tellomi-feed-friend`）是双方各报一次的旧边。联系人 / 密友不要再靠它。引擎把这两档改成调用上面的接口之后，旧的 friend 边可以留着，但 `can_view` 不要用它决定这两档。

## 错误

| HTTP | `error` | 引擎怎么做 |
| --- | --- | --- |
| 401 | `unauthorized` | 密钥不对。受限帖子不要展示 |
| 400 | `bad_request` | `sub` 不合法或名单为空、超过 100。不要展示 |
| 503 | `unavailable` | 数据库暂时不可用。不要展示 |
| 404 | `not_found` | 路径不对 |

失败不能当成「所有人可见」。

## 以后由 teamgram 签 audience / friend 断言

这个接口不签发断言。引擎 `internal/engine/auth/assertion.go` 现在认的两种 JWT 可以以后改由 teamgram 签，校验不用改。签名密钥就是凭证那把 `TELLOMI_DISCOVER_SIGNING_KEY`（引擎 `ENGINE_FEED_AUTH_KEYS` 的第一把），头同样是 `{"alg":"HS256","typ":"JWT"}`。`sub` 的编码和凭证一样：16 字节的 base64url，无填充。

好友断言 `tellomi-feed-friend`，字段顺序 `typ`、`exp`、`a`、`b`、`op`（`op` 是 `add` 或 `remove`）。这是快照。联系人 / 密友以本接口的实时查询为准，不要用这条边代替 `can_view`。

受众断言 `tellomi-feed-audience`，字段顺序 `typ`、`exp`、`owner`、`kind`、`members`。`kind` 用 `include`。发帖时交给引擎，写入 `audience_list_member`。选定的用户仍是这份发帖时的名单，不是本接口。本 PR 还不提供签发方法；加上时不需要新的密钥，也不改这个查询。

一个内容池（D-44）和机审拿不准只给作者看（D-46）都在引擎里改，不在这个接口里。
