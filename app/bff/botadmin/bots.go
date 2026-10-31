// Package botadmin serves the layer-232 bots.* methods official clients call
// while managing a bot: creation, profile text, usernames, previews, mini-app
// helpers, emoji-status permission, and affiliate programs.
//
// Owned bots live in this process. Usernames are also checked against the user
// service when one is configured, so a name already taken by an account is
// rejected. The handlers return the TL result immediately. They do not wait
// on a bot webhook or on the updates fanout.
package botadmin

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"

	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Limits from the public client configuration.
// https://core.telegram.org/api/config#bots-create-limit-default
// https://core.telegram.org/api/config#starref-min-commission-permille
// https://core.telegram.org/api/config#bot-verification-description-length-limit
const (
	botsCreateLimit            = 20
	maxActiveUsernames         = 10
	maxPreviewsPerLang         = 12
	maxCommands                = 100
	aboutMaxRunes              = 120
	descriptionMaxRunes        = 512
	nameMaxRunes               = 64
	verifyDescriptionMax       = 70
	starRefMinPermille   int32 = 1
	starRefMaxPermille   int32 = 1000
	starRefCoolDown            = 24 * time.Hour
	popularPageDefault   int32 = 50
	popularPageMax       int32 = 100
)

// Directory is the user service surface these methods consult.
// A nil directory means only bots created in this process are known.
type Directory interface {
	UserCheckUsername(ctx context.Context, in *userpb.TLUserCheckUsername) (*userpb.UsernameExist, error)
	UserIsBot(ctx context.Context, in *userpb.TLUserIsBot) (*mtproto.Bool, error)
}

type emojiWriter interface {
	UserUpdateEmojiStatus(ctx context.Context, in *userpb.TLUserUpdateEmojiStatus) (*mtproto.Bool, error)
}

type uname struct {
	name      string
	active    bool
	editable  bool
	deletable bool
}

type texts struct {
	name, about, desc string
}

type preview struct {
	key   string
	at    int32
	media *mtproto.MessageMedia
}

type starProg struct {
	permille  int32
	months    int32
	hasMonths bool
	end       int64
	closed    bool
}

type botRec struct {
	id             int64
	hash           int64
	owner          int64
	manager        int64
	display        string
	token          string
	viaLink        bool
	canManage      bool
	verifier       bool
	mainApp        bool
	infoVer        int32
	names          []uname
	info           map[string]texts
	previews       map[string][]preview
	star           *starProg
	restricted     bool
	access         []int64
	commands       map[string][]*mtproto.BotCommand
	menuDefault    menuHold
	menuUsers      map[int64]menuHold
	groupRights    *mtproto.ChatAdminRights
	channelRights  *mtproto.ChatAdminRights
	pendingUpdates int32
	pendingMessage string
	pendingSet     bool
}

type buttonHold struct {
	botID int64
	btn   *mtproto.KeyboardButton
}

type world struct {
	next    int64
	dir     Directory
	bots    map[int64]*botRec
	names   map[string]int64
	owned   map[int64][]int64
	allow   map[int64]map[int64]bool
	emoji   map[int64]map[int64]bool
	buttons map[string]buttonHold
	joins   map[int64]int64
	verify  map[string]string
	refs    map[string][]*starRefLink
}

var (
	mu sync.Mutex
	w  = newWorld()
)

func newWorld() *world {
	return &world{
		next:    10_000_000_000,
		bots:    map[int64]*botRec{},
		names:   map[string]int64{},
		owned:   map[int64][]int64{},
		allow:   map[int64]map[int64]bool{},
		emoji:   map[int64]map[int64]bool{},
		buttons: map[string]buttonHold{},
		joins:   map[int64]int64{},
		verify:  map[string]string{},
		refs:    map[string][]*starRefLink{},
	}
}

// Reset drops process-local bot state. Tests call it between cases.
func Reset() {
	mu.Lock()
	w = newWorld()
	mu.Unlock()
}

// UseUsers points username and bot lookups at the user service.
func UseUsers(d Directory) {
	mu.Lock()
	w.dir = d
	mu.Unlock()
}

func auth(self int64) error {
	if self == 0 {
		return mtproto.ErrAuthKeyUnregistered
	}
	return nil
}

func directory() Directory {
	mu.Lock()
	d := w.dir
	mu.Unlock()
	return d
}

func remoteTaken(ctx context.Context, name string) error {
	d := directory()
	if d == nil {
		return nil
	}
	r, err := d.UserCheckUsername(ctx, &userpb.TLUserCheckUsername{Username: name})
	if err != nil {
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case mtproto.ErrBadRequest, mtproto.ErrUnauthorized, mtproto.ErrForbidden:
				return err
			}
		}
		return mtproto.ErrUsernameInvalid
	}
	switch r.GetPredicateName() {
	case userpb.Predicate_usernameExisted, userpb.Predicate_usernameExistedNotMe:
		return mtproto.ErrUsernameOccupied
	default:
		return nil
	}
}

