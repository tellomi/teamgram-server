# API coverage matrix

Methods **called by at least one official client**, classified against Tellomi `main`.

Layer **232** schema (`proto/schema/telegram_api_layer232.tl`, 848 functions). 782 of those functions are called by a client. Methods below include client calls that are not in the schema.

Regenerate with `go run ./tools/apicoverage`. Refresh client call sites with `go run ./tools/apicoverage -extract` (see the source notes). CI runs `go run ./tools/apicoverage -check` and fails when this file or `docs/api-coverage.csv` drifts from the server tree.

This file is generated. Edit the generator, not the tables.

## How a cell is decided

**Server status** is what a connected client gets from the `bff` process session actually routes to (`teamgramd/etc/session.yaml` `BFFProxyClients.IDMap`, same map in `teamgramd/etc2/session.yaml`).

| Status | Meaning |
| --- | --- |
| `implemented` | IDMap routes the RPC service, `app/bff/bff/internal/server/server.go` registers it, and the handler reads or writes real state (or returns a filled static payload). |
| `stub` | The client gets a successful empty object, `Bool`, or `*NotModified`, either from a placeholder handler or from `TryReturnFakeRpcResult` when the service is not routed. |
| `routed-but-unregistered` | IDMap routes the service, but the bff process does not register a handler for that method, so the call fails. |
| `missing` | No IDMap route and no empty proxy response. The session returns `ENTERPRISE_IS_BLOCKED`. A handler file may still exist; the notes say when it does. |

`effect=error` is a response that fails the RPC. `empty`, `bool`, and `not_modified` succeed with no data. `ok` is a real result.

**When** is `startup`, `background`, `startup+background`, or `interactive`. Startup and background sites are the ones that run from session bootstrap (config, updates difference, presence, langpack, login, push registration) rather than from a screen the user opened. Any such site makes the method **P0**.

**Phases.** P0: startup or background, and the server must not fail the RPC. P1: core chat (auth, account, users, contacts, messages, updates, photos, upload, channels) that is not already P0. P2: everything else.

## Client sources

| Client | Repository | Commit | What was counted |
| --- | --- | --- | --- |
| android | `https://github.com/DrKLO/Telegram` | `3de37c8cfb03` | TL_* request constructions in Java and jni/tgnet |
| desktop | `https://github.com/telegramdesktop/tdesktop` | `f985723b552b` | MTPnamespace_Method requests in Telegram/SourceFiles |
| ios | `https://github.com/TelegramMessenger/Telegram-iOS` | `fb0e62ba8a14` | TelegramCore Api.functions call sites, excluding Sources/MacOS |
| macos | `https://github.com/TelegramMessenger/telegram-macos` | `de608bb0d6b5` | TelegramCore methods reached by telegram-macos (overtake/TelegramSwift is not published) |

iOS is `Api.functions` call sites in TelegramCore, excluding `Sources/MacOS`.

Android is `TL_*` request constructions in Java and in `jni/tgnet` (including native `TL_help_getConfig`), excluding generated `TLRPC.java`, `tl/TL_*.java`, and `ApiScheme`.

Desktop is `MTPnamespace_Method` requests under `Telegram/SourceFiles`: direct calls, `::` flag builders, and `PrepareMessage<MTPmessages_SendMessage>`. Generated `mtproto/scheme` and tests are excluded.

`overtake/TelegramSwift` is not cloneable (the remote returns "repository not found"). The current macOS source is `TelegramMessenger/telegram-macos`, which pins the same Telegram-iOS commit and does not call `Api.functions` itself. The macOS column is TelegramCore methods reached from that app: every RPC in the account/session files it constructs (`State`, `Account`, `Network`, `SyncCore`, `PendingMessages`, auth, langpack, `Sources/MacOS`) plus TelegramCore functions whose names appear in `Telegram-Mac` and `packages`.

## Counts by module

| Module | Called | Implemented | Stub | Routed-unregistered | Missing | P0 | P0 errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| auth | 22 | 21 | 1 | 0 | 0 | 17 | 0 |
| account | 119 | 99 | 4 | 6 | 10 | 31 | 0 |
| users | 6 | 6 | 0 | 0 | 0 | 0 | 0 |
| contacts | 27 | 27 | 0 | 0 | 0 | 6 | 0 |
| messages | 216 | 215 | 0 | 0 | 1 | 14 | 0 |
| updates | 3 | 3 | 0 | 0 | 0 | 3 | 0 |
| photos | 5 | 5 | 0 | 0 | 0 | 0 | 0 |
| upload | 7 | 7 | 0 | 0 | 0 | 0 | 0 |
| help | 25 | 22 | 0 | 0 | 3 | 12 | 0 |
| channels | 59 | 58 | 0 | 0 | 1 | 1 | 0 |
| stickers | 39 | 20 | 4 | 14 | 1 | 0 | 0 |
| phone/calls | 43 | 10 | 0 | 0 | 33 | 0 | 0 |
| langpack | 5 | 5 | 0 | 0 | 0 | 5 | 0 |
| folders | 17 | 5 | 0 | 12 | 0 | 5 | 0 |
| stats | 8 | 2 | 0 | 6 | 0 | 0 | 0 |
| stories | 32 | 0 | 0 | 0 | 32 | 0 | 0 |
| premium/payments | 100 | 1 | 2 | 17 | 80 | 0 | 0 |
| bots | 23 | 0 | 0 | 15 | 8 | 0 | 0 |
| business | 10 | 0 | 0 | 0 | 10 | 0 | 0 |
| aicompose | 7 | 0 | 0 | 7 | 0 | 0 | 0 |
| communities | 9 | 0 | 0 | 0 | 9 | 0 | 0 |
| ephemeral | 8 | 0 | 0 | 0 | 8 | 0 | 0 |
| feed | 5 | 0 | 0 | 0 | 5 | 0 | 0 |
| iv | 1 | 0 | 0 | 0 | 1 | 0 | 0 |
| toncenter | 4 | 0 | 0 | 0 | 4 | 0 | 0 |
| wallet | 26 | 0 | 0 | 0 | 26 | 0 | 0 |
| **total** | **826** | **506** | **11** | **77** | **232** | **94** | **0** |

## Counts by client

A method counts for a client when that client has at least one call site. Implemented / stub / missing describe the **server** for those methods, not the client.

| Client | Called | Implemented | Stub | Routed-unregistered | Missing | P0 | P0 errors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| ios | 755 | 477 | 10 | 74 | 194 | 91 | 0 |
| android | 709 | 468 | 9 | 63 | 169 | 87 | 0 |
| desktop | 692 | 441 | 9 | 62 | 180 | 79 | 0 |
| macos | 677 | 454 | 10 | 59 | 154 | 90 | 0 |

540 methods are called by all four clients.

## Phase proposal

### P0

Startup and background calls. These run before the chat list is usable. `effect=error` drops the session or the login; empty stubs return success with no data and still break clients that require a config, difference, or langpack.

No gaps.

### P1

Core chat that is not already covered by P0: sign-in extras, profiles, contacts, history, send, media, groups, and channels.

23 gaps (23 error, 0 empty or bool or not-modified).

| Module | Gaps |
| --- | ---: |
| account | 20 |
| auth | 1 |
| channels | 1 |
| messages | 1 |

### P2

Everything else the official clients call: stories, payments, bots, folders, business, stats, and similar.

297 gaps (293 error, 4 empty or bool or not-modified).

| Module | Gaps |
| --- | ---: |
| aicompose | 7 |
| bots | 23 |
| business | 10 |
| communities | 9 |
| ephemeral | 8 |
| feed | 5 |
| folders | 12 |
| help | 3 |
| iv | 1 |
| phone/calls | 33 |
| premium/payments | 99 |
| stats | 6 |
| stickers | 19 |
| stories | 32 |
| toncenter | 4 |
| wallet | 26 |

## Methods

Columns: which clients call the method, when, server status, and the effect a client observes.

### auth

22 called, 21 implemented, 1 stub, 0 routed-unregistered, 0 missing.

| Method | iOS | Android | Desktop | macOS | When | Status | Effect |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `auth.acceptLoginToken` | yes | yes | no | yes | startup | implemented | ok |
| `auth.cancelCode` | no | yes | yes | no | startup | implemented | ok |
| `auth.checkPassword` | yes | yes | yes | yes | startup | implemented | ok |
| `auth.checkRecoveryPassword` | yes | yes | yes | yes | startup | implemented | ok |
| `auth.exportLoginToken` | yes | no | yes | yes | startup | implemented | ok |
| `auth.finishPasskeyLogin` | yes | no | yes | yes | startup | implemented | ok |
| `auth.importAuthorization` | yes | yes | yes | yes | startup+background | implemented | ok |
| `auth.importLoginToken` | yes | no | yes | yes | startup | implemented | ok |
| `auth.recoverPassword` | yes | yes | yes | yes | startup | implemented | ok |
| `auth.reportMissingCode` | yes | yes | no | yes | startup | implemented | ok |
| `auth.requestFirebaseSms` | yes | yes | no | yes | startup | implemented | ok |
| `auth.requestPasswordRecovery` | yes | yes | yes | yes | startup | implemented | ok |
| `auth.resendCode` | yes | yes | yes | yes | startup | implemented | ok |
| `auth.resetLoginEmail` | yes | yes | no | yes | startup | implemented | ok |
| `auth.sendCode` | yes | yes | yes | yes | startup | implemented | ok |
| `auth.signIn` | yes | yes | yes | yes | startup | implemented | ok |
| `auth.signUp` | yes | yes | yes | yes | startup | implemented | ok |
| `auth.initPasskeyLogin` | yes | no | yes | yes | interactive | stub | error |
| `auth.bindTempAuthKey` | no | yes | yes | no | interactive | implemented | ok |
| `auth.exportAuthorization` | yes | yes | yes | yes | interactive | implemented | ok |
| `auth.logOut` | yes | yes | yes | yes | interactive | implemented | ok |
| `auth.resetAuthorizations` | yes | yes | yes | yes | interactive | implemented | ok |

### account

119 called, 99 implemented, 4 stub, 6 routed-unregistered, 10 missing.

