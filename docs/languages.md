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
| classic-zh-cn | 简体中文 (upstream classic pack; `base_lang_code=zh-hans`) |
| taiwan | 正體中文 (upstream Taiwan pack; `base_lang_code=zh-hant`) |
| ja, ko, ru, es, de, fr, it, pt-br | full packs |
| ar, tr, id, ms, vi, uk, nl, pl, fa | full packs |
| hi | Hindi. Android is a large partial pack. iOS, macOS, and tdesktop are smaller. |
| th | Thai. As of the 2026-11-02 export, translations.telegram.org has no Thai strings for these platforms (the Android file is an English copy, which is not shipped). The language is listed and falls back to English. |
| be, ca, cs, el, fi, he, nb, ro, sv, uz | shipped from the same export. Greek is only partial on iOS and empty on desktop. |

BCP-47 `zh-TW`, `zh-HK`, and `zh-Hant` still resolve to `zh-hant` (the full traditional pack). The code `taiwan` is the separate 正體中文 pack. `classic-zh-cn` is simplified, not traditional: its export `Localization.LanguageName` is 简体中文.

Untranslated strings that are an exact copy of the official English text are omitted. The client loads `base_lang_code` for the missing keys.

## Tellomi overrides

`data/langpack/overrides/{ios,android}/{en,zh-hans,zh-hant}.json.gz` is the Tellomi catalog from before this import.

- `AppName`, `AppNameBeta`, `Application.Name`, and `Watch.AppName` are Tellomi.
- Where the Tellomi English string differs from the branded official English string, that Tellomi wording wins in en, zh-hans, and zh-hant. Those strings keep their previous version (2 or 3).
- Every other key is the official translation. The word Telegram becomes Tellomi. `t.me` / `telegram.me` / `telegram.dog` become `tell.cc`. `telegram.org` becomes `tellomi.cn`.

A client that already stored Chinese at version 3 receives the replaced strings from `langpack.getDifference` because their version is the upstream export version.

## Deploy

The packs are in the image. After this merges, CI tags the image with the 7-character commit SHA. On the production host:

```bash
/opt/tellomi/deploy-pull.sh <shortSHA>
```

That argument sets `TELLOMI_IMAGE_TAG` and wins over `.env`. The script pulls `teamgram` and recreates the stack. It does not build on the server and does not start MinIO. See [CI and ACR](ci-acr.md).
