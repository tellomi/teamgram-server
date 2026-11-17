package mediapurge

import (
	"encoding/json"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/proto"
)

const (
	KindPhoto    = "photo"
	KindDocument = "document"

	// BoxUser is one row in a user's message box (including saved messages).
	BoxUser = 1
	// BoxChannel is one channel_messages row. The channel has a single copy.
	BoxChannel = 2
	// BoxScheduled is a message that has not been sent yet.
	BoxScheduled = 3
)

// Media is one photo_id or document_id a message points at.
type Media struct {
	Kind string
	ID   int64
}

// Box is one message copy and the media it carried before the text was wiped.
type Box struct {
	Kind     int
	Owner    int64
	PeerType int32
	PeerID   int64
	ID       int32
	Media    []Media
}

func (b Box) Medias() []Media {
	out := b.Media[:0:0]
	seen := map[Media]bool{}
	for _, m := range b.Media {
		if m.ID == 0 || (m.Kind != KindPhoto && m.Kind != KindDocument) {
			continue
		}
		if seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, m)
	}
	return out
}

// FromMessage reads the photo and document ids a client message carries,
// including a webpage preview attached to the same message.
func FromMessage(m *mtproto.Message) []Media {
	if m == nil {
		return nil
	}
	return fromMedia(m.GetMedia())
}

func fromMedia(media *mtproto.MessageMedia) []Media {
	if media == nil {
		return nil
	}
	var out []Media
	add := func(kind string, id int64) {
		if id == 0 {
			return
		}
		out = append(out, Media{Kind: kind, ID: id})
	}
	if p := media.GetPhoto_FLAGPHOTO(); p != nil {
		add(KindPhoto, p.GetId())
	}
	if d := media.GetDocument(); d != nil {
		add(KindDocument, d.GetId())
	}
	// messageMediaPhoto.video: the video half of a live photo.
	if d := media.GetVideo_FLAGDOCUMENT(); d != nil {
		add(KindDocument, d.GetId())
	}
	if w := media.GetWebpage(); w != nil {
		if p := w.GetPhoto(); p != nil {
			add(KindPhoto, p.GetId())
		}
		if d := w.GetDocument(); d != nil {
			add(KindDocument, d.GetId())
		}
	}
	if g := media.GetGame(); g != nil {
		if d := g.GetDocument(); d != nil {
			add(KindDocument, d.GetId())
		}
	}
	return out
}

// FromJSON parses a user-box message_data blob written by jsonx.Marshal.
func FromJSON(raw string) []Media {
	if raw == "" || raw == "{}" || raw == "null" {
		return nil
	}
	var m mtproto.Message
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil
	}
	return FromMessage(&m)
}

// FromProto parses a channel message body or a scheduled message body.
func FromProto(raw []byte) []Media {
	if len(raw) == 0 {
		return nil
	}
	msg := &mtproto.Message{}
	if err := proto.Unmarshal(raw, msg); err != nil {
		return nil
	}
	return FromMessage(msg)
}

// FromStory reads the media on a story item.
func FromStory(raw []byte) []Media {
	if len(raw) == 0 {
		return nil
	}
	item := &mtproto.StoryItem{}
	if err := proto.Unmarshal(raw, item); err != nil {
		return nil
	}
	return fromMedia(item.GetMedia())
}
