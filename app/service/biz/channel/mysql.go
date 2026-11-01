package channel

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/readtime"
)

type mysqlStore struct {
	db *sql.DB
}

func newMySQLStore(db *sql.DB) Store {
	return &mysqlStore{db: db}
}

func ensureSchema(ctx context.Context, db *sql.DB) error {
	for _, stmt := range strings.Split(schemaSQL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *mysqlStore) AllocID(ctx context.Context) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE channel_id_counter SET next_id = LAST_INSERT_ID(next_id) + 1 WHERE id = 1`); err != nil {
		return 0, err
	}
	var id int64
	if err = tx.QueryRowContext(ctx, `SELECT LAST_INSERT_ID()`).Scan(&id); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	if id < mtproto.MinNebulaChatChannelID {
		return 0, mtproto.ErrInternalServerError
	}
	return id, nil
}

func (s *mysqlStore) CreateChannel(ctx context.Context, ch *Channel, creator *Participant) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if ch.ID == 0 {
		if _, err = tx.ExecContext(ctx, `UPDATE channel_id_counter SET next_id = LAST_INSERT_ID(next_id) + 1 WHERE id = 1`); err != nil {
			return err
		}
		if err = tx.QueryRowContext(ctx, `SELECT LAST_INSERT_ID()`).Scan(&ch.ID); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `
INSERT INTO channels (
  id, access_hash, creator_user_id, title, about, username, photo_id,
  broadcast, megagroup, forum, signatures, pts, top_message, participants_count,
  date2, migrated_from_chat_id, migrated_from_max_id, ttl_period, address,
  default_banned_rights, deleted, version)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		ch.ID, ch.AccessHash, ch.CreatorID, ch.Title, ch.About, nullString(ch.Username), ch.PhotoID,
		boolInt(ch.Broadcast), boolInt(ch.Megagroup), boolInt(ch.Forum), boolInt(ch.Signatures),
		ch.Pts, ch.TopMessage, ch.ParticipantsCount, ch.Date, ch.MigratedFromChatID, ch.MigratedFromMaxID,
		ch.TtlPeriod, ch.Address, ch.DefaultBanned, boolInt(ch.Deleted), ch.Version,
	); err != nil {
		return err
	}
	creator.ChannelID = ch.ID
	if err = insertParticipant(ctx, tx, creator); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *mysqlStore) GetChannel(ctx context.Context, id int64) (*Channel, error) {
	row := s.db.QueryRowContext(ctx, channelSelect+` WHERE id=?`, id)
	ch, err := scanChannel(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return ch, err
}

func (s *mysqlStore) SaveChannel(ctx context.Context, ch *Channel) error {
	// pts and top_message stay under Commit* so this write cannot roll a message back.
	_, err := s.db.ExecContext(ctx, `
UPDATE channels SET access_hash=?, creator_user_id=?, title=?, about=?, username=?, photo_id=?,
  broadcast=?, megagroup=?, forum=?, signatures=?, participants_count=?,
  date2=?, migrated_from_chat_id=?, migrated_from_max_id=?, ttl_period=?, address=?,
  default_banned_rights=?, deleted=?, version=? WHERE id=?`,
		ch.AccessHash, ch.CreatorID, ch.Title, ch.About, nullString(ch.Username), ch.PhotoID,
		boolInt(ch.Broadcast), boolInt(ch.Megagroup), boolInt(ch.Forum), boolInt(ch.Signatures),
		ch.ParticipantsCount, ch.Date, ch.MigratedFromChatID, ch.MigratedFromMaxID,
		ch.TtlPeriod, ch.Address, ch.DefaultBanned, boolInt(ch.Deleted), ch.Version, ch.ID)
	return err
}

func (s *mysqlStore) FindByUsername(ctx context.Context, username string) (*Channel, error) {
	id, active, found, err := s.LookupUsername(ctx, username)
	if err != nil || !found || !active {
		return nil, err
	}
	ch, err := s.GetChannel(ctx, id)
	if err != nil || ch == nil || ch.Deleted {
		return nil, err
	}
	return ch, nil
}

func (s *mysqlStore) LookupUsername(ctx context.Context, username string) (int64, bool, bool, error) {
	if username == "" {
		return 0, false, false, nil
	}
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM channels WHERE username=? AND deleted=0`, username).Scan(&id)
	if err == nil {
		return id, true, true, nil
	}
	if err != sql.ErrNoRows {
		return 0, false, false, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT channel_id, data FROM channel_feature`)
	if err != nil {
		return 0, false, false, err
	}
	defer rows.Close()
	var inactiveID int64
	var inactive bool
	for rows.Next() {
		var channelID int64
		var data []byte
		if err := rows.Scan(&channelID, &data); err != nil {
			return 0, false, false, err
		}
		var f Features
		if json.Unmarshal(data, &f) != nil {
			continue
		}
		for _, u := range f.Usernames {
			if !equalFold(u.Username, username) {
				continue
			}
			if u.Active {
				return channelID, true, true, nil
			}
			inactiveID = channelID
			inactive = true
		}
	}
	if err := rows.Err(); err != nil {
		return 0, false, false, err
	}
	if inactive {
		return inactiveID, false, true, nil
	}
	return 0, false, false, nil
}

