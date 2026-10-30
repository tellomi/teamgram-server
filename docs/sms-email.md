# SMS and email verification

SMS for `auth.sendCode` / `auth.resendCode` can use Aliyun Dysmsapi. Email OTP uses the same RPCs when `phone_number` contains `@`. See [email-signup.md](email-signup.md).

Phone behavior does not change when email is enabled. The development phone code stays **12345**. The email code is a different value: the one that was logged or sent.

## SMS

`TELLOMI_SMS_PROVIDER` selects the sender behind `code.VerifyCodeInterface`:

| Value | Behavior |
| --- | --- |
| unset, `fixed`, or `none` | Development mode. Every non-test phone is accepted with `Code.FixedCode` (default **12345**). Nothing is sent. |
| `aliyun` | Random numeric code, Aliyun `SendSms`, checked against the code stored in redis. |

Test numbers (`99966XYYYY`, or `TestPhones` in bff config) keep their fixed code (`11111` when the datacenter digit is 1) and never call the SMS API.

Aliyun mode:

- Code length is `TELLOMI_SMS_CODE_LENGTH` (5 or 6, default 5 in fixed mode, 6 when the value is unset and Aliyun is on). `auth.sentCodeTypeSms` reports that length.
- The code is stored on the existing phone-code redis record until `TELLOMI_SMS_CODE_TTL_SECONDS` (default 180). Wrong checks stop after `TELLOMI_SMS_MAX_ATTEMPTS` (default 5), then `FLOOD_WAIT_900`.
- Resend cooldown is `TELLOMI_SMS_RESEND_SECONDS` (default 60) per phone. Daily caps are `TELLOMI_SMS_DAILY_LIMIT` (default 10) per phone and `TELLOMI_SMS_IP_DAILY_LIMIT` (default 30) per client IP. Over the daily cap the RPC returns `PHONE_NUMBER_FLOOD`; inside the cooldown it returns `FLOOD_WAIT_<seconds>`.
- Only `+86` mobiles are sent, as the 11-digit national number. Any other country returns `PHONE_NUMBER_INVALID` unless `TELLOMI_SMS_ALLOW_INTERNATIONAL=true` (Aliyun uses the same `SendSms` API with country code + number).
- Template parameter JSON is `{"code":"<digits>"}`. The production template text is `您的验证码为：${code}，请勿泄露于他人！`.
- A provider or network failure returns `SMS_CODE_CREATE_FAILED`. Missing access keys fail the same way; they do not stop process startup.

Credentials use the standard Aliyun variables. Do not put secrets in git or in yaml.

## Email

`TELLOMI_EMAIL_PROVIDER` selects `verify.CodeSender`:

| Value | Behavior |
| --- | --- |
| unset or `log` | Default for dev and staging. A random code is stored and printed: `tellomi email verification code to=<email> code=<digits>`. Nothing is delivered to a mailbox. |
| `resend` | Production OTP path. The same code is sent with the Resend HTTP API. |
| `postal` | Legacy optional sender. The same code is sent with Postal's HTTP API when a self-hosted Postal server is up. |
| `disabled`, `none`, `off` | Email `auth.sendCode` returns `EMAIL_NOT_SETUP`. Phone SMS is unchanged. |
| anything else, including `aliyun_dm` | Treated as disabled. Aliyun DirectMail is not the P0 sender. |

The code length is `TELLOMI_EMAIL_CODE_LENGTH` (4 through 8, default 6). `auth.sentCodeTypeEmailCode` reports that length and a masked `email_pattern`. Sign-in compares the submitted `phone_code` or `emailVerificationCode` to the stored code. Quotas use a separate redis prefix from SMS, so an email send does not consume the SMS daily cap.

Production mail is Resend over HTTPS. SMTP is not used. Outbound TCP 25 and 587 stay unused. The process starts without a Resend API key; the key is required only when a send runs.

### Resend

`POST https://api.resend.com/emails`

| Header | Value |
| --- | --- |
| `Content-Type` | `application/json` |
| `Authorization` | `Bearer <TELLOMI_RESEND_API_KEY>` |

JSON body: `from` (`Tellomi <noreply@tellomi.app>` by default), `to` (one address), `subject` `Tellomi 验证码 / Verification code`, `text`:

