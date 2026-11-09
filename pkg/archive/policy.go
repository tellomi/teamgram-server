// Package archive matches official Telegram archive rules.
//
// Folder 0 is the main chat list and folder 1 is the archive. An archived
// chat stays archived. The server moves it back to the main list only when a
// new incoming message arrives, the chat is not muted, keep_archived_unmuted
// is false, and (when the chat belongs to a custom folder) keep_archived_folders
// is false. Muted archived chats stay archived. Outgoing messages, reading
// history, and pinning do not change the folder.
package archive

import (
	"time"

	"github.com/teamgram/proto/mtproto"
)

// MuteForever is the mute_until value official clients send for "muted".
const MuteForever int32 = 2147483647

// PeerClass is how a peer matches dialog-filter category flags.
type PeerClass int

const (
	// ClassUser is a private chat that is not a contact and not a bot.
	ClassUser PeerClass = iota
	ClassContact
	ClassBot
	ClassGroup
	ClassBroadcast
)

// Settings is the archive slice of globalPrivacySettings.
type Settings struct {
	ArchiveAndMuteNewNoncontactPeers bool
	KeepArchivedUnmuted              bool
	KeepArchivedFolders              bool
}

// ShouldUnarchive reports whether one incoming message moves an archived
// dialog back to the main list.
func ShouldUnarchive(folderID int32, muted bool, settings Settings, inCustomFolder bool) bool {
	if folderID != 1 {
		return false
	}
	if muted {
		return false
	}
	if settings.KeepArchivedUnmuted {
		return false
	}
	if inCustomFolder && settings.KeepArchivedFolders {
		return false
	}
	return true
}

// ShouldArchiveNewPrivate reports whether the first incoming private message
// from a non-contact is created already in the archive and muted.
// archive_and_mute_new_noncontact_peers does not apply to groups or channels,
// and it does not apply to a dialog that already exists.
func ShouldArchiveNewPrivate(created bool, peerType int32, settings Settings, contact bool) bool {
	return created && peerType == mtproto.PEER_USER && settings.ArchiveAndMuteNewNoncontactPeers && !contact
}

// MutedAt reports whether notify settings mute the chat at now.
// A peer-level mute_until of 0 is explicitly unmuted. An unset peer value
// falls through to the users/chats/channels default.
func MutedAt(peer, category *mtproto.PeerNotifySettings, now int64) bool {
	if peer != nil && peer.GetMuteUntil() != nil {
		return futureMute(peer.GetMuteUntil().GetValue(), now)
	}
	if category != nil && category.GetMuteUntil() != nil {
		return futureMute(category.GetMuteUntil().GetValue(), now)
	}
	return false
}

func futureMute(until int32, now int64) bool {
	if until == 0 {
		return false
	}
	if until == MuteForever {
		return true
	}
	return int64(until) > now
}

// CategoryPeerType is the inputNotifyUsers/Chats/Broadcasts peer used when a
// chat has no peer-level mute.
func CategoryPeerType(peerType int32, class PeerClass) int32 {
	switch class {
	case ClassBroadcast:
		return mtproto.PEER_BROADCASTS
	case ClassGroup:
		return mtproto.PEER_CHATS
	default:
		if peerType == mtproto.PEER_CHAT {
			return mtproto.PEER_CHATS
		}
		return mtproto.PEER_USERS
	}
}

// InCustomFolder reports whether any real folder (not All Chats) contains the peer.
func InCustomFolder(filters []*mtproto.DialogFilter, peerType int32, peerID int64, class PeerClass) bool {
	for _, filter := range filters {
		if filter == nil || filter.GetId() == 0 || filter.GetPredicateName() == mtproto.Predicate_dialogFilterDefault {
			continue
		}
		if FilterContains(filter, peerType, peerID, class) {
			return true
		}
	}
	return false
}

// FilterContains reports whether one dialog filter lists the peer.
// exclude_archived only hides an archived chat from the folder tab; the chat
// still belongs to the folder for keep_archived_folders.
func FilterContains(filter *mtproto.DialogFilter, peerType int32, peerID int64, class PeerClass) bool {
	if filter == nil {
		return false
	}
	if peerListed(filter.GetExcludePeers(), peerType, peerID) {
		return false
	}
	if peerListed(filter.GetIncludePeers(), peerType, peerID) || peerListed(filter.GetPinnedPeers(), peerType, peerID) {
		return true
	}
	switch class {
	case ClassContact:
		return filter.GetContacts()
	case ClassUser:
		return filter.GetNonContacts()
	case ClassBot:
		return filter.GetBots()
	case ClassGroup:
		return filter.GetGroups()
	case ClassBroadcast:
		return filter.GetBroadcasts()
	default:
		return false
	}
}

func peerListed(peers []*mtproto.InputPeer, peerType int32, peerID int64) bool {
	for _, peer := range peers {
		if peer == nil {
			continue
		}
		switch peer.GetPredicateName() {
		case mtproto.Predicate_inputPeerUser, mtproto.Predicate_inputPeerUserFromMessage:
			if peerType == mtproto.PEER_USER && peer.GetUserId() == peerID {
				return true
			}
		case mtproto.Predicate_inputPeerChat:
			if peerType == mtproto.PEER_CHAT && peer.GetChatId() == peerID {
				return true
			}
		case mtproto.Predicate_inputPeerChannel, mtproto.Predicate_inputPeerChannelFromMessage:
			if peerType == mtproto.PEER_CHANNEL && peer.GetChannelId() == peerID {
				return true
			}
		}
	}
	return false
}

// NowUnix is the clock archive decisions use.
func NowUnix() int64 {
	return time.Now().Unix()
}
