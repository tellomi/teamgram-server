# 客户端 IP 消费点审计（gnetway → session / BFF）

审计日期：2026-11-11

基线：`origin/master` `e2bb7ea796527da090eca61d98ef04b4622a980b`

范围：只读。本文不改协议、不改监听、不切流量。结论对应当前仓库源码，行号以该提交为准。

场景：另有一台阿里云轻量应用服务器做 MTProto 443/10443 的 L4 TCP 透传，回源私网地址为 `172.19.44.222`。没有 PROXY protocol 时，gnetway 看到的对端是这台中继，不是用户。此前结论是：`pp` 包能解析 PROXY v2，但接入路径只认 v1，且没有来源网段校验。本审计把「谁在用这个 IP」补全，并给出最小服务器改动设计。

## 0. 结论

下游只有一个身份字段。gnetway 把 `connContext.clientIp` 放进 `session.SessionClientData.client_ip`，session 再放进 `RpcMetadata.client_addr`。`initConnection` 时把它写入 `auths.client_ip`。活跃会话、登录通知、发现页地区、登录类 IP 限流，读的都是这一个字符串。没有单独的 `socket_peer_ip`。

不启用 PROXY、或者启用了却不校验来源，会有两类事故：

- 所有经这台中继的用户被记成 `172.19.44.222`。登录 IP 桶变成全站共享；设备列表和国家/地区变成中继地址；邮箱账号的发现页地区失去「人在大陆」这个信号。
- 现有 `0.0.0.0:10444` 的 PPV1 监听只要收得到连接，就会把 `PROXY ` 行里的源地址当成用户。没有 /32 白名单。不能把这台中继指到 10444，也不能把该监听开到私网。

`pp.ReadHeader` 能解析 v2，`OnTraffic` 不会走到那里。v2 签名不是 `PROXY `，会被当成 MTProto 字节。

最小改动是新增一个默认关闭的私网监听：只接受配置的 /32（本次 `172.19.44.222/32`）发来的一条 v1 或 v2 TCP 头，然后沿用现有的 `client_ip`。其它监听保持今天的行为。公网 10443 继续不解析 PROXY。

## 1. 从 gnetway 到 session / BFF

### 1.1 连接上怎么得到 IP

`OnOpen` 用 socket 对端的 host 作为初值，并按**本端监听地址的完整字符串**决定要不要等 PROXY v1。

```111:120:app/interface/gnetway/internal/server/gnet/server_gnet.go
func (s *Server) OnOpen(c gnet.Conn) (out []byte, action gnet.Action) {
	logx.Debugf("onNewConn - conn(%s)", c)

	ctx := newConnContext()
	if host, _, err := net.SplitHostPort(c.RemoteAddr().String()); err == nil {
		ctx.setClientIp(host)
	} else {
		ctx.setClientIp(c.RemoteAddr().String())
	}
	ctx.ppv1 = s.c.Gnetway.IsProxyProtocolV1(c.LocalAddr().String())
```

字段只有一个。`connContext.clientIp` 定义在 `app/interface/gnetway/internal/server/gnet/conn.go:42`，写入函数是同文件 `72:74`。socket 对端被覆盖后不再保留。

PPV1 打开时，`OnTraffic` 先 `Peek(-1)`。前缀是 `PROXY `（`pp.V1Identifier`，`pp/proxy.go:34`）才调用 `pp.ReadHeader`，用头里的源地址覆盖 `clientIp`，丢掉头的字节，然后把 `ppv1` 清掉。没有对 socket 对端做 CIDR 判断。

```227:270:app/interface/gnetway/internal/server/gnet/server_gnet.go
	if ctx.ppv1 {
		ppv1, err := c.Peek(-1)
		// ...
		if bytes.HasPrefix(ppv1, pp.V1Identifier) {
			r := bytes.NewReader(ppv1)
			h, err := pp.ReadHeader(r)
			// ...
			if h.Source == nil {
				logx.Errorf("conn(%s) ppv1 header has no source address", c)
				return gnet.Close
			}
			if host, _, err := net.SplitHostPort(h.Source.String()); err == nil {
				ctx.setClientIp(host)
			} else {
				ctx.setClientIp(h.Source.String())
			}
			logx.Infof("ppv1 accepted client_ip=%s local=%s", ctx.clientIp, c.LocalAddr().String())
			_, _ = c.Discard(len(ppv1) - r.Len())
			ctx.ppv1 = false
			// ...
		} else {
			ctx.ppv1 = false
		}
	}
```

