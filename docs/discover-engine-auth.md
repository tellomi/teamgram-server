# 发现：content-engine 凭证

登录 teamgram 的客户端调用下面的 TL 方法，拿到 content-engine 认的 HS256 JWT。引擎的校验代码不改。方法默认关闭。

## TL

```
discover.engineCredential#0578ccaa token:string expires:int region:string = discover.EngineCredential;
discover.getEngineCredential#a80a7f76 = discover.EngineCredential;
```

| | |
| --- | --- |
| 方法 | `discover.getEngineCredential` |
| constructor id | `0xa80a7f76`（十进制 `-1475707018`） |
| 参数 | 无 |
| 返回 | `discover.engineCredential` |
| 返回 constructor id | `0x0578ccaa`（十进制 `91802794`） |
| `token` | JWT 字符串 |
| `expires` | `exp`，Unix 秒，TL `int` |
| `region` | `cn` 或 `global`，和 JWT 里的 `rgn` 相同 |

constructor id 是去掉 `#id` 之后那一行的 CRC32（和 Telegram 一样，`Vector<T>` 写成 `Vector T`）。会话路由是 `/mtproto.RPCDiscover` → `bff.bff`。没有用户 id 时返回 `AUTH_KEY_UNREGISTERED`。开关关闭或密钥不可用时返回 `DISCOVER_DISABLED`。已删除账号返回 `USER_ID_INVALID`。

客户端在 `expires` 之前重新取。有效期 1 小时，引擎另给 60 秒时钟误差。

## JWT

头固定为 `{"alg":"HS256","typ":"JWT"}`，引擎按原文比对。claims 只有三个字段，顺序是 `sub`、`rgn`、`exp`，紧凑 JSON，没有多余空格。

`sub` 是 16 字节的 base64url（无填充）。这 16 字节是 `HMAC-SHA256(TELLOMI_DISCOVER_PSEUDONYM_KEY, 十进制 user_id 的 UTF-8)` 的前 16 字节。`42` 就是两个字符 `42`，没有符号，没有前导零。引擎不保存 user_id，也不持有这把派生密钥，所以不能从 `sub` 反查账号。

`rgn` 取严，先命中者为 `cn`，否则 `global`：

1. 账号 `country_code` 是 `CN`，或手机号国家码是 `+86`。
2. 否则客户端 IP 被现有 GeoIP 判成中国大陆（香港、澳门、台湾不是大陆）。
3. 否则 `global`。

只有一个 MTProto 入口，所以没有「从 tellomi.cn 进来就算大陆」这一条。邮箱账号没有手机号，只看 IP。

签名密钥是 `TELLOMI_DISCOVER_SIGNING_KEY` 做标准 base64 解码之后的字节，必须和引擎 `ENGINE_FEED_AUTH_KEYS` 的第一把（最新那把）是同一串字节，至少 32 字节。派生密钥另一把，也至少 32 字节，并且不能和签名密钥相同。两把都只从环境变量读，不进仓库。派生密钥一旦上线不能换，换了每个人在引擎里的身份都会变。

## 环境变量

| 名字 | 默认 | 作用 |
| --- | --- | --- |
| `TELLOMI_DISCOVER` | 关 | 设为 `1` 才签发。其它值都是关。 |
| `TELLOMI_DISCOVER_SIGNING_KEY` | 空 | HS256 密钥，标准 base64。 |
| `TELLOMI_DISCOVER_PSEUDONYM_KEY` | 空 | 派生 `sub` 的 HMAC 密钥，标准 base64。 |

开关开着但密钥缺失、太短或两把相同，方法仍返回 `DISCOVER_DISABLED`，进程不退出。日志不写密钥和 token。

## 和 D-44 的关系

`rgn` 仍然是 `cn` 或 `global`，因为引擎现在的 JWT 和内容池就是这两个值。一个内容池、同一套审核（D-1、D-44）要改 content-engine 的 `internal/engine/moderation/worker.go` `regions()`，本仓库不改引擎。teamgram 不另造第三个 `rgn`。
