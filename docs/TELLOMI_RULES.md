# Tellomi 硬规则

改规则开 PR，不要只改聊天记忆。产品 1–9 三仓同文（本仓、iOS、Android）；改口径时一起改。

1. 对人装的测包/Release：安卓必须 `-PTELLOMI_SERVER_PROFILE=dev` → `8.137.39.82:10443`；禁止打出 `10.0.2.2`（local）；出包前核 BuildConfig DC。
2. DC 用 IP:端口（native `inet_pton`），勿把域名塞进 DC 列表；域名留给官网/协议页。
3. 客户端改动对照上游 Telegram 双端，最小 diff + UPSTREAM_PATCHES；登录/注册一比一按 owner 稿，勿随意自写。
4. 对 owner 只报阶段完成；双端测包齐再报；禁止完善一点就出包。
5. 包体积：瘦安装、肥运行；贴纸/主题/媒体按需下；勿把未用素材/so 打进 afat/通用包。
6. 补服务器 API 必须对照官方 TL/API 文档，不拍脑袋 stub。
7. 大陆默认只 +86；品牌 Tellomi；营销版本从 14.0.x；密钥找「所有权限管理」。
8. 登录改牌后清死引用；出包前过 checklist（DC / 死引用 / 攒批 / 双端）。
9. 预发 `staging-api.tellomi.cn`、生产 `api.tellomi.cn`；端口 10443 MTProto / 11443 WS；生产 Redis 禁 FLUSHALL。

## 本仓补充

- API 覆盖只跟官方四端实际调用。
- 「已实现」= 路由 + handler。
