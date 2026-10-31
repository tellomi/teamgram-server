package botadmin

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/proto"
)

// menuHold is one bots.setBotMenuButton value.
// https://core.telegram.org/method/bots.setBotMenuButton
type menuHold struct {
	kind string
	text string
	url  string
}

const (
	menuDefault  = "default"
	menuCommands = "commands"
	menuCustom   = "custom"
)

// SetMenuButton stores the bot menu button for one user, or the default when
// user_id is inputUserEmpty.
func SetMenuButton(ctx context.Context, self int64, user *mtproto.InputUser, button *mtproto.BotMenuButton) (*mtproto.Bool, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	if !selfIsBot(ctx, self) {
		return nil, mtproto.ErrUserBotRequired
	}
	target, err := menuUser(self, user)
	if err != nil {
		return nil, err
	}
	hold, err := menuFrom(button)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	b := w.bots[self]
	if b == nil {
		return nil, mtproto.ErrUserBotRequired
	}
	if target == 0 {
		b.menuDefault = hold
		return mtproto.BoolTrue, nil
	}
	if b.menuUsers == nil {
		b.menuUsers = map[int64]menuHold{}
	}
	b.menuUsers[target] = hold
	return mtproto.BoolTrue, nil
}

// MenuButton returns the button for user_id, or the bot default.
// https://core.telegram.org/method/bots.getBotMenuButton
func MenuButton(ctx context.Context, self int64, user *mtproto.InputUser) (*mtproto.BotMenuButton, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	if !selfIsBot(ctx, self) {
		return nil, mtproto.ErrUserBotRequired
	}
	target, err := menuUser(self, user)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	b := w.bots[self]
	if b == nil {
		return nil, mtproto.ErrUserBotRequired
	}
	return holdButton(b.menuOf(target)), nil
}

func (b *botRec) menuOf(user int64) menuHold {
	if b == nil {
		return menuHold{kind: menuDefault}
	}
	if user != 0 && b.menuUsers != nil {
		if hold, ok := b.menuUsers[user]; ok {
			return hold
		}
	}
	if b.menuDefault.kind == "" {
		return menuHold{kind: menuDefault}
	}
	return b.menuDefault
}

func menuUser(self int64, in *mtproto.InputUser) (int64, error) {
	if in == nil || in.GetPredicateName() == "" || in.GetPredicateName() == mtproto.Predicate_inputUserEmpty {
		return 0, nil
	}
	if in.GetPredicateName() == mtproto.Predicate_inputUserSelf {
		return self, nil
	}
	if in.GetPredicateName() != mtproto.Predicate_inputUser || in.GetUserId() == 0 {
		return 0, mtproto.ErrUserIdInvalid
	}
	return in.GetUserId(), nil
}

func menuFrom(in *mtproto.BotMenuButton) (menuHold, error) {
	if in == nil || in.GetPredicateName() == "" || in.GetPredicateName() == mtproto.Predicate_botMenuButtonDefault {
		return menuHold{kind: menuDefault}, nil
	}
	if in.GetPredicateName() == mtproto.Predicate_botMenuButtonCommands {
		return menuHold{kind: menuCommands}, nil
	}
	if in.GetPredicateName() != mtproto.Predicate_botMenuButton {
		return menuHold{}, mtproto.ErrButtonTextInvalid
	}
	text := strings.TrimSpace(in.GetText())
	if text == "" || utf8.RuneCountInString(text) > 64 {
		return menuHold{}, mtproto.ErrButtonTextInvalid
	}
	raw := strings.TrimSpace(in.GetUrl())
	if !strings.HasPrefix(raw, "https://") || len(raw) > 2048 || strings.Contains(raw, " ") {
		return menuHold{}, mtproto.ErrButtonUrlInvalid
	}
	return menuHold{kind: menuCustom, text: text, url: raw}, nil
}

func holdButton(hold menuHold) *mtproto.BotMenuButton {
	switch hold.kind {
	case menuCommands:
		return mtproto.MakeTLBotMenuButtonCommands(&mtproto.BotMenuButton{}).To_BotMenuButton()
	case menuCustom:
		return mtproto.MakeTLBotMenuButton(&mtproto.BotMenuButton{Text: hold.text, Url: hold.url}).To_BotMenuButton()
	default:
		return mtproto.MakeTLBotMenuButtonDefault(&mtproto.BotMenuButton{}).To_BotMenuButton()
	}
}

// SetDefaultAdminRights stores the rights a bot asks for in groups or channels.
// The caller is the bot. https://core.telegram.org/method/bots.setBotGroupDefaultAdminRights
func SetDefaultAdminRights(ctx context.Context, self int64, channel bool, rights *mtproto.ChatAdminRights) (*mtproto.Bool, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	if !selfIsBot(ctx, self) {
		return nil, mtproto.ErrUserBotRequired
	}
	mu.Lock()
	defer mu.Unlock()
	b := w.bots[self]
	if b == nil {
		return nil, mtproto.ErrUserBotRequired
	}
	copied := copyAdminRights(rights)
	if channel {
		b.channelRights = copied
	} else {
		b.groupRights = copied
	}
	return mtproto.BoolTrue, nil
}

// DefaultAdminRights returns the rights stored for a bot, or nil when the bot
// has not set them. users.getFullUser publishes these on the bot profile.
func DefaultAdminRights(botID int64) (group, channel *mtproto.ChatAdminRights) {
	if botID == 0 {
		return nil, nil
	}
	mu.Lock()
	defer mu.Unlock()
	b := w.bots[botID]
	if b == nil {
		return nil, nil
	}
	return copyAdminRights(b.groupRights), copyAdminRights(b.channelRights)
}

func copyAdminRights(in *mtproto.ChatAdminRights) *mtproto.ChatAdminRights {
	if in == nil {
		return nil
	}
	c := proto.Clone(in).(*mtproto.ChatAdminRights)
	return mtproto.MakeTLChatAdminRights(c).To_ChatAdminRights()
}

// SetUpdatesStatus records help.setBotUpdatesStatus for a bot that has no
// separate update queue on this process.
// https://core.telegram.org/method/help.setBotUpdatesStatus
func SetUpdatesStatus(ctx context.Context, self int64, pending int32, message string) (*mtproto.Bool, error) {
	if err := auth(self); err != nil {
		return nil, err
	}
	if !selfIsBot(ctx, self) {
		return nil, mtproto.ErrUserBotRequired
	}
	if len(message) > 4096 {
		return nil, mtproto.ErrMessageTooLong
	}
	mu.Lock()
	defer mu.Unlock()
	b := w.bots[self]
	if b == nil {
		return nil, mtproto.ErrUserBotRequired
	}
	b.pendingUpdates = pending
	b.pendingMessage = message
	b.pendingSet = true
	return mtproto.BoolTrue, nil
}

// CallerIsBot reports whether self is a bot this process knows about.
func CallerIsBot(ctx context.Context, self int64) bool {
	return selfIsBot(ctx, self)
}

// UpdatesStatus returns the last help.setBotUpdatesStatus report.
func UpdatesStatus(botID int64) (pending int32, message string, ok bool) {
	mu.Lock()
	defer mu.Unlock()
	b := w.bots[botID]
	if b == nil || !b.pendingSet {
		return 0, "", false
	}
	return b.pendingUpdates, b.pendingMessage, true
}
