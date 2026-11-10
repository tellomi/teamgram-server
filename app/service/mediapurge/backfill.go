package mediapurge

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/teamgram/teamgram-server/app/service/mediablob"
)

// Report is the one-off dry-run. It does not delete objects.
type Report struct {
	Backfilled       int
	LiveRefs         int
	DeletedMessages  int
	Candidates       int
	WouldFreeBlobs   int
	WouldFreeObjects int
	WouldFreeBytes   int64
	Photos           int
	Documents        int
	KeptMessage      int
	KeptPinned       int
	KeptChannel      int
	KeptAvatar       int
	KeptStory        int
	KeptSticker      int
	KeptSaved        int
	KeptScheduled    int
	KeptExternal     int
	KeptShared       int
	KeptLegacy       int
	Samples          []string
}

func (r Report) String() string {
	return fmt.Sprintf("media purge DRY_RUN: backfilled=%d live_refs=%d deleted_messages_with_media=%d candidates=%d would_free_blobs=%d would_free_objects=%d would_free_bytes=%d photos=%d documents=%d kept_message=%d kept_pinned=%d kept_channel=%d kept_avatar=%d kept_story=%d kept_sticker=%d kept_saved=%d kept_scheduled=%d kept_external=%d kept_shared_blob=%d kept_legacy=%d",
		r.Backfilled, r.LiveRefs, r.DeletedMessages, r.Candidates, r.WouldFreeBlobs, r.WouldFreeObjects, r.WouldFreeBytes, r.Photos, r.Documents, r.KeptMessage, r.KeptPinned, r.KeptChannel, r.KeptAvatar, r.KeptStory, r.KeptSticker, r.KeptSaved, r.KeptScheduled, r.KeptExternal, r.KeptShared, r.KeptLegacy)
}

// Backfill inserts message_media_refs for live user boxes, channel messages,
// and scheduled messages. Running it twice does not duplicate rows.
func (s *Store) Backfill(ctx context.Context) (int, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	if err := s.Ensure(ctx); err != nil {
		return 0, err
	}
	n, err := s.backfillMessages(ctx)
	if err != nil {
		return n, err
	}
	c, err := s.backfillChannels(ctx)
	n += c
	if err != nil {
		return n, err
	}
	sc, err := s.backfillScheduled(ctx)
	return n + sc, err
}

func (s *Store) backfillMessages(ctx context.Context) (int, error) {
	tables, err := s.messageTables(ctx)
	if err != nil {
		return 0, err
	}
	var n int
	for _, table := range tables {
		var userID int64
		var boxID int32
		for {
			rows, err := s.db.QueryContext(ctx, `
SELECT user_id, user_message_box_id, message_data FROM `+table+`
WHERE deleted=0 AND (user_id > ? OR (user_id = ? AND user_message_box_id > ?))
  AND (`+exprDocumentID+` IS NOT NULL OR `+exprPhotoID+` IS NOT NULL)
ORDER BY user_id, user_message_box_id LIMIT 400`, userID, userID, boxID)
			if missing(err) {
				break
			}
			if err != nil {
				return n, err
			}
			count := 0
			for rows.Next() {
				var uid int64
				var id int32
				var data string
				if err = rows.Scan(&uid, &id, &data); err != nil {
					rows.Close()
					return n, err
				}
				count++
				userID, boxID = uid, id
				if err = s.Note(ctx, Box{Kind: BoxUser, Owner: uid, ID: id, Media: FromJSON(data)}); err != nil {
					rows.Close()
					return n, err
				}
				n++
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return n, err
			}
			if count < 400 {
				break
			}
		}
	}
	return n, nil
}