func (s *mysqlStore) ListPublic(ctx context.Context) ([]*Channel, error) {
	rows, err := s.db.QueryContext(ctx, channelSelect+`
 WHERE deleted=0 AND username IS NOT NULL AND username <> ''
 ORDER BY participants_count DESC, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChannels(rows)
}

func (s *mysqlStore) ListLeftByUser(ctx context.Context, userID int64) ([]*Channel, error) {
	rows, err := s.db.QueryContext(ctx, channelSelect+`
 WHERE id IN (SELECT channel_id FROM channel_participants WHERE user_id=? AND state=?) AND deleted=0
 ORDER BY id`, userID, StateLeft)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChannels(rows)
}

func (s *mysqlStore) FindByMigratedChat(ctx context.Context, chatID int64) (*Channel, error) {
	row := s.db.QueryRowContext(ctx, channelSelect+` WHERE migrated_from_chat_id=? AND deleted=0`, chatID)
	ch, err := scanChannel(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return ch, err
}

func (s *mysqlStore) ListChannels(ctx context.Context, ids []int64) ([]*Channel, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	q, args := inClause(channelSelect+` WHERE id IN (`, ids)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChannels(rows)
}

func (s *mysqlStore) ListByUser(ctx context.Context, userID int64) ([]*Channel, error) {
	rows, err := s.db.QueryContext(ctx, channelSelect+`
 WHERE id IN (SELECT channel_id FROM channel_participants WHERE user_id=? AND state=?) AND deleted=0
 ORDER BY id`, userID, StateNormal)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChannels(rows)
}

func (s *mysqlStore) PutParticipant(ctx context.Context, p *Participant) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO channel_participants (
  channel_id, user_id, role, state, admin_rights, banned_rights, rank_name, inviter_user_id, is_bot, date2)
VALUES (?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE role=VALUES(role), state=VALUES(state), admin_rights=VALUES(admin_rights),
  banned_rights=VALUES(banned_rights), rank_name=VALUES(rank_name), inviter_user_id=VALUES(inviter_user_id),
  is_bot=VALUES(is_bot), date2=VALUES(date2)`,
		p.ChannelID, p.UserID, p.Role, p.State, p.AdminRights, p.BannedRights, p.Rank, p.InviterID, boolInt(p.IsBot), p.Date)
	return err
}

func (s *mysqlStore) GetParticipant(ctx context.Context, channelID, userID int64) (*Participant, error) {
	row := s.db.QueryRowContext(ctx, participantSelect+` WHERE channel_id=? AND user_id=?`, channelID, userID)
	p, err := scanParticipant(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

func (s *mysqlStore) ListParticipants(ctx context.Context, channelID int64) ([]*Participant, error) {
	rows, err := s.db.QueryContext(ctx, participantSelect+` WHERE channel_id=? ORDER BY date2, user_id`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Participant
	for rows.Next() {
		p, err := scanParticipant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *mysqlStore) CountMembers(ctx context.Context, channelID int64) (int32, error) {
	var n int32
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM channel_participants WHERE channel_id=? AND state=?`, channelID, StateNormal).Scan(&n)
	return n, err
}

func (s *mysqlStore) CommitNew(ctx context.Context, channelID, senderID, randomID int64, date int32, build BuildMessage) (int32, int32, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, false, err
	}
	defer tx.Rollback()
	var pts, top int32
	var deleted int
	err = tx.QueryRowContext(ctx, `SELECT pts, top_message, deleted FROM channels WHERE id=? FOR UPDATE`, channelID).Scan(&pts, &top, &deleted)
	if err == sql.ErrNoRows || deleted != 0 {
		return 0, 0, false, mtproto.ErrChannelInvalid
	}
	if err != nil {
		return 0, 0, false, err
	}
	if randomID != 0 {
		var id, oldPts int32
		err = tx.QueryRowContext(ctx, `SELECT message_id, pts FROM channel_messages WHERE channel_id=? AND sender_user_id=? AND random_id=? AND deleted=0 LIMIT 1`,
			channelID, senderID, randomID).Scan(&id, &oldPts)
		if err == nil {
			if err = tx.Commit(); err != nil {
				return 0, 0, false, err
			}
			return id, oldPts, true, nil
		}
		if err != sql.ErrNoRows {
			return 0, 0, false, err
		}
	}
	top++
	pts++
	payload := build(top, pts)
	if _, err = tx.ExecContext(ctx, `UPDATE channels SET pts=?, top_message=? WHERE id=?`, pts, top, channelID); err != nil {
		return 0, 0, false, err
	}
	if _, err = tx.ExecContext(ctx, `
INSERT INTO channel_messages (channel_id, message_id, sender_user_id, date2, edit_date, random_id, pts, deleted, message_data)
VALUES (?,?,?,?,0,?,?,0,?)`, channelID, top, senderID, date, randomID, pts, payload); err != nil {
		return 0, 0, false, err
	}
	if _, err = tx.ExecContext(ctx, `
INSERT INTO channel_pts_updates (channel_id, pts, pts_count, kind, message_id, message_ids, payload)
VALUES (?,?,?,?,?,?,?)`, channelID, pts, 1, KindNew, top, "", payload); err != nil {
		return 0, 0, false, err
	}
	if err = tx.Commit(); err != nil {
		return 0, 0, false, err
	}
	return top, pts, false, nil
}

func (s *mysqlStore) CommitEdit(ctx context.Context, channelID int64, messageID int32, editDate int32, data []byte) (int32, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var pts, deleted int32
	err = tx.QueryRowContext(ctx, `SELECT pts, deleted FROM channels WHERE id=? FOR UPDATE`, channelID).Scan(&pts, &deleted)
	if err == sql.ErrNoRows || deleted != 0 {
		return 0, mtproto.ErrChannelInvalid
	}
	if err != nil {
		return 0, err
	}
	var msgDeleted int
	err = tx.QueryRowContext(ctx, `SELECT deleted FROM channel_messages WHERE channel_id=? AND message_id=? FOR UPDATE`, channelID, messageID).Scan(&msgDeleted)
	if err == sql.ErrNoRows || msgDeleted != 0 {
		return 0, mtproto.ErrMessageIdInvalid
	}
	if err != nil {
		return 0, err
	}
	pts++
	if _, err = tx.ExecContext(ctx, `UPDATE channel_messages SET message_data=?, edit_date=? WHERE channel_id=? AND message_id=?`, data, editDate, channelID, messageID); err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE channels SET pts=? WHERE id=?`, pts, channelID); err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `
INSERT INTO channel_pts_updates (channel_id, pts, pts_count, kind, message_id, message_ids, payload)
VALUES (?,?,?,?,?,?,?)`, channelID, pts, 1, KindEdit, messageID, "", data); err != nil {
		return 0, err
	}
	return pts, tx.Commit()
}

func (s *mysqlStore) CommitDelete(ctx context.Context, channelID int64, ids []int32) (int32, int32, []int32, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, nil, err
	}
	defer tx.Rollback()
	var pts, deletedCh int32
	err = tx.QueryRowContext(ctx, `SELECT pts, deleted FROM channels WHERE id=? FOR UPDATE`, channelID).Scan(&pts, &deletedCh)
	if err == sql.ErrNoRows || deletedCh != 0 {
		return 0, 0, nil, mtproto.ErrChannelInvalid
	}
	if err != nil {
		return 0, 0, nil, err
	}
	var deleted []int32
	for _, id := range ids {
		res, err := tx.ExecContext(ctx, `UPDATE channel_messages SET deleted=1 WHERE channel_id=? AND message_id=? AND deleted=0`, channelID, id)
		if err != nil {
			return 0, 0, nil, err
		}
		n, _ := res.RowsAffected()
		if n > 0 {
			deleted = append(deleted, id)
		}
	}
	if len(deleted) == 0 {
		if err = tx.Commit(); err != nil {
			return 0, 0, nil, err
		}
		return pts, 0, nil, nil
	}
	pts += int32(len(deleted))
	idText := joinInt32(deleted)
	if _, err = tx.ExecContext(ctx, `UPDATE channels SET pts=? WHERE id=?`, pts, channelID); err != nil {
		return 0, 0, nil, err
	}
	if _, err = tx.ExecContext(ctx, `
INSERT INTO channel_pts_updates (channel_id, pts, pts_count, kind, message_id, message_ids, payload)
VALUES (?,?,?,?,0,?,?)`, channelID, pts, len(deleted), KindDelete, idText, []byte{}); err != nil {
		return 0, 0, nil, err
	}
	if err = tx.Commit(); err != nil {
		return 0, 0, nil, err
	}
	return pts, int32(len(deleted)), deleted, nil
}

func (s *mysqlStore) ReplaceMessageBody(ctx context.Context, channelID int64, id int32, data []byte) error {
	res, err := s.db.ExecContext(ctx, `UPDATE channel_messages SET message_data=? WHERE channel_id=? AND message_id=? AND deleted=0`, data, channelID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return mtproto.ErrMessageIdInvalid
	}
	return nil
}

func (s *mysqlStore) GetMessage(ctx context.Context, channelID int64, id int32) (*StoredMessage, error) {
	row := s.db.QueryRowContext(ctx, messageSelect+` WHERE channel_id=? AND message_id=?`, channelID, id)
	m, err := scanMessage(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return m, err
}

func (s *mysqlStore) ListMessages(ctx context.Context, channelID int64) ([]*StoredMessage, error) {
	rows, err := s.db.QueryContext(ctx, messageSelect+` WHERE channel_id=?`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*StoredMessage
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *mysqlStore) ListPtsAfter(ctx context.Context, channelID int64, pts int32, limit int) ([]*PtsEvent, error) {
	q := `SELECT channel_id, pts, pts_count, kind, message_id, message_ids, payload FROM channel_pts_updates WHERE channel_id=? AND pts>? ORDER BY pts`
	args := []any{channelID, pts}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*PtsEvent
	for rows.Next() {
		var e PtsEvent
		var ids string
		if err := rows.Scan(&e.ChannelID, &e.Pts, &e.PtsCount, &e.Kind, &e.MessageID, &ids, &e.Payload); err != nil {
			return nil, err
		}
		e.MessageIDs = splitInt32(ids)
		out = append(out, &e)
	}
	return out, rows.Err()
}

func (s *mysqlStore) GetRead(ctx context.Context, channelID, userID int64) (*ReadState, error) {
	st := &ReadState{ChannelID: channelID, UserID: userID}
	err := s.db.QueryRowContext(ctx, `SELECT read_inbox_max_id, read_outbox_max_id, unread_count FROM channel_read_state WHERE channel_id=? AND user_id=?`,
		channelID, userID).Scan(&st.ReadInboxMaxID, &st.ReadOutboxMaxID, &st.UnreadCount)
	if err == sql.ErrNoRows {
		return st, nil
	}
	return st, err
}

func (s *mysqlStore) SaveRead(ctx context.Context, st *ReadState) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO channel_read_state (channel_id, user_id, read_inbox_max_id, read_outbox_max_id, unread_count)
VALUES (?,?,?,?,?)
ON DUPLICATE KEY UPDATE read_inbox_max_id=VALUES(read_inbox_max_id), read_outbox_max_id=VALUES(read_outbox_max_id), unread_count=VALUES(unread_count)`,
		st.ChannelID, st.UserID, st.ReadInboxMaxID, st.ReadOutboxMaxID, st.UnreadCount)
	return err
}

func (s *mysqlStore) SaveReadMark(ctx context.Context, ownerUserID, peerDialogID, readerUserID int64, maxID int32, date int64) error {
	if ownerUserID == 0 || readerUserID == 0 || ownerUserID == readerUserID || maxID <= 0 || date <= 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx, readtime.PositionalUpsert(), ownerUserID, peerDialogID, readerUserID, maxID, date)
	return err
}

func (s *mysqlStore) ListReadMarks(ctx context.Context, ownerUserID, peerDialogID int64, msgID int32) ([]ReadMark, error) {
	rows, err := s.db.QueryContext(ctx, readtime.SelectHitsSQL(false, true), ownerUserID, peerDialogID, msgID)
	if err != nil {
		rows, err = s.db.QueryContext(ctx, readtime.SelectHitsSQL(false, false), ownerUserID, peerDialogID, msgID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReadMark
	for rows.Next() {
		var userID, date int64
		if err := rows.Scan(&userID, &date); err != nil {
			return nil, err
		}
		picked := readtime.Pick(date)
		if userID == 0 || picked == 0 {
			continue
		}
		out = append(out, ReadMark{UserID: userID, MaxID: msgID, Date: int64(picked)})
	}
	return out, rows.Err()
}

const channelSelect = `SELECT id, access_hash, creator_user_id, title, about, username, photo_id, broadcast, megagroup, forum, signatures, pts, top_message, participants_count, date2, migrated_from_chat_id, migrated_from_max_id, ttl_period, address, default_banned_rights, deleted, version FROM channels`

const participantSelect = `SELECT channel_id, user_id, role, state, admin_rights, banned_rights, rank_name, inviter_user_id, is_bot, date2 FROM channel_participants`

const messageSelect = `SELECT channel_id, message_id, sender_user_id, date2, edit_date, random_id, pts, deleted, message_data FROM channel_messages`

type scanner interface {
	Scan(dest ...any) error
}

func scanChannel(row scanner) (*Channel, error) {
	var ch Channel
	var username sql.NullString
	var broadcast, megagroup, forum, signatures, deleted int
	err := row.Scan(&ch.ID, &ch.AccessHash, &ch.CreatorID, &ch.Title, &ch.About, &username, &ch.PhotoID,
		&broadcast, &megagroup, &forum, &signatures, &ch.Pts, &ch.TopMessage, &ch.ParticipantsCount, &ch.Date,
		&ch.MigratedFromChatID, &ch.MigratedFromMaxID, &ch.TtlPeriod, &ch.Address, &ch.DefaultBanned, &deleted, &ch.Version)
	if err != nil {
		return nil, err
	}
	if username.Valid {
		ch.Username = username.String
	}
	ch.Broadcast = broadcast != 0
	ch.Megagroup = megagroup != 0
	ch.Forum = forum != 0
	ch.Signatures = signatures != 0
	ch.Deleted = deleted != 0
	return &ch, nil
}

func scanChannels(rows *sql.Rows) ([]*Channel, error) {
	var out []*Channel
	for rows.Next() {
		ch, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

func scanParticipant(row scanner) (*Participant, error) {
	var p Participant
	var bot int
	err := row.Scan(&p.ChannelID, &p.UserID, &p.Role, &p.State, &p.AdminRights, &p.BannedRights, &p.Rank, &p.InviterID, &bot, &p.Date)
	if err != nil {
		return nil, err
	}
	p.IsBot = bot != 0
	return &p, nil
}

func scanMessage(row scanner) (*StoredMessage, error) {
	var m StoredMessage
	var deleted int
	err := row.Scan(&m.ChannelID, &m.ID, &m.SenderID, &m.Date, &m.EditDate, &m.RandomID, &m.Pts, &deleted, &m.Data)
	if err != nil {
		return nil, err
	}
	m.Deleted = deleted != 0
	return &m, nil
}

func insertParticipant(ctx context.Context, tx *sql.Tx, p *Participant) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO channel_participants (
  channel_id, user_id, role, state, admin_rights, banned_rights, rank_name, inviter_user_id, is_bot, date2)
VALUES (?,?,?,?,?,?,?,?,?,?)`,
		p.ChannelID, p.UserID, p.Role, p.State, p.AdminRights, p.BannedRights, p.Rank, p.InviterID, boolInt(p.IsBot), p.Date)
	return err
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func inClause(prefix string, ids []int64) (string, []any) {
	var b strings.Builder
	b.WriteString(prefix)
	args := make([]any, len(ids))
	for i, id := range ids {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('?')
		args[i] = id
	}
	b.WriteByte(')')
	return b.String(), args
}

func joinInt32(ids []int32) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(int64(id), 10)
	}
	return strings.Join(parts, ",")
}

func splitInt32(s string) []int32 {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]int32, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.ParseInt(p, 10, 32)
		if err == nil {
			out = append(out, int32(n))
		}
	}
	return out
}

