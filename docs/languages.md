# Languages

`langpack.getLanguages`, `langpack.getLanguage`, `langpack.getLangPack`, `langpack.getStrings`, and `langpack.getDifference` are served for `ios`, `android`, `tdesktop`, and `macos` (plus the aliases `android_x`, `desktop`, and `macosx`). The session service allows all five before login, which is when the iOS and Android language pickers call them. A signed-out auth key (no user id) still receives the list, including `zh-hans` and `en`. `getLangPack` / `getStrings` / `getDifference` for those codes return different text, so the login language picker can switch between 简体中文 and English. `zh-CN` is read as `zh-hans`.

Simplified Chinese, Traditional Chinese, and English stay first. Every other language is listed with `name`, `native_name`, `plural_code`, and `base_lang_code`.

## Source

String bodies are the public export from translations.telegram.org:

```
https://translations.telegram.org/{code}/{platform}/export
```

No login is required. The export filename carries the pack version (`android_ja_v61521017.xml`). That integer is `langPackDifference.version`, so a client that cached the old empty version-1 pack receives the strings from `langpack.getDifference`.

Refresh with:

```bash
python3 scripts/langpack_official.py
```

The script writes `data/langpack/<platform>/<code>.json.gz` and `data/langpack/SOURCE.json`. Packs are embedded in the server binary. `TELLOMI_LANGPACK_DIR` replaces that directory at startup.

`scripts/langpack_import.py --languages-only` only fills a missing metadata file. It does not replace a pack that already has strings.

## Languages

Served for ios, android, tdesktop, and macos:

| code | name |
|---|---|
| en | English |
| zh-hans | 简体中文 |
| zh-hant | 繁體中文 |
| ja, ko, ru, es, de, fr, it, pt-br | full packs |
| ar, tr, id, ms, vi, uk, nl, pl, fa | full packs |
| hi | Hindi. Re-exported 2026-11-04. Android is still a partial official pack; iOS, macOS, and tdesktop are smaller. Missing keys are machine-translated. Not offered by default. |
| th | Thai. Re-exported 2026-11-04. translations.telegram.org still has no Thai strings (the Android file is an English copy, which is not shipped). The pack is machine-translated. Not offered by default. |
| el | Greek. Re-exported 2026-11-04. Official coverage is still partial. Missing keys are machine-translated. Not offered by default. |
| be, ca, cs, fi, he, nb, ro, sv, uz | shipped from the same export. |

`langpack.getLanguages` omits codes in `TELLOMI_LANGPACK_HIDDEN`. When that variable is unset the list is `hi,th,el`. Those three stay hidden until the machine translations are reviewed. Set the variable to empty to list every pack. `langpack.getLanguage`, `langpack.getLangPack`, `langpack.getStrings`, and `langpack.getDifference` still serve a hidden code, and that pack now has the same keys as English.

`help.getConfig` `suggested_lang_code` follows `initConnection.system_lang_code`. An empty, unknown, or hidden device language uses `lang_code` when that names a pack we offer, otherwise `en`. A hidden code is never suggested. `lang_pack_version` stays on the app language (`lang_code`).

One entry per language. `classic-zh-cn` (Chinese (Simplified, Classic), the old community 简体中文 pack) and `taiwan` (Chinese (Traditional, Taiwan), 正體中文) are not listed. On iOS those packs were about 5.4k and 3.7k strings beside the official ~12.4k packs, and both used the same language as `zh-hans` / `zh-hant`. `langpack.getLanguage`, `langpack.getLangPack`, `langpack.getStrings`, and `langpack.getDifference` for those codes serve `zh-hans` or `zh-hant` instead of returning an error. The `lang_code` in the response is the official code.

BCP-47 `zh-TW`, `zh-HK`, and `zh-Hant` still resolve to `zh-hant`. `zh`, `zh-CN`, and `zh-Hans` resolve to `zh-hans`.

Untranslated strings that are an exact copy of the official English text are omitted. The client loads `base_lang_code` for the missing keys. Every shipped pack except English has `base_lang_code=en`. English has no base. The removed packs used to set their base to `zh-hans` or `zh-hant`; the packs that remain still fall back only to English.

