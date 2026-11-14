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

`help.getConfig` `suggested_lang_code` follows `initConnection.system_lang_code`. An empty, unknown, or hidden device language uses `lang_code` when that names a pack we offer, otherwise `en`. A hidden code is never suggested. `lang_pack_version` stays on the app language (`lang_code`). Text this server writes itself (`help.getAppUpdate`, 777000 notices, `help.getPremiumPromo`) uses `lang_code`, then `lang_pack` when that value is a language, then `system_lang_code`, and English only after those three miss.

One entry per language. `classic-zh-cn` (Chinese (Simplified, Classic), the old community 简体中文 pack) and `taiwan` (Chinese (Traditional, Taiwan), 正體中文) are not listed. On iOS those packs were about 5.4k and 3.7k strings beside the official ~12.4k packs, and both used the same language as `zh-hans` / `zh-hant`. `langpack.getLanguage`, `langpack.getLangPack`, `langpack.getStrings`, and `langpack.getDifference` for those codes serve `zh-hans` or `zh-hant` instead of returning an error. The `lang_code` in the response is the official code.

BCP-47 `zh-TW`, `zh-HK`, `zh-MO`, `zh-Hant`, `zh-rTW`, `zh-cht`, `cmn-Hant`, and `yue` / `yue-HK` resolve to `zh-hant`. `zh`, `zh-CN`, `zh-Hans`, `zh-rCN`, `zh-chs`, and `cmn-Hans` resolve to `zh-hans`. A regional tag such as `ja-JP` or `de-DE` resolves to the pack (`ja`, `de`) before English. `zh-Latn` does not.

Untranslated strings that are an exact copy of the official English text are omitted. The client loads `base_lang_code` for the missing keys. Every shipped pack except English has `base_lang_code=en`. English has no base. The removed packs used to set their base to `zh-hans` or `zh-hant`; the packs that remain still fall back only to English.

## Tellomi overrides

`data/langpack/overrides/{ios,android}/en.json.gz` is the Tellomi English catalog. The Chinese overrides are the strings that must differ from the official zh-hans / zh-hant export: branding, login, push, and permission lines, plus Tellomi-only keys such as Discover. Everything else is the official translation.

- `AppName`, `AppNameBeta`, `Application.Name`, and `Watch.AppName` are Tellomi.
- A Chinese key is replaced only when that key is present in the Chinese override. English divergences do not pull the old word-by-word translation back in.
- The word Telegram becomes Tellomi, including a case ending or markdown emphasis such as `__Telegram Premium__`. A markdown link target such as `](telegram)` or `](web)` stays a client identifier. `t.me` is rewritten only when it is a host, so `Chat.Message` stays intact. `telegram.org` becomes `tellomi.cn`. A tips-channel URL becomes `https://tellomi.cn/premium`. Those fixes are pinned in `data/langpack/reviewed` so a later import keeps them.

The Android export gained 91 keys after the previous import (for example `CompactMode`, `TopicsSidebar`, `AgentStart`). Those keys are in every Android pack. Strings the official iOS export left in English — login email templates, a few `PUSH_*` lines, and the App Store preview copy — are translated and marked `mt`, and `data/langpack/reviewed` keeps them across a later import. `Login.OpenFragment` stays "打开 Fragment".

Every Android pack is version `61730402`. `android_xml_fix_version` stays `61730400`. `langpack.getLanguages` / `getLanguage` report `lang_code` `zh-hans` (the client lowercases and turns `-` into `_`), `plural_code` `zh`, `base_lang_code` `en`, and `official`. Those three codes match the official Telegram catalog (`zh-hans` / `zh-hant` on translations.telegram.org) and are left as they are. Stock Android `hasBaseLang()` is false for an official pack, so it does not download the English base. `LocaleController.applyLanguage` builds the Android `Locale` from `plural_code` first, so `zh` becomes `Locale("zh")`. Built-in assets are keyed by `locale.toLanguageTag()` (`values-zh-rCN` is `zh-CN`, `values-zh-rTW` is `zh-TW`), and there is no `zh` asset, so a key missing from the cloud pack is the English built-in string. The Tellomi-only keys are therefore stored in the pack itself. A phone at `61730400` receives every string added since the cutover from `langpack.getDifference`, including this fill. A phone at `61730401` receives the strings whose text changed at `61730402`. A phone already at `61730402` receives an empty difference. Each iOS pack version moves forward by one the same way. `lang_code` and `plural_code` are unchanged. The plural code stays `zh`, which is the code the plural-rule table is registered under.

