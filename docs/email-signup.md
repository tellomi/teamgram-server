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

`postal` sends through a self-hosted Postal **HTTP** API (`POST {TELLOMI_POSTAL_BASE_URL}/api/v1/send/message`, header `X-Server-API-Key`). SMTP is not used: outbound TCP 25 and 587 are blocked. Postal does not have to be up for this build. An empty base URL or API key does not stop startup; `auth.sendCode` returns `SMS_CODE_CREATE_FAILED` until both are set. Until Postal is reachable, leave the provider at `log`.

From address defaults to `noreply@tellomi.app` with display name `Tellomi`.

`disabled`, `none`, or `off` makes email `auth.sendCode` return `EMAIL_NOT_SETUP`. Aliyun DirectMail is not a sender for this path.

## Limits

Email quotas are separate from SMS (`kind=email` in redis). Over the daily cap the RPC returns `PHONE_NUMBER_FLOOD`. Inside the resend cooldown it returns `FLOOD_WAIT_<seconds>`. Wrong codes use `TELLOMI_EMAIL_MAX_ATTEMPTS`, then `FLOOD_WAIT_900`.

## Errors

| Code | When |
| --- | --- |
| `EMAIL_INVALID` | Address fails the mailbox check, or the client sent an Apple/Google token |
| `EMAIL_NOT_SETUP` | Email provider is disabled |
| `PHONE_NUMBER_FLOOD` | Daily email cap |
| `FLOOD_WAIT_X` | Resend cooldown or too many wrong codes |
| `PHONE_NUMBER_OCCUPIED` | Email already registered |
| `PHONE_NUMBER_UNOCCUPIED` | Sign-in on an old layer before sign-up |
| `SMS_CODE_CREATE_FAILED` | Postal (or the sender) failed. The log has the provider error |
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

When Postal is up, set `TELLOMI_EMAIL_PROVIDER=postal`, `TELLOMI_POSTAL_BASE_URL` to the HTTP origin (no `/api/v1/...` suffix required), and `TELLOMI_POSTAL_API_KEY`. The same RPCs then deliver through Postal instead of only the log.