前缀不是 `PROXY ` 时，包括 v2 的 12 字节签名 `\r\n\r\n\x00\r\nQUIT\n`（`pp/proxy.go:35`），走 `268:270`：不再解析，这些字节进入后面的 MTProto 解码。`pp.ReadHeader`（`pp/proxy.go:66-88`）本身会先认 v2 再认 v1，但接入层进不去。

源地址为空就关闭连接（`252:254`）。v1 的 `UNKNOWN` 和 v2 的 `LOCAL` 都会让 `Source` 为空（`pp/v1.go:22-27`，`pp/v2.go:74-77`）。这条关闭只发生在已经匹配 `PROXY ` 的路径上。

### 1.2 加密 TCP 进 session

每条加密消息在进协程前把当时的 `ctx.clientIp` 抄出来（`server_gnet.go:409-412`），填进 `SessionClientData.ClientIp`（同文件 `456:467`）。对应 proto 字段是 `client_ip = 9`（`app/interface/session/session/session.tl.proto:63`）。

session 收到后原样下传：

```51:59:app/interface/session/internal/core/session.sendDataToSession_handler.go
	_ = mainAuth.SessionDataArrived(
		c.ctx,
		int(data.KeyType),
		data.AuthKeyId,
		data.ServerId,
		data.ClientIp,
		data.SessionId,
		data.Salt,
		data.Payload)
```

`SessionDataArrived` 把它放进队列消息（`app/interface/session/internal/sess/main_auth_wrapper.go:938-945`）。解包后 `onInitConnection` 用这个字符串更新会话资料（`session_invoke.go:368-376` → `main_auth_wrapper.go:468-555`）。RPC 则先过 flood，再放进 `rpcApiMessage.clientIp`（`session_invoke.go:479-493`）。

gRPC 调试日志会打这个字段，但不参与判断：`app/interface/session/internal/server/grpc/service/session_service_impl.go:74-81`。

### 1.3 关闭连接时的 `ip|纳秒`

关闭时 `ClientIp` 被拼成 `地址|unixnano`（`server_gnet.go:202-205`），用来防止重启后的迟到关闭把新连接清掉。`SplitCloseStamp`（`app/interface/session/internal/sess/session.go:229-240`）拆开它。`session.closeSession` 只用时间戳，地址被丢掉：

```47:48:app/interface/session/internal/core/session.closeSession_handler.go
	_, closedAt := sess.SplitCloseStamp(cli.ClientIp)
	if err := mainAuth.SessionClientClosed(c.ctx, int(cli.KeyType), cli.AuthKeyId, cli.ServerId, cli.SessionId, closedAt); err != nil {
```

所以关闭路径不更新 `auths.client_ip`，也不进 flood。已验证地址不能包含 `|`，否则时间戳解析会失败。IPv4 / IPv6 文本没有这个字符。

### 1.4 写入 BFF 元数据

真正发往 BFF 时，`finishRPC` 构造元数据。`client_addr` 就是这条 RPC 上的 `clientIp`，不是 `auths` 表里上次 `initConnection` 留下的值。

```824:838:app/interface/session/internal/sess/main_auth_wrapper.go
	md := &metadata.RpcMetadata{
		ServerId:       m.cb.Dao.MyServerId,
		ClientAddr:     request.clientIp,
		AuthId:         request.sessList.authId,
		SessionId:      request.sessionId,
		ReceiveTime:    time.Now().Unix(),
		UserId:         m.AuthUserId,
		ClientMsgId:    request.reqMsgId,
		Layer:          m.Layer(),
		Client:         m.ClientName(),
		Langpack:       m.LangPack(),
		PermAuthKeyId:  m.authKeyId,
		LangCode:       m.LangCode(),
		SystemLangCode: m.SystemLangCode(),
		Takeout:        request.takeout,
	}
```

字段定义：`proto/mtproto/rpc/metadata/rpc_metadata.proto:30`（`string client_addr = 3`）。`doRpcRequest`（`main_auth_wrapper.go:1357`）调用 `BFFProxyClient.InvokeContext`。`app/bff/bff/client/bff_proxy_client.go:182` 用 `RpcMetadataToOutgoing` 把整份元数据 base64 放进 gRPC 头 `rpc_metadata`（`proto/mtproto/rpc/metadata/rpc_metadata_codec.go:31-33`、`73-81`）。BFF 侧 `RpcMetadataFromIncoming` 还原到 `c.MD.ClientAddr`。

因此：

