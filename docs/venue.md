# Venue search

Attach → Location on the official clients resolves `help.getConfig`
`venue_search_username` (`venue`) and calls `messages.getInlineBotResults`
with the user's WGS-84 point. An empty query is the nearby list. A text
query is search. `messages.sendInlineBotResult` sends `messageMediaVenue`.

The server picks the provider from the session:

| Client | How it is recognized | Provider |
|---|---|---|
| iOS | `lang_pack` `ios`, or a device model containing iPhone/iPad/iPod, or a system version starting with `iOS`/`iPadOS`, or `api_id` 8 | Apple Maps |
| Android | `lang_pack` `android`, or a system version starting with `SDK` or containing `Android`, or `api_id` 4 or 6 | AMap (高德) |
| Anything else | desktop, macOS, or a Tellomi `api_id` shared by both apps (`100001`) with no device hint | Apple, then AMap |

A provider is on only when its credentials are set. With none set, the bot
returns `messages.botResults` with an empty `results` vector and no RPC
error. Send-my-location and live location do not use this list.

Results use `botInlineMessageMediaVenue`. `venue_type` is a Foursquare
category path (`food/cafe`, `shops/default`, `travel/hotel`, …) so the
clients can load `ss3.4sqi.net` icons. AMap coordinates are converted
WGS-84 → GCJ-02 for the query inside China and GCJ-02 → WGS-84 on the
way back. Outside China the values are sent through unchanged. Apple Maps
uses WGS-84 on both sides.

Rows are cached for about 10 minutes per provider, rounded coordinate, and
query. Each user is rate limited. Upstream calls time out. Logs do not
include the coordinate query string.

## Environment

| Variable | Required for | Meaning |
|---|---|---|
| `TELLOMI_APPLE_MAPS_TEAM_ID` | Apple | 10-character Apple Developer Team ID |
| `TELLOMI_APPLE_MAPS_KEY_ID` | Apple | 10-character Maps key id |
| `TELLOMI_APPLE_MAPS_PRIVATE_KEY_PATH` | Apple | path of the `.p8` private key inside the container |
| `TELLOMI_AMAP_KEY` | AMap | 高德 Web服务 key |
| `TELLOMI_AMAP_SIG` | optional | 高德数字签名 secret |
| `TELLOMI_TENCENT_LBS_KEY` | optional | used only when AMap is set and that call fails |
| `TELLOMI_AMAP_QPS` | optional | global upstream cap, default 3 |
| `TELLOMI_AMAP_DAILY` | optional | shared daily budget, default 5000 |
| `TELLOMI_VENUE_REDIS` | optional | `host:port` for the shared cache. Unset uses memory |

Mount the `.p8` into the container and set `TELLOMI_APPLE_MAPS_PRIVATE_KEY_PATH`
to that path. Do not commit the key.

## Apple Maps key

1. Sign in to [Apple Developer](https://developer.apple.com/account) → Certificates, Identifiers & Profiles → Keys.
2. Create a key, enable **Maps** (Maps IDs / MapKit JS), and register a Maps ID if asked. Download the `.p8` once. Its 10-character Key ID is `TELLOMI_APPLE_MAPS_KEY_ID`.
3. Membership shows the 10-character Team ID (`TELLOMI_APPLE_MAPS_TEAM_ID`).
4. The server signs a short ES256 JWT (`kid`, Team ID, scope `server_api`) and exchanges it at `GET https://maps-api.apple.com/v1/token`. Nearby search is `GET /v1/searchNearby`. Text search is `GET /v1/search`. The access token is cached until it expires.

## AMap key

Create a Web服务 key in the 高德 console and set `TELLOMI_AMAP_KEY`. The calls are `v5/place/around` and `v5/place/text`. Leave it empty until that key exists. Android location previews (`upload.getWebFile`) also stay unavailable without it.
