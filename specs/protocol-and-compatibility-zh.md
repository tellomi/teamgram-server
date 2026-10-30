# 协议与兼容性

本文档说明 Teamgram Server 所支持的 MTProto 子协议、API Layer 及与官方客户端的兼容范围。

## MTProto 2.0

Teamgram 实现 [MTProto 2.0](https://core.telegram.org/mtproto)，支持以下传输格式：

- **Abridged**
- **Intermediate**
- **Padded intermediate**
- **Full**

客户端通过 TCP 或 WebSocket 连接 gnetway，使用上述任一格式进行握手与 RPC。

## API Layer

- 当前支持的 **API Layer** 为 **232**。编解码在仓库内的 [`proto/`](../proto/README.md)（上游 teamgram/proto 停在 layer 231）。
- TL 方法与类型以该 fork 为准，layer 232 的 constructor 与 Telegram Desktop `api.tl`（`// LAYER 232`）一致。

## 兼容客户端

| 客户端 | 说明 |
|--------|------|
| [Android (teamgram-android)](../clients/teamgram-android.md) | 修改 ConnectionsManager.cpp 中的服务器地址与端口 |
| [iOS (teamgram-ios)](../clients/teamgram-ios.md) | 配置连接自建服务器 |
| [桌面端 (teamgram-tdesktop)](../clients/teamgram-tdesktop.md) | 配置连接自建服务器 |

**重要**：未设置 `TELLOMI_SMS_PROVIDER` 时，手机登录验证码仍是 **12345**（仅开发用）。设为 `aliyun` 时发送真实短信。邮箱登录使用 `auth.sendCode` 发出的验证码；`TELLOMI_EMAIL_PROVIDER` 默认为 `log`，验证码打在日志里。见 [docs/sms-email.md](../docs/sms-email.md) 与 [docs/email-signup.md](../docs/email-signup.md)。

## 部署说明

- 客户端需能访问 gnetway 端口（默认 10443、11443、5222）。TLS 或反向代理（如 Nginx WebSocket）需按需配置。
