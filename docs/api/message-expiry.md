# 消息到期

云端聊天的自动删除按官方语义执行，不靠环境开关。

`messages.setHistoryTTL` 把秒数记在对话上（私聊两边的 `dialogs.ttl_period`，普通群在同一个事务里写 `chats.ttl_period` 和每个成员的对话行，频道/超级群写 `channels.ttl_period`），并写入 `messageActionSetMessagesTTL` 服务消息，再推 `updatePeerHistoryTTL`。`messages.setDefaultHistoryTTL` 只保存账号默认值；官方客户端在新建对话时自己再调用 `messages.setHistoryTTL`。`messages.getDialogs` 只在 `ttl_period > 0` 时带上这个字段；0 表示没开定时器，字段不出现。

之后发出的消息（发出方和每个收件人的盒子，以及频道消息）带上当时的 `ttl_period`。倒计时从消息的 `date` 开始，不是从已读开始。之后再改定时器，不会改已经写上的 `ttl_period`。定时器打开之前的消息保持 `ttl_period = 0`，不会被删。服务消息自己也带上新的周期，到点同样删除；设成 0 的那条服务消息不带周期，留着。

## 查看一次和限时媒体（ttl_seconds）

这条规则不看 `TELLOMI_MESSAGE_EXPIRY`。`ttl_seconds > 0` 的照片、视频、语音都会带 `media_unread`。`0x7FFFFFFF` 是查看一次，打开后立刻过期。其它正数从打开时刻起算。

收件人调用 `messages.readMessageContents` 之后：

- 发送方收到 `updateReadMessagesContents`，`date` 是服务器上的打开时间。官方 Android 把缺省的 `date` 当成 0，限时内容会立刻被算成已过期。
- 同一条更新写入收件人的 pts，其它设备用 `updates.getDifference` 对齐；在线的其它设备同时收到 `updatesTooLong` 之外的 `SyncUpdatesNotMe`。
- 查看一次马上改写双方的 `message_data`：保留 `ttl_seconds`，照片换成 `photoEmpty`，文件换成 `documentEmpty`。限时内容写到 `message_ttl_open`，到期后由 msg 进程里的循环改写。`messages.getHistory` 和新设备在 difference 过长之后拉到的历史都是这个过期形态。在线客户端自己根据 `updateReadMessagesContents.date` 把气泡涂成已过期，服务器不另推 `updateEditMessage`（对照 Android `MessagesStorage.markMessagesContentAsRead` 和 iOS `markMessageContentAsConsumedRemotely`）。
- `upload.getFile` 在改写之后返回 `FILE_ID_INVALID`。对象先进入 `media_purge_queue`（默认 24 小时后再删，而且只在没有其它引用时删）。`expired_at` 在入队时就写上，所以宽限期内也不能下载。
- `messages.forwardMessages` 对仍带 `ttl_seconds` 的消息返回 `CHAT_FORWARDS_RESTRICTED`。

未打开的不改。一次性命令默认只打印：

```sh
ttlexpire -f /app/etc/msg.yaml
ttlexpire -f /app/etc/msg.yaml -dry-run=false -backup /var/backups/ttl-media.json
```

没有 `-backup` 时不会改库。备份先写入并 fsync。历史照片和视频如果从来没有 `media_unread`，命令无法区分打开过和没打开，会计入 `skip_no_signal` 并跳过。已经打开的语音（列上的 `media_unread` 被清掉，JSON 里仍是 true）会过期。

迁移：`teamgramd/deploy/sql/migrate-20261123-message-ttl-open.sql`。只建新表。msg 进程启动时也会建这张表。

## 删除

msg 进程里的清扫每次启动先跑一遍，之后默认每 2 秒一轮（`TELLOMI_HISTORY_TTL_INTERVAL`，单位秒；`0` 关闭循环）。多副本用 Redis `SETNX teamgram_history_ttl`（120 秒过期）挡住同时删。拿不到锁就跳过这一轮。重启接着扫库里还没删的行，不靠内存账本。不使用 `FLUSHALL`。

一轮做两件事：

1. 补章。对话或群上已经有定时器、但消息行的 `ttl_period` 仍是 0 时，按该对话里 `messageActionSetMessagesTTL` 的时间，给「这条服务消息之后」的消息补上当时的周期。服务消息之前的消息不动。找不到服务消息就不补，避免把设定时器之前的历史整段删掉。补章只更新 `ttl_period = 0` 的行，重复跑没有新的写入。
2. 删除。`ttl_period > 0` 且 `date + ttl_period <= now` 的行用和 `messages.deleteMessages` 相同的 `deleted = 1`。媒体保留只把 `deleted = 0` 当成引用，所以这一步去掉的是同一条引用，不另写一套删对象存储的逻辑。每个用户拿到带 pts 的 `updateDeleteMessages`（写入 `user_pts_updates`，并推给在线会话）。频道走现有的删除提交，推 `updateDeleteChannelMessages`。对话的 `top_message` 和 `unread_count` 一起改。第二轮扫到的已删行不会再推。

`getHistory` / `getDifference` 只返回 `deleted = 0` 的行，并带上删除更新，所以重连、增量、客户端本地同步都不会把这些消息找回来。已经进备份的库和对象存储不在这次清扫里改写；备份恢复会把库行带回来，那是恢复备份，不是聊天同步。

云端历史的自动删除和查看一次都不读 `TELLOMI_MESSAGE_EXPIRY` / `TELLOMI_MESSAGE_EXPIRY_SWEEP`。这两个变量如果还开着，只作用于旧的内存账本，不删 `messages` 行，也不代替上面的清扫。

## 秘密聊天

秘密聊天的自毁秒数写在 `secret_chats.ttl_period`。重启后 `messages.sendEncrypted` 仍用这个秒数。已经发出的密文记在 `secret_message_expiry`，只在 `TELLOMI_MESSAGE_EXPIRY=1` 时拒绝 `upload.getFile` 和 `updates.getDifference` 里的那条密文。这个开关默认关。它不删普通私聊，也不代替查看一次。服务器不解密。

迁移 `migrate-20261124-secret-chat-ttl.sql` 在 `migrate-20261120-secret-qts.sql` 建好 `secret_chats` 之后才加列。更早的文件会在表还不存在时失败。

## 上线

部署新镜像后 msg 进程自己开始清扫，不需要另开开关，也不需要 `FLUSHALL`。

迁移 `migrate-20261123-history-ttl.sql` 给 `messages` 加到期索引，给 `channel_messages` 加 `ttl_period` 和索引。列或索引已经在时会跳过。大表上加索引用 `ALGORITHM=INPLACE, LOCK=NONE`。频道服务启动时也会补这列，所以迁移和服务启动的顺序哪边先完成都可以。

已经过期、而且行上已经有 `ttl_period` 的消息，第一轮清扫就会删掉。设过定时器但旧版本没有给消息打上 `ttl_period` 的，补章按服务消息的时间补上，补完之后到期的在同一轮或下一轮删除。没有服务消息的旧对话不会被整段清空。

回退镜像不会把已经标成 `deleted = 1` 的消息改回来。
