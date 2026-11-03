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

## 错误

| HTTP | `error` | 引擎怎么做 |
| --- | --- | --- |
| 401 | `unauthorized` | 密钥不对。受限帖子不要展示 |
| 400 | `bad_request` | `sub` 不合法或名单为空、超过 100。不要展示 |
| 503 | `unavailable` | 数据库暂时不可用。不要展示 |
| 404 | `not_found` | 路径不对 |

失败不能当成「所有人可见」。

## 和引擎现有模型的差别

引擎现在的好友要双方各交一份 Signal 签名。这里换成 teamgram 按聊天联系人和快拍密友实时回答，引擎不要再为这两档存边。

「选定的用户」仍是发帖时的 include 名单（快照，不是这个接口）。引擎现有的 `SignAudience` JWT 还要 teamgram 签，这一档本接口不覆盖，引擎侧接到 include 名单之前先不要用未签名的 user id。

一个内容池（D-44）和机审拿不准只给作者看（D-46）都在引擎里改，不在这个接口里。
