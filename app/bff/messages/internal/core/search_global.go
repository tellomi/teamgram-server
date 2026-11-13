package core

import (
	"strings"

	"github.com/teamgram/proto/mtproto"
)

// globalQuery is the part of messages.searchGlobal that can be applied
// after the text search: peer kind and date.
type globalQuery struct {
	UsersOnly      bool
	GroupsOnly     bool
	BroadcastsOnly bool
	MinDate        int32
	MaxDate        int32
}

type channelKind struct {
	Broadcast bool
	Megagroup bool
}

func (q globalQuery) narrows() bool {
	return q.UsersOnly || q.GroupsOnly || q.BroadcastsOnly || q.MinDate != 0 || q.MaxDate != 0
}

func (q globalQuery) allows(peerType int32, peerID int64, date int32, kinds map[int64]channelKind) bool {
	if q.MinDate != 0 && date < q.MinDate {
		return false
	}
	if q.MaxDate != 0 && date > q.MaxDate {
		return false
	}
	if !q.UsersOnly && !q.GroupsOnly && !q.BroadcastsOnly {
		return true
	}
	switch peerType {
	case mtproto.PEER_USER, mtproto.PEER_SELF:
		return q.UsersOnly
	case mtproto.PEER_CHAT:
		return q.GroupsOnly
	case mtproto.PEER_CHANNEL:
		kind, ok := kinds[peerID]
		if !ok {
			return false
		}
		if kind.Broadcast && q.BroadcastsOnly {
			return true
		}
		if kind.Megagroup && q.GroupsOnly {
			return true
		}
		return false
	default:
		return false
	}
}

// globalFilterUseful is a media filter the global search screen sends.
// An empty query with one of these filters returns that media. An empty
// query with any other filter is SEARCH_QUERY_EMPTY.
func globalFilterUseful(filter *mtproto.MessagesFilter) bool {
	if filter == nil {
		return false
	}
	switch mtproto.FromMessagesFilter(filter) {
	case mtproto.FilterPhotoVideo, mtproto.FilterPhotos, mtproto.FilterVideo,
		mtproto.FilterDocument, mtproto.FilterUrl, mtproto.FilterMusic,
		mtproto.FilterVoice, mtproto.FilterRoundVoice:
		return true
	default:
		return false
	}
}

// globalMediaKind is the stored message_filter_type for a global media scan.
// Voice and round video notes share MEDIA_AUDIO; the caller splits them.
func globalMediaKind(filter *mtproto.MessagesFilter) (int32, bool) {
	switch mtproto.FromMessagesFilter(filter) {
	case mtproto.FilterPhotos:
		return mtproto.MEDIA_PHOTOS_ONLY, true
	case mtproto.FilterVideo:
		return mtproto.MEDIA_VIDEOS_ONLY, true
	case mtproto.FilterPhotoVideo:
		return mtproto.MEDIA_PHOTOVIDEO, true
	case mtproto.FilterDocument:
		return mtproto.MEDIA_FILE, true
	case mtproto.FilterUrl:
		return mtproto.MEDIA_URL, true
	case mtproto.FilterMusic:
		return mtproto.MEDIA_MUSIC, true
	case mtproto.FilterVoice, mtproto.FilterRoundVoice:
		return mtproto.MEDIA_AUDIO, true
	default:
		return 0, false
	}
}

// messageMatchesGlobalFilter reports whether one message belongs in the
// client's filter. Url matches a url/textUrl/email entity, webpage media,
// or a message whose text contains a URL.
func messageMatchesGlobalFilter(msg *mtproto.Message, filter *mtproto.MessagesFilter) bool {
	if !globalFilterUseful(filter) {
		return true
	}
	switch mtproto.FromMessagesFilter(filter) {
	case mtproto.FilterPhotos:
		return mtproto.GetMediaType(msg) == mtproto.MEDIA_PHOTOS_ONLY
	case mtproto.FilterVideo:
		return mtproto.GetMediaType(msg) == mtproto.MEDIA_VIDEOS_ONLY
	case mtproto.FilterPhotoVideo:
		t := mtproto.GetMediaType(msg)
		return t == mtproto.MEDIA_PHOTOS_ONLY || t == mtproto.MEDIA_VIDEOS_ONLY || t == mtproto.MEDIA_PHOTOVIDEO
	case mtproto.FilterDocument:
		return mtproto.GetMediaType(msg) == mtproto.MEDIA_FILE
	case mtproto.FilterUrl:
		return messageHasURL(msg)
	case mtproto.FilterMusic:
		return mtproto.IsMusicMessage(msg) || mtproto.GetMediaType(msg) == mtproto.MEDIA_MUSIC
	case mtproto.FilterVoice:
		return mtproto.IsVoiceMessage(msg)
	case mtproto.FilterRoundVoice:
		return mtproto.IsRoundVideoMessage(msg)
	default:
		return true
	}
}

func messageHasURL(msg *mtproto.Message) bool {
	if msg == nil {
		return false
	}
	if mtproto.GetMediaType(msg) == mtproto.MEDIA_URL {
		return true
	}
	for _, entity := range msg.GetEntities() {
		switch entity.GetPredicateName() {
		case mtproto.Predicate_messageEntityUrl, mtproto.Predicate_messageEntityTextUrl, mtproto.Predicate_messageEntityEmail:
			return true
		}
	}
	if msg.GetMedia().GetPredicateName() == mtproto.Predicate_messageMediaWebPage {
		return true
	}
	text := strings.ToLower(msg.GetMessage())
	return strings.Contains(text, "http://") || strings.Contains(text, "https://") || strings.Contains(text, "www.")
}

// dropPrehistoryBoxes removes channel messages from before the viewer joined
// when that channel hides prehistory. blocked is consulted once per channel
// by the caller if it caches. Admins are the caller's concern: blocked
// returns false for them. searchGlobal (including a later rewrite) should
// keep this call so the history rule stays next to messages.search.
func dropPrehistoryBoxes(list *mtproto.MessageBoxList, blocked func(channelID int64, date int32) bool) *mtproto.MessageBoxList {
	if list == nil || blocked == nil {
		return list
	}
	kept := make([]*mtproto.MessageBox, 0, len(list.BoxList))
	for _, box := range list.BoxList {
		if box == nil {
			continue
		}
		if box.GetPeerType() == mtproto.PEER_CHANNEL {
			date := int32(0)
			if box.GetMessage() != nil {
				date = box.GetMessage().GetDate()
			}
			if blocked(box.GetPeerId(), date) {
				continue
			}
		}
		kept = append(kept, box)
	}
	list.BoxList = kept
	return list
}

func filterGlobalBoxes(list *mtproto.MessageBoxList, q globalQuery, kinds map[int64]channelKind, limit int32) *mtproto.MessageBoxList {
	if list == nil {
		return list
	}
	kept := make([]*mtproto.MessageBox, 0, len(list.BoxList))
	for _, box := range list.BoxList {
		if box == nil {
			continue
		}
		date := int32(0)
		if box.GetMessage() != nil {
			date = box.GetMessage().GetDate()
		}
		if !q.allows(box.GetPeerType(), box.GetPeerId(), date, kinds) {
			continue
		}
		kept = append(kept, box)
		if limit > 0 && int32(len(kept)) >= limit {
			break
		}
	}
	list.BoxList = kept
	return list
}