## Tellomi overrides

`data/langpack/overrides/{ios,android}/en.json.gz` is the Tellomi English catalog. The Chinese overrides are the strings that must differ from the official zh-hans / zh-hant export: branding, login, push, and permission lines, plus Tellomi-only keys such as Discover. Everything else is the official translation.

- `AppName`, `AppNameBeta`, `Application.Name`, and `Watch.AppName` are Tellomi.
- A Chinese key is replaced only when that key is present in the Chinese override. English divergences do not pull the old word-by-word translation back in.
- The word Telegram becomes Tellomi, except inside a markdown link target such as `](telegram)` or `](web)`. `t.me` is rewritten only when it is a host, so `Chat.Message` stays intact. `telegram.org` becomes `tellomi.cn`.

Chinese pack versions sit above the 2026-11-02 export. Android zh-hans is `61570954` and Android zh-hant is `61519204`. That covers the Tellomi strings added after the export (login, maps, intro, call-count plurals, and Discover), the city-key rename, and the later fill of keys the other Chinese pack or English already had. Clients that cached `61570950` still receive the earlier Tellomi strings from `langpack.getDifference`, and clients that cached `61570951` receive the Discover keys. Clients that cached `61570952` receive the four renamed city keys together with the filled keys. Clients that cached `61570953` receive only the filled Simplified Chinese keys. Android en is `61510241` (export `61510239`).

iOS Discover and sign-up avatar strings are en `22347346` (export `22347344`), zh-hans `22454347` (export `22454343`), and zh-hant `22421532` (export `22421528`). The four city ids (`cn_hong_kong`, `us_new_york`, `us_los_angeles`, `us_san_francisco`) use underscores in every language. On English they are the newest strings, so a client one version behind receives just those keys. On zh-hans and zh-hant they sit one version below the filled pack: a client that already has the renamed cities receives only the fill, and a client one version behind the rename receives the cities together with the fill. Every other built-in language has those keys too. The ten `Tellomi.Discover.Report*` keys are on Android only.

iOS English also carried 94 extra key names from an old host rewrite (`Chat.Message` became `Chatell.ccssage`) in `data/langpack/ios/en.json.gz` and in the English override. Those names are removed. The real keys were already in the served pack at string version `22347344`. Where the extra copy disagreed, the official string stays (`Topic Creator`, not `Topic Author`). The pack version stays `22347346`, so a client that already has that version does not receive a difference: it looks up the real names, and those strings did not change. The override does not keep a second copy of those official strings, so a later export can still update them. `scripts/langpack_official.py` rewrites hosts in values only and refuses a key that contains `tell.cc`. The fill uses that same check and does not copy the old names into other languages.

## Coverage

`scripts/langpack_gap.py` prints, per language and platform, the key-count gap versus English. `go test ./pkg/langpack` fails when any language is missing more than 1% of the English keys on ios, android, tdesktop, or macos.

Keys that are not in the official export are machine-translated and stored with `"mt": true` and `"mt_via"` (`google`, `google+opencc`, `opencc`, or `copy`). Placeholders (`%@`, `%1$s`, `{count}`, `{}`) and markdown link targets are not sent to the translator. `data/langpack/machine/<platform>/<code>.json.gz` is the review set. `scripts/langpack_official.py` reapplies it when a later export still lacks the key. A simplified string missing from traditional Chinese is OpenCC `s2twp` of the simplified text, and the other way is `tw2sp`.

```bash
python3 scripts/langpack_official.py --codes el,th,hi
python3 scripts/langpack_fill.py
python3 scripts/langpack_gap.py
```

## Deploy

The packs are in the image. After this merges, CI tags the image with the 7-character commit SHA. On the production host:

```bash
/opt/tellomi/deploy-pull.sh <shortSHA>
```

The script checks out that commit, pins `TELLOMI_IMAGE_TAG` in `.env`, pulls `teamgram` (and `asr`), runs `mysql-migrate`, and recreates only `asr` and `teamgram`. It does not build on the server and does not start MinIO. See [CI and ACR](ci-acr.md).