| 用途 | 读的值 | 何时更新 |
|---|---|---|
| 限流、短信/邮箱配额、发现页地区、登录通知 | 当前连接上的 `client_ip` → `client_addr` | 每条从 gnetway 上来的消息 |
| 设备列表里的 IP / 国家 / 地区 | `auths.client_ip` | `initConnection`（以及不把已有 IP 擦掉的 `setLayer`） |

`setLayer` 故意沿用内存里已有的 IP，避免空字符串把设备列表的地址清掉（`main_auth_wrapper.go:434-436`，写入 `auths.go:347-359`）。

### 1.5 同一连接上下文上的另外两条传输

gnetway 的 HTTP 传输使用同一个 `ctx.clientIp`（`app/interface/gnetway/internal/server/gnet/server_http.go:292-308`），经 `session.sendHttpDataToSession`（`session.sendHttpDataToSession_handler.go:45-54`）进入同一套 session 处理。当前 `teamgramd/etc/gnetway.yaml` 和 `teamgramd/etc2/gnetway.yaml` 没有 `Proto: http` 监听。

WebSocket `0.0.0.0:11443` 也走 `OnOpen` 的同一字段。该组 `PPV1` 为 false，所以 IP 是 socket 对端。本次只透传 443/10443，不经过 11443。

`initConnection.proxy` 是客户端自己声明的 MTProto 代理，序列化后放在 `auths.proxy`（`main_auth_wrapper.go:479`、`532-534`）。它不是 TCP 对端，也不能当成已验证的客户端地址。

## 2. 消费点，以及变成中继 IP 之后

「变成中继 IP」指 `client_ip` / `client_addr` / `auths.client_ip` 的值是 `172.19.44.222`。下面按影响排序。搜索范围是本仓库的 Go、SQL 和配置；生成代码里的 getter 不单列。

### 2.1 登录与发码的 IP 限流（会把所有人合成一把钥匙）

session 在调用 BFF 之前做 flood。IP 只参与两类方法。

```12:28:app/interface/session/internal/sess/flood.go
func (c *session) floodDecision(ctx context.Context, clientIP string, query mtproto.TLObject) flood.Decision {
	// ...
	return lim.Allow(ctx, flood.Request{
		Method:  methodName(query),
		UserID:  userID,
		AuthKey: authKey,
		IP:      hostOnly(clientIP),
		Phone:   phoneOf(query),
	})
}
```

调用点：`app/interface/session/internal/sess/session_invoke.go:479-482`。拒绝时返回 `FLOOD_WAIT_x`。

IP 桶只加在发码和登录上（`pkg/flood/limit.go:275-288`）：

| 桶 | 方法 | 默认 | 键 |
|---|---|---|---|
| `send_code_ip` | `auth.sendCode`、`auth.resendCode`、`account.sendChangePhoneCode`、`account.sendConfirmPhoneCode`（`pkg/flood/classify.go:26-28`） | 突发 200，之后每秒 2（`limit.go:77`） | `flood:send_code_ip:ip:<ip>` |
| `sign_in_ip` | `auth.signIn`、`auth.signUp`（`classify.go:29-30`） | 突发 200，之后每秒 2（`limit.go:81`） | `flood:sign_in_ip:ip:<ip>` |

同一次调用还有按手机号的桶（`send_code_phone` 突发 8、`sign_in_phone` 突发 20），以及按用户或 auth key 的全局桶。那些不按 IP。`subjectCharge`（`limit.go:325-332`）在有 user id 时用用户，否则用 auth key，没有 IP 分支。发消息、上传、举报、搜索都不按 IP。

影响：经中继的所有用户共用 `172.19.44.222` 这两把钥匙。突发 200 次之后，全站发码和登录被压到每秒 2 次，和人数无关。手机号桶仍然按号码，所以单个号码的上限还在，但 IP 桶会先把别人挡住。反过来，如果未校验的 PROXY 头可以被任意对端伪造，攻击者换头里的源地址就能绕过 IP 桶；手机号桶绕不过。

gnetway 自己没有按 IP 的连接数或握手配额。指标标签只有 `proto` / `result` / `type`（`app/interface/gnetway/internal/server/gnet/metrics.go:21-77`），没有 IP，不存在高基数标签被中继打爆的问题。不要在实现里加 IP 标签。

### 2.2 短信和邮箱的按日 IP 上限（默认关）

发码配额在 BFF，读 `c.MD.ClientAddr`。

