package report

import (
	"context"
	"database/sql"

	"github.com/teamgram/proto/mtproto"
)

// RowFromStoryReport maps one story_reports row onto the unified table.
// ownerKind 1 is a user and 2 is a channel, matching biz/story.
func RowFromStoryReport(id, reporter int64, ownerKind int8, ownerID int64, storyIDs, option, message string, date int32) Row {
	peerType := int32(mtproto.PEER_USER)
	var subject int64
	if ownerKind == 2 {
		peerType = mtproto.PEER_CHANNEL
	} else {
		subject = ownerID
	}
	ids := splitIDs(storyIDs)
	return Row{
		Source:        SourceStory,
		Reporter:      reporter,
		PeerType:      peerType,
		PeerID:        ownerID,
		SubjectUserID: subject,
		TargetKey:     TargetKey(SourceStory, peerType, ownerID, ids, 0, 0),
		Reason:        clipRunes(option, 64),
		Message:       message,
		ObjectIDs:     ids,
		Status:        StatusNeedsHuman,
		LegacyStoryID: id,
		CreatedAt:     date,
	}
}

// BackfillStoryReports copies story_reports into reports. A second run
// inserts nothing. A missing story_reports table is not an error.
func BackfillStoryReports(ctx context.Context, db *sql.DB, st Store) (int, error) {
	if db == nil || st == nil {
		return 0, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT id, reporter_user_id, owner_kind, owner_id, story_ids, option_text, IFNULL(message,''), date2 FROM story_reports`)
	if err != nil {
		if isNoTable(err) {
			return 0, nil
		}
		return 0, err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var (
			id, reporter, ownerID int64
			kind                  int
			storyIDs, option, msg string
			date                  int32
		)
		if err = rows.Scan(&id, &reporter, &kind, &ownerID, &storyIDs, &option, &msg, &date); err != nil {
			return n, err
		}
		if id == 0 || reporter == 0 {
			continue
		}
		seen, err := st.HasLegacy(ctx, id)
		if err != nil {
			return n, err
		}
		if seen {
			continue
		}
		row := RowFromStoryReport(id, reporter, int8(kind), ownerID, storyIDs, option, msg, date)
		if err = st.Insert(ctx, &row); err != nil {
			if isDup(err) {
				continue
			}
			return n, err
		}
		n++
	}
	return n, rows.Err()
}
