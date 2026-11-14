# Release and changelog

This document defines versioning, CHANGELOG format, and the release checklist.

## Versioning

- **Semantic versioning** in spirit; example format: `v0.96.0-teamgram-server`.
  - **Major**: Incompatible API or architecture changes.
  - **Minor**: Backward-compatible new features.
  - **Patch**: Backward-compatible fixes and small improvements.
- The suffix (e.g. `-teamgram-server`) may be used to distinguish from dependencies (e.g. proto).

## CHANGELOG

- **Location**: Root **CHANGELOG.md** (when present).
- **Format**: Prefer [Keep a Changelog](https://keepachangelog.com/) (sections: Added, Changed, Deprecated, Removed, Fixed, Security).
- **Responsibility**: Update CHANGELOG for each release with the changes for that version.

## Release checklist

Before cutting a new release:

1. **Version**: Update `VERSION` in Makefile or version injection if used.
2. **CHANGELOG**: Add an entry for the new version and summarize changes.
3. **Tag**: Create a Git tag (e.g. `v0.96.0`) and push.
4. **Artifacts**: Build and publish binaries or Docker images per project practice.
5. **README**: Update any version or “current release” references if needed.
6. **Android app**: Publish `latest.json` and confirm `help.getAppUpdate` (below). This is mandatory for every Android release.

## Android in-app update

Every Android release publishes `https://www.tellomi.cn/dl/android/latest.json`. Chengdu syncs that object from the R2 bucket `tellomi-updates`. `help.getAppUpdate` reads it and compares `versionName` with the client. Publishing the file is what shows the in-app prompt. Do not rely on editing `TELLOMI_APP_UPDATE_VERSION` in the production `.env`. Leave that variable empty. A non-empty value replaces the manifest and will go stale on the next APK.

The feed fields are `versionName`, `versionCode`, `url`, `fallbackUrl`, `sha256` or `sha256sum`, and `size`. The published file uses `sha256sum` at the top level and `sha256` on each `files[]` entry. Optional `changelog`, when it is a string, is the dialog text. Optional `notes` is an object of language code to dialog text, for example `{"zh-hans":"...","en":"..."}`. The client's initConnection language is `lang_code`, then `system_lang_code`, then `lang_pack` when that value is itself a language. A matching `notes` entry is used first, then `notes.en` when that key exists. A string `changelog` is still the dialog text when no note matches, so an existing `latest.json` keeps working. When both are empty, the server uses a built-in sentence for that language, or English when the language is unknown. A non-empty `TELLOMI_APP_UPDATE_TEXT` replaces the dialog text for every language. The prompt is skippable. It sets `can_not_skip` only when `TELLOMI_APP_UPDATE_CANNOT_SKIP=1` or the feed contains `"mandatory": true` (a JSON boolean; any other type is ignored). iOS still receives `help.noAppUpdate`.

`TELLOMI_APP_UPDATE_MANIFEST` selects a file path or an `http(s)` URL. When it is unset or empty, the server uses the first regular file that exists:

- `/etc/tellomi/updates/android/latest.json`
- `/opt/tellomi/updates/android/latest.json`
- `/var/www/tellomi/dl/android/latest.json`
- `/usr/share/nginx/html/dl/android/latest.json`

Otherwise it fetches `https://www.tellomi.cn/dl/android/latest.json`. `off` disables the feed. A successful read is cached for 5 minutes. A timeout (5 seconds) or other read error is cached for 30 seconds and then answers `help.noAppUpdate`. A non-empty `TELLOMI_APP_UPDATE_VERSION` is used instead of reading the feed, so a dead manifest cannot block that override. No process restart is required after the file changes.

After each Android release:

1. `curl -fsS https://www.tellomi.cn/dl/android/latest.json` shows the new `versionName`.
2. Within 5 minutes, `help.getAppUpdate` for an Android session older than that version returns `help.appUpdate` with the same `versionName` and the manifest `url`. `can_not_skip` is set only when the feed has `"mandatory": true` or `TELLOMI_APP_UPDATE_CANNOT_SKIP=1`. An iOS session still returns `help.noAppUpdate`.
