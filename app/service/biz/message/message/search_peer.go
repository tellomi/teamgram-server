package message

// SearchGlobalPeerType asks message.searchByMediaType to scan every dialog
// of the user. messages.search always names a real peer, so this value is
// only used by messages.searchGlobal.
const SearchGlobalPeerType int32 = -2
