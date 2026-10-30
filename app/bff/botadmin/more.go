package botadmin

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"

	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

var langRe = regexp.MustCompile(`^[a-z]{2,3}([_-][A-Za-z0-9]{2,8})?$`)
var methodRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)
var commandRe = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)

func validName(name string) bool {
	if name == "" || utf8.RuneCountInString(name) > nameMaxRunes {
		return false
	}
	return !strings.ContainsAny(name, "\r\n\x00")
}

func validLang(lang string) error {
	if lang == "" || langRe.MatchString(lang) {
		return nil
	}
	return mtproto.ErrLangCodeInvalid
}

func validateUsername(name string, primary bool) error {
	if primary && !strings.HasSuffix(strings.ToLower(name), "bot") {
		return mtproto.ErrUsernameSuffixMissing
	}
	if primary {
		if len(name) < 5 || len(name) > userpb.MaxUsernameLen || !userpb.CheckUsernameInvalid(name) {
			return mtproto.ErrUsernameInvalid
		}
		return nil
	}
	if !userpb.CheckUsernameInvalid(name) {
		return mtproto.ErrUsernameInvalid
	}
	return nil
}

func takenLocked(name string) bool {
	if _, ok := w.names[strings.ToLower(name)]; ok {
		return true
	}
	return userpb.GetBotIdByName(strings.ToLower(name)) != 0
}

func randInt63() int64 {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return time.Now().UnixNano()
	}
	n := int64(binary.BigEndian.Uint64(buf[:]) & 0x7fffffffffffffff)
	if n == 0 {
		return 1
	}
	return n
}

func newToken(id int64) string {
	buf := make([]byte, 26)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(id, 10) + ":token-unavailable-retry-export"
	}
	return strconv.FormatInt(id, 10) + ":" + base64.RawURLEncoding.EncodeToString(buf)
}

func (b *botRec) mtUser() *mtproto.User {
	var list []*mtproto.Username
	primary := ""
	for _, n := range b.names {
		list = append(list, mtproto.MakeTLUsername(&mtproto.Username{
			Editable: n.editable, Active: n.active, Deletable: n.deletable, Username: n.name,
		}).To_Username())
		if n.active && primary == "" {
			primary = n.name
		}
	}
	u := &mtproto.User{
		Bot: true, BotCanEdit: true, BotHasMainApp: b.mainApp, Id: b.id,
		AccessHash: wrapperspb.Int64(b.hash), FirstName: wrapperspb.String(b.display),
		BotInfoVersion: wrapperspb.Int32(b.infoVer),
	}
	if primary != "" {
		u.Username = wrapperspb.String(primary)
	}
	if len(list) > 0 {
		u.Usernames = list
	}
	return mtproto.MakeTLUser(u).To_User()
}

func (b *botRec) texts(lang string) (name, about, desc string) {
	base := b.info[""]
	row := b.info[lang]
	name = row.name
	if name == "" {
		name = base.name
	}
	if name == "" {
		name = b.display
	}
	about = row.about
	if about == "" && lang != "" {
		about = base.about
	}
	desc = row.desc
	if desc == "" && lang != "" {
		desc = base.desc
	}
	return name, about, desc
}

func (p *starProg) tl(botID int64) *mtproto.StarRefProgram {
	s := &mtproto.StarRefProgram{BotId: botID, CommissionPermille: p.permille}
	if p.hasMonths {
		s.DurationMonths = wrapperspb.Int32(p.months)
	}
	if p.end != 0 {
		s.EndDate = wrapperspb.Int32(int32(p.end))
	}
	return mtproto.MakeTLStarRefProgram(s).To_StarRefProgram()
}

func inputID(self int64, in *mtproto.InputUser) (int64, error) {
	if in == nil {
		return 0, mtproto.ErrBotInvalid
	}
	p := mtproto.FromInputUser(self, in)
	if p == nil || p.PeerType == mtproto.PEER_EMPTY {
		return 0, mtproto.ErrBotInvalid
	}
	id := p.PeerId
	if p.PeerType == mtproto.PEER_SELF {
		id = self
	}
	if id <= 0 {
		return 0, mtproto.ErrBotInvalid
	}
	return id, nil
}

func hashOK(b *botRec, in *mtproto.InputUser) bool {
	if b == nil || in == nil || in.GetPredicateName() != mtproto.Predicate_inputUser {
		return true
	}
	h := in.GetAccessHash()
	return h == 0 || h == b.hash
}

func managerID(ctx context.Context, self int64, in *mtproto.InputUser) (int64, error) {
	id, err := inputID(self, in)
	if err != nil {
		return 0, mtproto.ErrManagerInvalid
	}
	if id == userpb.BotFatherId {
		return id, nil
	}
	mu.Lock()
	b := w.bots[id]
	can := b != nil && b.canManage
	local := b != nil
	mu.Unlock()
	if can {
		return id, nil
	}
	if local || userpb.GetBotNameById(id) != "" || remoteIsBot(ctx, id) {
		return 0, mtproto.ErrManagerPermissionMissing
	}
	return 0, mtproto.ErrManagerInvalid
}