- 登录短信：`app/bff/authorization/internal/core/sms_delivery.go:14-23` 取出 host，`65` 行 `AllowSend(ctx, "sms", phone, ip)`。
- 登录邮箱：`app/bff/authorization/internal/core/email_code.go:90`。
- 更换手机号：`app/bff/account/internal/core/account.sendChangePhoneCode_handler.go:168` 调用 `guardChangePhone`。
- 确认手机号：`account.sendConfirmPhoneCode` → `deliverConfirmSMS` → `guardChangePhone`（`app/bff/account/internal/core/p0_confirm_phone.go:46-47`、`69-87`）。这里的 `confirmClientIP`（`91-96`）不做 `SplitHostPort`。gnetway 已经只存 host，所以和短信路径是同一个字符串。

计数在 `pkg/verify/limit.go:108-111`：`tellomi:code:day:<kind>:ip:<ip>:<上海日期>`。`DailyIP <= 0` 时这行不执行。默认就是 0：

- `TELLOMI_SMS_IP_DAILY_LIMIT`，`pkg/verify/config.go:143`
- `TELLOMI_EMAIL_IP_DAILY_LIMIT`，同文件 `150`

按号码/邮箱的每天 10 次（`config.go:142`、`149`）不看 IP，中继不影响它。重发冷却也是按号码，不是按 IP。

云密码邮箱把 IP 传成空字符串（`app/bff/twofa/password/service.go:114`），本来就不按 IP。

影响：默认配置下，中继不改变短信/邮箱日上限。一旦把上述环境变量设成正数，所有经中继的用户共用一个日计数，先到的人会把后面的人挡掉。未校验的 PROXY 头则可以换 IP 绕过这个上限。

### 2.3 活跃会话：IP、国家、地区

`initConnection` 把当前 `clientIp` 写入 `authsession.setInitConnection` 的 `Ip`（`main_auth_wrapper.go:543-545`）。落库：

```366:379:app/service/authsession/internal/dao/auths.go
func (d *Dao) SetInitConnection(ctx context.Context, i *authsession.TLAuthsessionSetInitConnection) error {
	// ...
				ClientIp:       i.GetIp(),
```

列是 `auths.client_ip varchar(32)`（`teamgramd/deploy/sql/1_teamgram.sql:44`）。读回在 `auths.go:440`。`account.getAuthorizations`（`app/bff/passport/internal/core/account.getAuthorizations_handler.go:32-35`）调用 `GetAuthorizations`，那里用 `cData.ClientIp()` 做地理查询并填进 `authorization.ip` / `country` / `region`（`app/service/authsession/internal/dao/authorization.go:277-302`）。单条 `GetAuthorization` 在同文件 `217-234`。

地理查询：`authorizationPlace`（`authorization.go:80-83`）→ `pkg/geoip`。查不到国家时，`fillAuthorizationPlace`（`authorization.go:72-76`）把国家写成 IP 本身，避免客户端显示 `UNKNOWN`。规则见 `docs/ip2region.md`。

`172.19.44.222` 是私网地址，ip2region / 城市库不会给出「中国 四川」。设备列表会显示 IP `172.19.44.222`，国家退回成这个字符串，地区为空。用户看到的不是自己的公网地址，也不是城市。

存储宽度：`varchar(32)` 放得下 IPv4，放不下不少 IPv6 文本（压缩形式最长 39，带区标识更长）。`auth_op_logs.ip` 已经是 `varchar(45)`（`teamgramd/deploy/sql/migrate-20260309.sql:21`），`auths.client_ip` 没有跟着改。实现若接受 v2 的 TCP6，不能依赖 MySQL 截断；要么同一次改动把列加到 45，要么拒绝写不进去的地址。这是既有列宽，不是中继 IPv4 本身的问题。

### 2.4 登录通知里的位置和 IP

完成 `auth.signIn`（不是 `SESSION_PASSWORD_NEEDED` 提前返回）时，用 `c.MD.ClientAddr` 做两件事（`app/bff/authorization/internal/core/auth.signIn_handler.go:234-264`）：

- `GetCountryAndRegionByIp`（`app/bff/authorization/internal/dao/api.go:31-45`）。库没有结果时，位置行退回成 IP 文本。
- 默认文案 `MakeSignInServiceNotification` 把这个 IP 放进链接（`proto/mtproto/service_notification_helper.go:57-84`）。配置了 `SignInServiceNotification` 时，模板变量 `region` 和 `clientIp` 用的是同一对值（`signIn_handler.go:256-262`）。

777000 的「新设备登录提醒」不带 IP。`loginnotice.Text` 只有设备和北京时间（`app/bff/authorization/loginnotice/notice.go:70-72`）。`pushLoginSecurityNotice`（`app/bff/authorization/internal/core/core.go:164-168`）用于 signIn、signUp、checkPassword、recoverPassword。密码登录这条站内信不会因为中继而印出 `172.19.44.222`；带 IP 的那条服务通知会。

