# Simulator UI acceptance

English below. [中文](#中文).

These flows are the gate before a staging deploy. GitHub Actions runs them on
`workflow_dispatch` and when a pull request is labeled `deploy-staging`.
They talk to `staging-api.tellomi.cn` (TCP `11443`). The `ready-for-deploy`
job is green only after the flows pass. Do not `docker compose pull` on the
staging host until that job is green.

The image publish workflow (`.github/workflows/docker-acr.yml`) is separate.
It does not run this suite.

Client source stays in `tellomi/Telegram` and `tellomi/Telegram-iOS`. Nothing
in those repositories is edited or pushed. Flows, fixtures, and the workflow
live here:

| Path | What it is |
| --- | --- |
| `e2e/ui/maestro/` | Maestro flows (one set for both platforms) |
| `e2e/ui/fixtures/group-avatar.png` | Avatar uploaded while creating a group |
| `e2e/ui/scripts/` | APK download, iOS build-or-download, contacts, hierarchy check |
| `.github/workflows/ui-acceptance.yml` | The gate |

Screenshots are uploaded as `ui-android-screenshots` and `ui-ios-screenshots`
(14 days), including Maestro's failure shots.

## Checklist v1

Two accounts, `E2EAda` and `E2EByron`, message each other on one device
(account switch). A third number is used only for the sign-up screen.

1. An outgoing message stays a single tick (`Not seen`) until the peer opens the chat, then it becomes a double tick (`Seen`).
2. Three messages stay in send order, top to bottom (`e2e-order-1`, then `2`, then `3`). A hierarchy dump checks the y coordinates.
3. Double-tap shows the default reaction chip (`👍`).
4. After a restart the chat list comes back. The screen must not stay on `正在更新` or `Updating...`.
5. Sign-up shows `我已阅读并同意《用户服务协议》和《隐私政策》`. Continue does nothing until the box is checked. There is no `服务条款` popup and no `同意 & 继续` button.
6. Create a group that includes the peer and set `fixtures/group-avatar.png` as the avatar.

The sign-up copy is hardcoded in the clients (iOS accessibility id
`Auth.SetName.AgreementCheckbox` on the checkbox branch). The Android release
and the iOS `master` build have to contain that screen or checklist item 5
fails. That is the gate.

## How a run reaches staging

Staging login uses the test range `99966XYYYY` (datacenter digit `X`, code
that digit five times: `9996612345` → `11111`). No SMS is sent. See
`docs/sms-email.md`. Each run picks fresh numbers unless the phone secrets
below are set. The device locale is Simplified Chinese so a stuck update
screen can show `正在更新`.

The clients are not rebuilt to embed a new Android IP, and a hosts file
cannot change the Android release: that APK dials an IPv4 literal
(`TELLOMI_PROFILE_DEV` in `tellomi/Telegram`, production port `10443`). The
emulator is `google_apis` so `adb root` works, and `iptables` DNAT sends
every TCP connection to that production port to the staging address on TCP
`11443`.

iOS dials a hostname. A downloaded simulator `.app` still has
`api.tellomi.cn` and production port `10443`, so the macOS runner maps that
name to the staging IP. The hosts entry remaps the name. The prebuilt app
keeps production port `10443`. A from-source build rewrites
`Tellomi/tellomi-server.json` in the CI checkout only (seeds pointed at
staging, `seed_port` `11443`), relaxes the production host and port
assertions in that checkout's `Tellomi/check_server_config.py`, regenerates
the header, and builds `debug_sim_arm64`. The checkout is never pushed. A
device IPA is rejected: it cannot boot in the Simulator.

Staging must present the same development RSA key the clients ship
(fingerprint `12240908862933197005`). The runner must be able to open TCP
`11443` on the host. If GitHub-hosted runners cannot, set
`E2E_ANDROID_RUNNER` and `E2E_IOS_RUNNER` to self-hosted runners that can.

## Runners

| Job | Default runner | What it does |
| --- | --- | --- |
| Android | `ubuntu-latest` (`E2E_ANDROID_RUNNER`) | `reactivecircus/android-emulator-runner` (API 34, `google_apis`, `x86_64`, `pixel_6`). Installs the latest `tellomi/Telegram` release APK (`app.tellomi.chat`) and runs Maestro. |
| iOS | `xcode-27` (`E2E_IOS_RUNNER`) | Same image as `tellomi/Telegram-iOS` TestFlight (Xcode from that repo's `versions.json`, currently 27.1). Downloads a simulator `.app` zip when one is configured or attached to the latest iOS release; otherwise builds `debug_sim_arm64`. |

Create the pull request label `deploy-staging` once. The workflow file has
to be on the default branch before a label on another PR can start it.
`workflow_dispatch` can target this branch directly.

`ready-for-deploy` fails if both platform jobs were skipped.

## Secrets and variables

No secret is required. Test numbers are generated, and their code is the
datacenter digit five times. Do not add SMS, Aliyun, or ACR secrets for this
workflow.

| Name | Kind | Required | Purpose |
| --- | --- | --- | --- |
| `TELLOMI_GH_TOKEN` | secret | no | PAT used only if the default token cannot read `tellomi/Telegram` releases or `tellomi/Telegram-iOS`. Public repos need nothing. |
| `TELLOMI_ANDROID_APK_URL` | secret | no | Override the APK URL. Default: the `.apk` asset on the latest `tellomi/Telegram` release. |
| `IOS_SIMULATOR_APP_URL` | secret | no | Zip that contains a simulator `Tellomi.app` / `Telegram.app` (`app.tellomi.chat`). Skips the Bazel build. Dispatch input `ios_app_url` wins. |
| `E2E_PHONE_A` | secret | no | Stable number for account A, digits `999661xxxx`. Default: a fresh number per run. |
| `E2E_PHONE_B` | secret | no | Stable number for account B. |
| `E2E_PHONE_SIGNUP` | secret | no | Stable number for the sign-up checkbox flow. Reuse only after the account is deleted, or the sign-up screen will not appear. |
| `E2E_API_HOST` | env / dispatch `api_host` | no | Default `staging-api.tellomi.cn`. |
| `E2E_API_PORT` | env | no | Default `11443` (staging). Release clients still dial production `10443`; Android DNAT rewrites that to this port. |
| `E2E_DC_DIGIT` | env | no | Default `1` (code `11111`). |
| `E2E_ANDROID_RUNNER` | variable | no | Default `ubuntu-latest`. |
| `E2E_IOS_RUNNER` | variable | no | Default `xcode-27`. |
| `IOS_GIT_REF` | env / dispatch `ios_git_ref` | no | Branch or tag to build. Default `master`. |

## Local checks

```bash
python3 e2e/ui/scripts/test_support.py
```

That does not boot an emulator. The shell scripts are parsed with `bash -n`.

## 中文

这是 staging 部署前的界面验收。Actions 里手动运行（`workflow_dispatch`），或给 pull request 打上 `deploy-staging` 标签时，会在模拟器里跑 Maestro。目标是 `staging-api.tellomi.cn` 的 TCP `11443`。`ready-for-deploy` 成功之后，才能在 staging 主机上 `docker compose pull`。

镜像发布（`.github/workflows/docker-acr.yml`）不跑这套测试。

客户端仓库 `tellomi/Telegram` 和 `tellomi/Telegram-iOS` 不会被修改，也不会被推送。流程、夹具和工作流都在本仓库的 `e2e/ui/`。截图作为 artifact 保存 14 天：`ui-android-screenshots`、`ui-ios-screenshots`。

### 清单 v1

一台设备上两个账号（`E2EAda`、`E2EByron`）互发消息，用切换账号代替第二台模拟器。注册勾选框另用一个新号码。

1. 对方没打开会话时，发出的消息保持单勾（`Not seen`）；对方打开后才变成双勾（`Seen`）。
2. 三条消息按发送顺序从上到下排列（`e2e-order-1`、`2`、`3`）。层级导出检查纵坐标。
3. 双击消息出现默认反应（`👍`）。
4. 重启后能回到会话列表，不能停在 `正在更新` 或 `Updating...`。
5. 注册页有 `我已阅读并同意《用户服务协议》和《隐私政策》`。没勾选时不能继续。不能出现 `服务条款` 弹窗，也不能出现 `同意 & 继续`。
6. 建群，拉进对方，并把 `fixtures/group-avatar.png` 设为头像。

第 5 条依赖客户端里的勾选框（iOS 的无障碍 id 是 `Auth.SetName.AgreementCheckbox`）。当前发布的 APK 或 iOS `master` 还没有这个界面时，这一条会失败，部署门保持关闭。

### 怎么连上 staging

登录用测试号 `99966XYYYY`。`X` 是数据中心数字，验证码是这个数字重复五次（`9996612345` → `11111`）。不发短信。说明见 `docs/sms-email.md`。没设置下面的号码 secret 时，每次运行换新号。系统语言是简体中文，这样卡住的更新页才会显示 `正在更新`。

Android 发布包拨的是写死的 IPv4（`tellomi/Telegram` 的 `TELLOMI_PROFILE_DEV`，生产端口 `10443`），改 hosts 没用。模拟器使用 `google_apis` 镜像，`adb root` 之后用 `iptables` DNAT，把连向该端口的 TCP 转到 staging 的 TCP `11443`。

iOS 拨的是主机名。下载的模拟器包如果仍是 `api.tellomi.cn`，并且仍是生产端口 `10443`，macOS runner 会把这个名字指到 staging 的 IP。hosts 只改名字，预编译包仍拨生产端口 `10443`。从源码构建时，只改 CI 工作副本里的 `Tellomi/tellomi-server.json`（种子指向 staging，`seed_port` 为 `11443`），并放宽该副本里 `Tellomi/check_server_config.py` 对生产主机和端口的断言，然后重新生成头文件，再编 `debug_sim_arm64`。不会推回客户端仓库。设备用的 IPA 不能在模拟器里启动，脚本会拒绝。

staging 必须使用客户端自带的同一把开发 RSA 公钥（指纹 `12240908862933197005`）。runner 必须能连上该主机的 TCP `11443`。GitHub 托管 runner 连不上时，把变量 `E2E_ANDROID_RUNNER` 和 `E2E_IOS_RUNNER` 设成能连上的 self-hosted runner。

### Runner

| 作业 | 默认 runner | 行为 |
| --- | --- | --- |
| Android | `ubuntu-latest`（变量 `E2E_ANDROID_RUNNER`） | `reactivecircus/android-emulator-runner`（API 34，`google_apis`，`x86_64`，`pixel_6`）。安装 `tellomi/Telegram` 最新 release APK（`app.tellomi.chat`），再跑 Maestro。 |
| iOS | `xcode-27`（变量 `E2E_IOS_RUNNER`） | 与 `tellomi/Telegram-iOS` TestFlight 相同的镜像（Xcode 版本见该仓库 `versions.json`，目前 27.1）。有模拟器 `.app` 压缩包就下载；否则构建 `debug_sim_arm64`。 |

仓库里需要先建好标签 `deploy-staging`。工作流文件在默认分支上之后，别的 PR 打这个标签才会触发。`workflow_dispatch` 可以直接选本分支。

Android 和 iOS 都跳过时，`ready-for-deploy` 失败。

### 需要的 secret 和变量

没有必填 secret。测试号是现生成的，验证码是数据中心数字重复五次。不要为这个工作流配置短信、阿里云或 ACR 的 secret。

| 名称 | 类型 | 必填 | 用途 |
| --- | --- | --- | --- |
| `TELLOMI_GH_TOKEN` | secret | 否 | 默认 token 读不到 `tellomi/Telegram` release 或 `tellomi/Telegram-iOS` 时才用的 PAT。公开仓库不需要。 |
| `TELLOMI_ANDROID_APK_URL` | secret | 否 | 指定 APK 地址。默认是 `tellomi/Telegram` 最新 release 里的 `.apk`。 |
| `IOS_SIMULATOR_APP_URL` | secret | 否 | 含模拟器 `Tellomi.app` / `Telegram.app`（`app.tellomi.chat`）的 zip。设置后跳过 Bazel。手动运行时的 `ios_app_url` 优先。 |
| `E2E_PHONE_A` | secret | 否 | 账号 A 的固定号码，数字形式 `999661xxxx`。默认每次换新号。 |
| `E2E_PHONE_B` | secret | 否 | 账号 B 的固定号码。 |
| `E2E_PHONE_SIGNUP` | secret | 否 | 注册勾选框用的固定号码。账号还在时不会再进入注册页，所以不要复用没删掉的号。 |
| `E2E_API_HOST` | 环境变量 / 手动输入 `api_host` | 否 | 默认 `staging-api.tellomi.cn`。 |
| `E2E_API_PORT` | 环境变量 | 否 | 默认 `11443`（staging）。发布版客户端仍拨生产端口 `10443`；Android DNAT 把该端口转到这个端口。 |
| `E2E_DC_DIGIT` | 环境变量 | 否 | 默认 `1`（验证码 `11111`）。 |
| `E2E_ANDROID_RUNNER` | 变量 | 否 | 默认 `ubuntu-latest`。 |
| `E2E_IOS_RUNNER` | 变量 | 否 | 默认 `xcode-27`。 |
| `IOS_GIT_REF` | 环境变量 / 手动输入 `ios_git_ref` | 否 | 要构建的分支或 tag。默认 `master`。 |

### 本地检查

```bash
python3 e2e/ui/scripts/test_support.py
```

这不会启动模拟器。shell 脚本只用 `bash -n` 做语法检查。
