// Package announce fans a launch notice out from the system account.
//
// TELLOMI_ANNOUNCE must be 1 and TELLOMI_ANNOUNCE_KEY must decode to at
// least 32 bytes. Otherwise nothing listens, and help.getAppConfig keeps
// tellomi_announce_enabled false. Login and messages.sendMessage are not
// hooked. The contract is docs/api/announce.md.
package announce