| Method | iOS | Android | Desktop | macOS | When | Status | Effect |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `account.cancelPasswordEmail` | yes | yes | yes | yes | startup | implemented | ok |
| `account.changePhone` | yes | yes | no | yes | startup | implemented | ok |
| `account.confirmPasswordEmail` | yes | yes | yes | yes | startup | implemented | ok |
| `account.confirmPhone` | yes | yes | yes | yes | startup | implemented | ok |
| `account.declinePasswordReset` | yes | yes | yes | yes | startup | implemented | ok |
| `account.deleteAccount` | yes | yes | yes | yes | startup | implemented | ok |
| `account.getAutoDownloadSettings` | yes | yes | no | yes | startup+background | implemented | ok |
| `account.getContactSignUpNotification` | yes | yes | yes | yes | startup+background | implemented | ok |
| `account.getGlobalPrivacySettings` | yes | yes | yes | yes | startup | implemented | ok |
| `account.getNotifySettings` | yes | yes | yes | yes | startup+background | implemented | ok |
| `account.getPassword` | yes | yes | yes | yes | startup | implemented | ok |
| `account.getPasswordSettings` | yes | yes | yes | yes | startup | implemented | ok |
| `account.getReactionsNotifySettings` | yes | yes | yes | yes | startup+background | implemented | ok |
| `account.getTmpPassword` | yes | yes | yes | yes | startup | implemented | ok |
| `account.invalidateSignInCodes` | yes | no | no | yes | startup | implemented | ok |
| `account.registerDevice` | yes | yes | no | yes | startup+background | implemented | ok |
| `account.resendPasswordEmail` | yes | yes | yes | yes | startup | implemented | ok |
| `account.resetNotifySettings` | yes | yes | no | yes | startup+background | implemented | ok |
| `account.resetPassword` | yes | yes | yes | yes | startup | implemented | ok |
| `account.saveAutoDownloadSettings` | yes | yes | no | yes | startup+background | implemented | ok |
| `account.sendChangePhoneCode` | yes | yes | no | no | startup | implemented | ok |
| `account.sendConfirmPhoneCode` | yes | yes | yes | yes | startup | implemented | ok |
| `account.sendVerifyEmailCode` | yes | yes | yes | yes | startup | implemented | ok |
| `account.setContactSignUpNotification` | yes | yes | yes | yes | startup+background | implemented | ok |
| `account.setGlobalPrivacySettings` | yes | yes | yes | yes | startup | implemented | ok |
| `account.setReactionsNotifySettings` | yes | yes | yes | yes | startup+background | implemented | ok |
| `account.unregisterDevice` | yes | yes | no | yes | startup+background | implemented | ok |
| `account.updateNotifySettings` | yes | yes | yes | yes | startup+background | implemented | ok |
| `account.updatePasswordSettings` | yes | yes | yes | yes | startup | implemented | ok |
| `account.updateStatus` | yes | yes | yes | yes | startup+background | implemented | ok |
| `account.verifyEmail` | yes | yes | yes | yes | startup | implemented | ok |
| `account.deleteWebBrowserSettingsExceptions` | yes | yes | no | no | interactive | missing | error |
| `account.finishPasskeyLogin` | no | yes | no | no | interactive | missing | error |
| `account.getBirthdays` | no | yes | no | no | interactive | missing | error |
| `account.getRequirementsToContact` | no | yes | no | no | interactive | missing | error |
| `account.getWebBrowserSettings` | yes | yes | no | yes | interactive | missing | error |
| `account.getWebPagePreview` | no | yes | no | no | interactive | missing | error |
| `account.initPasskeyLogin` | no | yes | no | no | interactive | missing | error |
| `account.reorderProfileTabs` | no | yes | no | no | interactive | missing | error |
| `account.toggleWebBrowserSettingsException` | yes | yes | no | no | interactive | missing | error |
| `account.updateWebBrowserSettings` | yes | yes | no | no | interactive | missing | error |
| `account.finishTakeoutSession` | no | no | yes | yes | interactive | routed-but-unregistered | error |
| `account.getPaidMessagesRevenue` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `account.initTakeoutSession` | no | no | yes | yes | interactive | routed-but-unregistered | error |
| `account.reorderUsernames` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `account.toggleNoPaidMessagesException` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `account.toggleUsername` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `account.deletePasskey` | yes | yes | yes | yes | interactive | stub | error |
| `account.getPasskeys` | yes | yes | yes | yes | interactive | stub | error |
| `account.initPasskeyRegistration` | yes | yes | yes | yes | interactive | stub | error |
| `account.registerPasskey` | yes | yes | yes | yes | interactive | stub | error |
| `account.acceptAuthorization` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.changeAuthorizationSettings` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.checkUsername` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.clearRecentEmojiStatuses` | no | yes | no | no | interactive | implemented | ok |
| `account.confirmBotConnection` | yes | yes | no | no | interactive | implemented | ok |
| `account.createTheme` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.deleteSecureValue` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.disablePeerConnectedBot` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.getAccountTTL` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.getAllSecureValues` | yes | yes | no | yes | interactive | implemented | ok |
| `account.getAuthorizationForm` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.getAuthorizations` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.getChannelDefaultEmojiStatuses` | yes | yes | no | yes | interactive | implemented | ok |
| `account.getChannelRestrictedStatusEmojis` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.getChatThemes` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.getCollectibleEmojiStatuses` | yes | no | yes | yes | interactive | implemented | ok |
| `account.getConnectedBots` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.getContentSettings` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.getDefaultBackgroundEmojis` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.getDefaultEmojiStatuses` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.getDefaultGroupPhotoEmojis` | yes | no | yes | yes | interactive | implemented | ok |
| `account.getDefaultProfilePhotoEmojis` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.getMultiWallPapers` | no | yes | no | no | interactive | implemented | ok |
| `account.getNotifyExceptions` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.getPrivacy` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.getRecentEmojiStatuses` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.getSavedMusicIds` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.getSavedRingtones` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.getTheme` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.getThemes` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.getUniqueGiftChatThemes` | yes | no | yes | yes | interactive | implemented | ok |
| `account.getWallPaper` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.getWallPapers` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.getWebAuthorizations` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.installTheme` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.installWallPaper` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.reportPeer` | yes | yes | no | yes | interactive | implemented | ok |
| `account.reportProfilePhoto` | yes | yes | yes | no | interactive | implemented | ok |
| `account.resetAuthorization` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.resetWallPapers` | yes | yes | no | no | interactive | implemented | ok |
| `account.resetWebAuthorization` | yes | yes | yes | no | interactive | implemented | ok |
| `account.resetWebAuthorizations` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.saveMusic` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.saveRingtone` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.saveSecureValue` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.saveTheme` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.saveWallPaper` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.sendVerifyPhoneCode` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.setAccountTTL` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.setAuthorizationTTL` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.setContentSettings` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.setMainProfileTab` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.setPrivacy` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.toggleConnectedBotPaused` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.toggleSponsoredMessages` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.updateBirthday` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.updateColor` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.updateConnectedBot` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.updateDeviceLocked` | no | no | no | yes | interactive | implemented | ok |
| `account.updateEmojiStatus` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.updatePersonalChannel` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.updateProfile` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.updateTheme` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.updateUsername` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.uploadRingtone` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.uploadTheme` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.uploadWallPaper` | yes | yes | yes | yes | interactive | implemented | ok |
| `account.verifyPhone` | yes | yes | yes | yes | interactive | implemented | ok |

### users

6 called, 6 implemented, 0 stub, 0 routed-unregistered, 0 missing.

| Method | iOS | Android | Desktop | macOS | When | Status | Effect |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `users.getFullUser` | yes | yes | yes | yes | interactive | implemented | ok |
| `users.getRequirementsToContact` | yes | no | yes | yes | interactive | implemented | ok |
| `users.getSavedMusic` | yes | no | yes | yes | interactive | implemented | ok |
| `users.getSavedMusicByID` | yes | no | yes | yes | interactive | implemented | ok |
| `users.getUsers` | yes | yes | yes | yes | interactive | implemented | ok |
| `users.suggestBirthday` | yes | yes | yes | yes | interactive | implemented | ok |

### contacts

27 called, 27 implemented, 0 stub, 0 routed-unregistered, 0 missing.

| Method | iOS | Android | Desktop | macOS | When | Status | Effect |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `contacts.deleteContacts` | yes | yes | yes | yes | startup | implemented | ok |
| `contacts.getBirthdays` | yes | no | yes | yes | startup | implemented | ok |
| `contacts.getContacts` | yes | yes | yes | yes | startup | implemented | ok |
| `contacts.getStatuses` | yes | yes | yes | yes | startup+background | implemented | ok |
| `contacts.importContacts` | yes | yes | yes | yes | startup | implemented | ok |
| `contacts.resetSaved` | yes | yes | no | yes | startup | implemented | ok |
| `contacts.acceptContact` | yes | yes | yes | no | interactive | implemented | ok |
| `contacts.addContact` | yes | yes | yes | yes | interactive | implemented | ok |
| `contacts.block` | yes | yes | yes | no | interactive | implemented | ok |
| `contacts.blockFromReplies` | yes | yes | yes | no | interactive | implemented | ok |
| `contacts.editCloseFriends` | yes | no | no | yes | interactive | implemented | ok |
| `contacts.exportContactToken` | yes | yes | yes | yes | interactive | implemented | ok |
| `contacts.exportNoPaidMessagesToken` | yes | yes | yes | yes | interactive | implemented | ok |
| `contacts.getBlocked` | yes | yes | yes | yes | interactive | implemented | ok |
| `contacts.getSaved` | no | no | yes | no | interactive | implemented | ok |
| `contacts.getSponsoredPeers` | yes | yes | yes | yes | interactive | implemented | ok |
| `contacts.getTopPeers` | yes | yes | yes | yes | interactive | implemented | ok |
| `contacts.importContactToken` | yes | yes | yes | yes | interactive | implemented | ok |
| `contacts.importNoPaidMessagesToken` | yes | yes | yes | yes | interactive | implemented | ok |
| `contacts.resetTopPeerRating` | yes | yes | yes | yes | interactive | implemented | ok |
| `contacts.resolvePhone` | yes | yes | yes | yes | interactive | implemented | ok |
| `contacts.resolveUsername` | yes | yes | yes | yes | interactive | implemented | ok |
| `contacts.search` | yes | yes | yes | yes | interactive | implemented | ok |
| `contacts.setBlocked` | yes | yes | no | yes | interactive | implemented | ok |
| `contacts.toggleTopPeers` | yes | yes | yes | yes | interactive | implemented | ok |
| `contacts.unblock` | yes | yes | yes | no | interactive | implemented | ok |
| `contacts.updateContactNote` | yes | no | yes | yes | interactive | implemented | ok |

### messages

216 called, 215 implemented, 0 stub, 0 routed-unregistered, 1 missing.

| Method | iOS | Android | Desktop | macOS | When | Status | Effect |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `messages.getAvailableReactions` | yes | yes | yes | yes | startup+background | implemented | ok |
| `messages.getDefaultHistoryTTL` | yes | yes | yes | yes | startup+background | implemented | ok |
| `messages.getDialogs` | yes | yes | yes | yes | startup | implemented | ok |
| `messages.getForumTopicsByID` | yes | yes | yes | yes | startup+background | implemented | ok |
| `messages.getMessages` | yes | yes | yes | yes | startup+background | implemented | ok |
| `messages.getPaidReactionPrivacy` | no | yes | yes | no | startup | implemented | ok |
| `messages.getPeerDialogs` | yes | yes | yes | yes | startup+background | implemented | ok |
| `messages.getPinnedDialogs` | yes | yes | yes | yes | startup+background | implemented | ok |
| `messages.getPinnedSavedDialogs` | yes | no | yes | yes | startup+background | implemented | ok |
| `messages.getSavedDialogs` | yes | yes | yes | yes | startup | implemented | ok |
| `messages.getSavedDialogsByID` | yes | yes | yes | yes | startup+background | implemented | ok |
| `messages.receivedMessages` | yes | no | no | yes | startup+background | implemented | ok |
| `messages.receivedQueue` | yes | yes | no | yes | startup+background | implemented | ok |
| `messages.reorderPinnedDialogs` | yes | yes | yes | yes | startup+background | implemented | ok |
| `messages.sendEncryptedMultiMedia` | no | yes | no | no | interactive | missing | error |
| `messages.acceptEncryption` | yes | yes | no | yes | interactive | implemented | ok |
| `messages.acceptUrlAuth` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.addChatUser` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.addPollAnswer` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.appendTodoList` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.checkChatInvite` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.checkHistoryImport` | yes | yes | no | no | interactive | implemented | ok |
| `messages.checkHistoryImportPeer` | yes | yes | no | no | interactive | implemented | ok |
| `messages.checkUrlAuthMatchCode` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.clearAllDrafts` | no | yes | no | no | interactive | implemented | ok |
| `messages.clearRecentReactions` | yes | yes | no | yes | interactive | implemented | ok |
| `messages.clickSponsoredMessage` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.composeMessageWithAI` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.composeRichMessageWithAI` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.createChat` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.createForumTopic` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.declineUrlAuth` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.deleteChat` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.deleteChatUser` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.deleteExportedChatInvite` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.deleteFactCheck` | yes | no | yes | yes | interactive | implemented | ok |
| `messages.deleteHistory` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.deleteMessages` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.deleteParticipantReaction` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.deleteParticipantReactions` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.deletePhoneCallHistory` | yes | yes | yes | no | interactive | implemented | ok |
| `messages.deletePollAnswer` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.deleteQuickReplyMessages` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.deleteQuickReplyShortcut` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.deleteRevokedExportedChatInvites` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.deleteSavedHistory` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.deleteScheduledMessages` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.deleteTopicHistory` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.discardEncryption` | yes | yes | no | yes | interactive | implemented | ok |
| `messages.editChatAbout` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.editChatAdmin` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.editChatCreator` | yes | no | yes | yes | interactive | implemented | ok |
| `messages.editChatDefaultBannedRights` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.editChatParticipantRank` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.editChatPhoto` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.editChatTitle` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.editExportedChatInvite` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.editFactCheck` | yes | no | yes | yes | interactive | implemented | ok |
| `messages.editForumTopic` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.editMessage` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.editQuickReplyShortcut` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.exportChatInvite` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.forwardMessages` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getAdminsWithInvites` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getAllDrafts` | yes | yes | no | yes | interactive | implemented | ok |
| `messages.getAttachMenuBot` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getAttachMenuBots` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getAvailableEffects` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getBotApp` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getBotCallbackAnswer` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getChatInviteImporters` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getChats` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getCommonChats` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getCustomEmojiDocuments` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getDefaultTagReactions` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getDhConfig` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getDialogUnreadMarks` | yes | yes | no | no | interactive | implemented | ok |
| `messages.getDiscussionMessage` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getEmojiGameInfo` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getEmojiGroups` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getEmojiKeywords` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getEmojiKeywordsDifference` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getEmojiKeywordsLanguages` | no | no | yes | no | interactive | implemented | ok |
| `messages.getEmojiProfilePhotoGroups` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getEmojiStatusGroups` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getEmojiURL` | no | yes | no | no | interactive | implemented | ok |
| `messages.getExportedChatInvite` | no | yes | no | no | interactive | implemented | ok |
| `messages.getExportedChatInvites` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getExtendedMedia` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getFactCheck` | yes | no | yes | yes | interactive | implemented | ok |
| `messages.getForumTopics` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getFullChat` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getFutureChatCreatorAfterLeave` | yes | no | yes | yes | interactive | implemented | ok |
| `messages.getHistory` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getInlineBotResults` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getMessageEditData` | no | yes | no | no | interactive | implemented | ok |
| `messages.getMessageReactionsList` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getMessageReadParticipants` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getMessagesReactions` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getMessagesViews` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getOnlines` | yes | yes | no | yes | interactive | implemented | ok |
| `messages.getOutboxReadDate` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getPeerSettings` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getPollResults` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getPollVotes` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getPreparedInlineMessage` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getQuickReplies` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getQuickReplyMessages` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getRecentLocations` | yes | yes | no | yes | interactive | implemented | ok |
| `messages.getRecentReactions` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getReplies` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getRichMessage` | yes | no | yes | yes | interactive | implemented | ok |
| `messages.getSavedHistory` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getSavedReactionTags` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getScheduledHistory` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getScheduledMessages` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getSearchCounters` | no | yes | no | no | interactive | implemented | ok |
| `messages.getSearchResultsCalendar` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getSearchResultsPositions` | yes | yes | no | no | interactive | implemented | ok |
| `messages.getShareThemeBackdrops` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getSplitRanges` | no | no | yes | no | interactive | implemented | ok |
| `messages.getSponsoredMessages` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getStatsURL` | no | yes | no | no | interactive | implemented | ok |
| `messages.getTopReactions` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getUnreadMentions` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getUnreadPollVotes` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getUnreadReactions` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getWebPage` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getWebPagePreview` | yes | no | yes | yes | interactive | implemented | ok |
| `messages.hideAllChatJoinRequests` | yes | no | no | yes | interactive | implemented | ok |
| `messages.hideChatJoinRequest` | yes | yes | yes | no | interactive | implemented | ok |
| `messages.hidePeerSettingsBar` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.importChatInvite` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.initHistoryImport` | yes | yes | no | no | interactive | implemented | ok |
| `messages.markDialogUnread` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.migrateChat` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.prolongWebView` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.rateTranscribedAudio` | yes | yes | yes | no | interactive | implemented | ok |
| `messages.readDiscussion` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.readEncryptedHistory` | yes | yes | no | yes | interactive | implemented | ok |
| `messages.readHistory` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.readMentions` | no | yes | yes | no | interactive | implemented | ok |
| `messages.readMessageContents` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.readPollVotes` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.readReactions` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.readSavedHistory` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.reorderPinnedForumTopics` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.reorderPinnedSavedDialogs` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.reorderQuickReplies` | yes | yes | no | yes | interactive | implemented | ok |
| `messages.report` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.reportEncryptedSpam` | yes | yes | no | yes | interactive | implemented | ok |
| `messages.reportMessagesDelivery` | yes | no | yes | yes | interactive | implemented | ok |
| `messages.reportMusicListen` | yes | yes | yes | no | interactive | implemented | ok |
| `messages.reportReaction` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.reportReadMetrics` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.reportSpam` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.reportSponsoredMessage` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.requestAppWebView` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.requestChatJoinWebView` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.requestEncryption` | yes | yes | no | yes | interactive | implemented | ok |
| `messages.requestMainWebView` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.requestSimpleWebView` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.requestUrlAuth` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.requestWebView` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.saveDefaultSendAs` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.saveDraft` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.search` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.searchCustomEmoji` | yes | yes | no | yes | interactive | implemented | ok |
| `messages.searchGlobal` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.searchSentMedia` | yes | no | no | yes | interactive | implemented | ok |
| `messages.sendBotRequestedPeer` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.sendEncrypted` | yes | yes | no | yes | interactive | implemented | ok |
| `messages.sendEncryptedFile` | yes | yes | no | yes | interactive | implemented | ok |
| `messages.sendEncryptedService` | yes | yes | no | yes | interactive | implemented | ok |
| `messages.sendInlineBotResult` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.sendMedia` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.sendMessage` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.sendMultiMedia` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.sendPaidReaction` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.sendQuickReplyMessages` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.sendReaction` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.sendScheduledMessages` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.sendScreenshotNotification` | yes | yes | no | yes | interactive | implemented | ok |
| `messages.sendVote` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.sendWebViewData` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.setChatAvailableReactions` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.setChatTheme` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.setChatWallPaper` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.setDefaultHistoryTTL` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.setDefaultReaction` | yes | no | yes | yes | interactive | implemented | ok |
| `messages.setEncryptedTyping` | yes | yes | no | yes | interactive | implemented | ok |
| `messages.setHistoryTTL` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.setShareTheme` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.setTyping` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.startBot` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.startHistoryImport` | yes | yes | no | no | interactive | implemented | ok |
| `messages.summarizeText` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.toggleBotInAttachMenu` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.toggleDialogPin` | no | yes | yes | no | interactive | implemented | ok |
| `messages.toggleNoForwards` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.togglePaidReactionPrivacy` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.togglePeerTranslations` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.toggleSavedDialogPin` | no | no | yes | no | interactive | implemented | ok |
| `messages.toggleSuggestedPostApproval` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.toggleTodoCompleted` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.transcribeAudio` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.translateRichMessage` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.translateText` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.unpinAllMessages` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.updatePinnedForumTopic` | no | yes | yes | no | interactive | implemented | ok |
| `messages.updatePinnedMessage` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.updateSavedReactionTag` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.uploadEncryptedFile` | yes | no | no | yes | interactive | implemented | ok |
| `messages.uploadImportedMedia` | yes | yes | no | no | interactive | implemented | ok |
| `messages.uploadMedia` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.viewSponsoredMessage` | yes | yes | yes | yes | interactive | implemented | ok |