// ownedBot resolves a bot the user created. The bot may also edit itself.
func ownedBot(ctx context.Context, self int64, in *mtproto.InputUser) (int64, error) {
	id, err := inputID(self, in)
	if err != nil {
		return 0, err
	}
	mu.Lock()
	b := w.bots[id]
	ok := b != nil && hashOK(b, in) && (b.owner == self || b.id == self)
	mu.Unlock()
	if ok {
		return id, nil
	}
	if b != nil && !hashOK(b, in) {
		return 0, mtproto.ErrBotInvalid
	}
	return 0, mtproto.ErrBotInvalid
}

// editable is ownedBot, but a missing bot argument means the caller is the bot.
func editable(ctx context.Context, self int64, in *mtproto.InputUser, allowOmitted bool) (int64, error) {
	if in == nil {
		if !allowOmitted {
			return 0, mtproto.ErrBotInvalid
		}
		mu.Lock()
		_, local := w.bots[self]
		mu.Unlock()
		if local || userpb.GetBotNameById(self) != "" || remoteIsBot(ctx, self) {
			if !local && userpb.GetBotNameById(self) == "" {
				return 0, mtproto.ErrBotInvalid
			}
			if local {
				return self, nil
			}
		}
		return 0, mtproto.ErrUserBotInvalid
	}
	return ownedBot(ctx, self, in)
}

func anyBot(ctx context.Context, self int64, in *mtproto.InputUser) (int64, error) {
	id, err := inputID(self, in)
	if err != nil {
		return 0, err
	}
	mu.Lock()
	b := w.bots[id]
	local := b != nil && hashOK(b, in)
	badHash := b != nil && !hashOK(b, in)
	mu.Unlock()
	if badHash {
		return 0, mtproto.ErrBotInvalid
	}
	if local || userpb.GetBotNameById(id) != "" || remoteIsBot(ctx, id) {
		return id, nil
	}
	return 0, mtproto.ErrBotInvalid
}

func selfIsBot(ctx context.Context, self int64) bool {
	mu.Lock()
	_, local := w.bots[self]
	mu.Unlock()
	return local || userpb.GetBotNameById(self) != "" || remoteIsBot(ctx, self)
}

func verifierBot(ctx context.Context, self int64, in *mtproto.InputUser) (int64, error) {
	if selfIsBot(ctx, self) {
		if in != nil {
			return 0, mtproto.ErrBotInvalid
		}
		mu.Lock()
		_, local := w.bots[self]
		mu.Unlock()
		if !local {
			return 0, mtproto.ErrBotVerifierForbidden
		}
		return self, nil
	}
	if in == nil {
		return 0, mtproto.ErrBotInvalid
	}
	return ownedBot(ctx, self, in)
}

func peerRef(self int64, in *mtproto.InputPeer) (string, error) {
	if in == nil {
		return "", mtproto.ErrPeerIdInvalid
	}
	p := mtproto.FromInputPeer2(self, in)
	if p == nil || p.PeerId == 0 && p.PeerType != mtproto.PEER_SELF {
		return "", mtproto.ErrPeerIdInvalid
	}
	if p.PeerType == mtproto.PEER_SELF {
		p.PeerType = mtproto.PEER_USER
		p.PeerId = self
	}
	if !p.IsUserOrChatOrChannel() || p.PeerId == 0 {
		return "", mtproto.ErrPeerIdInvalid
	}
	return strconv.Itoa(int(p.PeerType)) + ":" + strconv.FormatInt(p.PeerId, 10), nil
}

func grantVerifier(id int64) {
	mu.Lock()
	if b := w.bots[id]; b != nil {
		b.verifier = true
	}
	mu.Unlock()
}

func grantManager(id int64) {
	mu.Lock()
	if b := w.bots[id]; b != nil {
		b.canManage = true
	}
	mu.Unlock()
}

// AddUsername attaches another username to an owned bot.
// https://core.telegram.org/method/bots.addUsername
func AddUsername(ctx context.Context, self int64, bot *mtproto.InputUser, username string) (*mtproto.Bool, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	username = strings.TrimSpace(username)
	if err := validateUsername(username, false); err != nil {
		return nil, err
	}
	if err := remoteTaken(ctx, username); err != nil {
		return nil, err
	}
	id, err := ownedBot(ctx, self, bot)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	cur := w.bots[id]
	if cur == nil {
		return nil, mtproto.ErrBotInvalid
	}
	if takenLocked(username) {
		return nil, mtproto.ErrUsernameOccupied
	}
	if activeCount(cur.names) >= maxActiveUsernames {
		return nil, mtproto.ErrUsernamesActiveTooMuch
	}
	cur.names = append(cur.names, uname{name: username, active: true, deletable: true})
	w.names[strings.ToLower(username)] = id
	return mtproto.BoolTrue, nil
}