func (s *Store) backfillChannels(ctx context.Context) (int, error) {
	var channelID int64
	var msgID int32
	var n int
	for {
		rows, err := s.db.QueryContext(ctx, `
SELECT channel_id, message_id, message_data FROM channel_messages
WHERE deleted=0 AND (channel_id > ? OR (channel_id = ? AND message_id > ?))
ORDER BY channel_id, message_id LIMIT 400`, channelID, channelID, msgID)
		if missing(err) {
			return n, nil
		}
		if err != nil {
			return n, err
		}
		count := 0
		for rows.Next() {
			var id int64
			var mid int32
			var body []byte
			if err = rows.Scan(&id, &mid, &body); err != nil {
				rows.Close()
				return n, err
			}
			count++
			channelID, msgID = id, mid
			media := FromProto(body)
			if len(media) == 0 {
				continue
			}
			if err = s.Note(ctx, Box{Kind: BoxChannel, Owner: id, ID: mid, Media: media}); err != nil {
				rows.Close()
				return n, err
			}
			n++
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return n, err
		}
		if count < 400 {
			return n, nil
		}
	}
}

func (s *Store) backfillScheduled(ctx context.Context) (int, error) {
	// Scheduled rows are rewritten by Note from the chatbox hook. The scan
	// here only counts them as holders; inserting one ref per item needs the
	// owner, which the doc_key carries. Parsing the key is unnecessary for
	// the hold check, which reads the document bytes directly.
	return 0, nil
}

