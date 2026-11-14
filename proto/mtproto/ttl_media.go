package mtproto

import "google.golang.org/protobuf/types/known/wrapperspb"

// ViewOnceTTL is the ttl_seconds official clients send for view-once media.
// core.telegram.org/api/views: do not start a timer; the media is consumed
// when it is opened. Any other positive ttl_seconds starts a countdown at
// the open time carried by updateReadMessagesContents.date.
const ViewOnceTTL int32 = 0x7FFFFFFF

const (
	MediaKindPhoto    = "photo"
	MediaKindDocument = "document"
)

// RemovedMedia is one photo or document id stripped from a ttl message.
type RemovedMedia struct {
	Kind string
	ID   int64
}

// TTLSeconds is the self-destruct timer on the message media.
// Zero means the message is not ttl media (the field is absent, or it is 0,
// which official clients use to turn self-destruction off).
func TTLSeconds(m *Message) int32 {
	if m == nil || m.GetMedia() == nil || m.GetMedia().GetTtlSeconds() == nil {
		return 0
	}
	v := m.GetMedia().GetTtlSeconds().GetValue()
	if v <= 0 {
		return 0
	}
	return v
}

// IsViewOnce reports the official view-once sentinel.
func IsViewOnce(ttl int32) bool {
	return ttl == ViewOnceTTL
}

// TTLDue reports whether an opened ttl message should be stored as the
// expired form. openedAt is the unix time messages.readMessageContents ran.
// An unopened message (openedAt == 0) is never due: the timer starts at open,
// not at send.
func TTLDue(ttl, openedAt, now int32) bool {
	if ttl <= 0 || openedAt <= 0 {
		return false
	}
	if IsViewOnce(ttl) {
		return true
	}
	return openedAt+ttl <= now
}

// MakeUpdateReadMessagesContents builds the update both sides receive when
// media is opened. date is the server time of the open. Official Android
// treats a missing date as 0, so a timed item looks already expired.
func MakeUpdateReadMessagesContents(ids []int32, pts, ptsCount, date int32) *Update {
	upd := &Update{
		Messages:  ids,
		Pts_INT32: pts,
		PtsCount:  ptsCount,
	}
	if date != 0 {
		upd.Date_FLAGINT32 = &wrapperspb.Int32Value{Value: date}
	}
	return MakeTLUpdateReadMessagesContents(upd).To_Update()
}

// StripTTLMedia rewrites ttl media to the expired form official clients
// render as "has expired": ttl_seconds stays, the photo is photoEmpty, and
// the document is documentEmpty. Voice and round flags are kept so the
// expired bubble still says which kind it was. A message that is not ttl
// media is left unchanged and returns nil.
func StripTTLMedia(m *Message) []RemovedMedia {
	if m == nil || TTLSeconds(m) == 0 {
		return nil
	}
	media := m.GetMedia()
	var removed []RemovedMedia
	add := func(kind string, id int64, pred, empty string) {
		if id == 0 || pred == empty {
			return
		}
		removed = append(removed, RemovedMedia{Kind: kind, ID: id})
	}
	if p := media.GetPhoto_FLAGPHOTO(); p != nil {
		add(MediaKindPhoto, p.GetId(), p.GetPredicateName(), Predicate_photoEmpty)
	}
	if d := media.GetDocument(); d != nil {
		if IsVoiceDocument(d) {
			media.Voice = true
		}
		if IsRoundVideoDocument(d) {
			media.Round = true
		}
		add(MediaKindDocument, d.GetId(), d.GetPredicateName(), Predicate_documentEmpty)
	}
	if d := media.GetVideo_FLAGDOCUMENT(); d != nil {
		add(MediaKindDocument, d.GetId(), d.GetPredicateName(), Predicate_documentEmpty)
	}
	switch media.GetPredicateName() {
	case Predicate_messageMediaPhoto:
		media.Photo_FLAGPHOTO = MakeTLPhotoEmpty(&Photo{}).To_Photo()
		// live_photo and video share flags.4: clearing one without the
		// other sets the bit with no Document and breaks client parsing.
		media.Video_FLAGDOCUMENT = nil
		media.LivePhoto = false
	case Predicate_messageMediaDocument:
		media.Document = MakeTLDocumentEmpty(&Document{}).To_Document()
		media.AltDocuments = nil
		media.VideoCover = nil
	default:
		return nil
	}
	m.MediaUnread = false
	return removed
}