// RemoveUsername detaches an additional username.
// https://core.telegram.org/method/bots.removeUsername
func RemoveUsername(ctx context.Context, self int64, bot *mtproto.InputUser, username string) (*mtproto.Bool, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	username = strings.TrimSpace(username)
	id, err := ownedBot(ctx, self, bot)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	cur := w.bots[id]
	if cur == nil {
		return nil, mtproto.ErrBotInvalid
	}
	idx := -1
	for i, n := range cur.names {
		if strings.EqualFold(n.name, username) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, mtproto.ErrUsernameNotOccupied
	}
	if !cur.names[idx].deletable {
		return nil, mtproto.ErrUsernameInvalid
	}
	delete(w.names, strings.ToLower(cur.names[idx].name))
	cur.names = append(cur.names[:idx], cur.names[idx+1:]...)
	return mtproto.BoolTrue, nil
}

// ToggleUsername activates or deactivates one username of an owned bot.
// https://core.telegram.org/method/bots.toggleUsername
func ToggleUsername(ctx context.Context, self int64, bot *mtproto.InputUser, username string, active bool) (*mtproto.Bool, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	username = strings.TrimSpace(username)
	if err := validateUsername(username, false); err != nil && !strings.HasSuffix(strings.ToLower(username), "bot") {
		return nil, err
	}
	if username == "" || !userpb.CheckUsernameInvalid(username) {
		return nil, mtproto.ErrUsernameInvalid
	}
	id, err := ownedBot(ctx, self, bot)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	cur := w.bots[id]
	if cur == nil {
		return nil, mtproto.ErrBotInvalid
	}
	idx := indexName(cur.names, username)
	if idx < 0 {
		return nil, mtproto.ErrUsernameInvalid
	}
	if cur.names[idx].active == active {
		return nil, mtproto.ErrUsernameNotModified
	}
	if active && activeCount(cur.names) >= maxActiveUsernames {
		return nil, mtproto.ErrUsernamesActiveTooMuch
	}
	cur.names[idx].active = active
	return mtproto.BoolTrue, nil
}

// ReorderUsernames sets the order of every active username.
// https://core.telegram.org/method/bots.reorderUsernames
func ReorderUsernames(ctx context.Context, self int64, bot *mtproto.InputUser, order []string) (*mtproto.Bool, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	id, err := ownedBot(ctx, self, bot)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	cur := w.bots[id]
	if cur == nil {
		return nil, mtproto.ErrBotInvalid
	}
	var active, inactive []uname
	for _, n := range cur.names {
		if n.active {
			active = append(active, n)
		} else {
			inactive = append(inactive, n)
		}
	}
	if len(order) != len(active) {
		return nil, mtproto.ErrOrderInvalid
	}
	used := make([]bool, len(active))
	next := make([]uname, 0, len(active))
	for _, name := range order {
		found := -1
		for i, n := range active {
			if !used[i] && strings.EqualFold(n.name, strings.TrimSpace(name)) {
				found = i
				break
			}
		}
		if found < 0 {
			return nil, mtproto.ErrOrderInvalid
		}
		used[found] = true
		next = append(next, active[found])
	}
	same := true
	for i := range next {
		if !strings.EqualFold(next[i].name, active[i].name) {
			same = false
			break
		}
	}
	if same {
		return nil, mtproto.ErrUsernameNotModified
	}
	cur.names = append(next, inactive...)
	return mtproto.BoolTrue, nil
}

func activeCount(list []uname) int {
	n := 0
	for _, row := range list {
		if row.active {
			n++
		}
	}
	return n
}

func indexName(list []uname, username string) int {
	for i, n := range list {
		if strings.EqualFold(n.name, username) {
			return i
		}
	}
	return -1
}

// ExportToken returns the bot token. revoke rotates it.
// https://core.telegram.org/method/bots.exportBotToken
func ExportToken(ctx context.Context, self int64, bot *mtproto.InputUser, revoke bool) (*mtproto.Bots_ExportedBotToken, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	id, err := ownedBot(ctx, self, bot)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	cur := w.bots[id]
	if cur == nil {
		return nil, mtproto.ErrBotInvalid
	}
	if revoke || cur.token == "" {
		cur.token = newToken(id)
	}
	return mtproto.MakeTLBotsExportedBotToken(&mtproto.Bots_ExportedBotToken{Token: cur.token}).To_Bots_ExportedBotToken(), nil
}

// AccessSettings returns the bot's group-join restriction list.
// https://core.telegram.org/method/bots.getAccessSettings
func AccessSettings(ctx context.Context, self int64, bot *mtproto.InputUser) (*mtproto.Bots_AccessSettings, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	id, err := ownedBot(ctx, self, bot)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	cur := w.bots[id]
	if cur == nil {
		return nil, mtproto.ErrBotInvalid
	}
	var users []*mtproto.User
	for _, uid := range cur.access {
		users = append(users, mtproto.MakeTLUser(&mtproto.User{Id: uid}).To_User())
	}
	return mtproto.MakeTLBotsAccessSettings(&mtproto.Bots_AccessSettings{
		Restricted: cur.restricted, AddUsers: users,
	}).To_Bots_AccessSettings(), nil
}