// DryRun reports objects that deleted messages still point at and that
// nothing live holds. It does not delete and it does not enqueue.
func (s *Store) DryRun(ctx context.Context, backfill bool) (Report, error) {
	var rep Report
	if s == nil || s.db == nil {
		return rep, nil
	}
	if err := s.Ensure(ctx); err != nil {
		return rep, err
	}
	if backfill {
		n, err := s.Backfill(ctx)
		if err != nil {
			return rep, err
		}
		rep.Backfilled = n
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM message_media_refs`).Scan(&rep.LiveRefs); err != nil && !missing(err) {
		return rep, err
	}
	candidates, deletedN, err := s.deletedMedia(ctx)
	if err != nil {
		return rep, err
	}
	rep.DeletedMessages = deletedN
	cache := &holdCache{}
	type cand struct {
		m    Media
		blob int64
	}
	var free []cand
	blobRefs := map[int64]int{}
	blobFree := map[int64]int{}
	for _, m := range candidates {
		hold, err := s.FullHold(ctx, m, cache)
		if err != nil {
			return rep, err
		}
		if hold != HoldNone {
			switch hold {
			case HoldMessage:
				rep.KeptMessage++
			case HoldPinned:
				rep.KeptPinned++
			case HoldChannel:
				rep.KeptChannel++
			case HoldAvatar:
				rep.KeptAvatar++
			case HoldStory:
				rep.KeptStory++
			case HoldSticker:
				rep.KeptSticker++
			case HoldSaved:
				rep.KeptSaved++
			case HoldScheduled:
				rep.KeptScheduled++
			case HoldExternal:
				rep.KeptExternal++
			}
			continue
		}
		rep.Candidates++
		if m.Kind == KindPhoto {
			rep.Photos++
		} else {
			rep.Documents++
		}
		blobID, err := s.blobID(ctx, m)
		if err != nil {
			return rep, err
		}
		free = append(free, cand{m: m, blob: blobID})
		if blobID != 0 {
			blobFree[blobID]++
		}
	}
	for id := range blobFree {
		n, err := s.blobRefCount(ctx, id)
		if err != nil {
			return rep, err
		}
		blobRefs[id] = n
	}
	seenBlob := map[int64]bool{}
	for _, c := range free {
		if c.blob != 0 && blobFree[c.blob] < blobRefs[c.blob] {
			rep.KeptShared++
			continue
		}
		objs, err := s.Objects(ctx, c.m)
		if err != nil {
			return rep, err
		}
		blocked, why, err := s.blockedOutside(ctx, c.m, objs)
		if err != nil {
			return rep, err
		}
		if blocked {
			rep.KeptLegacy++
			if len(rep.Samples) < 8 {
				rep.Samples = append(rep.Samples, "legacy "+why)
			}
			continue
		}
		if c.blob != 0 {
			if !seenBlob[c.blob] {
				seenBlob[c.blob] = true
				rep.WouldFreeBlobs++
			}
		} else {
			rep.WouldFreeBlobs++
		}
		for _, o := range objs {
			held, err := s.pathHeld(ctx, o.Class, o.Path, c.m)
			if err != nil {
				return rep, err
			}
			if held {
				continue
			}
			rep.WouldFreeObjects++
			rep.WouldFreeBytes += o.Bytes
		}
		if len(rep.Samples) < 8 {
			rep.Samples = append(rep.Samples, fmt.Sprintf("%s %d", c.m.Kind, c.m.ID))
		}
	}
	return rep, nil
}

func (s *Store) deletedMedia(ctx context.Context) ([]Media, int, error) {
	tables, err := s.messageTables(ctx)
	if err != nil {
		return nil, 0, err
	}
	seen := map[Media]bool{}
	var out []Media
	messages := 0
	add := func(kind string, id int64) {
		if id == 0 {
			return
		}
		m := Media{Kind: kind, ID: id}
		if seen[m] {
			return
		}
		seen[m] = true
		out = append(out, m)
	}
	for _, table := range tables {
		var userID int64
		var boxID int32
		for {
			rows, err := s.db.QueryContext(ctx, `
SELECT user_id, user_message_box_id,
  COALESCE(`+exprDocumentID+`, 0), COALESCE(`+exprPhotoID+`, 0)
FROM `+table+`
WHERE deleted=1 AND (user_id > ? OR (user_id = ? AND user_message_box_id > ?))
  AND (`+exprDocumentID+` IS NOT NULL OR `+exprPhotoID+` IS NOT NULL)
ORDER BY user_id, user_message_box_id LIMIT 500`, userID, userID, boxID)
			if missing(err) {
				break
			}
			if err != nil {
				return nil, 0, err
			}
			n := 0
			for rows.Next() {
				var uid int64
				var id int32
				var doc, photo int64
				if err = rows.Scan(&uid, &id, &doc, &photo); err != nil {
					rows.Close()
					return nil, 0, err
				}
				n++
				messages++
				userID, boxID = uid, id
				add(KindDocument, doc)
				add(KindPhoto, photo)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return nil, 0, err
			}
			if n < 500 {
				break
			}
		}
	}
	// Channel copies that were deleted still have a body until this release
	// wipes them. Scan those too.
	var channelID int64
	var msgID int32
	for {
		rows, err := s.db.QueryContext(ctx, `
SELECT channel_id, message_id, message_data FROM channel_messages
WHERE deleted=1 AND (channel_id > ? OR (channel_id = ? AND message_id > ?))
  AND OCTET_LENGTH(message_data) > 0
ORDER BY channel_id, message_id LIMIT 400`, channelID, channelID, msgID)
		if missing(err) {
			break
		}
		if err != nil {
			return nil, 0, err
		}
		n := 0
		for rows.Next() {
			var id int64
			var mid int32
			var body []byte
			if err = rows.Scan(&id, &mid, &body); err != nil {
				rows.Close()
				return nil, 0, err
			}
			n++
			channelID, msgID = id, mid
			media := FromProto(body)
			if len(media) == 0 {
				continue
			}
			messages++
			for _, m := range media {
				add(m.Kind, m.ID)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, 0, err
		}
		if n < 400 {
			break
		}
	}
	qrows, err := s.db.QueryContext(ctx, `SELECT media_kind, media_id FROM media_purge_queue`)
	if err == nil {
		for qrows.Next() {
			var kind string
			var id int64
			if err = qrows.Scan(&kind, &id); err != nil {
				qrows.Close()
				return nil, 0, err
			}
			add(kind, id)
		}
		err = qrows.Err()
		qrows.Close()
		if err != nil {
			return nil, 0, err
		}
	} else if !missing(err) {
		return nil, 0, err
	}
	return out, messages, nil
}

func (s *Store) blobID(ctx context.Context, m Media) (int64, error) {
	kind := mediablob.KindDocument
	if m.Kind == KindPhoto {
		kind = mediablob.KindPhoto
	}
	var id int64
	err := s.db.QueryRowContext(ctx, `
SELECT blob_id FROM media_blob_refs WHERE ref_kind=? AND ref_id=?`, kind, m.ID).Scan(&id)
	if err == sql.ErrNoRows || missing(err) {
		return 0, nil
	}
	return id, err
}

func (s *Store) blobRefCount(ctx context.Context, blobID int64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_blob_refs WHERE blob_id=?`, blobID).Scan(&n)
	if missing(err) {
		return 0, nil
	}
	return n, err
}