func (s *mysqlStore) GetFeatures(ctx context.Context, channelID int64) (*Features, error) {
	var data []byte
	err := s.db.QueryRowContext(ctx, `SELECT data FROM channel_feature WHERE channel_id=?`, channelID).Scan(&data)
	if err == sql.ErrNoRows || len(data) == 0 {
		return &Features{}, nil
	}
	if err != nil {
		return nil, err
	}
	f := &Features{}
	if err := json.Unmarshal(data, f); err != nil {
		return &Features{}, nil
	}
	return f, nil
}

func (s *mysqlStore) SaveFeatures(ctx context.Context, channelID int64, f *Features) error {
	raw, err := json.Marshal(f)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO channel_feature (channel_id, data) VALUES (?,?)
ON DUPLICATE KEY UPDATE data=VALUES(data)`, channelID, raw)
	return err
}

func (s *mysqlStore) GetPersonalChannel(ctx context.Context, userID int64) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT channel_id FROM channel_personal WHERE user_id=?`, userID).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return id, err
}

func (s *mysqlStore) SetPersonalChannel(ctx context.Context, userID, channelID int64) error {
	if channelID == 0 {
		_, err := s.db.ExecContext(ctx, `DELETE FROM channel_personal WHERE user_id=?`, userID)
		return err
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO channel_personal (user_id, channel_id) VALUES (?,?)
ON DUPLICATE KEY UPDATE channel_id=VALUES(channel_id)`, userID, channelID)
	return err
}
