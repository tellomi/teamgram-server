# Email sign-up and login

Email OTP is implemented on the existing phone RPCs. The client puts the email address in `phone_number`. There is no mandatory password. `account.sendVerifyEmailCode` and `account.verifyEmail` stay on the enterprise-blocked error; P0 does not need them.

Phone SMS is unchanged. See [sms-email.md](sms-email.md).

## Flow

1. `auth.sendCode` with the email in `phone_number`.
2. The server checks the address, stores a numeric code the same way it stores an SMS code, and sends it.
3. The reply is `auth.sentCode` with `auth.sentCodeTypeEmailCode`: masked `email_pattern` (for example `u***@tellomi.app`), `length`, and no Apple or Google sign-in flag.
4. `auth.signIn` with the same `phone_number`, `phone_code_hash`, and either `phone_code` or `emailVerificationCode`.
5. An address that is not registered returns `auth.authorizationSignUpRequired` (layer 104 and later). No cloud password is required.
6. `auth.signUp` with that `phone_number`, `phone_code_hash`, and `first_name` (the nickname) creates the account.
7. A later `auth.sendCode` / `auth.signIn` for the same address returns `auth.authorization`.

`auth.resendCode` sends the same kind of email again, subject to the email resend cooldown.

Apple and Google `EmailVerification` tokens are rejected with `EMAIL_INVALID`.

## Accounts

`users.email` is `varchar(255) NULL` with a unique index. `users.phone` is nullable. An email-only row stores the normalized address and SQL `NULL` for phone, so it does not collide with the unique phone index (InnoDB allows many NULLs). Phone accounts keep a unique phone and a NULL email.

Lookup and create go through the existing user RPCs. `user.getImmutableUserByPhone` and `user.createNewUser` treat a value containing `@` as an email. Deleting an account sets `email` to NULL so the address can be registered again.

Migration: `teamgramd/deploy/sql/migrate-20261030-email.sql`. A fresh `1_teamgram.sql` already has the columns.

## Delivery

`TELLOMI_EMAIL_PROVIDER` defaults to `log`. The process logs:

```text
tellomi email verification code to=user@tellomi.app code=012345
```

That logged code is what `auth.signIn` checks. It is not the phone development code `12345`.

Production OTP uses Resend. Set `TELLOMI_EMAIL_PROVIDER=resend` and `TELLOMI_RESEND_API_KEY`. The sender calls `POST https://api.resend.com/emails` with `Authorization: Bearer <key>`. The key is an environment variable only; it is not in this repository. An empty key does not stop startup. `auth.sendCode` returns `SMS_CODE_CREATE_FAILED` until the key is set. From address defaults to `noreply@tellomi.app` with display name `Tellomi`. The subject is `Tellomi 验证码 / Verification code`. The body keeps the Postal Chinese line and adds a short English line. Both lines include the code:

```text
您的验证码为：012345，请勿泄露于他人！
Your verification code is 012345. Do not share it.
```

`postal` is a legacy optional sender. It calls a self-hosted Postal HTTP API (`POST {TELLOMI_POSTAL_BASE_URL}/api/v1/send/message`, header `X-Server-API-Key`). SMTP is not used. An empty base URL or API key does not stop startup; `auth.sendCode` returns `SMS_CODE_CREATE_FAILED` until both are set. New production mail should use `resend`.

`disabled`, `none`, or `off` makes email `auth.sendCode` return `EMAIL_NOT_SETUP`. Aliyun DirectMail is not a sender for this path.

## Limits

Email quotas are separate from SMS (`kind=email` in redis). Over the daily cap the RPC returns `PHONE_NUMBER_FLOOD`. Inside the email code resend cooldown it returns `FLOOD_WAIT_<seconds>`. Wrong codes use `TELLOMI_EMAIL_MAX_ATTEMPTS`, then `FLOOD_WAIT_900`.

## Errors