Tellomi product keys are translated in every language. Short labels reuse that pack's own wording (`Settings`, `Contacts`, `Followers`, and the other official strings named in `scripts/langpack_tellomi_mt.py`). Time-ago lines reuse the pack's plural form (`many` where the language has it, otherwise `other`) with the Tellomi placeholder left as it is. Those keys are still one string in the client, not a pluralized key. The rest is machine-translated and marked `mt`. Native speakers need to review it. A Tellomi value may equal its English string only when `data/langpack/tellomi-english-allowlist.json` says so: the whole value is a brand (`PDF`, `Tellomi`), or the entry names that key and why this language keeps the word (a proper noun, the product name when it is the same word, or an official string that is itself English). `scripts/langpack_gap.py` and `go test ./pkg/langpack` fail otherwise.

Production `35aed0c` already served these packs at `61638734` with the bare `&` removed. `LocaleController` still stores `localeInfo.version` when `getLocaleFileStrings` fails, and it only replaces the file on `from_version == 0`. `langpack.getDifference` at `61638734` was therefore empty, and a phone that had saved the broken file kept it until something asked for the whole pack. The XML-safe cutover was `61730399`. The rebrand that removed the remaining Telegram name is `61730400`, which is the same full-replace cutover moved forward: `android_xml_fix_version` is `61730400`. `getDifference` with `0 < from_version < 61730400` returns the whole pack and `from_version=0`, so a phone still at `61638734` or `61730399` replaces the cached file. Before the Tellomi key fill, a client already at `61730400` received an empty difference. `getLangPack` is unchanged: it is always `from_version=0`.

A value that contains `<` is escaped to `&lt;` by the client. On the next read that sets `reloadLastFile`, so `applyLanguage` asks for `getLangPack` again. The file is still applied. Strings that are not XML-safe are dropped before they are served; Greek `SentAppCode` is repaired to `**Tellomi**` instead of a broken `<![CDATA[` marker. The packs do not contain `SearchEngine[0-9]+(Name|SearchURL|AutocompleteURL|PrivacyPolicyURL)`.

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

Tellomi product keys (navigation, Moments, login) are compared with the client catalogs by `scripts/langpack_gap.py`. The working trees are Android `master` and iOS `release/ios`. Extra open PRs are `pr:N` refs: the command fetches the head and reads translation tables in the PR body. `--apply` writes every Android and iOS language, bumps that pack's version by one, and refreshes `data/langpack/tellomi-client-keys.json`. `go test ./pkg/langpack` fails if any listed key is missing.

```bash
python3 scripts/langpack_gap.py \
  --android /path/to/Telegram \
  --ios /path/to/Telegram-iOS \
  --upstream-android /tmp/upstream/android-en.xml \
  --upstream-ios /tmp/upstream/ios-en.strings \
  --android-ref pr:134 \
  --android-ref pr:135 \
  --ios-ref pr:161 \
  --ios-ref pr:160 \
  --ios-ref pr:162 \
  --apply
```

## Deploy

The packs are in the image. After this merges, CI tags the image with the 7-character commit SHA. On the production host:

```bash
/opt/tellomi/deploy-pull.sh <shortSHA>
```

The script checks out that commit, pins `TELLOMI_IMAGE_TAG` in `.env`, pulls `teamgram` (and `asr`), runs `mysql-migrate`, and recreates only `asr` and `teamgram`. It does not build on the server and does not start MinIO. See [CI and ACR](ci-acr.md).
