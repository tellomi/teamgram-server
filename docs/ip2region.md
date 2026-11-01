# 会话地区（ip2region）

`account.getAuthorizations` 的 `country` / `region`，以及登录服务通知里的位置，按地址选库：

- **中国大陆**：用离线库 [ip2region](https://github.com/lionsoul2014/ip2region)。省、市比通用城市库更准，输出中文，例如 country `中国`、region `四川 成都`。
- **其他地址**（境外）：用 DB-IP Lite 城市库。运维把它挂在 `/app/bin/GeoLite2-City.mmdb`（`docker-compose.geoip.yaml`）。读的是 MMDB 城市记录里的英文名。
- **香港、澳门、台湾**不是国家。有 ip2region 记录时用它；否则把城市库里的 ISO `HK` / `MO` / `TW`（或英文国名）写成 country `中国`，region 为 `香港` / `澳门` / `台湾`，后面可以带城市，例如 `中国 台湾 台北`。
- 只剩其中一个库时，用还在的那个。两个都没有时，`country` 回退成客户端 IP，不写 `UNKNOWN`。RPC 不失败。

以前启动日志里的 `GeoIP not loaded` 是因为进程只打开 `./GeoLite2-City.mmdb`，镜像里没有这个文件。现在 ip2region 打进镜像；城市库仍然是宿主机挂载，缺了只记一条说明，不让进程退出。

## 来源和许可

| 项 | 值 |
|---|---|
| 上游 | https://github.com/lionsoul2014/ip2region |
| 发行标签 | `v3.18.0` |
| 提交 | `c1a1fc7d5941760db3f8431dc05c48cf7f0e30a1` |
| Go 绑定 | `github.com/lionsoul2014/ip2region/binding/golang` `v0.0.0-20260901011515-c1a1fc7d5941`（同一提交） |
| 许可 | Apache-2.0 OR MIT（`SPDX-License-Identifier: Apache-2.0 OR MIT`） |
| 本仓库选用 | Apache License 2.0 |
| 许可原文 | https://github.com/lionsoul2014/ip2region/blob/v3.18.0/LICENSE.md |

数据文件在该 tag 的 `data/` 目录里。GitHub Release 页没有把 xdb 另挂成附件。下载地址：

- https://github.com/lionsoul2014/ip2region/raw/refs/tags/v3.18.0/data/ip2region_v4.xdb （11122036 字节）
- https://github.com/lionsoul2014/ip2region/raw/refs/tags/v3.18.0/data/ip2region_v6.xdb （37277813 字节）

SHA256 写在 `deploy/ip2region/SHA256SUMS`。归属说明在 `deploy/ip2region/NOTICE`。

```
8e31bbdccb5bf21028af10592d4312ec975da0bffa108c0c5d862a12190f9ad3  ip2region_v4.xdb
939f6b46bd2b8bec3cf7c5ceb8ba782266ae9b1f35b5ba7916700dec0b7506ed  ip2region_v6.xdb
```

换版本时改 `scripts/fetch-ip2region-xdb.sh` 里的 tag，重新下载后用 `sha256sum` 更新 `SHA256SUMS` 和 NOTICE 里的字节数。校验失败时脚本退出，镜像构建失败。

## 镜像

根目录 `Dockerfile` 在构建阶段运行 `scripts/fetch-ip2region-xdb.sh`：从上面的官方 tag 下载 v4 和 v6，核对 SHA256，再复制到镜像内的 `/app/data/ip2region/`。GitHub Actions 的镜像构建（`docs/ci-acr.md`）走的就是这个 Dockerfile。xdb 不进 git。

`authsession` 和 `bff` 各自在启动时把 xdb 整文件读进内存（ip2region 的 `BufferCache`）。查询不再读盘。两个进程各持有一份，大约是 v4 11MB 加 v6 36MB。

## 配置

两个库都会尝试打开。路径可以用环境变量改，不必选「只用某一个」。

| 变量 | 作用 |
|---|---|
| `IP2REGION_V4_XDB` | v4 xdb。空则依次找 `/app/data/ip2region/ip2region_v4.xdb`、`data/ip2region/ip2region_v4.xdb` |
| `IP2REGION_V6_XDB` | v6 xdb，规则同上。没有 v6 时，IPv6 走城市库；城市库也没有则回退成客户端 IP |
| `MMDB` | 城市库路径，优先 |
| `GEOIP_MMDB` | `MMDB` 为空时使用 |
| `TELLOMI_GEOIP_MMDB` | 上面两个都为空时使用。`docker-compose.geoip.yaml` 的容器内路径，默认 `/app/bin/GeoLite2-City.mmdb` |
| `-mmdb` | 环境变量都为空时使用，默认 `./GeoLite2-City.mmdb`。authsession 和 bff 的工作目录是 `/app/bin`，所以这个相对路径就是挂载点 |

`docker-compose.yaml` 把 v4/v6 指到镜像内路径，并把 `MMDB` / `GEOIP_MMDB` 指到 `/app/bin/GeoLite2-City.mmdb`。数据库文件本身由 `docker-compose.geoip.yaml` 从宿主机只读挂进来，不进 git，也不打进镜像。挂载不在时，非大陆地址改用 ip2region 的国家名。

城市库用 `geoip2` 读取 MMDB。DB-IP Lite city 和 GeoLite2-City 都走同一套 `country` / `region` 英文名。

## 字段

官方构造是 `authorization` 的 `country:string` 和 `region:string`。Telegram 客户端把两段用逗号拼起来显示（Android 会话单元格、Desktop、iOS 的会话行都是 `country, region`）。

ip2region 原文是 `国家|省份|城市|ISP|ISO`。ISP 不进地区。`0`、`Reserved`、`UNKNOWN`、`内网IP` 视为空。

大陆（国家为「中国」或 ISO 为 `CN`，且省份不是香港、澳门、台湾）去掉「省 / 市 / 自治区 / 特别行政区」后缀，并且不采用城市库的英文名：

| 地址 | country | region | 客户端上看到 | 登录通知位置行 |
|---|---|---|---|---|
| 成都 | 中国 | 四川 成都 | 中国, 四川 成都 | 中国 四川 成都 |
| 北京、上海（省市同名） | 中国 | 北京 | 中国, 北京 | 中国 北京 |
| 台北 | 中国 | 台湾 台北 | 中国, 台湾 台北 | 中国 台湾 台北 |
| 香港 | 中国 | 香港 | 中国, 香港 | 中国 香港 |
| 澳门 | 中国 | 澳门 | 中国, 澳门 | 中国 澳门 |

境外地址用城市库的英文国家名和一级行政区，例如 `8.8.8.8` 在 DB-IP / GeoLite2 里是 country `United States`、region `California`。城市库没有这份记录时，改用 ip2region：境外通常只剩国家名，`region` 为空。香港、澳门、台湾即使城市库把它们写成国家，输出仍然是 `中国` 加对应地区。

私网、保留地址和两边都查不到的地址不写「Reserved」。`country` 回退为规范化后的客户端 IP，`region` 为空。