```text
您的验证码为：<code>，请勿泄露于他人！
Your verification code is <code>. Do not share it.
```

`TELLOMI_RESEND_API_KEY` is read from the environment only. It is not stored in the repo. If the key is empty, startup still succeeds and `auth.sendCode` returns `SMS_CODE_CREATE_FAILED`. The from domain must be verified in Resend. `TELLOMI_EMAIL_FROM` and `TELLOMI_EMAIL_FROM_NAME` set the From header.

### Postal (legacy)

Postal remains available as an optional sender. It is not the production OTP path. Delivery is HTTP, not SMTP, and the Postal host is not required for this process to start.

`POST {TELLOMI_POSTAL_BASE_URL}/api/v1/send/message`

| Header | Value |
| --- | --- |
| `Content-Type` | `application/json` |
| `X-Server-API-Key` | `TELLOMI_POSTAL_API_KEY` |

JSON body: `to` (one address), `from` (`Tellomi <noreply@tellomi.app>` by default), `sender` (the bare address), `subject` `Tellomi 验证码`, `plain_body` `您的验证码为：<code>，请勿泄露于他人！`.

`TELLOMI_POSTAL_BASE_URL` is the origin, for example `https://postal.internal`. A value that already ends in `/api/v1/send/message` is used as-is. If the URL or API key is empty, startup still succeeds and `auth.sendCode` returns `SMS_CODE_CREATE_FAILED`. Use `resend` for production OTP. Leave the provider on `log` for local development.

## Environment variables

| Variable | Default | Purpose |
| --- | --- | --- |
| `TELLOMI_SMS_PROVIDER` | `fixed` | `fixed` or `aliyun` |
| `ALIBABA_CLOUD_ACCESS_KEY_ID` | empty | Aliyun access key id (SMS only) |
| `ALIBABA_CLOUD_ACCESS_KEY_SECRET` | empty | Aliyun access key secret (SMS only) |
| `TELLOMI_ALIYUN_REGION` | `cn-hangzhou` | Dysmsapi `RegionId` |
| `TELLOMI_ALIYUN_SMS_SIGN_NAME` | `重庆半格智能` | Sms sign name |
| `TELLOMI_ALIYUN_SMS_TEMPLATE_CODE` | `SMS_338570180` | Sms template code |
| `TELLOMI_ALIYUN_SMS_ENDPOINT` | `https://dysmsapi.aliyuncs.com/` | Optional endpoint override |
| `TELLOMI_SMS_ALLOW_INTERNATIONAL` | `false` | Allow non-`+86` numbers |
| `TELLOMI_SMS_CODE_LENGTH` | `6` when Aliyun and unset; fixed mode stays 5 | `4`–`8` |
| `TELLOMI_SMS_RESEND_SECONDS` | `60` | Per-phone resend cooldown |
| `TELLOMI_SMS_DAILY_LIMIT` | `10` | Per-phone daily cap |
| `TELLOMI_SMS_IP_DAILY_LIMIT` | `30` | Per-IP daily cap for SMS |
| `TELLOMI_SMS_MAX_ATTEMPTS` | `5` | Wrong SMS code attempts |
| `TELLOMI_SMS_CODE_TTL_SECONDS` | `180` | Redis TTL / SMS code expiry |
| `TELLOMI_EMAIL_PROVIDER` | `log` | `log`, `resend`, `postal`, or `disabled` |
| `TELLOMI_EMAIL_FROM` | `noreply@tellomi.app` | From address |
| `TELLOMI_EMAIL_FROM_NAME` | `Tellomi` | Display name |
| `TELLOMI_RESEND_API_KEY` | empty | Resend API key. Required only when provider is `resend` |
| `TELLOMI_EMAIL_CODE_LENGTH` | `6` | `4`–`8` |
| `TELLOMI_EMAIL_RESEND_SECONDS` | `60` | Per-address resend cooldown |
| `TELLOMI_EMAIL_DAILY_LIMIT` | `10` | Per-address daily cap |
| `TELLOMI_EMAIL_IP_DAILY_LIMIT` | `30` | Per-IP daily cap for email |
| `TELLOMI_EMAIL_MAX_ATTEMPTS` | `5` | Wrong email code attempts |
| `TELLOMI_EMAIL_CODE_TTL_SECONDS` | `180` | Email code expiry |
| `TELLOMI_POSTAL_BASE_URL` | empty | Legacy Postal HTTP origin. Required only when provider is `postal` |
| `TELLOMI_POSTAL_API_KEY` | empty | Legacy Postal `X-Server-API-Key`. Required only when provider is `postal` |
| `TELLOMI_EMAIL_SETUP_REQUIRED` | `false` | Read at startup. Not used by the P0 login RPCs |

