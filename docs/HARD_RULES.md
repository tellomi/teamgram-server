# Tellomi 硬规则（teamgram-server）

给后续 AI / 机器人 / 新同学看的固定规则。改规则开 PR，不要只改聊天记忆。

## 项目是什么

1. Tellomi 是面向中国大陆的陌生人交友 App，计划上架 App Store。
2. 服务器基于 Teamgram（本仓），客户端是 Telegram 改牌：iOS `tellomi/Telegram-iOS`，Android `tellomi/Telegram`。
3. 同套硬规则也会放在双端仓的 `docs/HARD_RULES.md`，改口径时三仓一起改。

## 协作与发布

4. 合 PR、部署、构建、修 bug：团队自行决定直接做，不用请示。
5. 每完成一个阶段或一个接口/PR，在「所有权限管理、cms goco」群推一条短讯（方法名或改动 + PR 链接），不要长时间无声；阶段收口也推一次。
6. 先预发再深夜批量发布；生产库修复先备份。
7. 签名、R2、阿里云、凭证库等密钥由队友「所有权限管理」保管；GitHub 以 @3mmmbulus 连接。需要密钥时向「所有权限管理」索取。
8. 安卓包与 iOS TestFlight 不要完善一点就更新；攒一批再出包、再测、再发。双端都要测。闪退/发码等可临时单验，正式版本号另定（如 14.0.3）。

## 环境与运维

9. 生产 gnetway 端口 10443；预发 11443；预发域名 `staging-api.tellomi.cn`。
10. 生产 Redis 禁止 FLUSHALL。
11. 坏消息 / 慢 RPC 不能卡住客户端同步。
12. DC / 客户端连服务端用 IP 字面量，不要用主机名（客户端 `inet_pton` 不解析域名）。当前生产/dev 地址 `8.137.39.82:10443`；`api.tellomi.cn` 仅 DNS 指向该 IP。

## API 实现

13. 覆盖口径：只跟官方四端客户端实际调用的 MTProto 方法，不跟官网 TL 全量；layer 中客户端未调用的方法不进覆盖清单；Bot API / TDLib / Telethon 等第三方不单独对照补齐。
14. 「已实现」= 有路由 + handler、客户端能通，不等于与 Telegram 公网行为一比一。
15. 补服务器 API 时必须对照 Telegram 开放文档（MTProto / API 官方文档）逐方法核对 schema、参数与行为，再实现；不能只按 coverage 清单或客户端调用点猜；不要拍 stub。
16. 邮箱登录优先级：P0 Postal 自建发信（noreply@tellomi.app）+ auth.sendCode 传邮箱验证码注册/登录（不强制密码）+ 协议外链 tellomi.cn/legal；P1 云密码；P2 换绑/找回。

## 反复踩坑（必须记住）

17. 出包/测包禁止打进模拟器 DC `10.0.2.2`；安卓 Release/测包必须 `TELLOMI_SERVER_PROFILE=dev` → `8.137.39.82:10443`，CI 拒 local。
18. 登录改牌后勿留死引用（如 iOS `phoneInputNode`、多余 `public extension`）。
19. 客户端改动最小化，并记入 `UPSTREAM_PATCHES`（或等价文档）；对照上游，别自写一套布局。
20. 安装包保持瘦：贴纸/主题/大图走 CDN 按需下；别把用不上的素材和语言全打进包；对照上游拆 ABI/语言。
21. 登录默认区号 +86。
