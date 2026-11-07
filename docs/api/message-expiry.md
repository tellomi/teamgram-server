# 消息到期

`messages.setHistoryTTL` 仍只记下周期并写一条服务消息。到期之后的行为由两个开关控制，默认都关。关着的时候，已有消息的读取和删除与现在相同。没有迁移，没有启动时清扫，也不会改已经写进库里的 `ttl_period`。

| 变量 | 默认 | 作用 |
| --- | --- | --- |
| `TELLOMI_MESSAGE_EXPIRY` | 空 | `1` 时，`ttl_period > 0` 且 `date + ttl <= now` 的消息不再从历史、按 id 读取、频道增量里返回正文。`ttl_period = 0` 的行永远留着。 |
| `TELLOMI_MESSAGE_EXPIRY_SWEEP` | 空 | `1` 时，频道成员读历史会把已经到期的频道消息标成删除，并向成员推 `updateDeleteChannelMessages`。第二次调用不再删除。私聊行只是读路径隐藏，这条开关不会去删私聊表。 |

频道到期看频道上的 `ttl_period`，不是某一条消息单独的值。私聊看消息行上的 `ttl_period`，没有列的时候看消息体内的 `ttl_period`。

## 上线

先不要在生产打开。打开 `TELLOMI_MESSAGE_EXPIRY` 之后，已经设过 TTL 的频道里、发送时间加上周期已经过去的消息，会从各端的历史和增量里消失，但行还在。打开 `TELLOMI_MESSAGE_EXPIRY_SWEEP` 之后，下一次有人读这个频道才会真正标删，并给成员推删除。没有人读的频道不会被后台扫到。私聊没有对应的删除。

这两个变量没有写进 `docker-compose.yaml`。部署新镜像不会自己打开。要开的话在 teamgram 的环境里单独加，然后重建 teamgram 进程。不需要 SQL。回退就是去掉变量再重建，读路径恢复；已经被标删的频道消息不会自动回来。
