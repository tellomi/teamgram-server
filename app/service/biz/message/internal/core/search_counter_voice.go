package core

import "github.com/teamgram/proto/mtproto"

// counterPlan is how messages.getSearchCounters counts one stored kind.
// indexed types are message_filter_type values, the same column a photo
// uses, so a row is visible as soon as the insert commits. legacyVoice and
// legacyRound add MEDIA_AUDIO rows from before voice and round video were
// stored apart. Those rows are not in the indexed list, so they are not
// counted twice.
func counterPlan(mediaType int32) (indexed []int32, legacyVoice, legacyRound, ok bool) {
	switch mediaType {
	case mtproto.MEDIA_VOICE_FILE:
		return []int32{mtproto.MEDIA_VOICE_FILE}, true, false, true
	case mtproto.MEDIA_ROUND_FILE:
		return []int32{mtproto.MEDIA_ROUND_FILE}, false, true, true
	case mtproto.MEDIA_AUDIO:
		return []int32{mtproto.MEDIA_AUDIO, mtproto.MEDIA_VOICE_FILE, mtproto.MEDIA_ROUND_FILE}, false, false, true
	default:
		return nil, false, false, false
	}
}

// searchMediaTypes is the filter types a media search has to read.
// New voice notes and round videos are not MEDIA_AUDIO. A search for the
// old bucket, or for one of the new ones, still has to see the other.
func searchMediaTypes(mediaType int32) []int32 {
	switch mediaType {
	case mtproto.MEDIA_AUDIO:
		return []int32{mtproto.MEDIA_AUDIO, mtproto.MEDIA_VOICE_FILE, mtproto.MEDIA_ROUND_FILE}
	case mtproto.MEDIA_VOICE_FILE:
		return []int32{mtproto.MEDIA_VOICE_FILE, mtproto.MEDIA_AUDIO}
	case mtproto.MEDIA_ROUND_FILE:
		return []int32{mtproto.MEDIA_ROUND_FILE, mtproto.MEDIA_AUDIO}
	default:
		return []int32{mediaType}
	}
}

// visibleMedia is one committed message row.
type visibleMedia struct {
	FilterType int32
	Message    *mtproto.Message
}

// countVisibleMedia applies counterPlan to rows that are already stored.
// It does not wait, and it does not read a row twice.
func countVisibleMedia(rows []visibleMedia, mediaType int32) int32 {
	indexed, legacyVoice, legacyRound, ok := counterPlan(mediaType)
	if !ok {
		return 0
	}
	want := map[int32]struct{}{}
	for _, kind := range indexed {
		want[kind] = struct{}{}
	}
	var n int32
	for _, row := range rows {
		if _, yes := want[row.FilterType]; yes {
			n++
			continue
		}
		if row.FilterType != mtproto.MEDIA_AUDIO {
			continue
		}
		if legacyVoice && mtproto.IsVoiceMessage(row.Message) {
			n++
		}
		if legacyRound && mtproto.IsRoundVideoMessage(row.Message) {
			n++
		}
	}
	return n
}