// EditAccessSettings replaces the restriction list.
// https://core.telegram.org/method/bots.editAccessSettings
func EditAccessSettings(ctx context.Context, self int64, bot *mtproto.InputUser, restricted bool, users []*mtproto.InputUser) (*mtproto.Bool, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	var ids []int64
	for _, u := range users {
		id, err := inputID(self, u)
		if err != nil {
			return nil, mtproto.ErrUserIdInvalid
		}
		ids = append(ids, id)
	}
	id, err := ownedBot(ctx, self, bot)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	cur := w.bots[id]
	if cur == nil {
		return nil, mtproto.ErrBotInvalid
	}
	cur.restricted = restricted
	cur.access = ids
	return mtproto.BoolTrue, nil
}

// SetJoinResult stores the bot's decision for a chat-join web app query.
// https://core.telegram.org/method/bots.setJoinChatResults
func SetJoinResult(ctx context.Context, self int64, queryID int64, result *mtproto.JoinChatBotResult) (*mtproto.Bool, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	if !selfIsBot(ctx, self) {
		return nil, mtproto.ErrUserBotInvalid
	}
	if queryID == 0 || result == nil {
		return nil, mtproto.ErrQueryIdInvalid
	}
	mu.Lock()
	w.joins[queryID] = self
	mu.Unlock()
	return mtproto.BoolTrue, nil
}

func scopeKey(s *mtproto.BotCommandScope) string {
	if s == nil || s.GetPredicateName() == "" || s.GetPredicateName() == mtproto.Predicate_botCommandScopeDefault {
		return "default"
	}
	key := s.GetPredicateName()
	if s.GetPeer() != nil {
		key += ":" + s.GetPeer().GetPredicateName() + ":" + strconv.FormatInt(s.GetPeer().GetUserId()+s.GetPeer().GetChatId()+s.GetPeer().GetChannelId(), 10)
	}
	if s.GetUserId() != nil {
		key += ":u:" + strconv.FormatInt(s.GetUserId().GetUserId(), 10)
	}
	return key + ":"
}

// SetCommands replaces the command list for one scope and language.
// https://core.telegram.org/method/bots.setBotCommands
func SetCommands(ctx context.Context, self int64, scope *mtproto.BotCommandScope, lang string, commands []*mtproto.BotCommand) (*mtproto.Bool, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	if err := validLang(lang); err != nil {
		return nil, err
	}
	if !selfIsBot(ctx, self) {
		return nil, mtproto.ErrUserBotInvalid
	}
	if len(commands) > maxCommands {
		return nil, mtproto.ErrBotCommandInvalid
	}
	seen := map[string]bool{}
	out := make([]*mtproto.BotCommand, 0, len(commands))
	for _, c := range commands {
		if c == nil {
			return nil, mtproto.ErrBotCommandInvalid
		}
		cmd := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(c.GetCommand())), "/")
		if !commandRe.MatchString(cmd) || seen[cmd] {
			return nil, mtproto.ErrBotCommandInvalid
		}
		if c.GetDescription() == "" || utf8.RuneCountInString(c.GetDescription()) > 256 {
			return nil, mtproto.ErrBotCommandDescriptionInvalid
		}
		seen[cmd] = true
		out = append(out, mtproto.MakeTLBotCommand(&mtproto.BotCommand{
			Command: cmd, Description: c.GetDescription(),
		}).To_BotCommand())
	}
	mu.Lock()
	defer mu.Unlock()
	cur := w.bots[self]
	if cur == nil {
		return nil, mtproto.ErrUserBotInvalid
	}
	if cur.commands == nil {
		cur.commands = map[string][]*mtproto.BotCommand{}
	}
	cur.commands[scopeKey(scope)+"|"+lang] = out
	return mtproto.BoolTrue, nil
}

// ResetCommands clears one scope and language.
// https://core.telegram.org/method/bots.resetBotCommands
func ResetCommands(ctx context.Context, self int64, scope *mtproto.BotCommandScope, lang string) (*mtproto.Bool, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	if !selfIsBot(ctx, self) {
		return nil, mtproto.ErrUserBotInvalid
	}
	mu.Lock()
	defer mu.Unlock()
	cur := w.bots[self]
	if cur == nil {
		return nil, mtproto.ErrUserBotInvalid
	}
	delete(cur.commands, scopeKey(scope)+"|"+lang)
	return mtproto.BoolTrue, nil
}

