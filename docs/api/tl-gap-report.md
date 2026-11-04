# TL gap report

Semantic gap between Tellomi and the official Telegram API at **layer 232**, the layer both Tellomi clients speak.

| Client | Repository | Layer |
| --- | --- | --- |
| iOS | `tellomi/Telegram-iOS` | `Serialization.currentLayer()` = **232** |
| Android | `tellomi/Telegram` | `TLRPC.LAYER` = **232** |

The schema in this repo is `proto/schema/telegram_api_layer232.tl` (1201 functions). `docs/api-coverage.md` is the routing matrix: which of those functions a client calls, and whether the bff process routes a handler. This report is the next cut. A method can be routed and still return an empty object, drop flags, or fail for a peer type the client uses every day.

Regenerate the routing matrix with `go run ./tools/apicoverage`. This file is written by hand from that matrix plus the handlers.

## How a row is classified

| Status | Meaning |
| --- | --- |
| `implemented` | The handler reads or writes real state and follows the TL result and the error codes a client branches on. |
| `stub` | Success with an empty object, `Bool`, or `*NotModified` and no state change, or a hard `ENTERPRISE_IS_BLOCKED` / `METHOD_NOT_IMPL`. |
| `partial` | Real work for the common case, but a flag, field, peer type, or error code is missing. |
| `missing` | No routed handler. The session returns `ENTERPRISE_IS_BLOCKED`. |

Client priority follows `docs/api-coverage.md`. **P0** runs at startup or in the background. **P1** is core chat. Anything a client calls from a screen the user opened and that is not P0/P1 is **P2**.

Chat invite RPCs (`messages.exportChatInvite`, `messages.getExportedChatInvites`, `messages.editExportedChatInvite`, `messages.deleteExportedChatInvite`, `messages.importChatInvite`, `messages.checkChatInvite`, `messages.getChatInviteImporters`, `messages.hideChatJoinRequest`, `messages.hideAllChatJoinRequests`, `messages.getChatJoinRequests`) are owned by another change and are not modified here.

## Routing snapshot

From `docs/api-coverage.md` on this tree: **799** called methods, **791** routed to a real handler, **8** stubs that return `ENTERPRISE_IS_BLOCKED`, **0** missing routes. The 8 stubs are sponsored-message and AI-compose methods. They are not on the login path.

The gaps below are `partial` or `stub` inside methods the routing matrix already calls implemented.

## Fixes in this pass

| Area | What changed | PR |
| --- | --- | --- |
| Channel description | `messages.editChatAbout` on a channel stores the text. 255 characters is `CHAT_ABOUT_TOO_LONG`, an unchanged text is `CHAT_ABOUT_NOT_MODIFIED`, a member without `change_info` is `CHAT_ADMIN_REQUIRED`. The admin log records `changeAbout`. | #162 |
| Signatures | `channels.toggleSignatures` reads layer-232 `signatures_enabled` and `profiles_enabled` (and the older `enabled:Bool`). Both show up on `channel`. | #162 |
| Admin log | Promote and ban write `participantToggleAdmin` and `participantToggleBan`. Signature toggles write their own events. | #162 |
| Forum topics | `messages.deleteTopicHistory` deletes that topic's messages and leaves the rest of the channel. Topic id is the creation message. A new forum gets a hidden General topic. Pin reorder keeps the requested order. `offset_topic` pages the list. `icon_emoji_id` and `hidden` are stored. The top message is included in `messages.forumTopics`. | #163 |
| Channel unpin | `messages.unpinAllMessages` on a channel clears that channel's pin. | #163 |
| Drafts | Media, `invert_media`, effect, and suggested post are stored. A channel draft no longer fails when the enterprise plugin is absent. | #165 |
| Global search | `users_only`, `groups_only`, `broadcasts_only`, `min_date`, and `max_date` filter the text hits. Channel chats are attached to the result. | #165 |
| Notify settings | `account.updateNotifySettings` and `account.getNotifyExceptions` resolve a channel without the enterprise plugin. | #165 |
| Contacts | `contacts.getContacts` returns `contacts.contactsNotModified` when `hash` matches. `contacts.deleteByPhones` deletes the matching contacts and unregistered phone-book rows. | #166 |

## Messages

Clients call these from the chat screen. Rows marked fixed are the behavior after the PR above is merged.