### 2.5 发现页凭证的地区

`discover.getEngineCredential` 把 `client_addr` 交给 `mainland`（`app/bff/discover/internal/server/grpc/service/handlers.go:20-49`）。判断在 `service.go:86-94`：`pkg/geoip.Lookup` 再加 `discoverauth.MainlandPlace`。

地区规则（`pkg/discoverauth/region.go:15-18`）：国家码 `CN`，或号码是 +86，或客户端地址被判成大陆，三者任一即 `rgn=CN`，否则 global。香港、澳门、台湾不算大陆（`region.go:37-50`）。

影响：中继私网地址不会被判成大陆。+86 和账号国家码 `CN` 仍然是大陆。没有这两项的账号（邮箱账号，或国家码不是 CN 但人在大陆）会因为 IP 信号消失而被标成 global。未校验的 PROXY 头则可以伪造一个大陆地址，把 `rgn` 翻过来。

### 2.6 审核、举报、封禁

本仓库没有按 IP 的封禁表，也没有把 TCP 地址写进举报行。

- `app/service/biz/report` 的 `Submission` / `Row` 没有 IP 字段。举报 flood 桶按用户或 auth key（`pkg/flood/classify.go:50-51`，`limit.go:313-316`），不按 IP。
- `pkg/enforcement` 按用户，不读 `client_addr`。
- `CheckCanDoAction` 是空实现，三个参数都丢掉（`app/bff/authorization/internal/dao/phone_code.go:118-128`；account 侧同名函数同样是空的）。
- `OnAuthAction` 的四个调用点会把 `c.MD.ClientAddr` 传进去：`auth.sendCode_handler.go:131`、`auth.resendCode_handler.go:193`、`auth.signIn_handler.go:144`、`auth.signUp_handler.go:64`。接口在 `app/bff/authorization/plugin/plugin.go:27`。本仓库没有这个方法的实现。`LogAuthAction` 整段注释掉了，里面本来会写 `auth_op_logs.ip`（`phone_code.go:130-143`）。

影响：今天的举报记录、封禁和操作日志不会因为中继而改内容，因为它们根本不记这条 IP。设备列表和登录通知才是现在真正留下的地址。以后若把 `OnAuthAction` 接上，它会记下中继地址，除非先做来源校验。

### 2.7 推送、通话、握手

推送：`account.registerDevice` 不读 `ClientAddr`。它落在 flood 的 account 类，按用户或 auth key（`classify.go:45-49`）。设备 token 表没有客户端 IP。

通话：`phoneConnection.ip` 是媒体服务自己的公网地址 `cfg.PublicIP`（`app/bff/phone/internal/engine/tl.go:111-112`），不是 MTProto 对端。SFU 的 `RemoteAddr` 是 UDP 媒体套接字。3479/3480 不在这次 443/10443 透传里。通话信令若走被透传的 MTProto，信令 IP 会变成中继，但下发给客户端的反射地址不会跟着变。

握手：`onHandshake` 不按 IP 计数。限流发生在密钥建立之后的 RPC。中继不会触发一个不存在的「每 IP 握手上限」，也不会把所有握手合成一条连接；每条用户 TCP 仍然是 gnetway 上的一条连接，只是 `RemoteAddr` 相同。

### 2.8 不在这条 MTProto 链上的 HTTP（不要顺手扩大信任）

这三处不读 gnetway 的 `clientIp`。本次 L4 不经过它们。实现 PROXY 时不能让它们开始信任 `X-Forwarded-For`、`X-Real-IP` 或 PROXY 链，也不能把 `172.19.44.222/32` 写进它们的信任列表。

| 入口 | 位置 | 现在怎么取 IP | 和中继的关系 |
|---|---|---|---|
| MTProto-over-HTTP `/api`、`/apiw1` | `app/interface/httpserver/internal/server/http/routes.go:75` | go-zero v1.10.0 `rest/httpx/util.go` 的 `GetRemoteAddr`：有 `X-Forwarded-For` 就整段返回，否则 `RemoteAddr`。没有可信代理名单 | 随后写入 `SessionClientData.ClientIp`（`httpserver.apiw1_handler.go:193`），于是进入第 1 节的同一条 session 链 |
| 网页注销 | `app/bff/accountdeletion/http.go:446-458` | 无条件采用 `X-Forwarded-For` 的第一段，解析成功即采信 | `195-208` 用它做注销验证码配额。直接打到该端口的客户端可以伪造 |
| 链接预览 | `app/interface/linkpreview/server.go:225-241` | 仅当 `TrustProxy` 为真时才信 `CF-Connecting-IP` 和 `X-Forwarded-For` 第一段。开关是 `TELLOMI_LINKPREVIEW_TRUST_PROXY`（`config.go:179-180`） | 只用于本服务的每 IP 频率（`server.go:67`、`103`），不进 `auths.client_ip` |

