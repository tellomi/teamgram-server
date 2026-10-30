# Business

Layer 232 `account.updateBusiness*` and `account.*BusinessChatLink` methods are
routed on their own RPC services (`RPCBusinessOpeningHours`,
`RPCBusinessLocation`, `RPCBusinessGreeting`, `RPCBusinessIntro`,
`RPCBusinessChatLinks`).

Hours, location, greeting, away message, and intro are stored in this process
and copied onto `userFull` by `users.getFullUser`. A restart drops them.
Greeting and away messages keep a quick-reply shortcut id; the shortcut must
already exist (`messages.sendMessage` with `quick_reply_shortcut`, or the
other quick-reply RPCs). This server does not send those messages on its own.

Chat links use the official `https://t.me/m/<slug>` shape. Slugs are random
and only resolve inside this process. `account.resolvedBusinessChatLinks`
carries the owner as `peerUser`. The users vector stays empty; the client
loads the profile with `users.getUsers`.

## Schema-valid results

- `account.updateBusinessWorkHours` stores `businessWorkHours` and returns
  `boolTrue`. `open_now` is recomputed from the timezone on each
  `users.getFullUser`. Omitting the hours clears them.
- `account.updateBusinessLocation` stores `businessLocation` and returns
  `boolTrue`. Omitting both the point and the address clears it.
- `account.updateBusinessGreetingMessage` and
  `account.updateBusinessAwayMessage` store the message and return `boolTrue`.
  Omitting the message clears it.
- `account.updateBusinessIntro` stores title and description and returns
  `boolTrue`. A sticker is refused. Omitting the intro clears it.
- `account.createBusinessChatLink` returns `businessChatLink` with views 0.
- `account.editBusinessChatLink` returns the same slug with the new message.
  Views are kept.
- `account.deleteBusinessChatLink` returns `boolTrue`.
- `account.getBusinessChatLinks` returns `account.businessChatLinks`.
- `account.resolveBusinessChatLink` returns
  `account.resolvedBusinessChatLinks` and increments views.

## Documented 400s

| Code | When |
| --- | --- |
| `TIMEZONE_INVALID` | timezone id is empty, longer than 64 bytes, or not an IANA name `time.LoadLocation` accepts |
| `BUSINESS_WORK_HOURS_EMPTY` | hours are set and `weekly_open` is empty |
| `BUSINESS_WORK_HOURS_PERIOD_INVALID` | a period is outside Monday 00:00 through Sunday 24:00, `start >= end`, periods overlap, or there are more than 28 |
| `GEO_POINT_INVALID` | the point is not `inputGeoPoint` / `inputGeoPointEmpty`, or latitude/longitude is out of range |
| `BUSINESS_ADDRESS_INVALID` | the address is longer than 96 UTF-16 code units |
| `BUSINESS_RECIPIENTS_EMPTY` | no recipient flag and no users, or `exclude_selected` with nothing else |
| `SHORTCUT_INVALID` | shortcut id is missing or not one of this user's quick replies |
| `NO_ACTIVITY_DAYS_INVALID` | greeting `no_activity_days` is negative or greater than 366 |
| `SCHEDULE_INVALID` | away schedule is missing or not always / outside work hours / custom |
| `SCHEDULE_DATE_INVALID` | custom away schedule has `start_date <= 0` or `end_date <= start_date` |
| `USER_ID_INVALID` | a recipient `InputUser` is empty or has user id 0 |
| `USERS_TOO_MUCH` | more than 100 recipient users |
| `TITLE_INVALID` | intro title is longer than 64 UTF-16 code units, or a link title is empty or longer than 32 |
| `ABOUT_TOO_LONG` | intro description is longer than 70 UTF-16 code units |
| `STICKER_DOCUMENT_INVALID` | intro sticker is set. Documents are not resolved here |
| `LINK_INVALID` | `inputBusinessChatLink` is missing |
| `MESSAGE_TOO_LONG` | link message is longer than 4096 UTF-16 code units |
| `ENTITIES_TOO_LONG` | more than 100 entities |
| `ENTITY_BOUNDS_INVALID` | an entity is empty or outside the message |
| `CHATLINKS_TOO_MUCH` | the user already has 100 links |
| `SLUG_INVALID` | edit, delete, or resolve names a slug this user does not own |

`AUTH_KEY_UNREGISTERED` (401) is returned when the RPC has no user id.
