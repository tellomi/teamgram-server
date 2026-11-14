# 版本与发布

本文档约定版本号规则、CHANGELOG 格式及发布检查清单。

## 版本号规则

- 采用**语义化版本**思想，格式示例：`v0.96.0-teamgram-server`。
  - **主版本号**：不兼容的 API 或架构变更。
  - **次版本号**：向后兼容的功能新增。
  - **修订号**：向后兼容的问题修复与小改进。
- 后缀（如 `-teamgram-server`）可用于与依赖库（如 proto）区分。

## CHANGELOG

- **位置**：仓库根目录 **CHANGELOG.md**（若已创建）。
- **格式**：推荐 [Keep a Changelog](https://keepachangelog.com/)（按版本分段：Added / Changed / Deprecated / Removed / Fixed / Security）。
- **责任**：每次发布前更新 CHANGELOG，将本次改动归入对应版本与类型。

## 发布检查清单

发布新版本时建议完成：

1. **版本号**：在 Makefile 或版本注入处更新 `VERSION`（若适用）。
2. **CHANGELOG**：在 CHANGELOG.md 中新增该版本条目并填写变更摘要。
3. **Tag**：在 Git 中打 tag（如 `v0.96.0`）并推送到远程。
4. **二进制/镜像**：按项目惯例构建并发布预编译二进制或 Docker 镜像。
5. **README**：若主 README 中有固定版本号或“当前版本”说明，按需更新。
6. **安卓客户端**：发布 `latest.json`，并确认 `help.getAppUpdate`（见下）。每次安卓 Release 都必须做。

## 安卓应用内更新

每次安卓 Release 都要发布 `https://www.tellomi.cn/dl/android/latest.json`。成都从 R2 桶 `tellomi-updates` 同步这份文件。`help.getAppUpdate` 读取它，并用 `versionName` 和客户端版本比较。弹出应用内更新靠的是这个文件，不要靠改生产 `.env` 里的 `TELLOMI_APP_UPDATE_VERSION`。生产环境把该变量留空。非空会盖住清单，下次 APK 发布后就会过期。

清单字段为 `versionName`、`versionCode`、`url`、`fallbackUrl`、`sha256` 或 `sha256sum`、`size`。已发布文件在顶层使用 `sha256sum`，在每条 `files[]` 上使用 `sha256`。可选的 `changelog`（字符串）是对话框正文。可选的 `notes` 是语言代码到正文的对象，例如 `{"zh-hans":"...","en":"..."}`。客户端语言取 initConnection 的 `lang_code`，其次 `system_lang_code`，再其次当 `lang_pack` 本身是语言代码时用它。先用匹配的 `notes`，没有时用 `notes.en`（如果有这个键）。没有任何匹配的 note 时，字符串 `changelog` 仍是对话框正文，因此现有 `latest.json` 保持兼容。两者都空时，服务器使用该语言的内置句子；未知语言用英语。非空的 `TELLOMI_APP_UPDATE_TEXT` 会替换所有语言的对话框正文。更新提示默认可跳过。只有 `TELLOMI_APP_UPDATE_CANNOT_SKIP=1`，或清单里的 `"mandatory": true`（JSON 布尔值，其他类型忽略），才会设置 `can_not_skip`。iOS 仍然收到 `help.noAppUpdate`。

`TELLOMI_APP_UPDATE_MANIFEST` 指定文件路径或 `http(s)` URL。未设置或为空时，服务器使用第一个存在的普通文件：

- `/etc/tellomi/updates/android/latest.json`
- `/opt/tellomi/updates/android/latest.json`
- `/var/www/tellomi/dl/android/latest.json`
- `/usr/share/nginx/html/dl/android/latest.json`

否则请求 `https://www.tellomi.cn/dl/android/latest.json`。`off` 关闭清单。成功读取缓存 5 分钟。超时（5 秒）或其他读取错误缓存 30 秒，然后回答 `help.noAppUpdate`。`TELLOMI_APP_UPDATE_VERSION` 非空时直接使用环境变量，不再读清单，因此清单故障不会挡住这次覆盖。文件更新后不必重启进程。

每次安卓发布之后：

1. `curl -fsS https://www.tellomi.cn/dl/android/latest.json` 能看到新的 `versionName`。
2. 5 分钟内，比该版本旧的安卓会话调用 `help.getAppUpdate`，返回 `help.appUpdate`，且 `versionName` 与清单 `url` 一致。只有清单含 `"mandatory": true` 或 `TELLOMI_APP_UPDATE_CANNOT_SKIP=1` 时 `can_not_skip` 才为真。iOS 会话仍返回 `help.noAppUpdate`。
