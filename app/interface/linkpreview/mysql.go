package linkpreview

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"google.golang.org/protobuf/proto"

	"github.com/teamgram/proto/mtproto"
)

// Queries below intentionally omit phone, bot token, secret_key_id, and access_hash.
const (
	qUsername    = `SELECT username, peer_type, peer_id, active, deleted FROM username WHERE username=? ORDER BY deleted ASC, active DESC, id ASC LIMIT 1`
	qUser        = `SELECT id, first_name, last_name, username, verified, scam, fake, premium, about, is_bot, photo_id, restricted, deleted FROM users WHERE id=? LIMIT 1`
	qBot         = `SELECT description, verified FROM bots WHERE bot_id=? LIMIT 1`
	qPrivacy     = `SELECT key_type, rules FROM user_privacies WHERE user_id=? AND key_type IN (?, ?)`
	qChannel     = `SELECT id, title, about, username, photo_id, broadcast, megagroup, participants_count, deleted FROM channels WHERE id=? LIMIT 1`
	qChannelUser = `SELECT id, title, about, username, photo_id, broadcast, megagroup, participants_count, deleted FROM channels WHERE username=? AND deleted=0 LIMIT 1`
	qChat        = `SELECT title, about, photo_id, participant_count, deactivated FROM chats WHERE id=? LIMIT 1`
	qInvite      = `SELECT chat_id, revoked, request_needed, start_date, expire_date, usage_limit, usage2, title FROM chat_invites WHERE link=? LIMIT 1`
	qMessage     = `SELECT message_id, date2, deleted, message_data FROM channel_messages WHERE channel_id=? AND message_id=?`
	qLatest      = `SELECT message_id, date2, deleted, message_data FROM channel_messages WHERE channel_id=? AND deleted=0 AND message_id>? ORDER BY message_id DESC LIMIT ?`
	qFeature     = `SELECT data FROM channel_feature WHERE channel_id=?`
	qStory       = `SELECT story_id, date2, expire_date, pinned, deleted, body FROM stories WHERE owner_kind=? AND owner_id=? AND story_id=?`
	qPack        = `SELECT short_name, title, count, thumb_path FROM sticker_sets WHERE short_name=? AND emoji=? AND public=1 AND deleted=0 LIMIT 1`
	qPhotoSizeID = `SELECT size_id FROM photos WHERE photo_id=? LIMIT 1`
	qPhotoSizes  = `SELECT size_type, width, file_path FROM photo_sizes WHERE photo_size_id=? AND file_path<>''`
	qDocument    = `SELECT file_path, mime_type, thumb_id FROM documents WHERE document_id=? LIMIT 1`
)