// Commands returns the list stored for one scope and language.
// https://core.telegram.org/method/bots.getBotCommands
func Commands(ctx context.Context, self int64, scope *mtproto.BotCommandScope, lang string) (*mtproto.Vector_BotCommand, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	if !selfIsBot(ctx, self) {
		return nil, mtproto.ErrUserBotInvalid
	}
	mu.Lock()
	defer mu.Unlock()
	cur := w.bots[self]
	if cur == nil {
		return nil, mtproto.ErrUserBotInvalid
	}
	list := cur.commands[scopeKey(scope)+"|"+lang]
	if list == nil {
		list = []*mtproto.BotCommand{}
	}
	return &mtproto.Vector_BotCommand{Datas: list}, nil
}

func previewOf(in *mtproto.InputMedia) (*mtproto.MessageMedia, string, error) {
	if in == nil || in.GetPredicateName() == "" || in.GetPredicateName() == mtproto.Predicate_inputMediaEmpty {
		return nil, "", mtproto.ErrMediaEmpty
	}
	photo := false
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputMediaPhoto, mtproto.Predicate_inputMediaUploadedPhoto, mtproto.Predicate_inputMediaPhotoExternal:
		photo = true
	case mtproto.Predicate_inputMediaDocument, mtproto.Predicate_inputMediaUploadedDocument, mtproto.Predicate_inputMediaDocumentExternal:
		photo = false
	default:
		return nil, "", mtproto.ErrMediaInvalid
	}
	key := mediaKey(in)
	if key == "" {
		return nil, "", mtproto.ErrMediaInvalid
	}
	if photo {
		id := int64(1)
		if in.GetId_INPUTPHOTO() != nil && in.GetId_INPUTPHOTO().GetId() != 0 {
			id = in.GetId_INPUTPHOTO().GetId()
		}
		return mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{
			Spoiler: in.GetSpoiler(),
			Photo_FLAGPHOTO: mtproto.MakeTLPhoto(&mtproto.Photo{
				Id: id, AccessHash: 1, FileReference: []byte{},
			}).To_Photo(),
		}).To_MessageMedia(), key, nil
	}
	id := int64(1)
	if in.GetId_INPUTDOCUMENT() != nil && in.GetId_INPUTDOCUMENT().GetId() != 0 {
		id = in.GetId_INPUTDOCUMENT().GetId()
	}
	return mtproto.MakeTLMessageMediaDocument(&mtproto.MessageMedia{
		Spoiler: in.GetSpoiler(),
		Document: mtproto.MakeTLDocument(&mtproto.Document{
			Id: id, AccessHash: 1, FileReference: []byte{}, MimeType: in.GetMimeType(),
		}).To_Document(),
	}).To_MessageMedia(), key, nil
}

func mediaKey(in *mtproto.InputMedia) string {
	if in.GetUrl() != "" {
		return "url:" + in.GetUrl()
	}
	if p := in.GetId_INPUTPHOTO(); p != nil && p.GetId() != 0 {
		return "photo:" + strconv.FormatInt(p.GetId(), 10)
	}
	if d := in.GetId_INPUTDOCUMENT(); d != nil && d.GetId() != 0 {
		return "doc:" + strconv.FormatInt(d.GetId(), 10)
	}
	if f := in.GetFile(); f != nil && (f.GetId_INT64() != 0 || f.GetName() != "") {
		return "file:" + strconv.FormatInt(f.GetId_INT64(), 10) + ":" + f.GetName()
	}
	return ""
}

func asPreview(p preview) *mtproto.BotPreviewMedia {
	return mtproto.MakeTLBotPreviewMedia(&mtproto.BotPreviewMedia{
		Date: p.at, Media: p.media,
	}).To_BotPreviewMedia()
}

// AddPreviewMedia appends one main-mini-app preview for an owned bot.
// https://core.telegram.org/method/bots.addPreviewMedia
func AddPreviewMedia(ctx context.Context, self int64, bot *mtproto.InputUser, lang string, media *mtproto.InputMedia) (*mtproto.BotPreviewMedia, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	if err := validLang(lang); err != nil {
		return nil, err
	}
	msg, key, err := previewOf(media)
	if err != nil {
		return nil, err
	}
	id, err := ownedBot(ctx, self, bot)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	cur := w.bots[id]
	if cur == nil {
		return nil, mtproto.ErrBotInvalid
	}
	row := cur.previews[lang]
	if len(row) >= maxPreviewsPerLang {
		return nil, mtproto.ErrMediaInvalid
	}
	for _, p := range row {
		if p.key == key {
			return nil, mtproto.ErrMediaInvalid
		}
	}
	item := preview{key: key, at: int32(time.Now().Unix()), media: msg}
	if cur.previews == nil {
		cur.previews = map[string][]preview{}
	}
	cur.previews[lang] = append(row, item)
	cur.mainApp = true
	return asPreview(item), nil
}