`/apiw1` 是唯一会把 HTTP 头带进 `client_ip` 的口。它今天已经信任任意 `X-Forwarded-For`。修 gnetway 时不要复制这个做法。

## 3. gnetway 的 PROXY v1 / v2 现状

### 3.1 配置

```30:33:app/interface/gnetway/internal/config/config.go
type GnetwayServer struct {
	Proto     string `json:",default=tcp,options=tcp|websocket|http"`
	PPV1      bool   `json:",optional"`
	Addresses []string
}
```

没有 v2 开关，没有可信 CIDR，没有「缺头是否拒绝」。`IsProxyProtocolV1`（`config.go:94-105`）是监听地址字符串全等，不是按端口，也不是网段。`0.0.0.0:10444` 匹配，`127.0.0.1:10444` 不匹配。原因写在 `config.go:83-93`：gnet 把配置里的监听地址复制到已接受连接的 `LocalAddr`，不是 `getsockname`。

生产配置（`teamgramd/etc/gnetway.yaml:42-47`，`teamgramd/etc2/gnetway.yaml` 同样）：

- `0.0.0.0:10443`、`0.0.0.0:5222`：TCP，`PPV1` 关
- `0.0.0.0:11443`：WebSocket，`PPV1` 关
- `0.0.0.0:10444`：TCP，`PPV1: true`

`docker-compose.yaml:361-364` 只把 10444 发布到宿主机 `127.0.0.1`。说明在 `docs/ops/mtproto-443.md:47-51`：头是可选的；没有来源 CIDR；安全依赖「只有本机 nginx 能连上」；公网 10443 不解析 PROXY。

### 3.2 库和接入不是一回事

| 能力 | 在哪 | 接入是否使用 |
|---|---|---|
| v1 `PROXY TCP4/TCP6/UNKNOWN` | `pp/v1.go`，测试 `pp/v1_test.go` | 使用。仅 PPV1 监听且前缀为 `PROXY ` |
| v2 签名、版本、LOCAL/PROXY、IPv4/IPv6、TCP/UDP、2048 字节上限 | `pp/v2.go:28-59`、`73-136`；`ReadHeader` 在 `pp/proxy.go:75-80` | 不使用。`OnTraffic` 不认 v2 签名 |
| UNIX | `pp/v2.go:127-129` 直接返回错误 | 不使用 |
| TLV | `pp/proxy.go:44-64` 能解析，`Header.RawTLVs` | 接入层不读 TLV，也不允许它改地址 |
| 来源是否可信 | 无 | 无 |

v2 的 UDP（family `0x12` / `0x22`）会被库解析成 `UDPAddr`（`pp/v2.go:99-101`）。即便接上 v2，也不能只因为 `Source != nil` 就采信；必须限定命令是 PROXY、传输是 TCP。

`pp/c2/c2.go` 是一个手工 hex 的 v1 小程序，不是服务路径。

现有行为还有两点实现时要避开：

- 头可选。10444 上没有 `PROXY ` 前缀就退回普通 MTProto（`server_gnet.go:268-270`）。中继监听不能沿用「没有头也放行」。
- 短读会把 peek 到的字节打成 hex（`server_gnet.go:234`）。`ReadHeader` 在签名不匹配时也会 `hex.Dump` 前 14 字节（`pp/proxy.go:91`）。10444 上紧跟头的是混淆传输的临时密钥。新路径只记对端、本端和原因码，不记原始字节。现有 v1 成功日志只打解析出的地址（`server_gnet.go:249-261`），这一点保持。

e2e 已经固定了两条边界（`e2e/client/mtproto443_test.go:81-100`）：10444 记录 v1 里的源地址；发到 10443 的 `PROXY ` 头不会被记成 `client_ip`，握手也不会把它当成合法传输。实现不得改掉 10443 这条。

## 4. 最小服务器改动设计

只改 gnetway 的接受路径和配置。session、BFF、`auths.client_ip`、`RpcMetadata.client_addr` 的字段名和含义不变。下游继续只看到一个已验证的客户端地址。

### 4.1 监听