const stickerSchema = `
CREATE TABLE IF NOT EXISTS sticker_sets (
  id BIGINT NOT NULL AUTO_INCREMENT,
  short_name VARCHAR(64) NOT NULL,
  title VARCHAR(128) NOT NULL DEFAULT '',
  count INT NOT NULL DEFAULT 0,
  emoji TINYINT NOT NULL DEFAULT 0,
  thumb_path VARCHAR(255) NOT NULL DEFAULT '',
  public TINYINT NOT NULL DEFAULT 1,
  deleted TINYINT NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY sticker_sets_short_name (short_name, emoji)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

// MySQL reads public rows from the teamgram schema.
type MySQL struct {
	db   *sql.DB
	once sync.Map
}

func OpenMySQL(dsn string) (*MySQL, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err = db.ExecContext(ctx, stickerSchema); err != nil {
		log.Printf("linkpreview: sticker_sets: %v", err)
	}
	return &MySQL{db: db}, nil
}

func (m *MySQL) Close() error {
	if m == nil || m.db == nil {
		return nil
	}
	return m.db.Close()
}

func (m *MySQL) PeerByUsername(ctx context.Context, username string) (*storedPeer, error) {
	var (
		name            string
		peerType        int32
		peerID          int64
		active, deleted int
	)
	err := m.db.QueryRowContext(ctx, qUsername, username).Scan(&name, &peerType, &peerID, &active, &deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return m.channelByUsername(ctx, username)
	}
	if err != nil {
		return nil, m.fail("username", err)
	}
	if deleted != 0 || active == 0 || name == "" {
		return nil, errNotFound
	}
	switch peerType {
	case mtproto.PEER_USER:
		return m.user(ctx, peerID, name)
	case mtproto.PEER_CHANNEL:
		return m.channel(ctx, peerID, false, name)
	case mtproto.PEER_CHAT:
		return m.basicGroup(ctx, peerID, name)
	default:
		return nil, errNotFound
	}
}

func (m *MySQL) PeerByChannelID(ctx context.Context, id int64) (*storedPeer, error) {
	return m.channel(ctx, id, true, "")
}

func (m *MySQL) channelByUsername(ctx context.Context, username string) (*storedPeer, error) {
	p, err := scanChannel(m.db.QueryRowContext(ctx, qChannelUser, username))
	if err != nil {
		return nil, m.fail("channel username", err)
	}
	if p.Deleted || p.Username == "" {
		return nil, errNotFound
	}
	return p.toPeer(), nil
}

func (m *MySQL) channel(ctx context.Context, id int64, requireUsername bool, username string) (*storedPeer, error) {
	p, err := scanChannel(m.db.QueryRowContext(ctx, qChannel, id))
	if err != nil {
		return nil, m.fail("channel", err)
	}
	if p.Deleted {
		return nil, errNotFound
	}
	if username != "" && p.Username == "" {
		p.Username = username
	}
	if requireUsername && p.Username == "" {
		return nil, errNotFound
	}
	if p.Username == "" {
		return nil, errNotFound
	}
	return p.toPeer(), nil
}

type channelRow struct {
	ID        int64
	Title     string
	About     string
	Username  string
	PhotoID   int64
	Broadcast bool
	Megagroup bool
	Count     int32
	Deleted   bool
}

func scanChannel(row *sql.Row) (*channelRow, error) {
	var (
		r                        channelRow
		username                 sql.NullString
		broadcast, mega, deleted int
	)
	err := row.Scan(&r.ID, &r.Title, &r.About, &username, &r.PhotoID, &broadcast, &mega, &r.Count, &deleted)
	if err != nil {
		return nil, err
	}
	if username.Valid {
		r.Username = username.String
	}
	r.Broadcast = broadcast != 0
	r.Megagroup = mega != 0
	r.Deleted = deleted != 0
	return &r, nil
}

func (r *channelRow) toPeer() *storedPeer {
	kind := KindChannel
	if r.Megagroup {
		kind = KindGroup
	}
	count := r.Count
	return &storedPeer{
		ID:          r.ID,
		Kind:        kind,
		Title:       r.Title,
		Username:    r.Username,
		About:       r.About,
		PhotoID:     r.PhotoID,
		MemberCount: &count,
		Broadcast:   r.Broadcast,
		Megagroup:   r.Megagroup,
	}
}

func (m *MySQL) user(ctx context.Context, id int64, username string) (*storedPeer, error) {
	var (
		gotID                                                   int64
		first, last, name, about                                string
		verified, scam, fake, premium, bot, restricted, deleted int
		photoID                                                 int64
	)
	err := m.db.QueryRowContext(ctx, qUser, id).Scan(
		&gotID, &first, &last, &name, &verified, &scam, &fake, &premium, &about, &bot, &photoID, &restricted, &deleted,
	)
	if err != nil {
		return nil, m.fail("user", err)
	}
	if deleted != 0 || restricted != 0 {
		return nil, errNotFound
	}
	if name == "" {
		name = username
	}
	if bot != 0 && strings.TrimSpace(about) == "" {
		var desc string
		var botVerified int
		berr := m.db.QueryRowContext(ctx, qBot, id).Scan(&desc, &botVerified)
		if berr == nil {
			about = desc
			if botVerified != 0 {
				verified = 1
			}
		} else if !errors.Is(berr, sql.ErrNoRows) && !isNoTable(berr) {
			return nil, m.fail("bot", berr)
		}
	}
	if bot == 0 {
		showAbout, showPhoto := m.publicProfile(ctx, id)
		if !showAbout {
			about = ""
		}
		if !showPhoto {
			photoID = 0
		}
	}
	kind := KindUser
	if bot != 0 {
		kind = KindBot
	}
	title := strings.TrimSpace(strings.TrimSpace(first) + " " + strings.TrimSpace(last))
	if title == "" {
		title = name
	}
	return &storedPeer{
		ID:       gotID,
		Kind:     kind,
		Title:    title,
		Username: name,
		About:    about,
		PhotoID:  photoID,
		Verified: verified != 0,
		Scam:     scam != 0,
		Fake:     fake != 0,
		Premium:  premium != 0,
	}, nil
}

func (m *MySQL) publicProfile(ctx context.Context, id int64) (about, photo bool) {
	about, photo = true, true
	rows, err := m.db.QueryContext(ctx, qPrivacy, id, mtproto.PROFILE_PHOTO, mtproto.ABOUT)
	if err != nil {
		if !isNoTable(err) {
			_ = m.fail("privacy", err)
		}
		return false, false
	}
	defer rows.Close()
	for rows.Next() {
		var key int32
		var rules string
		if err = rows.Scan(&key, &rules); err != nil {
			return false, false
		}
		if !rulesArePublic(rules) {
			if key == mtproto.ABOUT {
				about = false
			}
			if key == mtproto.PROFILE_PHOTO {
				photo = false
			}
		}
	}
	if err = rows.Err(); err != nil {
		return false, false
	}
	return about, photo
}

func (m *MySQL) basicGroup(ctx context.Context, id int64, username string) (*storedPeer, error) {
	var (
		title, about string
		photoID      int64
		count        int32
		deactivated  int
	)
	err := m.db.QueryRowContext(ctx, qChat, id).Scan(&title, &about, &photoID, &count, &deactivated)
	if err != nil {
		return nil, m.fail("chat", err)
	}
	if deactivated != 0 || username == "" {
		return nil, errNotFound
	}
	n := count
	return &storedPeer{
		ID:          id,
		Kind:        KindGroup,
		Title:       title,
		Username:    username,
		About:       about,
		PhotoID:     photoID,
		MemberCount: &n,
	}, nil
}

func (m *MySQL) Invite(ctx context.Context, hash string) (*storedInvite, error) {
	var (
		chatID            int64
		revoked, request  int
		start, expire     int64
		usageLimit, usage int32
		title             string
	)
	err := m.db.QueryRowContext(ctx, qInvite, hash).Scan(&chatID, &revoked, &request, &start, &expire, &usageLimit, &usage, &title)
	if err != nil {
		return nil, m.fail("invite", err)
	}
	now := time.Now().Unix()
	if revoked != 0 || (expire != 0 && expire < now) || (start != 0 && start > now) || (usageLimit > 0 && usage >= usageLimit) {
		return nil, errNotFound
	}
	inv := &storedInvite{Title: title, RequestNeeded: request != 0}
	if chatID >= mtproto.MinNebulaChatChannelID {
		row, rerr := scanChannel(m.db.QueryRowContext(ctx, qChannel, chatID))
		if rerr != nil || row.Deleted {
			return nil, errNotFound
		}
		if inv.Title == "" {
			inv.Title = row.Title
		}
		inv.About = row.About
		inv.PhotoID = row.PhotoID
		inv.MemberCount = row.Count
		inv.Broadcast = row.Broadcast
		inv.Megagroup = row.Megagroup
		return inv, nil
	}
	var (
		chatTitle, about string
		photoID          int64
		count            int32
		deactivated      int
	)
	err = m.db.QueryRowContext(ctx, qChat, chatID).Scan(&chatTitle, &about, &photoID, &count, &deactivated)
	if err != nil || deactivated != 0 {
		return nil, errNotFound
	}
	if inv.Title == "" {
		inv.Title = chatTitle
	}
	inv.About = about
	inv.PhotoID = photoID
	inv.MemberCount = count
	return inv, nil
}

func (m *MySQL) Message(ctx context.Context, channelID int64, id int32) (*storedMsg, error) {
	if id <= m.availableMin(ctx, channelID) {
		return nil, errNotFound
	}
	msg, deleted, err := m.scanMessage(m.db.QueryRowContext(ctx, qMessage, channelID, id))
	if err != nil {
		return nil, m.fail("message", err)
	}
	if deleted || msg == nil {
		return nil, errNotFound
	}
	return &storedMsg{Msg: msg}, nil
}

func (m *MySQL) Latest(ctx context.Context, channelID int64, limit int) ([]storedMsg, error) {
	if limit <= 0 || limit > 50 {
		limit = feedScan
	}
	rows, err := m.db.QueryContext(ctx, qLatest, channelID, m.availableMin(ctx, channelID), limit)
	if err != nil {
		return nil, m.fail("latest", err)
	}
	defer rows.Close()
	var out []storedMsg
	for rows.Next() {
		msg, deleted, err := scanMessage(rows)
		if err != nil {
			return nil, m.fail("latest row", err)
		}
		if deleted || msg == nil {
			continue
		}
		out = append(out, storedMsg{Msg: msg})
	}
	if err = rows.Err(); err != nil {
		return nil, m.fail("latest", err)
	}
	return out, nil
}

func (m *MySQL) availableMin(ctx context.Context, channelID int64) int32 {
	var data []byte
	err := m.db.QueryRowContext(ctx, qFeature, channelID).Scan(&data)
	if err != nil || len(data) == 0 {
		return 0
	}
	var f struct {
		AvailableMinID int32
	}
	if json.Unmarshal(data, &f) != nil {
		return 0
	}
	return f.AvailableMinID
}

type messageScanner interface {
	Scan(dest ...any) error
}

func (m *MySQL) scanMessage(row *sql.Row) (*mtproto.Message, bool, error) {
	return scanMessage(row)
}

func scanMessage(row messageScanner) (*mtproto.Message, bool, error) {
	var (
		id, date int32
		deleted  int
		data     []byte
	)
	if err := row.Scan(&id, &date, &deleted, &data); err != nil {
		return nil, false, err
	}
	if deleted != 0 {
		return nil, true, nil
	}
	msg := &mtproto.Message{}
	if err := proto.Unmarshal(data, msg); err != nil {
		return nil, false, err
	}
	if msg.Id == 0 {
		msg.Id = id
	}
	if msg.Date == 0 {
		msg.Date = date
	}
	return msg, false, nil
}

func (m *MySQL) Story(ctx context.Context, ownerKind int8, ownerID int64, id int32) (*storedStory, error) {
	var (
		sid, date, expire int32
		pinned, deleted   int
		body              []byte
	)
	err := m.db.QueryRowContext(ctx, qStory, ownerKind, ownerID, id).Scan(&sid, &date, &expire, &pinned, &deleted, &body)
	if err != nil {
		return nil, m.fail("story", err)
	}
	item := &mtproto.StoryItem{}
	if err = proto.Unmarshal(body, item); err != nil {
		return nil, errNotFound
	}
	item.Id = sid
	item.Date = date
	item.ExpireDate = expire
	item.Pinned = pinned != 0
	return &storedStory{Item: item, Deleted: deleted != 0}, nil
}

func (m *MySQL) Pack(ctx context.Context, short string, emoji bool) (*storedPack, error) {
	var (
		name, title, thumb string
		count              int32
	)
	flag := 0
	if emoji {
		flag = 1
	}
	err := m.db.QueryRowContext(ctx, qPack, short, flag).Scan(&name, &title, &count, &thumb)
	if err != nil {
		return nil, m.fail("sticker set", err)
	}
	return &storedPack{ShortName: name, Title: title, AvatarPath: thumb, Count: count, Emoji: emoji}, nil
}

func (m *MySQL) FilePath(ctx context.Context, photoID, documentID int64) (string, error) {
	if photoID != 0 {
		if p := m.photoPath(ctx, photoID); p != "" {
			return p, nil
		}
	}
	if documentID != 0 {
		var filePath, mime string
		var thumbID int64
		err := m.db.QueryRowContext(ctx, qDocument, documentID).Scan(&filePath, &mime, &thumbID)
		if err == nil {
			if thumbID != 0 {
				if p := m.sizesPath(ctx, thumbID); p != "" {
					return p, nil
				}
			}
			if strings.HasPrefix(strings.ToLower(mime), "image/") && filePath != "" {
				return filePath, nil
			}
		}
	}
	return "", errNotFound
}

func (m *MySQL) photoPath(ctx context.Context, photoID int64) string {
	var sizeID int64
	err := m.db.QueryRowContext(ctx, qPhotoSizeID, photoID).Scan(&sizeID)
	if err != nil || sizeID == 0 {
		sizeID = photoID
	}
	return m.sizesPath(ctx, sizeID)
}

func (m *MySQL) sizesPath(ctx context.Context, sizeID int64) string {
	rows, err := m.db.QueryContext(ctx, qPhotoSizes, sizeID)
	if err != nil {
		return ""
	}
	defer rows.Close()
	var sizes []sizeRow
	for rows.Next() {
		var s sizeRow
		if err = rows.Scan(&s.typ, &s.width, &s.path); err != nil {
			return ""
		}
		sizes = append(sizes, s)
	}
	return pickPhotoFile(sizes)
}

type sizeRow struct {
	typ   string
	width int32
	path  string
}

func pickPhotoFile(rows []sizeRow) string {
	order := []string{"m", "x", "y", "b", "a", "c", "s", "w"}
	byType := map[string]string{}
	var best string
	var bestW int32 = -1
	for _, r := range rows {
		if r.path == "" {
			continue
		}
		if _, ok := byType[r.typ]; !ok {
			byType[r.typ] = r.path
		}
		if r.width > bestW {
			bestW = r.width
			best = r.path
		}
	}
	for _, t := range order {
		if p := byType[t]; p != "" {
			return p
		}
	}
	return best
}

func (m *MySQL) fail(what string, err error) error {
	if err == nil || errors.Is(err, sql.ErrNoRows) {
		return errNotFound
	}
	if isNoTable(err) {
		m.logOnce(what, err)
		return errNotFound
	}
	return withCause(ErrInternal, err)
}

func (m *MySQL) logOnce(what string, err error) {
	if _, loaded := m.once.LoadOrStore(what, true); !loaded {
		log.Printf("linkpreview: %s: %v", what, err)
	}
}

func isNoTable(err error) bool {
	var me *mysqldriver.MySQLError
	return errors.As(err, &me) && me.Number == 1146
}
