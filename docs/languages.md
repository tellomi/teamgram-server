# Languages

`langpack.getLanguages`, `langpack.getLanguage`, `langpack.getLangPack`, `langpack.getStrings`, and `langpack.getDifference` are served for `ios`, `android`, `tdesktop`, and `macos` (plus the aliases `android_x`, `desktop`, and `macosx`). The session service allows all five before login, which is when the iOS and Android language pickers call them.

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
| hi | Hindi. Android is a large partial pack. iOS, macOS, and tdesktop are smaller. Not offered by default. |
| th | Thai. As of the 2026-11-02 export, translations.telegram.org has no Thai strings for these platforms (the Android file is an English copy, which is not shipped). Not offered by default. |
| el | Greek. Only partial on Android and a handful of strings on iOS and desktop. Not offered by default. |
| be, ca, cs, fi, he, nb, ro, sv, uz | shipped from the same export. |

`langpack.getLanguages` omits codes in `TELLOMI_LANGPACK_HIDDEN`. When that variable is unset the list is `hi,th,el`. Those packs are near-empty next to the ~11–12k string packs, and a client that offers them shows English. Set the variable to empty to list every pack. `langpack.getLanguage`, `langpack.getLangPack`, `langpack.getStrings`, and `langpack.getDifference` still serve a hidden code.

`help.getConfig` `suggested_lang_code` follows `initConnection.system_lang_code`. An empty, unknown, or hidden device language uses `lang_code` when that names a pack we offer, otherwise `en`. A hidden code is never suggested. `lang_pack_version` stays on the app language (`lang_code`).

One entry per language. `classic-zh-cn` (Chinese (Simplified, Classic), the old community 简体中文 pack) and `taiwan` (Chinese (Traditional, Taiwan), 正體中文) are not listed. On iOS those packs were about 5.4k and 3.7k strings beside the official ~12.4k packs, and both used the same language as `zh-hans` / `zh-hant`. `langpack.getLanguage`, `langpack.getLangPack`, `langpack.getStrings`, and `langpack.getDifference` for those codes serve `zh-hans` or `zh-hant` instead of returning an error. The `lang_code` in the response is the official code.

BCP-47 `zh-TW`, `zh-HK`, and `zh-Hant` still resolve to `zh-hant`. `zh`, `zh-CN`, and `zh-Hans` resolve to `zh-hans`.

Untranslated strings that are an exact copy of the official English text are omitted. The client loads `base_lang_code` for the missing keys. Every shipped pack except English has `base_lang_code=en`. English has no base. The removed packs used to set their base to `zh-hans` or `zh-hant`; the packs that remain still fall back only to English.

## Tellomi overrides

`data/langpack/overrides/{ios,android}/en.json.gz` is the Tellomi English catalog. The Chinese overrides are the strings that must differ from the official zh-hans / zh-hant export: branding, login, push, and permission lines, plus Tellomi-only keys such as Discover. Everything else is the official translation.

- `AppName`, `AppNameBeta`, `Application.Name`, and `Watch.AppName` are Tellomi.
- A Chinese key is replaced only when that key is present in the Chinese override. English divergences do not pull the old word-by-word translation back in.
- The word Telegram becomes Tellomi, except inside a markdown link target such as `](telegram)` or `](web)`. `t.me` is rewritten only when it is a host, so `Chat.Message` stays intact. `telegram.org` becomes `tellomi.cn`.

Chinese pack versions sit above the 2026-11-02 export. Android zh-hans is `61570952` and Android zh-hant is `61519202`. That covers the Tellomi strings added after the export (login, maps, intro, call-count plurals, and Discover). Clients that cached `61570950` still receive the earlier Tellomi strings from `langpack.getDifference`, and clients that cached `61570951` receive the Discover keys. Android en is `61510240` (export `61510239`).

iOS Discover and sign-up avatar strings are en `22347345` (export `22347344`), zh-hans `22454345` (export `22454343`), and zh-hant `22421530` (export `22421528`). A client cached at the previous pack version receives those keys from `langpack.getDifference`. Other languages omit the keys and fall back to English through `base_lang_code`. The ten `Tellomi.Discover.Report*` keys are on Android only.

## Deploy

The packs are in the image. After this merges, CI tags the image with the 7-character commit SHA. On the production host:

```bash
/opt/tellomi/deploy-pull.sh <shortSHA>
```

The script checks out that commit, pins `TELLOMI_IMAGE_TAG` in `.env`, pulls `teamgram` (and `asr`), runs `mysql-migrate`, and recreates only `asr` and `teamgram`. It does not build on the server and does not start MinIO. See [CI and ACR](ci-acr.md).