### updates

3 called, 3 implemented, 0 stub, 0 routed-unregistered, 0 missing.

| Method | iOS | Android | Desktop | macOS | When | Status | Effect |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `updates.getChannelDifference` | yes | yes | yes | yes | startup+background | implemented | ok |
| `updates.getDifference` | yes | yes | yes | yes | startup+background | implemented | ok |
| `updates.getState` | yes | yes | yes | yes | startup+background | implemented | ok |

### photos

5 called, 5 implemented, 0 stub, 0 routed-unregistered, 0 missing.

| Method | iOS | Android | Desktop | macOS | When | Status | Effect |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `photos.deletePhotos` | yes | yes | yes | yes | interactive | implemented | ok |
| `photos.getUserPhotos` | yes | yes | yes | yes | interactive | implemented | ok |
| `photos.updateProfilePhoto` | yes | yes | yes | yes | interactive | implemented | ok |
| `photos.uploadContactProfilePhoto` | yes | yes | yes | yes | interactive | implemented | ok |
| `photos.uploadProfilePhoto` | yes | yes | yes | yes | interactive | implemented | ok |

### upload

7 called, 7 implemented, 0 stub, 0 routed-unregistered, 0 missing.

| Method | iOS | Android | Desktop | macOS | When | Status | Effect |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `upload.getCdnFile` | yes | yes | yes | yes | interactive | implemented | ok |
| `upload.getCdnFileHashes` | yes | yes | yes | yes | interactive | implemented | ok |
| `upload.getFile` | yes | yes | yes | yes | interactive | implemented | ok |
| `upload.getWebFile` | yes | yes | yes | yes | interactive | implemented | ok |
| `upload.reuploadCdnFile` | yes | yes | yes | yes | interactive | implemented | ok |
| `upload.saveBigFilePart` | yes | yes | yes | yes | interactive | implemented | ok |
| `upload.saveFilePart` | yes | yes | yes | yes | interactive | implemented | ok |

### help

25 called, 22 implemented, 0 stub, 0 routed-unregistered, 3 missing.

| Method | iOS | Android | Desktop | macOS | When | Status | Effect |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `help.acceptTermsOfService` | yes | yes | yes | yes | startup | implemented | ok |
| `help.dismissSuggestion` | yes | yes | yes | yes | startup | implemented | ok |
| `help.getAppConfig` | yes | yes | yes | yes | startup+background | implemented | ok |
| `help.getConfig` | yes | yes | yes | yes | startup+background | implemented | ok |
| `help.getCountriesList` | yes | yes | yes | yes | startup+background | implemented | ok |
| `help.getNearestDc` | no | yes | yes | no | startup | implemented | ok |
| `help.getPeerColors` | yes | yes | yes | yes | startup+background | implemented | ok |
| `help.getPeerProfileColors` | yes | yes | yes | yes | startup+background | implemented | ok |
| `help.getPremiumPromo` | yes | yes | yes | yes | startup+background | implemented | ok |
| `help.getPromoData` | yes | yes | yes | yes | startup+background | implemented | ok |
| `help.getTermsOfServiceUpdate` | yes | yes | yes | yes | startup | implemented | ok |
| `help.hidePromoData` | yes | yes | yes | yes | startup+background | implemented | ok |
| `help.editUserInfo` | no | no | yes | no | interactive | missing | error |
| `help.getTimezonesList` | yes | yes | yes | yes | interactive | missing | error |
| `help.getUserInfo` | no | no | yes | no | interactive | missing | error |
| `help.getAppChangelog` | no | no | yes | no | interactive | implemented | ok |
| `help.getAppUpdate` | yes | yes | no | yes | interactive | implemented | ok |
| `help.getCdnConfig` | yes | yes | yes | yes | interactive | implemented | ok |
| `help.getDeepLinkInfo` | yes | yes | yes | yes | interactive | implemented | ok |
| `help.getInviteText` | no | yes | no | no | interactive | implemented | ok |
| `help.getPassportConfig` | yes | yes | yes | yes | interactive | implemented | ok |
| `help.getRecentMeUrls` | no | yes | no | no | interactive | implemented | ok |
| `help.getSupport` | yes | yes | yes | yes | interactive | implemented | ok |
| `help.getSupportName` | no | no | yes | no | interactive | implemented | ok |
| `help.saveAppLog` | yes | yes | yes | yes | interactive | implemented | ok |

### channels

59 called, 58 implemented, 0 stub, 0 routed-unregistered, 1 missing.

| Method | iOS | Android | Desktop | macOS | When | Status | Effect |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `channels.getMessages` | yes | yes | yes | yes | startup+background | implemented | ok |
| `channels.reorderProfileTabs` | no | yes | no | no | interactive | missing | error |
| `channels.checkSearchPostsFlood` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.checkUsername` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.convertToGigagroup` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.createChannel` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.deactivateAllUsernames` | yes | yes | yes | no | interactive | implemented | ok |
| `channels.deleteChannel` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.deleteHistory` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.deleteMessages` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.deleteParticipantHistory` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.editAdmin` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.editBanned` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.editCreator` | no | yes | no | no | interactive | implemented | ok |
| `channels.editLocation` | no | yes | no | no | interactive | implemented | ok |
| `channels.editPhoto` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.editTitle` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.exportMessageLink` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.getAdminLog` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.getAdminedPublicChannels` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.getChannelRecommendations` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.getChannels` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.getFullChannel` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.getFutureCreatorAfterLeave` | no | yes | no | no | interactive | implemented | ok |
| `channels.getGroupsForDiscussion` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.getInactiveChannels` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.getLeftChannels` | no | no | yes | no | interactive | implemented | ok |
| `channels.getMessageAuthor` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.getParticipant` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.getParticipants` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.getSendAs` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.inviteToChannel` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.joinChannel` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.leaveChannel` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.readHistory` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.readMessageContents` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.reorderUsernames` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.reportAntiSpamFalsePositive` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.reportSpam` | no | yes | yes | no | interactive | implemented | ok |
| `channels.restrictSponsoredMessages` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.searchPosts` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.setBoostsToUnblockRestrictions` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.setDiscussionGroup` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.setMainProfileTab` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.toggleAntiSpam` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.toggleAutotranslation` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.toggleForum` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.toggleJoinRequest` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.toggleJoinToSend` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.toggleParticipantsHidden` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.togglePreHistoryHidden` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.toggleSignatures` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.toggleSlowMode` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.toggleUsername` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.toggleViewForumAsMessages` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.updateColor` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.updateEmojiStatus` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.updatePaidMessagesPrice` | yes | no | yes | yes | interactive | implemented | ok |
| `channels.updateUsername` | yes | yes | yes | yes | interactive | implemented | ok |

### stickers

39 called, 20 implemented, 4 stub, 14 routed-unregistered, 1 missing.

| Method | iOS | Android | Desktop | macOS | When | Status | Effect |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `wallet.setCardStickers` | yes | yes | yes | no | interactive | missing | error |
| `messages.getArchivedStickers` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `messages.getAttachedStickers` | yes | yes | yes | no | interactive | routed-but-unregistered | error |
| `messages.getMyStickers` | yes | yes | no | yes | interactive | routed-but-unregistered | error |
| `messages.reorderStickerSets` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `messages.toggleStickerSets` | yes | yes | no | yes | interactive | routed-but-unregistered | error |
| `stickers.addStickerToSet` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `stickers.changeStickerPosition` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `stickers.checkShortName` | yes | yes | no | no | interactive | routed-but-unregistered | error |
| `stickers.createStickerSet` | yes | yes | no | yes | interactive | routed-but-unregistered | error |
| `stickers.deleteStickerSet` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `stickers.removeStickerFromSet` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `stickers.renameStickerSet` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `stickers.replaceSticker` | yes | yes | no | yes | interactive | routed-but-unregistered | error |
| `stickers.suggestShortName` | yes | yes | no | no | interactive | routed-but-unregistered | error |
| `messages.getMaskStickers` | yes | yes | yes | yes | interactive | stub | empty |
| `messages.getSavedGifs` | yes | yes | yes | yes | interactive | stub | empty |
| `messages.readFeaturedStickers` | yes | yes | yes | yes | interactive | stub | bool |
| `messages.saveGif` | yes | yes | yes | yes | interactive | stub | empty |
| `channels.setEmojiStickers` | yes | yes | yes | yes | interactive | implemented | ok |
| `channels.setStickers` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.clearRecentStickers` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.faveSticker` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getAllStickers` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getEmojiStickerGroups` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getEmojiStickers` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getFavedStickers` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getFeaturedEmojiStickers` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getFeaturedStickers` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getOldFeaturedStickers` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getRecentStickers` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getStickerSet` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.getStickers` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.installStickerSet` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.saveRecentSticker` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.searchEmojiStickerSets` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.searchStickerSets` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.searchStickers` | yes | yes | yes | yes | interactive | implemented | ok |
| `messages.uninstallStickerSet` | yes | yes | yes | yes | interactive | implemented | ok |

