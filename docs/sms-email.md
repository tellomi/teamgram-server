# SMS and email verification

SMS for `auth.sendCode` / `auth.resendCode` can use Aliyun Dysmsapi. Email delivery is not wired into authorization RPCs yet. The email sender interface exists so it can be turned on later; with `TELLOMI_EMAIL_PROVIDER` unset it stays disabled and the process starts with no DirectMail settings.

See [email-signup.md](email-signup.md) for the email-only account design. That design is not implemented.

## SMS

`TELLOMI_SMS_PROVIDER` selects the sender behind `code.VerifyCodeInterface`:

| Value | Behavior |
| --- | --- |
| unset, `fixed`, or `none` | Development mode. Every non-test phone is accepted with `Code.FixedCode` (default **12345**). Nothing is sent. |
| `aliyun` | Random numeric code, Aliyun `SendSms`, checked against the code stored in redis. |

Test numbers (`99966XYYYY`, or `TestPhones` in bff config) keep their fixed code (`11111` when the datacenter digit is 1) and never call the SMS API.

Aliyun mode:

- Code length is `TELLOMI_SMS_CODE_LENGTH` (5 or 6, default 5). `auth.sentCodeTypeSms` reports that length.
- The code is stored on the existing phone-code redis record until `TELLOMI_SMS_CODE_TTL_SECONDS` (default 180). Wrong checks stop after `TELLOMI_SMS_MAX_ATTEMPTS` (default 5), then `FLOOD_WAIT_900`.
- Resend cooldown is `TELLOMI_SMS_RESEND_SECONDS` (default 60) per phone. Daily caps are `TELLOMI_SMS_DAILY_LIMIT` (default 10) per phone and `TELLOMI_SMS_IP_DAILY_LIMIT` (default 30) per client IP. Over the daily cap the RPC returns `PHONE_NUMBER_FLOOD`; inside the cooldown it returns `FLOOD_WAIT_<seconds>`.
- Only `+86` mobiles are sent, as the 11-digit national number. Any other country returns `PHONE_NUMBER_INVALID` unless `TELLOMI_SMS_ALLOW_INTERNATIONAL=true` (Aliyun uses the same `SendSms` API with country code + number).
- Template parameter JSON is `{"code":"<digits>"}`. The production template text is `您的验证码为：${code}，请勿泄露于他人！`.
- A provider or network failure returns `SMS_CODE_CREATE_FAILED`. Missing access keys fail the same way; they do not stop process startup.

Credentials use the standard Aliyun variables. Do not put secrets in git or in yaml.

## Environment variables

| Variable | Default | Purpose |
| --- | --- | --- |
| `TELLOMI_SMS_PROVIDER` | `fixed` | `fixed` or `aliyun` |
| `ALIBABA_CLOUD_ACCESS_KEY_ID` | empty | Aliyun access key id |
| `ALIBABA_CLOUD_ACCESS_KEY_SECRET` | empty | Aliyun access key secret |
| `TELLOMI_ALIYUN_REGION` | `cn-hangzhou` | Dysmsapi `RegionId` |
| `TELLOMI_ALIYUN_SMS_SIGN_NAME` | `重庆半格智能` | Sms sign name |
| `TELLOMI_ALIYUN_SMS_TEMPLATE_CODE` | `SMS_338570180` | Sms template code |
| `TELLOMI_ALIYUN_SMS_ENDPOINT` | `https://dysmsapi.aliyuncs.com/` | Optional endpoint override |
| `TELLOMI_SMS_ALLOW_INTERNATIONAL` | `false` | Allow non-`+86` numbers |
| `TELLOMI_SMS_CODE_LENGTH` | `5` | `5` or `6` |
| `TELLOMI_SMS_RESEND_SECONDS` | `60` | Per-phone resend cooldown |
| `TELLOMI_SMS_DAILY_LIMIT` | `10` | Per-phone daily cap |
| `TELLOMI_SMS_IP_DAILY_LIMIT` | `30` | Per-IP daily cap |
| `TELLOMI_SMS_MAX_ATTEMPTS` | `5` | Wrong-code attempts |
| `TELLOMI_SMS_CODE_TTL_SECONDS` | `180` | Redis TTL / code expiry |
| `TELLOMI_EMAIL_PROVIDER` | unset (disabled) | `log` prints a code; `aliyun_dm` would use DirectMail. Auth RPCs do not send email yet. |
| `TELLOMI_ALIYUN_DM_ACCOUNT_NAME` | `noreply@mail.tellomi.cn` | DirectMail `AccountName`, unused until email auth is wired |
| `TELLOMI_ALIYUN_DM_FROM_ALIAS` | `Tellomi` | DirectMail `FromAlias` |
| `TELLOMI_ALIYUN_DM_ENDPOINT` | `https://dm.aliyuncs.com/` | DirectMail endpoint. Region is always `cn-hangzhou`. API action `SingleSendMail`. |
| `TELLOMI_EMAIL_SETUP_REQUIRED` | `false` | Reserved. Not read by auth RPCs. |

`Code.Name: none` in `teamgramd/etc/bff.yaml` is the yaml fallback. `TELLOMI_SMS_PROVIDER` overrides it when set to `aliyun`.

## docker-compose

`docker-compose.yaml` passes the variables into the `teamgram` service. Export them in the shell or a `.env` file that is not committed. Leave `TELLOMI_EMAIL_PROVIDER` unset.

```yaml
environment:
  TELLOMI_SMS_PROVIDER: ${TELLOMI_SMS_PROVIDER:-}
  ALIBABA_CLOUD_ACCESS_KEY_ID: ${ALIBABA_CLOUD_ACCESS_KEY_ID:-}
  ALIBABA_CLOUD_ACCESS_KEY_SECRET: ${ALIBABA_CLOUD_ACCESS_KEY_SECRET:-}
  TELLOMI_ALIYUN_REGION: ${TELLOMI_ALIYUN_REGION:-cn-hangzhou}
  TELLOMI_ALIYUN_SMS_SIGN_NAME: ${TELLOMI_ALIYUN_SMS_SIGN_NAME:-}
  TELLOMI_ALIYUN_SMS_TEMPLATE_CODE: ${TELLOMI_ALIYUN_SMS_TEMPLATE_CODE:-}
  TELLOMI_SMS_ALLOW_INTERNATIONAL: ${TELLOMI_SMS_ALLOW_INTERNATIONAL:-false}
  TELLOMI_EMAIL_PROVIDER: ${TELLOMI_EMAIL_PROVIDER:-}
  TELLOMI_ALIYUN_DM_ACCOUNT_NAME: ${TELLOMI_ALIYUN_DM_ACCOUNT_NAME:-}
  TELLOMI_ALIYUN_DM_FROM_ALIAS: ${TELLOMI_ALIYUN_DM_FROM_ALIAS:-}
```

An empty `TELLOMI_SMS_PROVIDER` keeps the fixed code `12345`. An empty `TELLOMI_EMAIL_PROVIDER` disables email.