// EditPreviewMedia replaces one preview in a language.
// https://core.telegram.org/method/bots.editPreviewMedia
func EditPreviewMedia(ctx context.Context, self int64, bot *mtproto.InputUser, lang string, old, next *mtproto.InputMedia) (*mtproto.BotPreviewMedia, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	if err := validLang(lang); err != nil {
		return nil, err
	}
	_, oldKey, err := previewOf(old)
	if err != nil {
		return nil, err
	}
	msg, newKey, err := previewOf(next)
	if err != nil {
		return nil, err
	}
	id, err := ownedBot(ctx, self, bot)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	cur := w.bots[id]
	if cur == nil {
		return nil, mtproto.ErrBotInvalid
	}
	row := cur.previews[lang]
	for i := range row {
		if row[i].key != oldKey {
			continue
		}
		row[i] = preview{key: newKey, at: int32(time.Now().Unix()), media: msg}
		cur.previews[lang] = row
		return asPreview(row[i]), nil
	}
	return nil, mtproto.ErrMediaInvalid
}

// DeletePreviewMedia removes the listed previews.
// https://core.telegram.org/method/bots.deletePreviewMedia
func DeletePreviewMedia(ctx context.Context, self int64, bot *mtproto.InputUser, lang string, media []*mtproto.InputMedia) (*mtproto.Bool, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	if err := validLang(lang); err != nil {
		return nil, err
	}
	if len(media) == 0 {
		return nil, mtproto.ErrMediaEmpty
	}
	keys := map[string]bool{}
	for _, m := range media {
		_, key, err := previewOf(m)
		if err != nil {
			return nil, err
		}
		keys[key] = true
	}
	id, err := ownedBot(ctx, self, bot)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	cur := w.bots[id]
	if cur == nil {
		return nil, mtproto.ErrBotInvalid
	}
	var kept []preview
	for _, p := range cur.previews[lang] {
		if !keys[p.key] {
			kept = append(kept, p)
		}
	}
	cur.previews[lang] = kept
	cur.mainApp = previewCount(cur) > 0
	return mtproto.BoolTrue, nil
}

// ReorderPreviewMedias sets the order of one language's previews.
// https://core.telegram.org/method/bots.reorderPreviewMedias
func ReorderPreviewMedias(ctx context.Context, self int64, bot *mtproto.InputUser, lang string, order []*mtproto.InputMedia) (*mtproto.Bool, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	if err := validLang(lang); err != nil {
		return nil, err
	}
	id, err := ownedBot(ctx, self, bot)
	if err != nil {
		return nil, err
	}
	var keys []string
	for _, m := range order {
		_, key, err := previewOf(m)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	mu.Lock()
	defer mu.Unlock()
	cur := w.bots[id]
	if cur == nil {
		return nil, mtproto.ErrBotInvalid
	}
	row := cur.previews[lang]
	if len(keys) != len(row) {
		return nil, mtproto.ErrOrderInvalid
	}
	used := make([]bool, len(row))
	next := make([]preview, 0, len(row))
	for _, key := range keys {
		found := -1
		for i, p := range row {
			if !used[i] && p.key == key {
				found = i
				break
			}
		}
		if found < 0 {
			return nil, mtproto.ErrOrderInvalid
		}
		used[found] = true
		next = append(next, row[found])
	}
	cur.previews[lang] = next
	return mtproto.BoolTrue, nil
}

// PreviewInfo returns one language's previews and every language that has any.
// https://core.telegram.org/method/bots.getPreviewInfo
func PreviewInfo(ctx context.Context, self int64, bot *mtproto.InputUser, lang string) (*mtproto.Bots_PreviewInfo, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	if err := validLang(lang); err != nil {
		return nil, err
	}
	id, err := ownedBot(ctx, self, bot)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	cur := w.bots[id]
	if cur == nil {
		return nil, mtproto.ErrBotInvalid
	}
	var media []*mtproto.BotPreviewMedia
	for _, p := range cur.previews[lang] {
		media = append(media, asPreview(p))
	}
	if media == nil {
		media = []*mtproto.BotPreviewMedia{}
	}
	var langs []string
	for code, row := range cur.previews {
		if len(row) > 0 {
			langs = append(langs, code)
		}
	}
	if langs == nil {
		langs = []string{}
	}
	return mtproto.MakeTLBotsPreviewInfo(&mtproto.Bots_PreviewInfo{
		Media: media, LangCodes: langs,
	}).To_Bots_PreviewInfo(), nil
}

// PreviewMedias returns the default-language previews of a bot.
// https://core.telegram.org/method/bots.getPreviewMedias
func PreviewMedias(ctx context.Context, self int64, bot *mtproto.InputUser) (*mtproto.Vector_BotPreviewMedia, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	id, err := anyBot(ctx, self, bot)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	cur := w.bots[id]
	var row []preview
	if cur != nil {
		row = cur.previews[""]
		if len(row) == 0 {
			for _, v := range cur.previews {
				if len(v) > 0 {
					row = v
					break
				}
			}
		}
	}
	var media []*mtproto.BotPreviewMedia
	for _, p := range row {
		media = append(media, asPreview(p))
	}
	if media == nil {
		media = []*mtproto.BotPreviewMedia{}
	}
	return &mtproto.Vector_BotPreviewMedia{Datas: media}, nil
}

func previewCount(b *botRec) int {
	n := 0
	for _, row := range b.previews {
		n += len(row)
	}
	return n
}

// PopularAppBots lists bots that have a main-app preview, newest id first.
// https://core.telegram.org/method/bots.getPopularAppBots
func PopularAppBots(self int64, offset string, limit int32) (*mtproto.Bots_PopularAppBots, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = popularPageDefault
	}
	if limit > popularPageMax {
		limit = popularPageMax
	}
	var after int64
	if offset != "" {
		n, err := strconv.ParseInt(offset, 10, 64)
		if err != nil {
			return mtproto.MakeTLBotsPopularAppBots(&mtproto.Bots_PopularAppBots{Users: []*mtproto.User{}}).To_Bots_PopularAppBots(), nil
		}
		after = n
	}
	mu.Lock()
	defer mu.Unlock()
	var ids []int64
	for _, b := range w.bots {
		if b.mainApp {
			ids = append(ids, b.id)
		}
	}
	// Descending id. The insertion order is not the public order.
	for i := 1; i < len(ids); i++ {
		j := i
		for j > 0 && ids[j] > ids[j-1] {
			ids[j], ids[j-1] = ids[j-1], ids[j]
			j--
		}
	}
	var users []*mtproto.User
	for _, id := range ids {
		if after != 0 && id >= after {
			continue
		}
		users = append(users, w.bots[id].mtUser())
		if int32(len(users)) == limit {
			break
		}
	}
	if users == nil {
		users = []*mtproto.User{}
	}
	out := &mtproto.Bots_PopularAppBots{Users: users}
	if int32(len(users)) == limit {
		var more bool
		last := users[len(users)-1].GetId()
		for _, id := range ids {
			if id < last {
				more = true
				break
			}
		}
		if more {
			out.NextOffset = wrapperspb.String(strconv.FormatInt(last, 10))
		}
	}
	return mtproto.MakeTLBotsPopularAppBots(out).To_Bots_PopularAppBots(), nil
}