### phone/calls

43 called, 10 implemented, 0 stub, 0 routed-unregistered, 33 missing.

| Method | iOS | Android | Desktop | macOS | When | Status | Effect |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `phone.checkGroupCall` | yes | yes | yes | yes | interactive | missing | error |
| `phone.createConferenceCall` | yes | yes | yes | yes | interactive | missing | error |
| `phone.createGroupCall` | yes | yes | yes | yes | interactive | missing | error |
| `phone.declineConferenceCallInvite` | yes | yes | yes | yes | interactive | missing | error |
| `phone.deleteConferenceCallParticipants` | yes | yes | yes | yes | interactive | missing | error |
| `phone.deleteGroupCallMessages` | yes | yes | yes | yes | interactive | missing | error |
| `phone.deleteGroupCallParticipantMessages` | yes | yes | yes | yes | interactive | missing | error |
| `phone.discardGroupCall` | yes | yes | yes | yes | interactive | missing | error |
| `phone.editGroupCallParticipant` | yes | yes | yes | yes | interactive | missing | error |
| `phone.editGroupCallTitle` | yes | yes | yes | yes | interactive | missing | error |
| `phone.exportGroupCallInvite` | yes | yes | yes | yes | interactive | missing | error |
| `phone.getGroupCall` | yes | yes | yes | yes | interactive | missing | error |
| `phone.getGroupCallChainBlocks` | yes | yes | yes | yes | interactive | missing | error |
| `phone.getGroupCallJoinAs` | yes | yes | yes | yes | interactive | missing | error |
| `phone.getGroupCallStars` | yes | yes | yes | no | interactive | missing | error |
| `phone.getGroupCallStreamChannels` | yes | yes | yes | no | interactive | missing | error |
| `phone.getGroupCallStreamRtmpUrl` | yes | yes | yes | yes | interactive | missing | error |
| `phone.getGroupParticipants` | yes | yes | yes | yes | interactive | missing | error |
| `phone.inviteConferenceCallParticipant` | yes | yes | yes | yes | interactive | missing | error |
| `phone.inviteToGroupCall` | yes | yes | yes | yes | interactive | missing | error |
| `phone.joinGroupCall` | yes | yes | yes | yes | interactive | missing | error |
| `phone.joinGroupCallPresentation` | yes | yes | yes | yes | interactive | missing | error |
| `phone.leaveGroupCall` | yes | yes | yes | yes | interactive | missing | error |
| `phone.leaveGroupCallPresentation` | yes | yes | yes | yes | interactive | missing | error |
| `phone.saveDefaultGroupCallJoinAs` | yes | yes | yes | yes | interactive | missing | error |
| `phone.saveDefaultSendAs` | no | yes | yes | no | interactive | missing | error |
| `phone.sendConferenceCallBroadcast` | yes | yes | yes | yes | interactive | missing | error |
| `phone.sendGroupCallEncryptedMessage` | yes | yes | yes | no | interactive | missing | error |
| `phone.sendGroupCallMessage` | yes | yes | yes | yes | interactive | missing | error |
| `phone.startScheduledGroupCall` | yes | yes | yes | yes | interactive | missing | error |
| `phone.toggleGroupCallRecord` | yes | yes | yes | yes | interactive | missing | error |
| `phone.toggleGroupCallSettings` | yes | yes | yes | yes | interactive | missing | error |
| `phone.toggleGroupCallStartSubscription` | yes | yes | yes | yes | interactive | missing | error |
| `phone.acceptCall` | yes | yes | yes | yes | interactive | implemented | ok |
| `phone.confirmCall` | yes | yes | yes | yes | interactive | implemented | ok |
| `phone.discardCall` | yes | yes | yes | yes | interactive | implemented | ok |
| `phone.getCallConfig` | yes | yes | yes | yes | interactive | implemented | ok |
| `phone.receivedCall` | yes | yes | yes | yes | interactive | implemented | ok |
| `phone.requestCall` | yes | yes | yes | yes | interactive | implemented | ok |
| `phone.saveCallDebug` | yes | yes | yes | yes | interactive | implemented | ok |
| `phone.saveCallLog` | yes | yes | no | no | interactive | implemented | ok |
| `phone.sendSignalingData` | yes | yes | yes | yes | interactive | implemented | ok |
| `phone.setCallRating` | yes | yes | yes | yes | interactive | implemented | ok |

### langpack

5 called, 5 implemented, 0 stub, 0 routed-unregistered, 0 missing.

| Method | iOS | Android | Desktop | macOS | When | Status | Effect |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `langpack.getDifference` | yes | yes | yes | yes | startup+background | implemented | ok |
| `langpack.getLangPack` | yes | yes | yes | yes | startup+background | implemented | ok |
| `langpack.getLanguage` | yes | yes | yes | yes | startup+background | implemented | ok |
| `langpack.getLanguages` | yes | yes | yes | yes | startup+background | implemented | ok |
| `langpack.getStrings` | yes | yes | yes | yes | startup+background | implemented | ok |

### folders

17 called, 5 implemented, 0 stub, 12 routed-unregistered, 0 missing.

| Method | iOS | Android | Desktop | macOS | When | Status | Effect |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `messages.getDialogFilters` | yes | yes | yes | yes | startup | implemented | ok |
| `messages.getSuggestedDialogFilters` | yes | yes | yes | yes | startup | implemented | ok |
| `messages.toggleDialogFilterTags` | yes | yes | yes | yes | startup | implemented | ok |
| `messages.updateDialogFilter` | yes | yes | yes | yes | startup | implemented | ok |
| `messages.updateDialogFiltersOrder` | yes | yes | yes | yes | startup | implemented | ok |
| `chatlists.checkChatlistInvite` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `chatlists.deleteExportedInvite` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `chatlists.editExportedInvite` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `chatlists.exportChatlistInvite` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `chatlists.getChatlistUpdates` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `chatlists.getExportedInvites` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `chatlists.getLeaveChatlistSuggestions` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `chatlists.hideChatlistUpdates` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `chatlists.joinChatlistInvite` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `chatlists.joinChatlistUpdates` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `chatlists.leaveChatlist` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `folders.editPeerFolders` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |

### stats

8 called, 2 implemented, 0 stub, 6 routed-unregistered, 0 missing.

| Method | iOS | Android | Desktop | macOS | When | Status | Effect |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `stats.getMessagePublicForwards` | yes | no | yes | yes | interactive | routed-but-unregistered | error |
| `stats.getMessageStats` | yes | no | yes | no | interactive | routed-but-unregistered | error |
| `stats.getPollStats` | yes | no | yes | no | interactive | routed-but-unregistered | error |
| `stats.getStoryPublicForwards` | yes | no | yes | yes | interactive | routed-but-unregistered | error |
| `stats.getStoryStats` | yes | yes | yes | no | interactive | routed-but-unregistered | error |
| `stats.loadAsyncGraph` | yes | no | yes | yes | interactive | routed-but-unregistered | error |
| `stats.getBroadcastStats` | yes | no | yes | no | interactive | implemented | ok |
| `stats.getMegagroupStats` | yes | no | yes | no | interactive | implemented | ok |

### stories

32 called, 0 implemented, 0 stub, 0 routed-unregistered, 32 missing.

| Method | iOS | Android | Desktop | macOS | When | Status | Effect |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `stories.activateStealthMode` | yes | yes | yes | yes | interactive | missing | error |
| `stories.canSendStory` | yes | yes | no | yes | interactive | missing | error |
| `stories.createAlbum` | yes | no | yes | yes | interactive | missing | error |
| `stories.deleteAlbum` | yes | no | yes | yes | interactive | missing | error |
| `stories.deleteStories` | yes | yes | yes | yes | interactive | missing | error |
| `stories.editStory` | yes | yes | no | yes | interactive | missing | error |
| `stories.exportStoryLink` | yes | yes | yes | yes | interactive | missing | error |
| `stories.getAlbumStories` | yes | yes | yes | yes | interactive | missing | error |
| `stories.getAlbums` | yes | no | yes | yes | interactive | missing | error |
| `stories.getAllReadPeerStories` | yes | yes | yes | yes | interactive | missing | error |
| `stories.getAllStories` | yes | yes | yes | yes | interactive | missing | error |
| `stories.getChatsToSend` | yes | yes | yes | yes | interactive | missing | error |
| `stories.getPeerMaxIDs` | yes | yes | no | yes | interactive | missing | error |
| `stories.getPeerStories` | yes | yes | yes | yes | interactive | missing | error |
| `stories.getPinnedStories` | yes | yes | yes | yes | interactive | missing | error |
| `stories.getStoriesArchive` | yes | yes | yes | yes | interactive | missing | error |
| `stories.getStoriesByID` | yes | yes | yes | yes | interactive | missing | error |
| `stories.getStoriesViews` | yes | yes | yes | yes | interactive | missing | error |
| `stories.getStoryReactionsList` | yes | no | yes | yes | interactive | missing | error |
| `stories.getStoryViewsList` | yes | yes | yes | yes | interactive | missing | error |
| `stories.incrementStoryViews` | yes | yes | yes | yes | interactive | missing | error |
| `stories.readStories` | yes | yes | yes | yes | interactive | missing | error |
| `stories.reorderAlbums` | yes | no | yes | yes | interactive | missing | error |
| `stories.report` | yes | yes | yes | yes | interactive | missing | error |
| `stories.searchPosts` | yes | yes | no | yes | interactive | missing | error |
| `stories.sendReaction` | yes | yes | yes | yes | interactive | missing | error |
| `stories.sendStory` | yes | yes | no | no | interactive | missing | error |
| `stories.startLive` | yes | no | no | no | interactive | missing | error |
| `stories.togglePeerStoriesHidden` | yes | yes | yes | yes | interactive | missing | error |
| `stories.togglePinned` | yes | yes | yes | yes | interactive | missing | error |
| `stories.togglePinnedToTop` | yes | no | yes | yes | interactive | missing | error |
| `stories.updateAlbum` | yes | no | yes | yes | interactive | missing | error |

### premium/payments

100 called, 1 implemented, 2 stub, 17 routed-unregistered, 80 missing.