新增一组 TCP，默认不出现在配置里，因此默认关闭。不要改 10443、5222、11443 的 `PPV1: false`。不要把 `172.19.44.222` 加进现有 10444：10444 仍是「头可选、无 CIDR、只靠 loopback 发布」的 nginx 口。

新监听绑定 ECS 私网地址，不发布到 `0.0.0.0`，也不新增公网端口。示例：

```yaml
Gnetway:
  Server:
    - Proto: tcp
      ProxyProtocol: v2
      RequireProxyHeader: true
      TrustedProxyCIDRs:
        - 172.19.44.222/32
      Addresses:
        - 10.20.0.218:10445
```

地址只是形状。真正绑定哪张网卡、容器里看到的对端是不是 `172.19.44.222`，要在部署时用一次连接确认。若 Docker userland-proxy 或 SNAT 把对端变成网桥地址，/32 对不上，连接会被拒绝。这是预期。不要为此把 `172.16.0.0/12` 或 Docker 网段写进白名单，先改转发方式。

### 4.2 配置键

加在现有 `GnetwayServer` 上，与 `PPV1` 并列。

| 键 | 含义 |
|---|---|
| `ProxyProtocol` | `""`（默认）、`v1`、`v2`、`v1v2`。空字符串等于今天：不进入新状态机。`PPV1: true` 继续只表示旧的可选 v1，供 10444 使用 |
| `TrustedProxyCIDRs` | 字符串列表。每一项必须是 IPv4 或 IPv6 的 /32 或 /128。启动时拒绝空列表、`0.0.0.0/0`、`172.16.0.0/12`、以及任何更宽的前缀 |
| `RequireProxyHeader` | 布尔，`ProxyProtocol` 非空时默认 `true`。`true`：可信对端在时限内没有合法头就关闭。`false`：可信对端没有头时，`client_ip` 保持 socket 对端。中继这组必须是 `true` |

启动失败条件：`ProxyProtocol` 非空但 `TrustedProxyCIDRs` 为空或含非单地址前缀；`ProxyProtocol` 与 `PPV1: true` 写在同一组（两套语义不能叠在一个监听上）。

匹配前把 IPv4-mapped IPv6（`::ffff:172.19.44.222`）还原成 IPv4，再和 `172.19.44.222/32` 比较。比的是 socket 对端，不是头里自称的源。

### 4.3 接受规则

只在 `ProxyProtocol` 非空的监听上进入。公网直连监听一个字节都不解释成 PROXY。

1. `OnOpen` 记下 socket 对端，此时不要把对端写进将要传给 session 的最终值。对端不在 `TrustedProxyCIDRs`：直接关闭，不解析后面的字节，不把任何头里的地址写入 `clientIp`。这样「其它来源带着 PROXY 头」和「其它来源不带头」都是拒绝，而不是退回成普通 MTProto。
2. 对端在名单里：在独立的短时限内增量读一条头。时限不要复用已握手连接的 300 秒空闲。头和紧随的 MTProto 可以在同一段缓冲里，读完头之后的字节必须原样交给现有 codec，不能丢、不能再扫第二遍。
3. 允许的头：
   - v1：`PROXY TCP4` 或 `PROXY TCP6`，且 `ProxyProtocol` 含 v1
   - v2：版本高 4 位为 `0x2`，命令低 4 位为 `0x1`（PROXY），family 为 `0x11`（TCP/IPv4）或 `0x21`（TCP/IPv6），且 `ProxyProtocol` 含 v2
4. 拒绝并关闭：缺头且 `RequireProxyHeader` 为真；v1 `UNKNOWN`；v2 `LOCAL`（命令 `0x0`）；UDP（`0x12`、`0x22`）；UNIX；UNSPEC；版本不对；长度大于现有库的 2048（`pp/v2.go:57-59`）；超时；截断；第二条 PROXY 头；用 TLV 改变地址。TLV 可以留在库的 `RawTLVs` 里，身份只取地址字段。
5. 通过之后，`setClientIp` 一次，值为头里源地址的 host（沿用 `SplitHostPort`，不再用按 `:` 切开的旧写法，否则 IPv6 会被切碎）。此后 `pp` 状态清掉，剩余字节进入现有 `onTcpData`。日志只打本端地址、socket 对端、已验证地址和原因码。
6. `RequireProxyHeader: false` 且对端可信、缓冲里没有 v1/v2 签名：`client_ip` 用 socket 对端，供将来某条内网直连使用。中继配置不走这个分支。对端不可信时没有这个退路。