// CanSendMessage reports whether the bot may message the current user.
// https://core.telegram.org/method/bots.canSendMessage
func CanSendMessage(ctx context.Context, self int64, bot *mtproto.InputUser) (*mtproto.Bool, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	id, err := anyBot(ctx, self, bot)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	ok := w.allow[self][id]
	mu.Unlock()
	return mtproto.ToBool(ok), nil
}

// AllowSendMessage lets the bot message the current user and returns the update.
// https://core.telegram.org/method/bots.allowSendMessage
func AllowSendMessage(ctx context.Context, self int64, bot *mtproto.InputUser) (*mtproto.Updates, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	id, err := anyBot(ctx, self, bot)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	if w.allow[self] == nil {
		w.allow[self] = map[int64]bool{}
	}
	w.allow[self][id] = true
	var user *mtproto.User
	if b := w.bots[id]; b != nil {
		user = b.mtUser()
	}
	mu.Unlock()
	if user == nil {
		name := userpb.GetBotNameById(id)
		if name == "" {
			name = "bot"
		}
		user = mtproto.MakeTLUser(&mtproto.User{
			Bot: true, Id: id, FirstName: wrapperspb.String(name),
		}).To_User()
	}
	now := int32(time.Now().Unix())
	return mtproto.MakeTLUpdates(&mtproto.Updates{
		Date:  now,
		Users: []*mtproto.User{user},
		Chats: []*mtproto.Chat{},
		Updates: []*mtproto.Update{
			mtproto.MakeTLUpdateBotStopped(&mtproto.Update{
				UserId: id, Date_INT32: now, Stopped: mtproto.BoolFalse, Qts: 0,
			}).To_Update(),
		},
	}).To_Updates(), nil
}

// InvokeWebViewCustomMethod answers a mini-app custom method immediately.
// The bot is not round-tripped; waiting on it would stall the client.
// https://core.telegram.org/method/bots.invokeWebViewCustomMethod
func InvokeWebViewCustomMethod(ctx context.Context, self int64, bot *mtproto.InputUser, method string, params *mtproto.DataJSON) (*mtproto.DataJSON, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	if _, err := anyBot(ctx, self, bot); err != nil {
		return nil, err
	}
	method = strings.TrimSpace(method)
	if !methodRe.MatchString(method) {
		return nil, mtproto.ErrMethodInvalid
	}
	raw := ""
	if params != nil {
		raw = strings.TrimSpace(params.GetData())
	}
	if raw == "" || !json.Valid([]byte(raw)) || (raw[0] != '{' && raw[0] != '[') {
		return nil, mtproto.ErrDataJsonInvalid
	}
	return mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: `{"ok":true}`}).To_DataJSON(), nil
}

// CheckDownloadFileParams reports whether a mini app may download url as fileName.
// A bad name or a non-http URL is boolFalse. Only an unknown bot is an error.
// https://core.telegram.org/method/bots.checkDownloadFileParams
func CheckDownloadFileParams(ctx context.Context, self int64, bot *mtproto.InputUser, fileName, rawURL string) (*mtproto.Bool, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	if _, err := anyBot(ctx, self, bot); err != nil {
		return nil, err
	}
	return mtproto.ToBool(downloadAllowed(fileName, rawURL)), nil
}