| Method | Status | What is still missing |
| --- | --- | --- |
| `messages.saveDraft` | partial | `rich_message` is `InputRichMessage` on the request and `RichMessage` on the stored draft. It is not converted, so a rich-only draft is still cleared. Other flags are stored. |
| `messages.getAllDrafts` | implemented | Channel cards come from the channel service. |
| `messages.clearAllDrafts` | implemented | Same as get. |
| `messages.sendReaction` | partial | `add_to_recent` is ignored; the reaction is always added to the recent list. Omitting the reaction vector clears the user's reactions. Custom emoji on a channel is `REACTION_INVALID`. |
| `messages.getMessagesReactions` | implemented | |
| `messages.getMessageReactionsList` | implemented | |
| `messages.setChatAvailableReactions` | partial | A private chat or basic group persists the rule and returns empty `Updates`. A channel returns the real `Updates`. |
| `messages.getAvailableReactions` | implemented | Hash can return `availableReactionsNotModified`. |
| `messages.readReactions` | partial | `saved_peer_id` and `top_msg_id` apply to channels only. |
| `messages.getUnreadReactions` | partial | `saved_peer_id` returns an empty page. |
| `messages.reportReaction` | partial | Writes an audit row. No channel moderation decision. |
| `messages.getTopReactions` | implemented | |
| `messages.getRecentReactions` | implemented | |
| `messages.clearRecentReactions` | implemented | |
| `messages.sendVote` | partial | Vote is stored. Closed polls and the public voter list are not the full TL rules. |
| `messages.getPollResults` | partial | `poll_hash` is checked. Same voter-list limit as send. |
| `messages.getPollVotes` | partial | `option` and `offset` are forwarded. |
| `messages.search` | partial | User and basic-group filters work. A channel search is text and sender only: no `filter`, dates, `hash`, `top_msg_id`, or `saved_reaction`. `saved_peer_id` and `inputMessagesFilterMyMentions` return an empty page. |
| `messages.searchGlobal` | partial | Kind flags and dates are applied (#165). `folder_id`, `community`, `filter`, `offset_rate`, and `offset_peer` are not. |
| `messages.getSearchCounters` | partial | `saved_peer_id` is an empty vector. `top_msg_id` is ignored. An unsupported filter is count 0, not an error. |
| `messages.getSearchResultsCalendar` | partial | User and basic-group media calendar works. `offset_id_offset` is not set. Channel history is weak. |
| `messages.getSearchResultsPositions` | partial | Same scan as the calendar. |
| `messages.searchSentMedia` | partial | Scans dialogs. No server-side pagination. |
| `messages.updatePinnedMessage` | partial | User and basic group honor `silent`, `unpin`, and `pm_oneside`. A channel pin ignores `silent` and `pm_oneside`. |
| `messages.unpinAllMessages` | partial | A channel pin is cleared (#163). `top_msg_id` and `saved_peer_id` are not applied. |
| `messages.getPinnedDialogs` | implemented | |
| `messages.sendMessage` with `schedule_date` | partial | A future date is stored and later sent. `schedule_repeat_period` is ignored. |
| `messages.getScheduledHistory` | implemented | |
| `messages.getScheduledMessages` | implemented | |
| `messages.sendScheduledMessages` | partial | Sends now and drops the original `schedule_date` on the live message. |
| `messages.deleteScheduledMessages` | implemented | |
| `messages.getForumTopics` | partial | `q`, `limit`, and `offset_topic` work (#163). `offset_date` / `offset_id` only apply when `offset_topic` is 0. |
| `messages.getForumTopicsByID` | partial | Filters the first 100 topics in memory. |
| `messages.createForumTopic` | partial | `random_id` and `send_as` are ignored, so a retry can create a second topic. |
| `messages.editForumTopic` | implemented | Title, closed, icon emoji, and hidden are stored (#163). |
| `messages.updatePinnedForumTopic` | implemented | |
| `messages.reorderPinnedForumTopics` | implemented | `force` unpins topics left out of `order`. Without `force`, a missing pinned topic is `TOPIC_ID_INVALID`. |
| `messages.deleteTopicHistory` | implemented | Topic-scoped (#163). Requires delete-message rights. |
| `channels.toggleForum` | partial | `enabled` works and creates General. The `tabs` flag is ignored. |
| `messages.deleteHistory` | partial | A channel peer returns `ENTERPRISE_IS_BLOCKED`. Clients should call `channels.deleteHistory`, which is implemented. |
| `messages.forwardMessages` | partial | A channel source can return `ENTERPRISE_IS_BLOCKED`. Private and basic-group forward works. |

## Channels and groups

| Method | Status | What is still missing |
| --- | --- | --- |
| `messages.editChatAbout` | implemented | Channel path fixed in #162. |
| `channels.editBanned` | implemented | Admin log event added in #162. |
| `channels.editAdmin` | implemented | Admin log event added in #162. |
| `messages.editChatDefaultBannedRights` | implemented | Megagroup and basic group. |
| `channels.getParticipants` | partial | Hidden participants return a count and an empty list to non-admins. Search matches id and rank, not display name. |
| `channels.getAdminedPublicChannels` | partial | `by_location`, `check_limit`, `for_personal`, and `for_community_peer` are ignored. |
| `channels.getAdminLog` | partial | Title, photo, about, signatures, ban, and promote are recorded. Delete and some settings still are not. The filter matches the action name, not every official flag. The log is per channel, not a separate events table. |
| `channels.toggleSlowMode` | implemented | Megagroup only. Seconds must be one of the official steps. |
| `channels.setDiscussionGroup` | implemented | Returns `Bool`, which is what the TL says. |
| `channels.getGroupsForDiscussion` | implemented | Admin megagroups. |
| `channels.toggleSignatures` | implemented | Both layer-232 flags (#162). |
| `channels.createChannel` | partial | `for_import` is ignored. A geo point's address is stored; latitude and longitude are not applied at create. |
| `channels.editTitle` | implemented | |
| `channels.editPhoto` | implemented | |
| `channels.updateUsername` | implemented | |
| `channels.deleteMessages` | implemented | Ids the caller cannot delete are skipped. |
| `channels.deleteHistory` | implemented | |
| `channels.leaveChannel` | implemented | |
| `channels.joinChannel` | implemented | A join request does not add the member. |
| `channels.inviteToChannel` | implemented | |
| `channels.deleteChannel` | partial | Sets the deleted flag. It does not purge history. |
| `channels.togglePreHistoryHidden` | implemented | |
| `channels.editLocation` | implemented | |
| `channels.setStickers` | implemented | |
| `messages.editChatAdmin` | implemented | Basic groups only, matching the TL. |
| `messages.getFullChat` | partial | Basic group. `has_scheduled`, `exported_invite`, and `pinned_msg_id` are still empty. Channels use `channels.getFullChannel`. |

`channels.getBanned` is not in layer 232. Clients list banned members with `channels.getParticipants`.

## Contacts and privacy

| Method | Status | What is still missing |
| --- | --- | --- |
| `contacts.getContacts` | implemented | Hash (#166). |
| `contacts.deleteByPhones` | implemented | (#166). |
| `contacts.importContacts` | implemented | |
| `contacts.deleteContacts` | partial | Skips anything that is not `inputUser`, including `inputUserFromMessage`. |
| `contacts.block` / `unblock` / `getBlocked` | partial | Block and unblock persist. `getBlocked` documents a cap of 50 and then forwards the client's limit. |
| `contacts.search` | implemented | |
| `contacts.resolveUsername` | implemented | |
| `contacts.getTopPeers` / `resetTopPeerRating` | implemented | |
| `contacts.getStatuses` | implemented | |
| `contacts.getBirthdays` | implemented | |
| `contacts.addContact` / `acceptContact` | implemented | |
| `account.getPrivacy` / `setPrivacy` | partial | User rules persist. A rule that names a channel is not resolved. |
| `account.getGlobalPrivacySettings` / `setGlobalPrivacySettings` | implemented | |
| `users.getRequirementsToContact` | implemented | Every peer is `requirementToContactEmpty`. Tellomi does not charge to message someone. |
| `account.getAccountTTL` / `setAccountTTL` | implemented | |
| `account.getContactSignUpNotification` / `setContactSignUpNotification` | partial | A storage error on get is reported as `boolFalse` instead of an RPC error. |

`users.getIsPremiumRequiredToContact` is not in layer 232. The replacement is `users.getRequirementsToContact`. The old handler still returns `ENTERPRISE_IS_BLOCKED` if an older constructor is decoded.

## Account settings

| Method | Status | What is still missing |
| --- | --- | --- |
| `account.getAuthorizations` | implemented | |
| `account.resetAuthorization` | implemented | |
| `account.changeAuthorizationSettings` | implemented | |
| `account.getNotifySettings` | implemented | |
| `account.updateNotifySettings` | implemented | Channel peer no longer needs the enterprise plugin (#165). |
| `account.getNotifyExceptions` | partial | Channel cards are filled (#165). `compare_sound` is not applied. |
| `account.resetNotifySettings` | implemented | |
| `account.registerDevice` / `unregisterDevice` / `updateDeviceLocked` | implemented | |
| `account.getAutoDownloadSettings` / `saveAutoDownloadSettings` | implemented | |
| `account.getThemes` / `getChatThemes` / `getTheme` / `installTheme` / `uploadTheme` / `updateTheme` | implemented | |
| `account.getWallPapers` / `getWallPaper` / `saveWallPaper` / `installWallPaper` | implemented | |
| `account.getContentSettings` / `setContentSettings` | implemented | |
| `account.updateProfile` / `updateStatus` | implemented | |
| `account.updateEmojiStatus` | implemented | |
| `account.getDefaultEmojiStatuses` | partial | Returns a valid empty list. There is no status catalog. |

## Photos and profile

| Method | Status |
| --- | --- |
| `photos.uploadProfilePhoto` | implemented |
| `photos.updateProfilePhoto` | implemented |
| `photos.deletePhotos` | implemented |
| `photos.getUserPhotos` | implemented |
| `photos.uploadContactProfilePhoto` | implemented |
| `users.getUsers` / `users.getFullUser` | implemented |
| `account.updateBirthday` | implemented |
| `account.updatePersonalChannel` | implemented |
| `account.setMainProfileTab` | implemented |

`photos.deletePhotos` does not enforce a profile-photo album cap. That is the remaining limit, not a missing method.

## Stickers and emoji

The sticker, emoji, reaction, and ringtone methods clients call are backed by `app/bff/stickers/catalog`. `messages.getStickerSet` honors `hash` with `stickerSetNotModified`.

| Method | Status | What is still missing |
| --- | --- | --- |
| `messages.getEmojiKeywords` | partial | Always reads version 0. The difference method is the one that takes a version. |
| `messages.getStickers` and the install, recent, featured, and custom-emoji methods | implemented | |
| `account.getSavedRingtones` | implemented | |

## Updates

| Method | Status | What is still missing |
| --- | --- | --- |
| `updates.getState` | implemented | Returns pts, qts, seq, and date. |
| `updates.getDifference` | partial | Pts and seq are merged and users and chats are filled. The request `qts` is not passed through, so secret-chat encrypted messages are not in the difference (`new_encrypted_messages` is empty). The state inside a difference does not repeat `qts`. `differenceTooLong` is not produced; an empty update list is `differenceEmpty`. The full-difference `date` is overwritten with the server time. This path runs at startup, so it was left unchanged in this pass. |
| `updates.getChannelDifference` | partial | Pts, too-long, and slice behavior are implemented. `force` and `ChannelMessagesFilter` are ignored. |

## help.\*

| Method | Status | What is still missing |
| --- | --- | --- |
| `help.getConfig` | implemented | DC options come from the server config. |
| `help.getAppConfig` | implemented | Hash can return `appConfigNotModified`. |
| `help.getNearestDc` | partial | Always `this_dc = 1`, `nearest_dc = 1`. Country comes from the default country table. |
| `help.getCountriesList` | implemented | |
| `help.getPeerColors` / `getPeerProfileColors` | implemented | |
| `help.getPremiumPromo` | partial | Static Tellomi copy. `period_options` is empty because there is no store. |
| `help.getTermsOfServiceUpdate` | implemented | |
| `help.getInviteText` | implemented | |
| `help.getSupport` | implemented | |
| `help.saveAppLog` | implemented | |
| `help.getAppUpdate` | implemented | |
| `help.dismissSuggestion` | implemented | |
| `help.getDeepLinkInfo` | stub | Always `help.deepLinkInfoEmpty`. The constructor is valid. No interstitial is resolved. |

## Stories

`stories.getAllStories`, `stories.getStoriesArchive`, and `stories.getPeerStories` read the story store and are not startup methods in the coverage matrix (P2, interactive). They were not changed. Clients do not call them unconditionally during login.

## Still returning ENTERPRISE_IS_BLOCKED

These are called, routed, and fail on purpose until the product exists. They are not chat-list blockers.

| Method | Why |
| --- | --- |
| `account.toggleSponsoredMessages` | No sponsored-message product. |
| `channels.restrictSponsoredMessages` | Same. |
| `messages.viewSponsoredMessage` | Same. |
| `messages.clickSponsoredMessage` | Same. |
| `messages.reportSponsoredMessage` | Same. |
| `account.confirmBotConnection` | Bot connection confirmation is not implemented. |
| `messages.composeRichMessageWithAI` | No model provider behind the RPC. |
| `messages.translateRichMessage` | Plain `messages.translateText` is separate and implemented. |
| `messages.deleteHistory` | Channel peer only. `channels.deleteHistory` is the working method. |
| `messages.forwardMessages` | One channel branch. |
| `upload.getFile` | Sticker-set thumbnail location when the plugin is absent. Ordinary files and secret-chat files are served. |
| `upload.getFileHashes` | Whole method. |
| `upload.getWebFile` | One location branch. |

## What to do next

1. `updates.getDifference`: carry `qts` on the state and stop rewriting `date`, with a regression test that does not change login. Encrypted-message replay is a separate secret-chat change.
2. `messages.search` on a channel: honor `filter`, dates, and `top_msg_id`.
3. `messages.saveDraft`: convert `InputRichMessage` into the stored `RichMessage`.
4. `channels.getAdminLog`: record message deletes and the remaining settings.
5. `messages.forwardMessages` and `messages.deleteHistory` for a channel peer, by calling the channel service instead of the enterprise error.
6. `account.getDefaultEmojiStatuses`: a real catalog, or an explicit empty list the clients already accept. The empty list is valid TL; it is partial because the picker is blank.