func remoteIsBot(ctx context.Context, id int64) bool {
	d := directory()
	if d == nil || id <= 0 {
		return false
	}
	ok, err := d.UserIsBot(ctx, &userpb.TLUserIsBot{Id: id})
	if err != nil {
		return false
	}
	return mtproto.FromBool(ok)
}

func knownBotLocked(id int64) bool {
	if id <= 0 {
		return false
	}
	if _, ok := w.bots[id]; ok {
		return true
	}
	return userpb.GetBotNameById(id) != ""
}

// CheckUsername reports whether username can be assigned to a bot.
// https://core.telegram.org/method/bots.checkUsername
func CheckUsername(ctx context.Context, self int64, additional bool, username string) (*mtproto.Bool, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	username = strings.TrimSpace(username)
	if err := validateUsername(username, !additional); err != nil {
		return nil, err
	}
	if err := remoteTaken(ctx, username); err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	if takenLocked(username) {
		return nil, mtproto.ErrUsernameOccupied
	}
	return mtproto.BoolTrue, nil
}

// CreateBot creates a managed bot owned by self.
// https://core.telegram.org/method/bots.createBot
func CreateBot(ctx context.Context, self int64, via bool, name, username string, manager *mtproto.InputUser) (*mtproto.User, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if !validName(name) {
		return nil, mtproto.ErrNameInvalid
	}
	username = strings.TrimSpace(username)
	if err := validateUsername(username, true); err != nil {
		return nil, err
	}
	mgr, err := managerID(ctx, self, manager)
	if err != nil {
		return nil, err
	}
	if err := remoteTaken(ctx, username); err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	if takenLocked(username) {
		return nil, mtproto.ErrUsernameOccupied
	}
	if len(w.owned[self]) >= botsCreateLimit {
		return nil, mtproto.ErrBotCreateLimitExceeded
	}
	id := w.next
	w.next++
	b := &botRec{
		id: id, hash: randInt63(), owner: self, manager: mgr, display: name, viaLink: via,
		token: newToken(id), infoVer: 1,
		names:    []uname{{name: username, active: true, editable: true}},
		info:     map[string]texts{"": {name: name}},
		previews: map[string][]preview{},
		commands: map[string][]*mtproto.BotCommand{},
	}
	w.bots[id] = b
	w.names[strings.ToLower(username)] = id
	w.owned[self] = append(w.owned[self], id)
	return b.mtUser(), nil
}

// AdminedBots returns bots created by self.
// https://core.telegram.org/method/bots.getAdminedBots
func AdminedBots(self int64) (*mtproto.Vector_User, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	var users []*mtproto.User
	for _, id := range w.owned[self] {
		if b := w.bots[id]; b != nil {
			users = append(users, b.mtUser())
		}
	}
	if users == nil {
		users = []*mtproto.User{}
	}
	return &mtproto.Vector_User{Datas: users}, nil
}

// GetBotInfo returns the name, about, and description for lang.
// https://core.telegram.org/method/bots.getBotInfo
func GetBotInfo(ctx context.Context, self int64, bot *mtproto.InputUser, lang string) (*mtproto.Bots_BotInfo, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	if err := validLang(lang); err != nil {
		return nil, err
	}
	b, err := editable(ctx, self, bot, true)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	cur := w.bots[b]
	if cur == nil {
		return nil, mtproto.ErrBotInvalid
	}
	name, about, desc := cur.texts(lang)
	return mtproto.MakeTLBotsBotInfo(&mtproto.Bots_BotInfo{
		Name: name, About: about, Description: desc,
	}).To_Bots_BotInfo(), nil
}

// SetBotInfo writes the profile strings the owner or the bot itself supplied.
// https://core.telegram.org/method/bots.setBotInfo
func SetBotInfo(ctx context.Context, self int64, bot *mtproto.InputUser, lang string, name, about, desc *wrapperspb.StringValue) (*mtproto.Bool, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	if err := validLang(lang); err != nil {
		return nil, err
	}
	if name != nil && !validName(strings.TrimSpace(name.GetValue())) {
		return nil, mtproto.ErrNameInvalid
	}
	if about != nil && utf8.RuneCountInString(about.GetValue()) > aboutMaxRunes {
		return nil, mtproto.ErrAboutTooLong
	}
	if desc != nil && utf8.RuneCountInString(desc.GetValue()) > descriptionMaxRunes {
		return nil, mtproto.ErrAboutTooLong
	}
	id, err := editable(ctx, self, bot, true)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	cur := w.bots[id]
	if cur == nil {
		return nil, mtproto.ErrBotInvalid
	}
	row := cur.info[lang]
	if name != nil {
		row.name = strings.TrimSpace(name.GetValue())
		if lang == "" {
			cur.display = row.name
		}
	}
	if about != nil {
		row.about = about.GetValue()
	}
	if desc != nil {
		row.desc = desc.GetValue()
	}
	if cur.info == nil {
		cur.info = map[string]texts{}
	}
	cur.info[lang] = row
	cur.infoVer++
	return mtproto.BoolTrue, nil
}

