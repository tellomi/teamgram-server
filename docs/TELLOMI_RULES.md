# Tellomi 硬规则

改规则开 PR，不要只改聊天记忆。产品 1–10 四仓同文（`tellomi/tellomi` 总仓、`tellomi/teamgram-server`、`tellomi/Telegram`、`tellomi/Telegram-iOS`）；改口径时四仓同一批 PR 一起改。基准版本：`tellomi/teamgram-server` `docs/TELLOMI_RULES.md`。

1. 对人装的测包/Release：安卓必须 `-PTELLOMI_SERVER_PROFILE=dev` → `8.137.39.82:10443`；禁止打出 `10.0.2.2`（local）；出包前核 BuildConfig DC。
2. DC 用 IP:端口（native `inet_pton`），勿把域名塞进 DC 列表；域名留给官网/协议页。
3. 客户端改动对照上游 Telegram 双端，最小 diff + UPSTREAM_PATCHES；登录/注册一比一按 owner 稿，勿随意自写。
4. 对 owner 只报阶段完成；双端测包齐再报；禁止完善一点就出包。
5. 包体积：瘦安装、肥运行；贴纸/主题/媒体按需下；勿把未用素材/so 打进 afat/通用包。
6. 补服务器 API 必须对照官方 TL/API 文档，不拍脑袋 stub。
7. 大陆默认只 +86；品牌 Tellomi；营销版本从 14.0.x；密钥找「所有权限管理」。
8. 登录改牌后清死引用；出包前过 checklist（DC / 死引用 / 攒批 / 双端）。
9. 预发 `staging-api.tellomi.cn`、生产 `api.tellomi.cn`；端口 10443 MTProto / 11443 WS；生产 Redis 禁 FLUSHALL。
10. 每次安卓 Release 必须发布 `latest.json`（`https://www.tellomi.cn/dl/android/latest.json`，成都从 R2 `tellomi-updates` 同步）。它驱动 `help.getAppUpdate`；禁止只改生产 `.env` 来弹更新。发布后确认安卓会话的 `help.getAppUpdate` 返回新 `versionName`。生产 `TELLOMI_APP_UPDATE_VERSION` 留空。

模块边界与 AI 入口见总仓 `tellomi/tellomi` 的 `docs/MODULE_BOUNDARIES.md` 与 `docs/AI_ENTRY.md`。

## 本仓补充

- API 覆盖只跟官方四端实际调用。
- 「已实现」= 路由 + handler。

## 永久开发规范（Telegram Native-First）

本节与本仓根目录 `AGENTS.md`「永久开发规范」同旨。**仓库内权威版本以该仓 `AGENTS.md` 该节为准。** 完整原则、检查清单和本仓对照路径见 `AGENTS.md`；改本规范必须开 PR，不要只改聊天记忆。

摘要（执行时以 `AGENTS.md` 全文为准）：

1. 增加功能前必须先查看、阅读并理解 Telegram 原有代码的标准与规范（移动端至少对照 iOS 和 Android）。
2. 原版已有的能力必须优先复用，不得平行再实现一套。例如动态里查看图片应走原有查看器；资料页标签遵循原生「有数据才出现」。
3. 只有原版不够用时才扩展，扩展仍须遵循 Telegram 的代码规范。
4. 保持瘦安装、肥运行：配置、文案、资源优先由服务端下发；接入原有缓存与自动清理，禁止自建不受管的本地存储，禁止把大资源打进安装包。
5. 能不修改 Telegram 原有源码就不修改。大多数问题先查服务端。不得不改时做最小 diff，并按本仓既有做法登记到 `UPSTREAM_PATCHES`。
6. 修复必须有真实测试；禁止桩代码和假 200。

开工前必须书面回答，并写入 PR 描述：原版有没有类似功能、在哪个文件、为什么不够用；服务端能不能解决；改了哪些原版文件（未改则写「未改原版」）；缓存与安装包如何处理；测试是什么。