`Code.Name: none` in `teamgramd/etc/bff.yaml` is the yaml fallback for SMS. `TELLOMI_SMS_PROVIDER` overrides it when set to `aliyun`. Email does not use that yaml block.

`TELLOMI_ALIYUN_DM_*` is not a P0 email path. Do not set DirectMail for sign-up.

## docker-compose

`docker-compose.yaml` passes the variables into the `teamgram` service. Export secrets in the shell or a `.env` file that is not committed. The email provider defaults to `log`, so a stack without Resend still issues email codes.

```yaml
environment:
  TELLOMI_SMS_PROVIDER: ${TELLOMI_SMS_PROVIDER:-}
  ALIBABA_CLOUD_ACCESS_KEY_ID: ${ALIBABA_CLOUD_ACCESS_KEY_ID:-}
  ALIBABA_CLOUD_ACCESS_KEY_SECRET: ${ALIBABA_CLOUD_ACCESS_KEY_SECRET:-}
  TELLOMI_ALIYUN_REGION: ${TELLOMI_ALIYUN_REGION:-cn-hangzhou}
  TELLOMI_ALIYUN_SMS_SIGN_NAME: ${TELLOMI_ALIYUN_SMS_SIGN_NAME:-}
  TELLOMI_ALIYUN_SMS_TEMPLATE_CODE: ${TELLOMI_ALIYUN_SMS_TEMPLATE_CODE:-}
  TELLOMI_SMS_ALLOW_INTERNATIONAL: ${TELLOMI_SMS_ALLOW_INTERNATIONAL:-false}
  TELLOMI_EMAIL_PROVIDER: ${TELLOMI_EMAIL_PROVIDER:-log}
  TELLOMI_EMAIL_FROM: ${TELLOMI_EMAIL_FROM:-noreply@tellomi.app}
  TELLOMI_EMAIL_FROM_NAME: ${TELLOMI_EMAIL_FROM_NAME:-Tellomi}
  TELLOMI_RESEND_API_KEY: ${TELLOMI_RESEND_API_KEY:-}
  TELLOMI_POSTAL_BASE_URL: ${TELLOMI_POSTAL_BASE_URL:-}
  TELLOMI_POSTAL_API_KEY: ${TELLOMI_POSTAL_API_KEY:-}
```

An empty `TELLOMI_SMS_PROVIDER` keeps the fixed phone code `12345`. An empty `TELLOMI_EMAIL_PROVIDER` keeps email on `log`.

## Checks

Log mode (no Resend key and no Postal host):

- `auth.sendCode` for an email returns `sentCodeTypeEmailCode` and the log line above.
- `auth.signIn` accepts that code, not `12345`.
- A new address then `auth.signUp` inserts `users.email` and a NULL `users.phone`.
- `auth.sendCode` for `+86` still uses the SMS provider rules.

Resend (production OTP):

- `TELLOMI_EMAIL_PROVIDER=resend`
- `TELLOMI_RESEND_API_KEY=<api key>`
- `TELLOMI_EMAIL_FROM=noreply@tellomi.app`
- `TELLOMI_EMAIL_FROM_NAME=Tellomi`
- The same RPCs. Mail is the Resend HTTP call. See [Verify with Resend](email-signup.md#verify-with-resend).

Postal (legacy, when that server exists):

- `TELLOMI_EMAIL_PROVIDER=postal`
- `TELLOMI_POSTAL_BASE_URL=https://<postal-host>`
- `TELLOMI_POSTAL_API_KEY=<server api key>`
- The same RPCs. Mail is the Postal HTTP call, not only a log line.
