# 发现：删号通知 content-engine

`account.deleteAccount` 把账号删掉之后，如果开了开关，会在后台调用引擎：

```
DELETE {TELLOMI_DISCOVER_ENGINE_URL}/internal/users/{sub}
Authorization: Bearer <TELLOMI_DISCOVER_SERVICE_KEY 的原文，即标准 base64>
```

`sub` 和凭证用同一把 `TELLOMI_DISCOVER_PSEUDONYM_KEY`、同一种十进制 user id。路径里是 16 字节的 base64url，无填充。引擎用 `base64.RawURLEncoding` 解码，再用标准 base64 解开头里的密钥，和 `ENGINE_SERVICE_KEY` 的原始字节比较。

引擎 `internal/engine/api/deletion.go` 的结果：成功是 `200 {"deleted_objects": n}`，再调一次仍是 200。没配服务密钥是 `503 service_disabled`（会重试）。密钥不对是 `401 service_credential_invalid`（不重试）。`sub` 长度不对是 `400 invalid_sub`（不重试）。

调用失败只打日志并重试，不让删号 RPC 失败，也不延长删号。重试最多 8 次（网络错误、超时、503 和其他 5xx、429）。401、400 和其他 4xx 不重试。404 不当成已删除。

第一次成功之后约 70 分钟再删一次。凭证最长 1 小时有效，这段时间里旧凭证还可能在引擎里把内容重新建出来。

队列在进程内存里。重启会丢掉还没完成的通知。派生密钥没配、地址不是 https（本机回环可以用 http）、或密钥短于 32 字节时，不发请求，删号照常完成。

| 名字 | 默认 |
| --- | --- |
| `TELLOMI_DISCOVER_DELETE` | 关。只有 `1` 才通知 |
| `TELLOMI_DISCOVER_ENGINE_URL` | 空。引擎根地址，不带路径 |
| `TELLOMI_DISCOVER_SERVICE_KEY` | 空。标准 base64，至少 32 字节 |
| `TELLOMI_DISCOVER_PSEUDONYM_KEY` | 和签发凭证是同一把 |

不跟随重定向。日志里没有 `sub`、密钥和响应正文。