| Code | When |
| --- | --- |
| `EMAIL_INVALID` | Address fails the mailbox check, or the client sent an Apple/Google token |
| `EMAIL_NOT_SETUP` | Email provider is disabled |
| `PHONE_NUMBER_FLOOD` | Daily email cap |
| `FLOOD_WAIT_X` | Email code resend cooldown or too many wrong codes |
| `PHONE_NUMBER_OCCUPIED` | Email already registered |
| `PHONE_NUMBER_UNOCCUPIED` | Sign-in on an old layer before sign-up |
| `SMS_CODE_CREATE_FAILED` | Resend or Postal (or the sender) failed, including a missing API key. The log has the provider error. Startup is unchanged |
| `PHONE_CODE_INVALID` / `PHONE_CODE_EXPIRED` / `PHONE_CODE_EMPTY` | Same as SMS |

## Client work (separate repos)

Unmodified clients only collect a phone number before `auth.sendCode`. They already render `sentCodeTypeEmailCode` and can submit `emailVerificationCode` on `auth.signIn` once the server returns that type.

Email-only sign-up still needs a client entry that:

- calls `auth.sendCode` with the email in `phone_number`
- shows the nickname screen from `authorizationSignUpRequired`
- calls `auth.signUp` with the email where it currently sends the phone, and does not require a SIM number

Those client edits are out of scope for this server change.

## Verify without Postal

1. Leave `TELLOMI_EMAIL_PROVIDER` unset, or set it to `log`. Do not set Postal variables.
2. `auth.sendCode` with `phone_number=user@example.com`. Expect `auth.sentCodeTypeEmailCode`, pattern `u***@example.com`, length 6 (unless `TELLOMI_EMAIL_CODE_LENGTH` says otherwise).
3. Read the code from the server log line `tellomi email verification code`.
4. `auth.signIn` with that code. A new address returns `authorizationSignUpRequired`.
5. `auth.signUp` with `first_name` set. The new row has `users.email=user@example.com` and `users.phone` NULL.
6. Sign in again with a new code. Expect `authorization` and no password step.
7. `auth.sendCode` with a normal `+86` number still accepts `12345` when `TELLOMI_SMS_PROVIDER` is unset, and still sends Aliyun SMS when it is `aliyun`.

## Verify with Resend

1. Set the provider and the key in the environment (a shell export or a gitignored `.env`, not the repo):

```text
TELLOMI_EMAIL_PROVIDER=resend
TELLOMI_RESEND_API_KEY=<api key>
TELLOMI_EMAIL_FROM=noreply@tellomi.app
TELLOMI_EMAIL_FROM_NAME=Tellomi
```

2. Confirm the from domain (`tellomi.app` for the default address) is verified in Resend.
3. `auth.sendCode` with `phone_number=user@example.com`. Expect `auth.sentCodeTypeEmailCode`, pattern `u***@example.com`, length 6 (unless `TELLOMI_EMAIL_CODE_LENGTH` says otherwise).
4. Open the mailbox. The subject is `Tellomi 验证码 / Verification code`. The body contains the 6-digit code in Chinese and in English.
5. `auth.signIn` with that code. A new address returns `authorizationSignUpRequired`. `auth.signUp` with `first_name` stores `users.email` and a NULL `users.phone`.
6. Sign in again with a new code. Expect `authorization`.
7. Unset `TELLOMI_RESEND_API_KEY` and call `auth.sendCode` again. The process stays up. The RPC returns `SMS_CODE_CREATE_FAILED`.
8. `auth.sendCode` for a normal `+86` number still follows the SMS provider rules.

`postal` is still accepted. Set `TELLOMI_EMAIL_PROVIDER=postal`, `TELLOMI_POSTAL_BASE_URL` to the HTTP origin (no `/api/v1/...` suffix required), and `TELLOMI_POSTAL_API_KEY`. The same RPCs then deliver through Postal. That path is legacy; production OTP is Resend.