func downloadAllowed(name, raw string) bool {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 255 || strings.ContainsAny(name, "/\\\x00") {
		return false
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// RequestWebViewButton stores a button a bot wants the user to open.
// https://core.telegram.org/method/bots.requestWebViewButton
func RequestWebViewButton(ctx context.Context, self int64, user *mtproto.InputUser, button *mtproto.KeyboardButton) (*mtproto.Bots_RequestedButton, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	if !selfIsBot(ctx, self) {
		return nil, mtproto.ErrUserBotInvalid
	}
	if _, err := inputID(self, user); err != nil {
		return nil, mtproto.ErrUserIdInvalid
	}
	if button == nil || strings.TrimSpace(button.GetText()) == "" {
		return nil, mtproto.ErrButtonTypeInvalid
	}
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	req := hex.EncodeToString(buf)
	stored := mtproto.MakeTLKeyboardButton(&mtproto.KeyboardButton{
		Text: button.GetText(), Type: button.GetType(), Url: button.GetUrl(),
	}).To_KeyboardButton()
	mu.Lock()
	w.buttons[req] = buttonHold{botID: self, btn: stored}
	mu.Unlock()
	return mtproto.MakeTLBotsRequestedButton(&mtproto.Bots_RequestedButton{WebappReqId: req}).To_Bots_RequestedButton(), nil
}

// RequestedWebViewButton returns the button stored for webappReqID.
// https://core.telegram.org/method/bots.getRequestedWebViewButton
func RequestedWebViewButton(ctx context.Context, self int64, bot *mtproto.InputUser, webappReqID string) (*mtproto.KeyboardButton, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	id, err := anyBot(ctx, self, bot)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	hold, ok := w.buttons[webappReqID]
	mu.Unlock()
	if !ok || hold.botID != id || hold.btn == nil {
		return nil, mtproto.ErrQueryIdInvalid
	}
	return hold.btn, nil
}

// ToggleEmojiPermission lets or blocks a bot from changing the user's emoji status.
// https://core.telegram.org/method/bots.toggleUserEmojiStatusPermission
func ToggleEmojiPermission(ctx context.Context, self int64, bot *mtproto.InputUser, enabled bool) (*mtproto.Bool, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	id, err := anyBot(ctx, self, bot)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	if w.emoji[self] == nil {
		w.emoji[self] = map[int64]bool{}
	}
	w.emoji[self][id] = enabled
	mu.Unlock()
	return mtproto.BoolTrue, nil
}

// UpdateUserEmojiStatus applies a status when the user has allowed this bot.
// https://core.telegram.org/method/bots.updateUserEmojiStatus
func UpdateUserEmojiStatus(ctx context.Context, self int64, user *mtproto.InputUser, st *mtproto.EmojiStatus) (*mtproto.Bool, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	if !selfIsBot(ctx, self) {
		return nil, mtproto.ErrUserBotInvalid
	}
	uid, err := inputID(self, user)
	if err != nil {
		return nil, mtproto.ErrUserIdInvalid
	}
	mu.Lock()
	allowed := w.emoji[uid][self]
	mu.Unlock()
	if !allowed {
		return nil, mtproto.ErrUserPrivacyRestricted
	}
	doc, until := emojiParts(st)
	if d := directory(); d != nil {
		if wr, ok := d.(emojiWriter); ok {
			_, err := wr.UserUpdateEmojiStatus(ctx, &userpb.TLUserUpdateEmojiStatus{
				UserId: uid, EmojiStatusDocumentId: doc, EmojiStatusUntil: until,
			})
			if err != nil {
				if stErr, ok := status.FromError(err); ok {
					switch stErr.Code() {
					case mtproto.ErrBadRequest, mtproto.ErrForbidden, mtproto.ErrUnauthorized:
						return nil, err
					}
				}
			}
		}
	}
	return mtproto.BoolTrue, nil
}

func emojiParts(st *mtproto.EmojiStatus) (int64, int32) {
	if st == nil || st.GetPredicateName() == mtproto.Predicate_emojiStatusEmpty {
		return 0, 0
	}
	until := st.GetUntil_INT32()
	if st.GetUntil_FLAGINT32() != nil {
		until = st.GetUntil_FLAGINT32().GetValue()
	}
	return st.GetDocumentId(), until
}

// DescriptionText reads either layer shape of custom_description.
func DescriptionText(in *mtproto.TLBotsSetCustomVerification) string {
	if in == nil {
		return ""
	}
	if t := in.GetCustomDescription_FLAGTEXTWITHENTITIES(); t != nil {
		return t.GetText()
	}
	if t := in.GetCustomDescription_FLAGSTRING(); t != nil {
		return t.GetValue()
	}
	return ""
}