| Method | iOS | Android | Desktop | macOS | When | Status | Effect |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `fragment.getCollectibleInfo` | yes | no | yes | yes | interactive | missing | error |
| `payments.applyGiftCode` | yes | yes | yes | yes | interactive | missing | error |
| `payments.changeStarsSubscription` | yes | no | yes | yes | interactive | missing | error |
| `payments.checkCanSendGift` | yes | no | yes | yes | interactive | missing | error |
| `payments.checkGiftCode` | yes | yes | yes | yes | interactive | missing | error |
| `payments.connectStarRefBot` | yes | yes | yes | yes | interactive | missing | error |
| `payments.convertStarGift` | yes | no | yes | yes | interactive | missing | error |
| `payments.craftStarGift` | yes | no | yes | no | interactive | missing | error |
| `payments.createStarGiftCollection` | yes | no | yes | yes | interactive | missing | error |
| `payments.deleteStarGift` | yes | no | yes | yes | interactive | missing | error |
| `payments.deleteStarGiftCollection` | yes | no | yes | yes | interactive | missing | error |
| `payments.depositStarGift` | yes | no | yes | yes | interactive | missing | error |
| `payments.displayStarGift` | yes | no | yes | yes | interactive | missing | error |
| `payments.editConnectedStarRefBot` | yes | yes | yes | no | interactive | missing | error |
| `payments.fulfillStarsSubscription` | yes | no | no | yes | interactive | missing | error |
| `payments.getConnectedStarRefBot` | yes | yes | yes | no | interactive | missing | error |
| `payments.getConnectedStarRefBots` | yes | yes | yes | yes | interactive | missing | error |
| `payments.getCraftStarGifts` | yes | no | yes | yes | interactive | missing | error |
| `payments.getGiveawayInfo` | yes | yes | yes | yes | interactive | missing | error |
| `payments.getPremiumGiftCodeOptions` | yes | yes | yes | yes | interactive | missing | error |
| `payments.getResaleStarGifts` | yes | no | yes | yes | interactive | missing | error |
| `payments.getSavedStarGift` | yes | no | yes | yes | interactive | missing | error |
| `payments.getSavedStarGifts` | yes | no | yes | yes | interactive | missing | error |
| `payments.getStarGiftActiveAuctions` | yes | no | yes | yes | interactive | missing | error |
| `payments.getStarGiftAuctionAcquiredGifts` | yes | no | yes | yes | interactive | missing | error |
| `payments.getStarGiftAuctionState` | yes | no | yes | yes | interactive | missing | error |
| `payments.getStarGiftCollections` | yes | no | yes | yes | interactive | missing | error |
| `payments.getStarGiftUpgradeAttributes` | yes | no | yes | yes | interactive | missing | error |
| `payments.getStarGiftUpgradePreview` | yes | no | yes | yes | interactive | missing | error |
| `payments.getStarGiftWithdrawalUrl` | yes | no | yes | yes | interactive | missing | error |
| `payments.getStarGifts` | yes | no | yes | yes | interactive | missing | error |
| `payments.getStarsGiftOptions` | yes | yes | yes | yes | interactive | missing | error |
| `payments.getStarsGiveawayOptions` | yes | yes | yes | yes | interactive | missing | error |
| `payments.getStarsRevenueAdsAccountUrl` | yes | yes | yes | yes | interactive | missing | error |
| `payments.getStarsRevenueStats` | yes | yes | yes | no | interactive | missing | error |
| `payments.getStarsRevenueWithdrawalUrl` | yes | yes | yes | yes | interactive | missing | error |
| `payments.getStarsStatus` | yes | yes | yes | yes | interactive | missing | error |
| `payments.getStarsSubscriptions` | yes | no | yes | yes | interactive | missing | error |
| `payments.getStarsTopupOptions` | yes | yes | yes | yes | interactive | missing | error |
| `payments.getStarsTransactions` | yes | yes | yes | yes | interactive | missing | error |
| `payments.getStarsTransactionsByID` | yes | no | no | yes | interactive | missing | error |
| `payments.getSuggestedStarRefBots` | yes | yes | yes | yes | interactive | missing | error |
| `payments.getUniqueStarGift` | yes | no | yes | yes | interactive | missing | error |
| `payments.getUniqueStarGiftValueInfo` | yes | no | yes | yes | interactive | missing | error |
| `payments.launchPrepaidGiveaway` | yes | yes | yes | yes | interactive | missing | error |
| `payments.reorderStarGiftCollections` | yes | no | yes | yes | interactive | missing | error |
| `payments.resolveStarGiftOffer` | yes | no | yes | yes | interactive | missing | error |
| `payments.saveStarGift` | yes | no | yes | yes | interactive | missing | error |
| `payments.sendStarGiftOffer` | yes | no | yes | yes | interactive | missing | error |
| `payments.sendStarsForm` | yes | yes | yes | yes | interactive | missing | error |
| `payments.toggleChatStarGiftNotifications` | yes | no | yes | yes | interactive | missing | error |
| `payments.toggleStarGiftsPinnedToTop` | yes | no | yes | yes | interactive | missing | error |
| `payments.transferStarGift` | yes | no | yes | yes | interactive | missing | error |
| `payments.updateStarGiftCollection` | yes | no | yes | yes | interactive | missing | error |
| `payments.updateStarGiftPrice` | yes | no | yes | yes | interactive | missing | error |
| `payments.upgradeStarGift` | yes | no | yes | yes | interactive | missing | error |
| `payments.withdrawStarGift` | yes | no | yes | yes | interactive | missing | error |
| `stars.checkCanSendGift` | no | yes | no | no | interactive | missing | error |
| `stars.convertStarGift` | no | yes | no | no | interactive | missing | error |
| `stars.createStarGiftCollection` | no | yes | no | no | interactive | missing | error |
| `stars.deleteStarGift` | no | yes | no | no | interactive | missing | error |
| `stars.deleteStarGiftCollection` | no | yes | no | no | interactive | missing | error |
| `stars.getCraftStarGifts` | no | yes | no | no | interactive | missing | error |
| `stars.getResaleStarGifts` | no | yes | no | no | interactive | missing | error |
| `stars.getSavedStarGift` | no | yes | no | no | interactive | missing | error |
| `stars.getSavedStarGifts` | no | yes | no | no | interactive | missing | error |
| `stars.getStarGiftCollections` | no | yes | no | no | interactive | missing | error |
| `stars.getStarGiftUpgradeAttributes` | no | yes | no | no | interactive | missing | error |
| `stars.getStarGiftUpgradePreview` | no | yes | no | no | interactive | missing | error |
| `stars.getStarGiftWithdrawalUrl` | no | yes | no | no | interactive | missing | error |
| `stars.getStarGifts` | no | yes | no | no | interactive | missing | error |
| `stars.getUniqueStarGift` | no | yes | no | no | interactive | missing | error |
| `stars.getUniqueStarGiftValueInfo` | no | yes | no | no | interactive | missing | error |
| `stars.reorderStarGiftCollections` | no | yes | no | no | interactive | missing | error |
| `stars.saveStarGift` | no | yes | no | no | interactive | missing | error |
| `stars.toggleChatStarGiftNotifications` | no | yes | no | no | interactive | missing | error |
| `stars.toggleStarGiftsPinnedToTop` | no | yes | no | no | interactive | missing | error |
| `stars.updatePaidMessagesPrice` | no | yes | no | no | interactive | missing | error |
| `stars.updateStarGiftCollection` | no | yes | no | no | interactive | missing | error |
| `stars.updateStarGiftPrice` | no | yes | no | no | interactive | missing | error |
| `payments.clearSavedInfo` | yes | yes | yes | no | interactive | routed-but-unregistered | error |
| `payments.createOnrampSession` | yes | no | yes | yes | interactive | routed-but-unregistered | error |
| `payments.getBankCardData` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `payments.getCurrencyRates` | yes | no | yes | no | interactive | routed-but-unregistered | error |
| `payments.getOnrampAvailability` | yes | no | no | no | interactive | routed-but-unregistered | error |
| `payments.getOnrampBaseCurrencies` | yes | no | yes | no | interactive | routed-but-unregistered | error |
| `payments.getOnrampLimits` | yes | no | no | no | interactive | routed-but-unregistered | error |
| `payments.getOnrampProviders` | yes | no | yes | yes | interactive | routed-but-unregistered | error |
| `payments.getOnrampQuote` | yes | no | no | no | interactive | routed-but-unregistered | error |
| `payments.getPaymentForm` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `payments.getPaymentReceipt` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `payments.sendPaymentForm` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `payments.validateRequestedInfo` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `premium.applyBoost` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `premium.getBoostsList` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `premium.getBoostsStatus` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `premium.getMyBoosts` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `payments.assignAppStoreTransaction` | yes | no | no | yes | interactive | stub | error |
| `payments.assignPlayMarketTransaction` | no | yes | no | no | interactive | stub | error |
| `payments.canPurchaseStore` | yes | yes | no | yes | interactive | implemented | ok |

### bots

23 called, 0 implemented, 0 stub, 15 routed-unregistered, 8 missing.

| Method | iOS | Android | Desktop | macOS | When | Status | Effect |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `bots.checkUsername` | yes | yes | yes | yes | interactive | missing | error |
| `bots.createBot` | yes | yes | yes | yes | interactive | missing | error |
| `bots.getAdminedBots` | yes | yes | yes | yes | interactive | missing | error |
| `bots.getBotInfo` | yes | no | yes | yes | interactive | missing | error |
| `bots.getBotRecommendations` | yes | yes | yes | yes | interactive | missing | error |
| `bots.setBotInfo` | yes | yes | yes | yes | interactive | missing | error |
| `bots.setCustomVerification` | yes | yes | yes | yes | interactive | missing | error |
| `bots.updateStarRefProgram` | yes | yes | yes | yes | interactive | missing | error |
| `bots.addPreviewMedia` | yes | yes | no | no | interactive | routed-but-unregistered | error |
| `bots.allowSendMessage` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `bots.canSendMessage` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `bots.checkDownloadFileParams` | yes | yes | yes | no | interactive | routed-but-unregistered | error |
| `bots.deletePreviewMedia` | yes | yes | no | no | interactive | routed-but-unregistered | error |
| `bots.editPreviewMedia` | no | yes | no | no | interactive | routed-but-unregistered | error |
| `bots.getPopularAppBots` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `bots.getPreviewInfo` | yes | yes | no | yes | interactive | routed-but-unregistered | error |
| `bots.getPreviewMedias` | yes | yes | no | yes | interactive | routed-but-unregistered | error |
| `bots.getRequestedWebViewButton` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `bots.invokeWebViewCustomMethod` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `bots.reorderPreviewMedias` | yes | yes | no | no | interactive | routed-but-unregistered | error |
| `bots.reorderUsernames` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `bots.toggleUserEmojiStatusPermission` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `bots.toggleUsername` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |

### business

10 called, 0 implemented, 0 stub, 0 routed-unregistered, 10 missing.

| Method | iOS | Android | Desktop | macOS | When | Status | Effect |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `account.createBusinessChatLink` | yes | yes | yes | yes | interactive | missing | error |
| `account.deleteBusinessChatLink` | yes | yes | yes | yes | interactive | missing | error |
| `account.editBusinessChatLink` | yes | yes | yes | yes | interactive | missing | error |
| `account.getBusinessChatLinks` | yes | yes | yes | yes | interactive | missing | error |
| `account.resolveBusinessChatLink` | yes | yes | yes | yes | interactive | missing | error |
| `account.updateBusinessAwayMessage` | yes | yes | yes | yes | interactive | missing | error |
| `account.updateBusinessGreetingMessage` | yes | yes | yes | yes | interactive | missing | error |
| `account.updateBusinessIntro` | yes | yes | yes | yes | interactive | missing | error |
| `account.updateBusinessLocation` | yes | yes | yes | yes | interactive | missing | error |
| `account.updateBusinessWorkHours` | yes | yes | yes | yes | interactive | missing | error |

### aicompose

7 called, 0 implemented, 0 stub, 7 routed-unregistered, 0 missing.

| Method | iOS | Android | Desktop | macOS | When | Status | Effect |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `aicompose.createTone` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `aicompose.deleteTone` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `aicompose.getTone` | yes | yes | yes | no | interactive | routed-but-unregistered | error |
| `aicompose.getToneExample` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `aicompose.getTones` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `aicompose.saveTone` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |
| `aicompose.updateTone` | yes | yes | yes | yes | interactive | routed-but-unregistered | error |

### communities

9 called, 0 implemented, 0 stub, 0 routed-unregistered, 9 missing.

| Method | iOS | Android | Desktop | macOS | When | Status | Effect |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `communities.create` | yes | yes | yes | yes | interactive | missing | error |
| `communities.getJoinedCommunities` | yes | yes | yes | yes | interactive | missing | error |
| `communities.getParticipantJoinedChats` | yes | yes | yes | yes | interactive | missing | error |
| `communities.getPeerLinkRequests` | yes | yes | yes | yes | interactive | missing | error |
| `communities.toggleAllPeerLinkRequestApproval` | yes | yes | yes | yes | interactive | missing | error |
| `communities.toggleCommunityCollapsedInDialogs` | yes | yes | yes | yes | interactive | missing | error |
| `communities.toggleParticipantBanned` | yes | yes | yes | yes | interactive | missing | error |
| `communities.togglePeerLink` | yes | yes | yes | yes | interactive | missing | error |
| `communities.togglePeerLinkRequestApproval` | yes | yes | yes | no | interactive | missing | error |

### ephemeral

8 called, 0 implemented, 0 stub, 0 routed-unregistered, 8 missing.

| Method | iOS | Android | Desktop | macOS | When | Status | Effect |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `ephemeral.deleteAllWelcomeMessages` | yes | no | yes | no | interactive | missing | error |
| `ephemeral.deleteMessage` | yes | no | yes | yes | interactive | missing | error |
| `ephemeral.deleteWelcomeMessage` | yes | no | yes | yes | interactive | missing | error |
| `ephemeral.editMessage` | yes | no | yes | yes | interactive | missing | error |
| `ephemeral.getCallbackAnswer` | yes | no | yes | yes | interactive | missing | error |
| `ephemeral.getWelcomeMessages` | yes | no | yes | yes | interactive | missing | error |
| `ephemeral.reportMessage` | yes | no | yes | yes | interactive | missing | error |
| `ephemeral.sendMessage` | yes | no | yes | yes | interactive | missing | error |

### feed

5 called, 0 implemented, 0 stub, 0 routed-unregistered, 5 missing.

| Method | iOS | Android | Desktop | macOS | When | Status | Effect |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `feed.getFeed` | yes | no | no | yes | interactive | missing | error |
| `feed.getForYou` | yes | no | no | yes | interactive | missing | error |
| `feed.getInterestCategories` | yes | no | no | no | interactive | missing | error |
| `feed.readFeed` | yes | no | no | no | interactive | missing | error |
| `feed.setInterestCategories` | yes | no | no | no | interactive | missing | error |

### iv

1 called, 0 implemented, 0 stub, 0 routed-unregistered, 1 missing.

| Method | iOS | Android | Desktop | macOS | When | Status | Effect |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `iv.getRichMessage` | no | yes | no | no | interactive | missing | error |

### toncenter

4 called, 0 implemented, 0 stub, 0 routed-unregistered, 4 missing.

