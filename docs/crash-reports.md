# Crash reports

iOS and Android read `tellomi_crash_report_bot` from `help.getAppConfig` and send crash logs to that username in a private chat. The account is `@TellomiBot` (display name Tellomi 反馈, user id 777006). `TELLOMI_CRASH_REPORT_BOT` does not rename it. At startup the process reads user 777006's username and publishes that. If the env value is different, it logs a warning and clients still see the database name.

The account is created by `teamgramd/deploy/sql/migrate-20261125-crash-report-bot.sql`, which `migrate.sh` runs on startup. It is safe to run again. If user 777006 or the username already belongs to someone else, the script stops and does not change that account. The same insert is `/app/bin/crashreport ensure -f /app/etc2/bff.yaml`.

A document or text message sent to the bot is copied into the attachments bucket at:

```
crash-reports/YYYY/MM/DD/<sender_user_id>/<msg_id>_<filename>
```

A row in `crash_reports` records the time, sender, message id, filename, caption, message text, a short device/app hint when the text contains one, the size, and that storage key. Album sends (`messages.sendMultiMedia`) are stored the same way, one row per item. A store error is logged and does not fail the send.

Objects larger than `TELLOMI_CRASH_REPORT_MAX_BYTES` (default 20 MiB) are not copied. The row is still written with `status` `skipped` and an empty storage key. Media purge does not delete the `crash-reports/` prefix.

## Query

The HTTP listener starts only when `TELLOMI_CRASH_REPORT_TOKEN` is set. It binds to `TELLOMI_CRASH_REPORT_ADDR` (default `127.0.0.1:8819`). Port 8818 is already bound by another listener in the bff process. Only loopback is accepted (`127.0.0.1`, `::1`, or `localhost`). A private-network address is refused. Do not publish the port. A failed bind is logged with the listen error. A password inside a DSN-shaped error is masked. The admin token is not part of that error and is not logged.

The isolated enforcement drill listens on `127.0.0.1:8819` when `TELLOMI_ENFORCEMENT_E2E=1`. Production leaves that variable unset. The client regression job sets `TELLOMI_CRASH_REPORT_ADDR=127.0.0.1:8818` so the two listeners do not share a port.

```
GET /internal/v1/crash-reports?from=<unix>&to=<unix>&sender=<user id>&device=<substring>&limit=<1-200>
Authorization: Bearer <TELLOMI_CRASH_REPORT_TOKEN>
```

`from` and `to` are inclusive unix seconds (RFC3339 is also accepted). `device` is matched against the device hint, app hint, filename, caption, and message. The JSON list includes a presigned download URL that lasts 15 minutes. The URL host is the configured S3 endpoint.

From inside the container, without opening a port:

```
docker compose exec teamgram /app/bin/crashreport list -f /app/etc2/bff.yaml -from 1762736400 -to 1762822800 -sender 9 -device ios
```

`TELLOMI_CRASH_REPORT_MYSQL` overrides the DSN. Otherwise the command reads `ChannelMySQL` from the bff config. Do not put the token or the DSN on the command line.
