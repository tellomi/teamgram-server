# 官方用户名占位（M5 P0 / D-27 / D-55 / D-60）

`account.checkUsername` 查的是 `username` 表，不是 `users.username`。旧的 `z_init.sql` 只把 `tellomi` 写在用户 777000 上，所以这个名字还能被注册。占位行在 Tellomi 的迁移里，不在第二份建表脚本里。

| 文件 | 作用 |
|---|---|
| `teamgramd/deploy/sql/migrate-20261104-tellomi-username.sql` | D-27。没有行时把 `@tellomi` 挂到用户 777000 |
| `teamgramd/deploy/sql/migrate-20261105-official-accounts.sql` | 附录 D。把 `@tellomi` 挪到 777001，并占住其余已分配的名字 |
| `teamgramd/deploy/sql/migrate-20261107-tellomi-ai-name.sql` | 777001 的显示名改为 Tellomi AI。777000 仍是登录验证码账号，主用户名 `@admin` |
| `teamgramd/deploy/sql/z_init.sql` | 上游种子。新库插入用户 777000，`users.username` 为 `admin`。已记入 `schema_migrations` 的库不会再跑这份文件 |

`username.id` 是自增，迁移不指定 id。`peer_type` 来自 `proto/mtproto/peer_util.go`：2 是用户，4 是频道，0 是空占位。`deleted` 为 0。列排序规则是 `utf8mb4_unicode_ci`，`Tellomi` 和 `TELLOMI` 与 `tellomi` 是同一行。

一个账号可以有多行 `username`（同一个 `peer_type` 和 `peer_id`）。附录 D 每一格的第一个名字是主用户名（`editable = 1`）。同一账号上的其他名字 `active = 1`、`editable = 0`。

Bot 不插入 `users` / `bots`。`bots.createBot` 把 Bot 放在进程内存里，SQL 插出来的行运行时开不了。这些名字是空占位：`peer_type = 0`，`peer_id = 0`，`active = 0`。`account.checkUsername` 只要表里有行就返回 `USERNAME_OCCUPIED`。`user.resolveUsername` 对 `peer_type` 0 返回 `USERNAME_NOT_OCCUPIED`，直到以后把这一行指到真实账号。

附录 D 最后一行（附录 C 里没有分给某个账号的名字，例如 wechat、google）不写入 `username` 表。那部分留给后面的词库（D-56）。`allowlist.toml` 在 tellomi/tellomi 仓库，不在这里。

登录验证码仍从用户 777000 发出。本变更不改举报，也不改 `@admin` 的发消息逻辑。`@admin` 只是这个服务账号的保留用户名。

上游对照写在 [UPSTREAM_PATCHES.md](../UPSTREAM_PATCHES.md) 的 “Official usernames” 一节。`z_init.sql` 里 777000 的 `users.username` 从 `tellomi` 改成了 `admin`，因为 `migrate.sh` 对已经跑过的 `z_init.sql` 不会再执行，已有库必须靠上面的 `migrate-*.sql`。

## 已有库（先备份）

先备份 MySQL，确认这份备份能还原，再升级或手工执行。备份方式见 [ops/BACKUP.md](ops/BACKUP.md)：`mysqldump --single-transaction`，不要锁全库。生产 Redis 禁止 `FLUSHALL`。这次迁移不碰 Redis。

正常发布不用手工插行。新镜像的入口和 `mysql-migrate` 都会跑 `teamgramd/deploy/migrate.sh`：先装 `0_migrate_helpers.sql`，再按文件名执行还没记入 `schema_migrations` 的 `migrate-*.sql`，最后才是尚未记录过的 `z_init.sql`。三个官方用户名文件会按 `20261104`、`20261105`、`20261107` 的顺序跑。

名字已经被别人占用时，脚本以 `D-27` 或 `D-55` 停住，不会把名字抢过来（D-26）。先处理那一行，再重跑同一个文件。已经成功的语句可以再跑，结果不变。

必须手工执行时，在备份完成之后、对着 `teamgram` 库按这个顺序：

```bash
mysql ... teamgram < teamgramd/deploy/sql/0_migrate_helpers.sql
mysql ... teamgram < teamgramd/deploy/sql/migrate-20261104-tellomi-username.sql
mysql ... teamgram < teamgramd/deploy/sql/migrate-20261105-official-accounts.sql
mysql ... teamgram < teamgramd/deploy/sql/migrate-20261107-tellomi-ai-name.sql
```

不要为了改用户名去重放旧的 `z_init.sql`。库里如果已经有用户 777000，`INSERT IGNORE` 不会把它的用户名改成 `admin`；改名是 `migrate-20261105` 和 `migrate-20261107` 做的，而且只改 `tellomi`、`telegram`、`teamgram` 或空字符串。

手工跑完后记一笔，避免下次 `migrate.sh` 再跑同一文件时你以为它没生效（语句本身可重复，重复跑是安全的）：

```sql
INSERT IGNORE INTO schema_migrations (version) VALUES
  ('migrate-20261104-tellomi-username.sql'),
  ('migrate-20261105-official-accounts.sql'),
  ('migrate-20261107-tellomi-ai-name.sql');
```

`schema_migrations` 这张表不存在时，先让 `migrate.sh` 跑一遍，或按 `teamgramd/deploy/migrate.sh` 里的 `CREATE TABLE IF NOT EXISTS schema_migrations` 建表。

## 核对

```sql
SELECT username, peer_type, peer_id, editable, active, deleted
FROM username
WHERE username IN (
  'tellomi', 'support', 'news', 'admin', 'help', 'report',
  'feedback', 'bots', 'updates', 'security', 'verify',
  'premium', 'stickers', 'dev'
)
ORDER BY username;
```

期望：每个名字一行，`deleted = 0`。`tellomi` 是用户 777001。`support` 是用户 777004。`admin` 是用户 777000。`news` 是频道 1073800001，`updates` 是 1073800002，`security` 是 1073800003，`dev` 是 1073800004（`peer_type` 都是 4）。`help`、`report`、`feedback`、`bots`、`verify`、`premium`、`stickers` 是 `peer_type = 0`、`peer_id = 0`、`active = 0`。

```sql
SELECT id, user_type, first_name, username FROM users
WHERE id BETWEEN 777000 AND 777005
ORDER BY id;
```

期望：777000 是 `user_type` 4、名字 Tellomi、用户名 `admin`。777001 是 Tellomi AI、用户名 `tellomi`。777002 `tellomi_official`，777003 `tellomi_team`，777004 `support`，777005 `faxian`。这五个是 `user_type` 2。它们没有登录密钥，不能登录。

同一条 `SELECT` 用 `TELLOMI` 或 `Tellomi` 也应只命中 `tellomi` 那一行。