旧的 `PPV1: true` 路径先留着，避免这次把 nginx 的 443 明文转发一起改掉。它仍然没有 CIDR，所以仍然只能绑在 loopback 发布上。新代码不要让 `IsProxyProtocolV1` 开始认 v2，以免 10444 意外接受 v2。

下游不用改。已验证地址继续走第 1 节的 `client_ip` → `client_addr` → `auths.client_ip`。socket 对端只留在 gnetway 的拒绝/接受日志里，不新增 BFF 字段，也不写进 `auths`。

### 4.4 要补的测试

库级（可直接喂 `bytes.Reader`，不启动 gnet）：

- 可信 /32，v1 与 v2，TCP4 与 TCP6：`client_ip` 等于头内源地址，不等于 `172.19.44.222`
- 非可信对端发送 v1 或 v2：连接被拒绝，`client_ip` 不被头覆盖
- 可信对端无头，`RequireProxyHeader true`：拒绝；设为 false：`client_ip` 等于对端
- `LOCAL`、`UNKNOWN`、UDP、UNIX、坏版本、超长、TLV 里另写一个地址：拒绝，身份不变
- 头拆成单字节到达；头和后续 MTProto 粘在一起：头被消费，后续字节一个不丢
- 头后面再放一条 PROXY：第二条不当成新的身份
- 配置里出现 `/12`、空 CIDR、`ProxyProtocol` 与 `PPV1` 同组：启动失败
- `::ffff:172.19.44.222` 能匹配 `172.19.44.222/32`

接入级：现有 `e2e/client/mtproto443_test.go` 的 10443「不信任 PROXY 头」保持。新监听另加一条：可信对端的 v2 头之后能完成 `resPQ`，且日志里的已验证地址是头内地址。flood 单测确认 `send_code_ip` 的键是已验证地址，不是中继对端。

不在本设计里：改 `dc_options`、把 10443 公网口打开 PROXY、信任整个 VPC、信任 HTTP 的 `X-Forwarded-For`。

## 5. 参照的原版设计

Telegram 的会话地址不是客户端在 `initConnection` 里上报的。官方 `authorization`（`account.getAuthorizations` 返回的每一项）包含 `ip`、`country`、`region`，表示数据中心看到的那条连接；`country` / `region` 由服务端地理库填写。`initConnection` 携带的是设备、语言、层，以及可选的 `proxy`（客户端自己配置的 MTProto 代理）。这个 `proxy` 和 TCP 对端是两件事。Teamgram 把前者放进 `auths.proxy`，把观察到的地址放进 `auths.client_ip`，再由 `account.getAuthorizations` 填回 `ip` / `country` / `region`。登录服务通知里的 IP 和位置同样来自这条观察到的地址，而不是来自客户端字段。

原版 teamgram 的网关只有这一个字符串。`OnOpen` 取 TCP `RemoteAddr` 的 host（早期按第一个 `:` 切开，后来改成 `SplitHostPort`，避免切坏 IPv6），放进连接上下文，加密帧里作为 `SessionClientData.client_ip` 交给 session。`initConnection` 调用 `authsession.setInitConnection` 写入 `auths.client_ip`。`setLayer` 必须带上已有 IP，否则 upsert 会把地址写成空。本仓库 `main_auth_wrapper.go:434-436` 的注释就是这条约束。关闭连接不更新这列。

上游后来加了可选的 PROXY v1：监听配置 `PPV1`，并且缓冲以 `PROXY ` 开头时，用头内源地址替换那同一个字符串。没有来源网段，头也可以不带。Tellomi 的 `0.0.0.0:10444` 就是这个机制，只发布在 `127.0.0.1`，安全边界写在 `docs/ops/mtproto-443.md`。`pp` 目录能解析 v2，是库的能力；接受循环只比较 v1 前缀，所以 v2 今天不是协议的一部分。

本次设计沿用这个单字符串合同：session 和 BFF 仍然只收一个已验证的客户端地址，字段名不增加。新增的只是网关在写入该字段之前的准入——对端必须是配置的 /32，且正好一条 v1 或 v2 的 TCP 头。公网直连继续「socket 对端即客户端」，与原版一致。不引入可被客户端伪造的 `X-Forwarded-For` 链，也不把中继对端和用户地址在各层来回覆盖。

## 6. 本审计没有做的事

没有改代码，没有打开新端口，没有把 `172.19.44.222` 配进任何监听。没有核实生产进程里 `TELLOMI_SMS_IP_DAILY_LIMIT` 的实际值；代码默认是 0。没有在运行中的 Docker 里测量回源后的 socket 对端，第 4.1 节的对端确认仍要在实现 PR 的部署说明里做。
