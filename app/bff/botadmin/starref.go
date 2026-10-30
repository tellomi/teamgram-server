package botadmin

import (
	"strconv"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"

	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Affiliate links are kept in this process and dropped on restart.
// A connection copies the bot's commission at connect time.
const (
	codeStarRefMissing = "STARREF_NOT_FOUND"
	codeStarRefLink    = "STARREF_LINK_INVALID"
	codeLimitInvalid   = "LIMIT_INVALID"
	codeOffsetInvalid  = "OFFSET_INVALID"
)

// starRefLink is one peer's connection to a bot affiliate program.
type starRefLink struct {
	botID     int64
	url       string
	date      int32
	revoked   bool
	permille  int32
	months    int32
	hasMonths bool
}

// ConnectStarRefBot joins the peer to a bot that has an open affiliate program.
func ConnectStarRefBot(self int64, peer *mtproto.InputPeer, bot *mtproto.InputUser) (*mtproto.Payments_ConnectedStarRefBots, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	kind, id, err := affiliatePeer(self, peer)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	rec, err := affiliateBotLocked(self, bot)
	if err != nil {
		return nil, err
	}
	if err := programOpen(rec); err != nil {
		return nil, err
	}
	key := kind + ":" + strconv.FormatInt(id, 10)
	link := starRefURL(rec, kind, id)
	row := findLink(key, rec.id)
	now := int32(time.Now().Unix())
	if row == nil {
		row = &starRefLink{botID: rec.id, url: link}
		w.refs[key] = append(w.refs[key], row)
	}
	row.url = link
	row.date = now
	row.revoked = false
	row.permille = rec.star.permille
	row.months = rec.star.months
	row.hasMonths = rec.star.hasMonths
	return connectedPage(key, nil, nil, 0), nil
}

// EditConnectedStarRefBot revokes or restores one link on the peer.
func EditConnectedStarRefBot(self int64, peer *mtproto.InputPeer, link string, revoked bool) (*mtproto.Payments_ConnectedStarRefBots, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	kind, id, err := affiliatePeer(self, peer)
	if err != nil {
		return nil, err
	}
	link = strings.TrimSpace(link)
	if link == "" {
		return nil, status.Error(mtproto.ErrBadRequest, codeStarRefLink)
	}
	mu.Lock()
	defer mu.Unlock()
	key := kind + ":" + strconv.FormatInt(id, 10)
	row := findURL(key, link)
	if row == nil {
		return nil, status.Error(mtproto.ErrBadRequest, codeStarRefLink)
	}
	if !revoked {
		rec := w.bots[row.botID]
		if err := programOpen(rec); err != nil {
			return nil, err
		}
		row.permille = rec.star.permille
		row.months = rec.star.months
		row.hasMonths = rec.star.hasMonths
	}
	row.revoked = revoked
	return connectedPage(key, nil, nil, 0), nil
}

// ConnectedStarRefBots lists the peer's affiliate connections.
func ConnectedStarRefBots(self int64, in *mtproto.TLPaymentsGetConnectedStarRefBots) (*mtproto.Payments_ConnectedStarRefBots, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	var peer *mtproto.InputPeer
	var date *wrapperspb.Int32Value
	var link *wrapperspb.StringValue
	var limit int32
	if in != nil {
		peer = in.GetPeer()
		date = in.GetOffsetDate()
		link = in.GetOffsetLink()
		limit = in.GetLimit()
	}
	kind, id, err := affiliatePeer(self, peer)
	if err != nil {
		return nil, err
	}
	page, err := affiliateLimit(limit)
	if err != nil {
		return nil, err
	}
	if (date == nil) != (link == nil) {
		return nil, status.Error(mtproto.ErrBadRequest, codeOffsetInvalid)
	}
	mu.Lock()
	defer mu.Unlock()
	key := kind + ":" + strconv.FormatInt(id, 10)
	out := connectedPage(key, date, link, page)
	if out == nil {
		return nil, status.Error(mtproto.ErrBadRequest, codeOffsetInvalid)
	}
	return out, nil
}

// ConnectedStarRefBot returns the one connection, or an empty list.
func ConnectedStarRefBot(self int64, peer *mtproto.InputPeer, bot *mtproto.InputUser) (*mtproto.Payments_ConnectedStarRefBots, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	kind, id, err := affiliatePeer(self, peer)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	rec, err := affiliateBotLocked(self, bot)
	if err != nil {
		return nil, err
	}
	key := kind + ":" + strconv.FormatInt(id, 10)
	row := findLink(key, rec.id)
	if row == nil {
		return emptyConnected(), nil
	}
	return presentConnected([]*starRefLink{row}, 1), nil
}

// SuggestedStarRefBots lists bots with an open affiliate program.
// order_by_revenue and order_by_date are accepted. No revenue or start
// date is stored, so the order is bot id. offset is a decimal index this
// method returns in next_offset. An empty offset starts at the beginning.
func SuggestedStarRefBots(self int64, in *mtproto.TLPaymentsGetSuggestedStarRefBots) (*mtproto.Payments_SuggestedStarRefBots, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	var peer *mtproto.InputPeer
	offset := ""
	var limit int32
	if in != nil {
		peer = in.GetPeer()
		offset = in.GetOffset()
		limit = in.GetLimit()
	}
	if _, _, err := affiliatePeer(self, peer); err != nil {
		return nil, err
	}
	page, err := affiliateLimit(limit)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	var rows []*botRec
	for _, rec := range w.bots {
		if programOpen(rec) == nil {
			rows = append(rows, rec)
		}
	}
	sortBots(rows)
	start := 0
	offset = strings.TrimSpace(offset)
	if offset != "" {
		n, err := strconv.Atoi(offset)
		if err != nil || n < 0 || n > len(rows) {
			return nil, status.Error(mtproto.ErrBadRequest, codeOffsetInvalid)
		}
		start = n
	}
	end := start + int(page)
	if end > len(rows) {
		end = len(rows)
	}
	out := &mtproto.Payments_SuggestedStarRefBots{Count: int32(len(rows))}
	for _, rec := range rows[start:end] {
		out.SuggestedBots = append(out.SuggestedBots, rec.star.tl(rec.id))
		out.Users = append(out.Users, rec.mtUser())
	}
	if out.SuggestedBots == nil {
		out.SuggestedBots = []*mtproto.StarRefProgram{}
		out.Users = []*mtproto.User{}
	}
	if end < len(rows) {
		out.NextOffset = wrapperspb.String(strconv.Itoa(end))
	}
	return mtproto.MakeTLPaymentsSuggestedStarRefBots(out).To_Payments_SuggestedStarRefBots(), nil
}

func affiliatePeer(self int64, in *mtproto.InputPeer) (string, int64, error) {
	if in == nil {
		return "", 0, mtproto.ErrPeerIdInvalid
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputPeerSelf:
		if self == 0 {
			return "", 0, mtproto.ErrPeerIdInvalid
		}
		return "user", self, nil
	case mtproto.Predicate_inputPeerUser:
		if in.GetUserId() == 0 {
			return "", 0, mtproto.ErrPeerIdInvalid
		}
		return "user", in.GetUserId(), nil
	case mtproto.Predicate_inputPeerChat:
		if in.GetChatId() == 0 {
			return "", 0, mtproto.ErrPeerIdInvalid
		}
		return "chat", in.GetChatId(), nil
	case mtproto.Predicate_inputPeerChannel:
		if in.GetChannelId() == 0 {
			return "", 0, mtproto.ErrPeerIdInvalid
		}
		return "channel", in.GetChannelId(), nil
	default:
		return "", 0, mtproto.ErrPeerIdInvalid
	}
}

func affiliateBotLocked(self int64, in *mtproto.InputUser) (*botRec, error) {
	id, err := inputID(self, in)
	if err != nil {
		return nil, err
	}
	rec := w.bots[id]
	if rec == nil || !hashOK(rec, in) {
		return nil, mtproto.ErrBotInvalid
	}
	return rec, nil
}

func programOpen(rec *botRec) error {
	if rec == nil || rec.star == nil || rec.star.permille <= 0 {
		return status.Error(mtproto.ErrBadRequest, codeStarRefMissing)
	}
	now := time.Now().Unix()
	if rec.star.closed && rec.star.end > now {
		return mtproto.ErrStarrefAwaitingEnd
	}
	if rec.star.closed {
		return status.Error(mtproto.ErrBadRequest, codeStarRefMissing)
	}
	return nil
}

func starRefURL(rec *botRec, kind string, id int64) string {
	name := ""
	for _, n := range rec.names {
		if n.active {
			name = n.name
			break
		}
	}
	if name == "" {
		name = "id" + strconv.FormatInt(rec.id, 10)
	}
	return "https://t.me/" + name + "?start=starref_" + kind + "_" + strconv.FormatInt(id, 10)
}

func findLink(key string, botID int64) *starRefLink {
	for _, row := range w.refs[key] {
		if row.botID == botID {
			return row
		}
	}
	return nil
}

func findURL(key, link string) *starRefLink {
	for _, row := range w.refs[key] {
		if row.url == link {
			return row
		}
	}
	return nil
}

func affiliateLimit(limit int32) (int32, error) {
	if limit == 0 {
		return 20, nil
	}
	if limit < 0 || limit > 100 {
		return 0, status.Error(mtproto.ErrBadRequest, codeLimitInvalid)
	}
	return limit, nil
}

func connectedPage(key string, date *wrapperspb.Int32Value, link *wrapperspb.StringValue, limit int32) *mtproto.Payments_ConnectedStarRefBots {
	rows := append([]*starRefLink(nil), w.refs[key]...)
	sortLinks(rows)
	start := 0
	if date != nil && link != nil {
		found := false
		for i, row := range rows {
			if row.date == date.GetValue() && row.url == link.GetValue() {
				start = i + 1
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	if limit <= 0 {
		limit = int32(len(rows))
	}
	end := start + int(limit)
	if end > len(rows) {
		end = len(rows)
	}
	return presentConnected(rows[start:end], len(rows))
}

func presentConnected(rows []*starRefLink, total int) *mtproto.Payments_ConnectedStarRefBots {
	out := &mtproto.Payments_ConnectedStarRefBots{Count: int32(total)}
	seen := map[int64]bool{}
	for _, row := range rows {
		item := &mtproto.ConnectedBotStarRef{
			Revoked:            row.revoked,
			Url:                row.url,
			Date:               row.date,
			BotId:              row.botID,
			CommissionPermille: row.permille,
		}
		if row.hasMonths {
			item.DurationMonths = wrapperspb.Int32(row.months)
		}
		out.ConnectedBots = append(out.ConnectedBots, mtproto.MakeTLConnectedBotStarRef(item).To_ConnectedBotStarRef())
		if !seen[row.botID] {
			seen[row.botID] = true
			if rec := w.bots[row.botID]; rec != nil {
				out.Users = append(out.Users, rec.mtUser())
			}
		}
	}
	if out.ConnectedBots == nil {
		out.ConnectedBots = []*mtproto.ConnectedBotStarRef{}
		out.Users = []*mtproto.User{}
	}
	return mtproto.MakeTLPaymentsConnectedStarRefBots(out).To_Payments_ConnectedStarRefBots()
}

func emptyConnected() *mtproto.Payments_ConnectedStarRefBots {
	return presentConnected(nil, 0)
}

func sortLinks(rows []*starRefLink) {
	for i := 1; i < len(rows); i++ {
		row := rows[i]
		j := i
		for j > 0 && linkLess(row, rows[j-1]) {
			rows[j] = rows[j-1]
			j--
		}
		rows[j] = row
	}
}

func linkLess(a, b *starRefLink) bool {
	if a.date != b.date {
		return a.date > b.date
	}
	return a.url < b.url
}

func sortBots(rows []*botRec) {
	for i := 1; i < len(rows); i++ {
		row := rows[i]
		j := i
		for j > 0 && row.id < rows[j-1].id {
			rows[j] = rows[j-1]
			j--
		}
		rows[j] = row
	}
}
