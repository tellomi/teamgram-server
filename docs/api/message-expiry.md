# 消息到期

`messages.setHistoryTTL` 仍只记下周期并写一条服务消息。到期之后的行为由两个开关控制，默认都关。关着的时候，已有消息的读取和删除与现在相同。没有迁移，没有启动时清扫，也不会改已经写进库里的 `ttl_period`。

| 变量 | 默认 | 作用 |
| --- | --- | --- |
| `TELLOMI_MESSAGE_EXPIRY` | 空 | `1` 时，`ttl_period > 0` 且 `date + ttl <= now` 的消息不再从历史、按 id 读取、频道增量里返回正文。`ttl_period = 0` 的行永远留着。 |
| `TELLOMI_MESSAGE_EXPIRY_SWEEP` | 空 | `1` 时，频道成员读历史会把已经到期的频道消息标成删除，并向成员推 `updateDeleteChannelMessages`。第二次调用不再删除。私聊行只是读路径隐藏，这条开关不会去删私聊表。 |

频道到期看频道上的 `ttl_period`。私聊看消息行上的 `ttl_period`。秘密聊天看 `SetTTL` 记下的秒数；到期后 `upload.getFile` 和 `updates.getDifference` 不再交出密文。服务器不解密。

## 查看一次和限时媒体（ttl_seconds）

这条规则不看 `TELLOMI_MESSAGE_EXPIRY`。`ttl_seconds > 0` 的照片、视频、语音都会带 `media_unread`。`0x7FFFFFFF` 是查看一次，打开后立刻过期。其它正数从打开时刻起算。

收件人调用 `messages.readMessageContents` 之后：

- 发送方收到 `updateReadMessagesContents`，`date` 是服务器上的打开时间。官方 Android 把缺省的 `date` 当成 0，限时内容会立刻被算成已过期。
- 同一条更新写入收件人的 pts，其它设备用 `updates.getDifference` 对齐；在线的其它设备同时收到 `updatesTooLong` 之外的 `SyncUpdatesNotMe`。
- 查看一次马上改写双方的 `message_data`：保留 `ttl_seconds`，照片换成 `photoEmpty`，文件换成 `documentEmpty`。限时内容写到 `message_ttl_open`，到期后由 msg 进程里的循环改写。`messages.getHistory` 和新设备在 difference 过长之后拉到的历史都是这个过期形态。在线客户端自己根据 `updateReadMessagesContents.date` 把气泡涂成已过期，服务器不另推 `updateEditMessage`（对照 Android `MessagesStorage.markMessagesContentAsRead` 和 iOS `markMessageContentAsConsumedRemotely`）。
- `upload.getFile` 在改写之后返回 `FILE_ID_INVALID`。对象先进入 `media_purge_queue`（PR 284，默认 24 小时后再删，而且只在没有其它引用时删）。`expired_at` 在入队时就写上，所以宽限期内也不能下载。
- `messages.forwardMessages` 对仍带 `ttl_seconds` 的消息返回 `CHAT_FORWARDS_RESTRICTED`。

未打开的不改。一次性命令默认只打印：

```sh
ttlexpire -f /app/etc/msg.yaml
ttlexpire -f /app/etc/msg.yaml -dry-run=false -backup /var/backups/ttl-media.json
```

没有 `-backup` 时不会改库。备份先写入并 fsync。历史照片和视频如果从来没有 `media_unread`，命令无法区分打开过和没打开，会计入 `skip_no_signal` 并跳过。已经打开的语音（列上的 `media_unread` 被清掉，JSON 里仍是 true）会过期。

迁移：`teamgramd/deploy/sql/migrate-20261123-message-ttl-open.sql`。只建新表。msg 进程启动时也会建这张表。

这四类在 `pkg/msgexpiry` 的账本里分开。清扫一类不会删另一类。第二次清扫同一条记录不再删除。对方和自己的用户 id 都会列在失效名单里，重连后再读仍是不可读。

`TELLOMI_MESSAGE_EXPIRY_SWEEP=1` 在进程启动时才会跑账本里的定时清扫。它只遍历本进程内存里的记录，不扫描 `messages` 表，不改备份，不碰对象存储里的历史文件。重启后这份内存账本是空的，所以一次重启不会接着去删重启前的生产行。频道存储上的标删仍是 239 的行为：只有清扫开关为 `1` 而且有人读该频道时，才会标删已经到期的频道消息。生产不要设这个开关。

法律和备份：读路径隐藏和下载拒绝都不是销毁备份。已经进备份的库和对象不会被这次清扫改写。查看一次是「打开后本进程不再下发」，和「时间到了就隐藏」不是同一条规则。秘密聊天清掉的是服务器上的密文副本，不是两端设备里的明文。

## 上线

先不要在生产打开。打开 `TELLOMI_MESSAGE_EXPIRY` 之后，已经设过 TTL 的频道里、发送时间加上周期已经过去的消息，会从各端的历史和增量里消失，但行还在。打开 `TELLOMI_MESSAGE_EXPIRY_SWEEP` 之后，下一次有人读这个频道才会真正标删，并给成员推删除。没有人读的频道不会被后台扫到。私聊没有对应的删除。

这两个变量没有写进 `docker-compose.yaml`。部署新镜像不会自己打开。要开的话在 teamgram 的环境里单独加，然后重建 teamgram 进程。不需要 SQL。回退就是去掉变量再重建，读路径恢复；已经被标删的频道消息不会自动回来。