| Method | iOS | Android | Desktop | macOS | When | Status | Effect |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `toncenter.createOnrampSession` | no | yes | no | no | interactive | missing | error |
| `toncenter.getOnrampProviders` | no | yes | no | no | interactive | missing | error |
| `toncenter.getStreamingUrl` | yes | yes | yes | no | interactive | missing | error |
| `toncenter.performApiRequest` | yes | yes | yes | no | interactive | missing | error |

### wallet

26 called, 0 implemented, 0 stub, 0 routed-unregistered, 26 missing.

| Method | iOS | Android | Desktop | macOS | When | Status | Effect |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `wallet.disableBackup` | yes | yes | yes | yes | interactive | missing | error |
| `wallet.enableBackup` | yes | yes | yes | yes | interactive | missing | error |
| `wallet.exportSecretPhrase` | yes | yes | yes | no | interactive | missing | error |
| `wallet.fetchEncryptedSecretPhrasePart` | yes | yes | yes | no | interactive | missing | error |
| `wallet.getBackupHolderDcs` | yes | yes | yes | no | interactive | missing | error |
| `wallet.getCurrencyRates` | no | yes | no | no | interactive | missing | error |
| `wallet.getExistingWaltBalance` | yes | yes | yes | yes | interactive | missing | error |
| `wallet.getGaslessInfo` | yes | yes | yes | no | interactive | missing | error |
| `wallet.getNfts` | yes | yes | yes | no | interactive | missing | error |
| `wallet.getProofChallenge` | yes | yes | yes | no | interactive | missing | error |
| `wallet.getState` | yes | yes | yes | yes | interactive | missing | error |
| `wallet.getTransactions` | yes | yes | yes | no | interactive | missing | error |
| `wallet.getTransactionsByIDs` | yes | yes | yes | no | interactive | missing | error |
| `wallet.getTransactionsByMsgHash` | yes | yes | yes | no | interactive | missing | error |
| `wallet.getUserAddresses` | yes | yes | yes | yes | interactive | missing | error |
| `wallet.replaceWallet` | yes | yes | yes | no | interactive | missing | error |
| `wallet.sendTransfer` | yes | yes | yes | no | interactive | missing | error |
| `wallet.tonConnectClaimRequest` | yes | yes | yes | no | interactive | missing | error |
| `wallet.tonConnectCloseSession` | yes | yes | yes | no | interactive | missing | error |
| `wallet.tonConnectCreateSession` | yes | yes | yes | no | interactive | missing | error |
| `wallet.tonConnectGetPending` | yes | yes | yes | no | interactive | missing | error |
| `wallet.tonConnectGetSessions` | yes | yes | yes | no | interactive | missing | error |
| `wallet.tonConnectNextEventId` | yes | no | yes | no | interactive | missing | error |
| `wallet.tonConnectRegisterKey` | yes | yes | yes | no | interactive | missing | error |
| `wallet.tonConnectSubmitConnectResult` | yes | yes | yes | no | interactive | missing | error |
| `wallet.tonConnectSubmitResponse` | yes | yes | yes | no | interactive | missing | error |

## Notes for non-implemented methods

