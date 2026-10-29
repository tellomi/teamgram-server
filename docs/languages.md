# Languages

`langpack.getLanguages` lists Telegram's official base languages. Simplified Chinese, Traditional Chinese, and English stay first. The other languages are listed with `name`, `native_name`, `plural_code`, and `lang_code`, and `base_lang_code` is `en`.

## Strings

English and Chinese packs in `data/langpack/ios` and `data/langpack/android` are Tellomi's own catalogs, imported by `scripts/langpack_import.py`.

The other official languages do not include Telegram's translated string bodies. `translations.telegram.org` will export a language (`/{code}/{platform}/export`) without stating a license for reuse. The same strings are shipped in the client trees, which are GPL-2.0 (Android and iOS) and GPL-3.0 (tdesktop). Copying those catalogs into this Apache-2.0 server would mix the licenses, so they are not vendored.

A client that selects one of those languages loads the English pack as the base and overlays an empty server pack. Desktop and macOS packs are the same list. Their English file only contains the app name; the desktop client keeps its built-in English for every other key.

`python3 scripts/langpack_import.py --languages-only` rewrites the metadata files. Running it again writes the same JSON and does not replace the full English or Chinese packs.

## Limited languages

Every language other than `en`, `zh-hans`, and `zh-hant` on iOS and Android is metadata plus the English fallback. `tdesktop` and `macos` are limited the same way, including English.