// BotInfoLangs is the legacy getBotInfo that returns the caller's strings.
func BotInfoLangs(self int64, lang string) (*mtproto.Vector_String, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	if err := validLang(lang); err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	cur := w.bots[self]
	if cur == nil && userpb.GetBotNameById(self) == "" {
		return nil, mtproto.ErrUserBotInvalid
	}
	name, about, desc := "", "", ""
	if cur != nil {
		name, about, desc = cur.texts(lang)
	}
	return &mtproto.Vector_String{Datas: []string{name, about, desc}}, nil
}

// Recommendations returns other main-app bots besides bot.
// Subscriber overlap is not stored, so the list is the other catalog bots.
// https://core.telegram.org/method/bots.getBotRecommendations
func Recommendations(ctx context.Context, self int64, bot *mtproto.InputUser) (*mtproto.Users_Users, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	id, err := anyBot(ctx, self, bot)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	var users []*mtproto.User
	for _, b := range w.bots {
		if b.id == id || !b.mainApp {
			continue
		}
		users = append(users, b.mtUser())
		if len(users) == 20 {
			break
		}
	}
	if users == nil {
		users = []*mtproto.User{}
	}
	return mtproto.MakeTLUsersUsers(&mtproto.Users_Users{Users: users}).To_Users_Users(), nil
}

// SetCustomVerification assigns or clears a verification mark.
// Bots that are not organization verifiers get BOT_VERIFIER_FORBIDDEN.
// https://core.telegram.org/method/bots.setCustomVerification
func SetCustomVerification(ctx context.Context, self int64, enabled bool, bot *mtproto.InputUser, peer *mtproto.InputPeer, description string) (*mtproto.Bool, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	if utf8.RuneCountInString(description) > verifyDescriptionMax {
		return nil, mtproto.ErrAboutTooLong
	}
	pref, err := peerRef(self, peer)
	if err != nil {
		return nil, err
	}
	id, err := verifierBot(ctx, self, bot)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	cur := w.bots[id]
	if cur == nil || !cur.verifier {
		return nil, mtproto.ErrBotVerifierForbidden
	}
	key := strconv.FormatInt(id, 10) + ":" + pref
	if !enabled {
		delete(w.verify, key)
		return mtproto.BoolTrue, nil
	}
	w.verify[key] = description
	return mtproto.BoolTrue, nil
}

// UpdateStarRefProgram creates, raises, or ends the bot's affiliate program.
// https://core.telegram.org/method/bots.updateStarRefProgram
func UpdateStarRefProgram(ctx context.Context, self int64, bot *mtproto.InputUser, permille int32, months *wrapperspb.Int32Value) (*mtproto.StarRefProgram, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	if permille < 0 || permille > starRefMaxPermille {
		return nil, mtproto.ErrStarrefPermilleInvalid
	}
	if permille > 0 && permille < starRefMinPermille {
		return nil, mtproto.ErrStarrefPermilleInvalid
	}
	var hasMonths bool
	var monthN int32
	if months != nil {
		monthN = months.GetValue()
		if monthN < 1 {
			return nil, mtproto.ErrStarrefPermilleInvalid
		}
		hasMonths = true
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
	now := time.Now().Unix()
	prog := cur.star
	if prog != nil && prog.closed && prog.end > now {
		if permille == 0 {
			return prog.tl(id), nil
		}
		return nil, mtproto.ErrStarrefAwaitingEnd
	}
	if prog != nil && prog.closed && prog.end <= now {
		prog = nil
	}
	if permille == 0 {
		if prog == nil {
			empty := &starProg{}
			return empty.tl(id), nil
		}
		prog.closed = true
		prog.end = now + int64(starRefCoolDown/time.Second)
		cur.star = prog
		return prog.tl(id), nil
	}
	if prog != nil && !prog.closed {
		if permille < prog.permille {
			return nil, mtproto.ErrStarrefPermilleTooLow
		}
		// A finite duration is shorter than an open-ended program, and a
		// smaller month count is shorter than the one already published.
		// Both have to be terminated before they can be lowered.
		if hasMonths && (!prog.hasMonths || monthN < prog.months) {
			return nil, mtproto.ErrStarrefPermilleInvalid
		}
	}
	next := &starProg{permille: permille, months: monthN, hasMonths: hasMonths}
	cur.star = next
	return next.tl(id), nil
}