| Method | Status | Notes |
| --- | --- | --- |
| `account.createBusinessChatLink` | missing | no route in session.yaml and no bff handler |
| `account.deleteBusinessChatLink` | missing | no route in session.yaml and no bff handler |
| `account.deletePasskey` | stub | returns ENTERPRISE_IS_BLOCKED or METHOD_NOT_IMPL app/bff/passkey/internal/core/account.deletePasskey_handler.go |
| `account.deleteWebBrowserSettingsExceptions` | missing | no route in session.yaml; handler source is implemented at app/bff/webbrowser/client/webbrowser_client.go but the service is not routed |
| `account.editBusinessChatLink` | missing | no route in session.yaml and no bff handler |
| `account.finishPasskeyLogin` | missing | no route in session.yaml and no bff handler not in layer schema |
| `account.finishTakeoutSession` | routed-but-unregistered | session.yaml routes RPCTakeout but the bff process has no AccountFinishTakeoutSession handler |
| `account.getBirthdays` | missing | no route in session.yaml and no bff handler not in layer schema |
| `account.getBusinessChatLinks` | missing | no route in session.yaml and no bff handler |
| `account.getPaidMessagesRevenue` | routed-but-unregistered | session.yaml routes RPCPaidMessage but the bff process has no AccountGetPaidMessagesRevenue handler |
| `account.getPasskeys` | stub | returns ENTERPRISE_IS_BLOCKED or METHOD_NOT_IMPL app/bff/passkey/internal/core/account.getPasskeys_handler.go |
| `account.getRequirementsToContact` | missing | no route in session.yaml and no bff handler not in layer schema |
| `account.getWebBrowserSettings` | missing | no route in session.yaml; handler source is implemented at app/bff/webbrowser/client/webbrowser_client.go but the service is not routed |
| `account.getWebPagePreview` | missing | no route in session.yaml and no bff handler not in layer schema |
| `account.initPasskeyLogin` | missing | no route in session.yaml and no bff handler not in layer schema |
| `account.initPasskeyRegistration` | stub | returns ENTERPRISE_IS_BLOCKED or METHOD_NOT_IMPL app/bff/passkey/internal/core/account.initPasskeyRegistration_handler.go |
| `account.initTakeoutSession` | routed-but-unregistered | session.yaml routes RPCTakeout but the bff process has no AccountInitTakeoutSession handler |
| `account.registerPasskey` | stub | returns ENTERPRISE_IS_BLOCKED or METHOD_NOT_IMPL app/bff/passkey/internal/core/account.registerPasskey_handler.go |
| `account.reorderProfileTabs` | missing | no route in session.yaml and no bff handler not in layer schema |
| `account.reorderUsernames` | routed-but-unregistered | session.yaml routes RPCFragment but the bff process has no AccountReorderUsernames handler |
| `account.resolveBusinessChatLink` | missing | no route in session.yaml and no bff handler |
| `account.toggleNoPaidMessagesException` | routed-but-unregistered | session.yaml routes RPCPaidMessage but the bff process has no AccountToggleNoPaidMessagesException handler |
| `account.toggleUsername` | routed-but-unregistered | session.yaml routes RPCFragment but the bff process has no AccountToggleUsername handler |
| `account.toggleWebBrowserSettingsException` | missing | no route in session.yaml; handler source is implemented at app/bff/webbrowser/client/webbrowser_client.go but the service is not routed |
| `account.updateBusinessAwayMessage` | missing | no route in session.yaml and no bff handler |
| `account.updateBusinessGreetingMessage` | missing | no route in session.yaml and no bff handler |
| `account.updateBusinessIntro` | missing | no route in session.yaml and no bff handler |
| `account.updateBusinessLocation` | missing | no route in session.yaml and no bff handler |
| `account.updateBusinessWorkHours` | missing | no route in session.yaml and no bff handler |
| `account.updateWebBrowserSettings` | missing | no route in session.yaml; handler source is implemented at app/bff/webbrowser/client/webbrowser_client.go but the service is not routed |
| `aicompose.createTone` | routed-but-unregistered | session.yaml routes RPCAiComposeTone but the bff process has no AicomposeCreateTone handler |
| `aicompose.deleteTone` | routed-but-unregistered | session.yaml routes RPCAiComposeTone but the bff process has no AicomposeDeleteTone handler |
| `aicompose.getTone` | routed-but-unregistered | session.yaml routes RPCAiComposeTone but the bff process has no AicomposeGetTone handler |
| `aicompose.getToneExample` | routed-but-unregistered | session.yaml routes RPCAiComposeTone but the bff process has no AicomposeGetToneExample handler |
| `aicompose.getTones` | routed-but-unregistered | session.yaml routes RPCAiComposeTone but the bff process has no AicomposeGetTones handler |
| `aicompose.saveTone` | routed-but-unregistered | session.yaml routes RPCAiComposeTone but the bff process has no AicomposeSaveTone handler |
| `aicompose.updateTone` | routed-but-unregistered | session.yaml routes RPCAiComposeTone but the bff process has no AicomposeUpdateTone handler |
| `auth.initPasskeyLogin` | stub | returns ENTERPRISE_IS_BLOCKED or METHOD_NOT_IMPL app/bff/passkey/internal/core/auth.initPasskeyLogin_handler.go |
| `bots.addPreviewMedia` | routed-but-unregistered | session.yaml routes RPCMainMiniBotApps but the bff process has no BotsAddPreviewMedia handler |
| `bots.allowSendMessage` | routed-but-unregistered | session.yaml routes RPCMiniBotApps but the bff process has no BotsAllowSendMessage handler |
| `bots.canSendMessage` | routed-but-unregistered | session.yaml routes RPCMiniBotApps but the bff process has no BotsCanSendMessage handler |
| `bots.checkDownloadFileParams` | routed-but-unregistered | session.yaml routes RPCMiniBotApps but the bff process has no BotsCheckDownloadFileParams handler |
| `bots.checkUsername` | missing | no route in session.yaml (RPCBots is commented out) and no bff handler |
| `bots.createBot` | missing | no route in session.yaml (RPCBots is commented out) and no bff handler |
| `bots.deletePreviewMedia` | routed-but-unregistered | session.yaml routes RPCMainMiniBotApps but the bff process has no BotsDeletePreviewMedia handler |
| `bots.editPreviewMedia` | routed-but-unregistered | session.yaml routes RPCMainMiniBotApps but the bff process has no BotsEditPreviewMedia handler |
| `bots.getAdminedBots` | missing | no route in session.yaml (RPCBots is commented out) and no bff handler |
| `bots.getBotInfo` | missing | no route in session.yaml (RPCBots is commented out) and no bff handler |
| `bots.getBotRecommendations` | missing | no route in session.yaml and no bff handler |
| `bots.getPopularAppBots` | routed-but-unregistered | session.yaml routes RPCMainMiniBotApps but the bff process has no BotsGetPopularAppBots handler |
| `bots.getPreviewInfo` | routed-but-unregistered | session.yaml routes RPCMainMiniBotApps but the bff process has no BotsGetPreviewInfo handler |
| `bots.getPreviewMedias` | routed-but-unregistered | session.yaml routes RPCMainMiniBotApps but the bff process has no BotsGetPreviewMedias handler |
| `bots.getRequestedWebViewButton` | routed-but-unregistered | session.yaml routes RPCMiniBotApps but the bff process has no BotsGetRequestedWebViewButton handler |
| `bots.invokeWebViewCustomMethod` | routed-but-unregistered | session.yaml routes RPCMiniBotApps but the bff process has no BotsInvokeWebViewCustomMethod handler |
| `bots.reorderPreviewMedias` | routed-but-unregistered | session.yaml routes RPCMainMiniBotApps but the bff process has no BotsReorderPreviewMedias handler |
| `bots.reorderUsernames` | routed-but-unregistered | session.yaml routes RPCFragment but the bff process has no BotsReorderUsernames handler |
| `bots.setBotInfo` | missing | no route in session.yaml (RPCBots is commented out) and no bff handler |
| `bots.setCustomVerification` | missing | no route in session.yaml and no bff handler |
| `bots.toggleUserEmojiStatusPermission` | routed-but-unregistered | session.yaml routes RPCEmojiStatus but the bff process has no BotsToggleUserEmojiStatusPermission handler |
| `bots.toggleUsername` | routed-but-unregistered | session.yaml routes RPCFragment but the bff process has no BotsToggleUsername handler |
| `bots.updateStarRefProgram` | missing | no route in session.yaml and no bff handler |
| `channels.reorderProfileTabs` | missing | no route in session.yaml and no bff handler not in layer schema |
| `chatlists.checkChatlistInvite` | routed-but-unregistered | session.yaml routes RPCFolders but the bff process has no ChatlistsCheckChatlistInvite handler |
| `chatlists.deleteExportedInvite` | routed-but-unregistered | session.yaml routes RPCFolders but the bff process has no ChatlistsDeleteExportedInvite handler |
| `chatlists.editExportedInvite` | routed-but-unregistered | session.yaml routes RPCFolders but the bff process has no ChatlistsEditExportedInvite handler |
| `chatlists.exportChatlistInvite` | routed-but-unregistered | session.yaml routes RPCFolders but the bff process has no ChatlistsExportChatlistInvite handler |
| `chatlists.getChatlistUpdates` | routed-but-unregistered | session.yaml routes RPCFolders but the bff process has no ChatlistsGetChatlistUpdates handler |
| `chatlists.getExportedInvites` | routed-but-unregistered | session.yaml routes RPCFolders but the bff process has no ChatlistsGetExportedInvites handler |
| `chatlists.getLeaveChatlistSuggestions` | routed-but-unregistered | session.yaml routes RPCFolders but the bff process has no ChatlistsGetLeaveChatlistSuggestions handler |
| `chatlists.hideChatlistUpdates` | routed-but-unregistered | session.yaml routes RPCFolders but the bff process has no ChatlistsHideChatlistUpdates handler |
| `chatlists.joinChatlistInvite` | routed-but-unregistered | session.yaml routes RPCFolders but the bff process has no ChatlistsJoinChatlistInvite handler |
| `chatlists.joinChatlistUpdates` | routed-but-unregistered | session.yaml routes RPCFolders but the bff process has no ChatlistsJoinChatlistUpdates handler |
| `chatlists.leaveChatlist` | routed-but-unregistered | session.yaml routes RPCFolders but the bff process has no ChatlistsLeaveChatlist handler |
| `communities.create` | missing | no route in session.yaml and no bff handler |
| `communities.getJoinedCommunities` | missing | no route in session.yaml and no bff handler |
| `communities.getParticipantJoinedChats` | missing | no route in session.yaml and no bff handler |
| `communities.getPeerLinkRequests` | missing | no route in session.yaml and no bff handler |
| `communities.toggleAllPeerLinkRequestApproval` | missing | no route in session.yaml and no bff handler |
| `communities.toggleCommunityCollapsedInDialogs` | missing | no route in session.yaml and no bff handler |
| `communities.toggleParticipantBanned` | missing | no route in session.yaml and no bff handler |
| `communities.togglePeerLink` | missing | no route in session.yaml and no bff handler |
| `communities.togglePeerLinkRequestApproval` | missing | no route in session.yaml and no bff handler |
| `ephemeral.deleteAllWelcomeMessages` | missing | no route in session.yaml and no bff handler |
| `ephemeral.deleteMessage` | missing | no route in session.yaml and no bff handler |
| `ephemeral.deleteWelcomeMessage` | missing | no route in session.yaml and no bff handler |
| `ephemeral.editMessage` | missing | no route in session.yaml and no bff handler |
| `ephemeral.getCallbackAnswer` | missing | no route in session.yaml and no bff handler |
| `ephemeral.getWelcomeMessages` | missing | no route in session.yaml and no bff handler |
| `ephemeral.reportMessage` | missing | no route in session.yaml and no bff handler |
| `ephemeral.sendMessage` | missing | no route in session.yaml and no bff handler |
| `feed.getFeed` | missing | no route in session.yaml and no bff handler not in layer schema |
| `feed.getForYou` | missing | no route in session.yaml and no bff handler not in layer schema |
| `feed.getInterestCategories` | missing | no route in session.yaml and no bff handler not in layer schema |
| `feed.readFeed` | missing | no route in session.yaml and no bff handler not in layer schema |
| `feed.setInterestCategories` | missing | no route in session.yaml and no bff handler not in layer schema |
| `folders.editPeerFolders` | routed-but-unregistered | session.yaml routes RPCFolders but the bff process has no FoldersEditPeerFolders handler |
| `fragment.getCollectibleInfo` | missing | no route in session.yaml and no bff handler |
| `help.editUserInfo` | missing | no route in session.yaml (RPCTsf is commented out) and no bff handler |
| `help.getTimezonesList` | missing | no route in session.yaml and no bff handler |
| `help.getUserInfo` | missing | no route in session.yaml (RPCTsf is commented out) and no bff handler |
| `iv.getRichMessage` | missing | no route in session.yaml and no bff handler not in layer schema |
| `messages.getArchivedStickers` | routed-but-unregistered | session.yaml routes RPCStickers but the bff process has no MessagesGetArchivedStickers handler |
| `messages.getAttachedStickers` | routed-but-unregistered | session.yaml routes RPCStickers but the bff process has no MessagesGetAttachedStickers handler |
| `messages.getMaskStickers` | stub | returns an empty object app/bff/stickers/internal/server/grpc/service/handlers.go |
| `messages.getMyStickers` | routed-but-unregistered | session.yaml routes RPCStickers but the bff process has no MessagesGetMyStickers handler |
| `messages.getSavedGifs` | stub | session.yaml does not route RPCGifs; proxy returns an empty object (IDMap entry is commented out) |
| `messages.readFeaturedStickers` | stub | returns Bool only app/bff/stickers/internal/server/grpc/service/handlers.go |
| `messages.reorderStickerSets` | routed-but-unregistered | session.yaml routes RPCStickers but the bff process has no MessagesReorderStickerSets handler |
| `messages.saveGif` | stub | session.yaml does not route RPCGifs; proxy returns an empty object (IDMap entry is commented out) |
| `messages.sendEncryptedMultiMedia` | missing | no route in session.yaml and no bff handler not in layer schema |
| `messages.toggleStickerSets` | routed-but-unregistered | session.yaml routes RPCStickers but the bff process has no MessagesToggleStickerSets handler |
| `payments.applyGiftCode` | missing | no route in session.yaml and no bff handler |
| `payments.assignAppStoreTransaction` | stub | returns ENTERPRISE_IS_BLOCKED or METHOD_NOT_IMPL app/bff/premium/internal/core/payments.assignAppStoreTransaction_handler.go |
| `payments.assignPlayMarketTransaction` | stub | returns ENTERPRISE_IS_BLOCKED or METHOD_NOT_IMPL app/bff/premium/internal/core/payments.assignPlayMarketTransaction_handler.go |
| `payments.changeStarsSubscription` | missing | no route in session.yaml and no bff handler |
| `payments.checkCanSendGift` | missing | no route in session.yaml and no bff handler |
| `payments.checkGiftCode` | missing | no route in session.yaml and no bff handler |
| `payments.clearSavedInfo` | routed-but-unregistered | session.yaml routes RPCPayments but the bff process has no PaymentsClearSavedInfo handler |
| `payments.connectStarRefBot` | missing | no route in session.yaml and no bff handler |
| `payments.convertStarGift` | missing | no route in session.yaml and no bff handler |
| `payments.craftStarGift` | missing | no route in session.yaml and no bff handler |
| `payments.createOnrampSession` | routed-but-unregistered | session.yaml routes RPCPayments but the bff process has no PaymentsCreateOnrampSession handler |
| `payments.createStarGiftCollection` | missing | no route in session.yaml and no bff handler |
| `payments.deleteStarGift` | missing | no route in session.yaml and no bff handler |
| `payments.deleteStarGiftCollection` | missing | no route in session.yaml and no bff handler |
| `payments.depositStarGift` | missing | no route in session.yaml and no bff handler |
| `payments.displayStarGift` | missing | no route in session.yaml and no bff handler |
| `payments.editConnectedStarRefBot` | missing | no route in session.yaml and no bff handler |
| `payments.fulfillStarsSubscription` | missing | no route in session.yaml and no bff handler |
| `payments.getBankCardData` | routed-but-unregistered | session.yaml routes RPCPayments but the bff process has no PaymentsGetBankCardData handler |
| `payments.getConnectedStarRefBot` | missing | no route in session.yaml and no bff handler |
| `payments.getConnectedStarRefBots` | missing | no route in session.yaml and no bff handler |
| `payments.getCraftStarGifts` | missing | no route in session.yaml and no bff handler |
| `payments.getCurrencyRates` | routed-but-unregistered | session.yaml routes RPCPayments but the bff process has no PaymentsGetCurrencyRates handler |
| `payments.getGiveawayInfo` | missing | no route in session.yaml and no bff handler |
| `payments.getOnrampAvailability` | routed-but-unregistered | session.yaml routes RPCPayments but the bff process has no PaymentsGetOnrampAvailability handler |
| `payments.getOnrampBaseCurrencies` | routed-but-unregistered | session.yaml routes RPCPayments but the bff process has no PaymentsGetOnrampBaseCurrencies handler |
| `payments.getOnrampLimits` | routed-but-unregistered | session.yaml routes RPCPayments but the bff process has no PaymentsGetOnrampLimits handler |
| `payments.getOnrampProviders` | routed-but-unregistered | session.yaml routes RPCPayments but the bff process has no PaymentsGetOnrampProviders handler |
| `payments.getOnrampQuote` | routed-but-unregistered | session.yaml routes RPCPayments but the bff process has no PaymentsGetOnrampQuote handler |
| `payments.getPaymentForm` | routed-but-unregistered | session.yaml routes RPCPayments but the bff process has no PaymentsGetPaymentForm handler |
| `payments.getPaymentReceipt` | routed-but-unregistered | session.yaml routes RPCPayments but the bff process has no PaymentsGetPaymentReceipt handler |
| `payments.getPremiumGiftCodeOptions` | missing | no route in session.yaml and no bff handler |
| `payments.getResaleStarGifts` | missing | no route in session.yaml and no bff handler |
| `payments.getSavedStarGift` | missing | no route in session.yaml and no bff handler |
| `payments.getSavedStarGifts` | missing | no route in session.yaml and no bff handler |
| `payments.getStarGiftActiveAuctions` | missing | no route in session.yaml and no bff handler |
| `payments.getStarGiftAuctionAcquiredGifts` | missing | no route in session.yaml and no bff handler |
| `payments.getStarGiftAuctionState` | missing | no route in session.yaml and no bff handler |
| `payments.getStarGiftCollections` | missing | no route in session.yaml and no bff handler |
| `payments.getStarGiftUpgradeAttributes` | missing | no route in session.yaml and no bff handler |
| `payments.getStarGiftUpgradePreview` | missing | no route in session.yaml and no bff handler |
| `payments.getStarGiftWithdrawalUrl` | missing | no route in session.yaml and no bff handler |
| `payments.getStarGifts` | missing | no route in session.yaml and no bff handler |
| `payments.getStarsGiftOptions` | missing | no route in session.yaml and no bff handler |
| `payments.getStarsGiveawayOptions` | missing | no route in session.yaml and no bff handler |
| `payments.getStarsRevenueAdsAccountUrl` | missing | no route in session.yaml and no bff handler |
| `payments.getStarsRevenueStats` | missing | no route in session.yaml and no bff handler |
| `payments.getStarsRevenueWithdrawalUrl` | missing | no route in session.yaml and no bff handler |
| `payments.getStarsStatus` | missing | no route in session.yaml and no bff handler |
| `payments.getStarsSubscriptions` | missing | no route in session.yaml and no bff handler |
| `payments.getStarsTopupOptions` | missing | no route in session.yaml and no bff handler |
| `payments.getStarsTransactions` | missing | no route in session.yaml and no bff handler |
| `payments.getStarsTransactionsByID` | missing | no route in session.yaml and no bff handler |
| `payments.getSuggestedStarRefBots` | missing | no route in session.yaml and no bff handler |
| `payments.getUniqueStarGift` | missing | no route in session.yaml and no bff handler |
| `payments.getUniqueStarGiftValueInfo` | missing | no route in session.yaml and no bff handler |
| `payments.launchPrepaidGiveaway` | missing | no route in session.yaml and no bff handler |
| `payments.reorderStarGiftCollections` | missing | no route in session.yaml and no bff handler |
| `payments.resolveStarGiftOffer` | missing | no route in session.yaml and no bff handler |
| `payments.saveStarGift` | missing | no route in session.yaml and no bff handler |
| `payments.sendPaymentForm` | routed-but-unregistered | session.yaml routes RPCPayments but the bff process has no PaymentsSendPaymentForm handler |
| `payments.sendStarGiftOffer` | missing | no route in session.yaml and no bff handler |
| `payments.sendStarsForm` | missing | no route in session.yaml and no bff handler |
| `payments.toggleChatStarGiftNotifications` | missing | no route in session.yaml and no bff handler |
| `payments.toggleStarGiftsPinnedToTop` | missing | no route in session.yaml and no bff handler |
| `payments.transferStarGift` | missing | no route in session.yaml and no bff handler |
| `payments.updateStarGiftCollection` | missing | no route in session.yaml and no bff handler |
| `payments.updateStarGiftPrice` | missing | no route in session.yaml and no bff handler |
| `payments.upgradeStarGift` | missing | no route in session.yaml and no bff handler |
| `payments.validateRequestedInfo` | routed-but-unregistered | session.yaml routes RPCPayments but the bff process has no PaymentsValidateRequestedInfo handler |
| `payments.withdrawStarGift` | missing | no route in session.yaml and no bff handler |
| `phone.checkGroupCall` | missing | no route in session.yaml (RPCGroupCalls is commented out); handler source is implemented at app/bff/phone/internal/server/grpc/service/groups.go but the service is not routed |
| `phone.createConferenceCall` | missing | no route in session.yaml and no bff handler |
| `phone.createGroupCall` | missing | no route in session.yaml (RPCGroupCalls is commented out); handler source is implemented at app/bff/phone/internal/server/grpc/service/groups.go but the service is not routed |
| `phone.declineConferenceCallInvite` | missing | no route in session.yaml and no bff handler |
| `phone.deleteConferenceCallParticipants` | missing | no route in session.yaml and no bff handler |
| `phone.deleteGroupCallMessages` | missing | no route in session.yaml (RPCGroupCalls is commented out) and no bff handler |
| `phone.deleteGroupCallParticipantMessages` | missing | no route in session.yaml (RPCGroupCalls is commented out) and no bff handler |
| `phone.discardGroupCall` | missing | no route in session.yaml (RPCGroupCalls is commented out); handler source is implemented at app/bff/phone/internal/server/grpc/service/groups.go but the service is not routed |
| `phone.editGroupCallParticipant` | missing | no route in session.yaml (RPCGroupCalls is commented out); handler source is implemented at app/bff/phone/internal/server/grpc/service/groups.go but the service is not routed |
| `phone.editGroupCallTitle` | missing | no route in session.yaml (RPCGroupCalls is commented out); handler source is implemented at app/bff/phone/internal/server/grpc/service/groups.go but the service is not routed |
| `phone.exportGroupCallInvite` | missing | no route in session.yaml (RPCGroupCalls is commented out); handler source is implemented at app/bff/phone/internal/server/grpc/service/groups.go but the service is not routed |
| `phone.getGroupCall` | missing | no route in session.yaml (RPCGroupCalls is commented out); handler source is implemented at app/bff/phone/internal/server/grpc/service/groups.go but the service is not routed |
| `phone.getGroupCallChainBlocks` | missing | no route in session.yaml and no bff handler |
| `phone.getGroupCallJoinAs` | missing | no route in session.yaml (RPCGroupCalls is commented out); handler source is implemented at app/bff/phone/internal/server/grpc/service/groups.go but the service is not routed |
| `phone.getGroupCallStars` | missing | no route in session.yaml (RPCGroupCalls is commented out) and no bff handler |
| `phone.getGroupCallStreamChannels` | missing | no route in session.yaml (RPCGroupCalls is commented out); handler source is implemented at app/bff/phone/internal/server/grpc/service/groups.go but the service is not routed |
| `phone.getGroupCallStreamRtmpUrl` | missing | no route in session.yaml (RPCGroupCalls is commented out); handler source is implemented at app/bff/phone/internal/server/grpc/service/groups.go but the service is not routed |
| `phone.getGroupParticipants` | missing | no route in session.yaml (RPCGroupCalls is commented out); handler source is implemented at app/bff/phone/internal/server/grpc/service/groups.go but the service is not routed |
| `phone.inviteConferenceCallParticipant` | missing | no route in session.yaml and no bff handler |
| `phone.inviteToGroupCall` | missing | no route in session.yaml (RPCGroupCalls is commented out); handler source is implemented at app/bff/phone/internal/server/grpc/service/groups.go but the service is not routed |
| `phone.joinGroupCall` | missing | no route in session.yaml (RPCGroupCalls is commented out); handler source is implemented at app/bff/phone/internal/server/grpc/service/groups.go but the service is not routed |
| `phone.joinGroupCallPresentation` | missing | no route in session.yaml (RPCGroupCalls is commented out); handler source is implemented at app/bff/phone/internal/server/grpc/service/groups.go but the service is not routed |
| `phone.leaveGroupCall` | missing | no route in session.yaml (RPCGroupCalls is commented out); handler source is implemented at app/bff/phone/internal/server/grpc/service/groups.go but the service is not routed |
| `phone.leaveGroupCallPresentation` | missing | no route in session.yaml (RPCGroupCalls is commented out); handler source is implemented at app/bff/phone/internal/server/grpc/service/groups.go but the service is not routed |
| `phone.saveDefaultGroupCallJoinAs` | missing | no route in session.yaml (RPCGroupCalls is commented out); handler source is implemented at app/bff/phone/internal/server/grpc/service/groups.go but the service is not routed |
| `phone.saveDefaultSendAs` | missing | no route in session.yaml (RPCGroupCalls is commented out); handler source is implemented at app/bff/phone/internal/server/grpc/service/groups.go but the service is not routed |
| `phone.sendConferenceCallBroadcast` | missing | no route in session.yaml and no bff handler |
| `phone.sendGroupCallEncryptedMessage` | missing | no route in session.yaml and no bff handler |
| `phone.sendGroupCallMessage` | missing | no route in session.yaml (RPCGroupCalls is commented out) and no bff handler |
| `phone.startScheduledGroupCall` | missing | no route in session.yaml (RPCGroupCalls is commented out); handler source is implemented at app/bff/phone/internal/server/grpc/service/groups.go but the service is not routed |
| `phone.toggleGroupCallRecord` | missing | no route in session.yaml (RPCGroupCalls is commented out); handler source is implemented at app/bff/phone/internal/server/grpc/service/groups.go but the service is not routed |
| `phone.toggleGroupCallSettings` | missing | no route in session.yaml (RPCGroupCalls is commented out); handler source is implemented at app/bff/phone/internal/server/grpc/service/groups.go but the service is not routed |
| `phone.toggleGroupCallStartSubscription` | missing | no route in session.yaml (RPCGroupCalls is commented out); handler source is implemented at app/bff/phone/internal/server/grpc/service/groups.go but the service is not routed |
| `premium.applyBoost` | routed-but-unregistered | session.yaml routes RPCBoosts but the bff process has no PremiumApplyBoost handler |
| `premium.getBoostsList` | routed-but-unregistered | session.yaml routes RPCBoosts but the bff process has no PremiumGetBoostsList handler |
| `premium.getBoostsStatus` | routed-but-unregistered | session.yaml routes RPCBoosts but the bff process has no PremiumGetBoostsStatus handler |
| `premium.getMyBoosts` | routed-but-unregistered | session.yaml routes RPCBoosts but the bff process has no PremiumGetMyBoosts handler |
| `stars.checkCanSendGift` | missing | no route in session.yaml and no bff handler not in layer schema |
| `stars.convertStarGift` | missing | no route in session.yaml and no bff handler not in layer schema |
| `stars.createStarGiftCollection` | missing | no route in session.yaml and no bff handler not in layer schema |
| `stars.deleteStarGift` | missing | no route in session.yaml and no bff handler not in layer schema |
| `stars.deleteStarGiftCollection` | missing | no route in session.yaml and no bff handler not in layer schema |
| `stars.getCraftStarGifts` | missing | no route in session.yaml and no bff handler not in layer schema |
| `stars.getResaleStarGifts` | missing | no route in session.yaml and no bff handler not in layer schema |
| `stars.getSavedStarGift` | missing | no route in session.yaml and no bff handler not in layer schema |
| `stars.getSavedStarGifts` | missing | no route in session.yaml and no bff handler not in layer schema |
| `stars.getStarGiftCollections` | missing | no route in session.yaml and no bff handler not in layer schema |
| `stars.getStarGiftUpgradeAttributes` | missing | no route in session.yaml and no bff handler not in layer schema |
| `stars.getStarGiftUpgradePreview` | missing | no route in session.yaml and no bff handler not in layer schema |
| `stars.getStarGiftWithdrawalUrl` | missing | no route in session.yaml and no bff handler not in layer schema |
| `stars.getStarGifts` | missing | no route in session.yaml and no bff handler not in layer schema |
| `stars.getUniqueStarGift` | missing | no route in session.yaml and no bff handler not in layer schema |
| `stars.getUniqueStarGiftValueInfo` | missing | no route in session.yaml and no bff handler not in layer schema |
| `stars.reorderStarGiftCollections` | missing | no route in session.yaml and no bff handler not in layer schema |
| `stars.saveStarGift` | missing | no route in session.yaml and no bff handler not in layer schema |
| `stars.toggleChatStarGiftNotifications` | missing | no route in session.yaml and no bff handler not in layer schema |
| `stars.toggleStarGiftsPinnedToTop` | missing | no route in session.yaml and no bff handler not in layer schema |
| `stars.updatePaidMessagesPrice` | missing | no route in session.yaml and no bff handler not in layer schema |
| `stars.updateStarGiftCollection` | missing | no route in session.yaml and no bff handler not in layer schema |
| `stars.updateStarGiftPrice` | missing | no route in session.yaml and no bff handler not in layer schema |
| `stats.getMessagePublicForwards` | routed-but-unregistered | session.yaml routes RPCStatistics but the bff process has no StatsGetMessagePublicForwards5F150144 handler |
| `stats.getMessageStats` | routed-but-unregistered | session.yaml routes RPCStatistics but the bff process has no StatsGetMessageStats handler |
| `stats.getPollStats` | routed-but-unregistered | session.yaml routes RPCStatistics but the bff process has no StatsGetPollStats handler |
| `stats.getStoryPublicForwards` | routed-but-unregistered | session.yaml routes RPCStatistics but the bff process has no StatsGetStoryPublicForwards handler |
| `stats.getStoryStats` | routed-but-unregistered | session.yaml routes RPCStatistics but the bff process has no StatsGetStoryStats handler |
| `stats.loadAsyncGraph` | routed-but-unregistered | session.yaml routes RPCStatistics but the bff process has no StatsLoadAsyncGraph handler |
| `stickers.addStickerToSet` | routed-but-unregistered | session.yaml routes RPCStickers but the bff process has no StickersAddStickerToSet handler |
| `stickers.changeStickerPosition` | routed-but-unregistered | session.yaml routes RPCStickers but the bff process has no StickersChangeStickerPosition handler |
| `stickers.checkShortName` | routed-but-unregistered | session.yaml routes RPCStickers but the bff process has no StickersCheckShortName handler |
| `stickers.createStickerSet` | routed-but-unregistered | session.yaml routes RPCStickers but the bff process has no StickersCreateStickerSet handler |
| `stickers.deleteStickerSet` | routed-but-unregistered | session.yaml routes RPCStickers but the bff process has no StickersDeleteStickerSet handler |
| `stickers.removeStickerFromSet` | routed-but-unregistered | session.yaml routes RPCStickers but the bff process has no StickersRemoveStickerFromSet handler |
| `stickers.renameStickerSet` | routed-but-unregistered | session.yaml routes RPCStickers but the bff process has no StickersRenameStickerSet handler |
| `stickers.replaceSticker` | routed-but-unregistered | session.yaml routes RPCStickers but the bff process has no StickersReplaceSticker handler |
| `stickers.suggestShortName` | routed-but-unregistered | session.yaml routes RPCStickers but the bff process has no StickersSuggestShortName handler |
| `stories.activateStealthMode` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.canSendStory` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.createAlbum` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.deleteAlbum` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.deleteStories` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.editStory` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.exportStoryLink` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.getAlbumStories` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.getAlbums` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.getAllReadPeerStories` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.getAllStories` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.getChatsToSend` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.getPeerMaxIDs` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.getPeerStories` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.getPinnedStories` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.getStoriesArchive` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.getStoriesByID` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.getStoriesViews` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.getStoryReactionsList` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.getStoryViewsList` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.incrementStoryViews` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.readStories` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.reorderAlbums` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.report` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.searchPosts` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.sendReaction` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.sendStory` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.startLive` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.togglePeerStoriesHidden` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.togglePinned` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.togglePinnedToTop` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `stories.updateAlbum` | missing | no route in session.yaml; handler source is implemented at app/bff/stories/internal/server/grpc/service/handlers.go but the service is not routed |
| `toncenter.createOnrampSession` | missing | no route in session.yaml and no bff handler not in layer schema |
| `toncenter.getOnrampProviders` | missing | no route in session.yaml and no bff handler not in layer schema |
| `toncenter.getStreamingUrl` | missing | no route in session.yaml and no bff handler |
| `toncenter.performApiRequest` | missing | no route in session.yaml and no bff handler |
| `wallet.disableBackup` | missing | no route in session.yaml and no bff handler |
| `wallet.enableBackup` | missing | no route in session.yaml and no bff handler |
| `wallet.exportSecretPhrase` | missing | no route in session.yaml and no bff handler |
| `wallet.fetchEncryptedSecretPhrasePart` | missing | no route in session.yaml and no bff handler |
| `wallet.getBackupHolderDcs` | missing | no route in session.yaml and no bff handler |
| `wallet.getCurrencyRates` | missing | no route in session.yaml and no bff handler not in layer schema |
| `wallet.getExistingWaltBalance` | missing | no route in session.yaml and no bff handler |
| `wallet.getGaslessInfo` | missing | no route in session.yaml and no bff handler |
| `wallet.getNfts` | missing | no route in session.yaml and no bff handler |
| `wallet.getProofChallenge` | missing | no route in session.yaml and no bff handler |
| `wallet.getState` | missing | no route in session.yaml and no bff handler |
| `wallet.getTransactions` | missing | no route in session.yaml and no bff handler |
| `wallet.getTransactionsByIDs` | missing | no route in session.yaml and no bff handler |
| `wallet.getTransactionsByMsgHash` | missing | no route in session.yaml and no bff handler |
| `wallet.getUserAddresses` | missing | no route in session.yaml and no bff handler |
| `wallet.replaceWallet` | missing | no route in session.yaml and no bff handler |
| `wallet.sendTransfer` | missing | no route in session.yaml and no bff handler |
| `wallet.setCardStickers` | missing | no route in session.yaml and no bff handler |
| `wallet.tonConnectClaimRequest` | missing | no route in session.yaml and no bff handler |
| `wallet.tonConnectCloseSession` | missing | no route in session.yaml and no bff handler |
| `wallet.tonConnectCreateSession` | missing | no route in session.yaml and no bff handler |
| `wallet.tonConnectGetPending` | missing | no route in session.yaml and no bff handler |
| `wallet.tonConnectGetSessions` | missing | no route in session.yaml and no bff handler |
| `wallet.tonConnectNextEventId` | missing | no route in session.yaml and no bff handler |
| `wallet.tonConnectRegisterKey` | missing | no route in session.yaml and no bff handler |
| `wallet.tonConnectSubmitConnectResult` | missing | no route in session.yaml and no bff handler |
| `wallet.tonConnectSubmitResponse` | missing | no route in session.yaml and no bff handler |

