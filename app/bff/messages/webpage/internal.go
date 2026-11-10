// Package webpage builds native link cards for messages.getWebPage,
// messages.getWebPagePreview, and messages.sendMessage.
//
// Internal tell.cc and t.me links are filled from data this process already
// has. External pages are not downloaded unless TELLOMI_SERVER_LINK_FETCH=1.
// A card the sending client fetched itself is accepted by ClientMedia.
package webpage

import (
	"context"
	"fmt"
	"hash/fnv"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"github.com/teamgram/teamgram-server/pkg/brand"
	"github.com/teamgram/teamgram-server/pkg/langpack"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	siteTellomi = "Tellomi"

	maxTitleRunes       = 128
	maxDescriptionRunes = 400
	maxSiteRunes        = 64
	maxURLBytes         = 2048
	maxFieldBytes       = 16 << 10
	maxStickerPreview   = 5
	maxMessageRunes     = 200

	// UnverifiedAuthor is webPage.author when the card was submitted by the
	// sending client. The server did not fetch the page. Clients hide this
	// author instead of printing it under the title.
	UnverifiedAuthor = "unverified"
)

var (
	usernameRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{3,31}$`)
	shortRE    = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)
	hashRE     = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)
)

func init() {
	chatbox.InternalCard = internalCard
}

func internalCard(ctx context.Context, u *url.URL) (*mtproto.WebPage, []*mtproto.Chat, []*mtproto.User, bool) {
	return resolve(ctx, u)
}

// Get is messages.getWebPage.
func Get(ctx context.Context, raw string, hash int32) (*mtproto.WebPage, []*mtproto.Chat, []*mtproto.User, error) {
	return chatbox.WebPageContext(ctx, raw, hash)
}

// Preview is messages.getWebPagePreview.
func Preview(ctx context.Context, text string, entities []*mtproto.MessageEntity) (*mtproto.MessageMedia, []*mtproto.Chat, []*mtproto.User) {
	return chatbox.WebPagePreviewContext(ctx, text, entities)
}

// PreviewMedia is the card attached to a plain text send. Only internal
// telegram_* pages are attached. External pages stay text until the client
// submits a card with messages.sendMedia.
func PreviewMedia(ctx context.Context, text string, entities []*mtproto.MessageEntity) *mtproto.MessageMedia {
	media, _, _ := Preview(ctx, text, entities)
	if !internalType(media.GetWebpage().GetType().GetValue()) {
		return nil
	}
	return media
}

func internalType(typ string) bool {
	return strings.HasPrefix(typ, "telegram_")
}

func resolve(ctx context.Context, u *url.URL) (*mtproto.WebPage, []*mtproto.Chat, []*mtproto.User, bool) {
	if u == nil || !internalHost(u.Hostname()) {
		return nil, nil, nil, false
	}
	raw := canonical(u)
	segs := segments(u.Path)
	if segs == nil {
		return empty(raw), nil, nil, true
	}
	if len(segs) == 0 {
		return empty(raw), nil, nil, true
	}
	head := segs[0]
	switch strings.ToLower(head) {
	case "addstickers":
		if len(segs) != 2 || !shortRE.MatchString(segs[1]) {
			return empty(raw), nil, nil, true
		}
		page := stickerPage(raw, segs[1], false)
		return page, nil, nil, true
	case "addemoji":
		if len(segs) != 2 || !shortRE.MatchString(segs[1]) {
			return empty(raw), nil, nil, true
		}
		page := stickerPage(raw, segs[1], true)
		return page, nil, nil, true
	case "joinchat":
		if len(segs) != 2 || !hashRE.MatchString(segs[1]) {
			return empty(raw), nil, nil, true
		}
		page, chats := invitePage(ctx, raw, segs[1])
		return page, chats, nil, true
	case "s":
		if len(segs) < 2 || len(segs) > 3 || !validUsername(segs[1]) {
			return empty(raw), nil, nil, true
		}
		if len(segs) == 3 {
			id, err := parsePositive32(segs[2])
			if err != nil {
				return empty(raw), nil, nil, true
			}
			page, chats := messageByUsername(ctx, raw, segs[1], id)
			return page, chats, nil, true
		}
		page, chats, users := peerPage(ctx, raw, segs[1])
		return page, chats, users, true
	case "c":
		if len(segs) != 3 {
			return empty(raw), nil, nil, true
		}
		id, err := parseChannelID(segs[1])
		if err != nil {
			return empty(raw), nil, nil, true
		}
		mid, err := parsePositive32(segs[2])
		if err != nil {
			return empty(raw), nil, nil, true
		}
		page, chats := messageByID(ctx, raw, id, mid)
		return page, chats, nil, true
	case "proxy", "socks", "share", "boost":
		return empty(raw), nil, nil, true
	default:
		if strings.HasPrefix(head, "+") {
			hash := strings.TrimPrefix(head, "+")
			if len(segs) != 1 || !hashRE.MatchString(hash) {
				return empty(raw), nil, nil, true
			}
			page, chats := invitePage(ctx, raw, hash)
			return page, chats, nil, true
		}
		if !validUsername(head) {
			return empty(raw), nil, nil, true
		}
		if len(segs) == 1 {
			page, chats, users := peerPage(ctx, raw, head)
			return page, chats, users, true
		}
		if len(segs) == 2 {
			if strings.EqualFold(segs[1], "boost") {
				return empty(raw), nil, nil, true
			}
			mid, err := parsePositive32(segs[1])
			if err != nil {
				return empty(raw), nil, nil, true
			}
			page, chats := messageByUsername(ctx, raw, head, mid)
			return page, chats, nil, true
		}
		return empty(raw), nil, nil, true
	}
}

func stickerPage(raw, short string, pathEmoji bool) *mtproto.WebPage {
	set, err := catalog.Must().GetSet(0, mtproto.MakeTLInputStickerSetShortName(&mtproto.InputStickerSet{
		ShortName: short,
	}).To_InputStickerSet(), 0)
	if err != nil || set == nil || set.GetSet() == nil || set.GetPredicateName() != mtproto.Predicate_messages_stickerSet {
		return empty(raw)
	}
	st := set.GetSet()
	docs := set.GetDocuments()
	n := st.GetCount()
	if n == 0 {
		n = int32(len(docs))
	}
	emoji := st.GetEmojis() || pathEmoji
	return makePage(raw, "telegram_stickerset", st.GetTitle(), packDescription(n, emoji), func(p *mtproto.WebPage) {
		var preview []*mtproto.Document
		if len(docs) > 0 {
			limit := len(docs)
			if limit > maxStickerPreview {
				limit = maxStickerPreview
			}
			preview = docs[:limit]
			p.Document = docs[0]
		}
		p.Attributes = []*mtproto.WebPageAttribute{
			mtproto.MakeTLWebPageAttributeStickerSet(&mtproto.WebPageAttribute{
				Emojis:                emoji,
				TextColor_FLAGBOOLEAN: st.GetTextColor(),
				Stickers:              preview,
			}).To_WebPageAttribute(),
		}
	})
}

func packDescription(n int32, emoji bool) string {
	if n < 0 {
		n = 0
	}
	if emoji {
		if n == 1 {
			return "Free emoji pack with 1 emoji."
		}
		return fmt.Sprintf("Free emoji pack with %d emoji.", n)
	}
	if n == 1 {
		return "Free stickerpack with 1 sticker."
	}
	return fmt.Sprintf("Free stickerpack with %d stickers.", n)
}

func peerPage(ctx context.Context, raw, name string) (*mtproto.WebPage, []*mtproto.Chat, []*mtproto.User) {
	if chat, ok := lookupChannel(ctx, name); ok {
		return channelPage(ctx, raw, chat), []*mtproto.Chat{chat}, nil
	}
	if card, ok := lookupUserCard(ctx, name); ok {
		var users []*mtproto.User
		if card.User != nil {
			users = []*mtproto.User{card.User}
		}
		return userPage(raw, name, card), nil, users
	}
	return empty(raw), nil, nil
}

func messageByUsername(ctx context.Context, raw, name string, id int32) (*mtproto.WebPage, []*mtproto.Chat) {
	chat, ok := lookupChannel(ctx, name)
	if !ok {
		return empty(raw), nil
	}
	return messagePage(ctx, raw, chat, id)
}

func messageByID(ctx context.Context, raw string, channelID int64, id int32) (*mtproto.WebPage, []*mtproto.Chat) {
	chats := channel.Shared().ChatsFor(ctx, viewerID(ctx), channelID)
	if len(chats) == 0 || chats[0].GetPredicateName() != mtproto.Predicate_channel {
		return empty(raw), nil
	}
	return messagePage(ctx, raw, chats[0], id)
}

func messagePage(ctx context.Context, raw string, chat *mtproto.Chat, id int32) (*mtproto.WebPage, []*mtproto.Chat) {
	box, err := channel.Shared().MessageBox(ctx, viewerID(ctx), chat.GetId(), id)
	if err != nil || box.GetMessage() == nil {
		return empty(raw), nil
	}
	text := box.GetMessage().GetMessage()
	return makePage(raw, "telegram_message", chat.GetTitle(), clamp(text, maxMessageRunes), nil), []*mtproto.Chat{chat}
}

func userPage(raw, name string, card UserCard) *mtproto.WebPage {
	typ := "telegram_user"
	if card.Bot {
		typ = "telegram_bot"
	}
	user := name
	if card.Username != "" {
		user = card.Username
	}
	return makePage(raw, typ, card.Title, userDescription(card.About, user), attachPhoto(card.Photo))
}

func userDescription(about, username string) string {
	if strings.TrimSpace(about) != "" {
		return about
	}
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	if username == "" {
		return ""
	}
	return fmt.Sprintf("You can contact @%s right away.", username)
}

func channelPage(ctx context.Context, raw string, chat *mtproto.Chat) *mtproto.WebPage {
	return makePage(raw, channelType(chat), chat.GetTitle(), channelDescription(ctx, chat), attachPhoto(chatPhoto(ctx, chat)))
}

func channelType(chat *mtproto.Chat) string {
	request := chat.GetJoinRequest() && chat.GetLeft()
	if chat.GetMegagroup() {
		if request {
			return "telegram_megagroup_request"
		}
		return "telegram_megagroup"
	}
	if request {
		return "telegram_channel_request"
	}
	return "telegram_channel"
}

func channelDescription(ctx context.Context, chat *mtproto.Chat) string {
	full, err := channel.Shared().GetFullChannel(ctx, viewerID(ctx), inputChannel(chat))
	if err == nil && full.GetFullChat() != nil {
		if about := strings.TrimSpace(full.GetFullChat().GetAbout()); about != "" {
			return about
		}
	}
	n := int32(0)
	if v := chat.GetParticipantsCount_FLAGINT32(); v != nil {
		n = v.GetValue()
	}
	if n <= 0 {
		return ""
	}
	return audienceLine(ctx, n, !chat.GetMegagroup())
}

func invitePage(ctx context.Context, raw, hash string) (*mtproto.WebPage, []*mtproto.Chat) {
	if basicInviteHash(hash) {
		return groupInvitePage(ctx, raw, hash)
	}
	inv, _, err := channel.Shared().CheckInvite(ctx, viewerID(ctx), hash)
	if err != nil || inv == nil {
		return empty(raw), nil
	}
	if chat := inv.GetChat(); chat != nil && chat.GetPredicateName() == mtproto.Predicate_channel {
		return channelPage(ctx, raw, chat), []*mtproto.Chat{chat}
	}
	if chat := inv.GetChat(); chat != nil && chat.GetPredicateName() == mtproto.Predicate_chat {
		return groupPage(ctx, raw, GroupCard{
			Title:   chat.GetTitle(),
			Members: chat.GetParticipantsCount_FLAGINT32().GetValue(),
			Photo:   chatPhoto(ctx, chat),
			Chat:    chat,
		}), []*mtproto.Chat{chat}
	}
	title := strings.TrimSpace(inv.GetTitle())
	if title == "" {
		return empty(raw), nil
	}
	desc := ""
	if inv.GetAbout() != nil {
		desc = inv.GetAbout().GetValue()
	}
	if desc == "" && inv.GetParticipantsCount() > 0 {
		desc = audienceLine(ctx, inv.GetParticipantsCount(), inv.GetBroadcast() && !inv.GetMegagroup())
	}
	return makePage(raw, inviteType(inv), title, desc, attachPhoto(usablePhoto(inv.GetPhoto()))), nil
}

func inviteType(inv *mtproto.ChatInvite) string {
	if inv.GetMegagroup() {
		if inv.GetRequestNeeded() {
			return "telegram_megagroup_request"
		}
		return "telegram_megagroup"
	}
	if inv.GetBroadcast() || inv.GetChannel() {
		if inv.GetRequestNeeded() {
			return "telegram_channel_request"
		}
		return "telegram_channel"
	}
	return "telegram_chat"
}

func groupInvitePage(ctx context.Context, raw, hash string) (*mtproto.WebPage, []*mtproto.Chat) {
	card, ok := lookupGroupCard(ctx, hash)
	if !ok || strings.TrimSpace(card.Title) == "" {
		return empty(raw), nil
	}
	var chats []*mtproto.Chat
	if card.Chat != nil {
		chats = []*mtproto.Chat{card.Chat}
	}
	return groupPage(ctx, raw, card), chats
}

func groupPage(ctx context.Context, raw string, card GroupCard) *mtproto.WebPage {
	desc := strings.TrimSpace(card.About)
	if desc == "" && card.Members > 0 {
		desc = audienceLine(ctx, card.Members, false)
	}
	return makePage(raw, "telegram_chat", card.Title, desc, attachPhoto(card.Photo))
}

// basicInviteHash is a basic-group invite. Channel hashes start with a letter.
func basicInviteHash(hash string) bool {
	if hash == "" {
		return false
	}
	c := hash[0]
	return c >= '0' && c <= '9'
}

func lookupChannel(ctx context.Context, name string) (*mtproto.Chat, bool) {
	chat, err := channel.Shared().ResolveUsername(ctx, viewerID(ctx), name)
	if err != nil || chat == nil || chat.GetPredicateName() != mtproto.Predicate_channel {
		return nil, false
	}
	return chat, true
}

func inputChannel(chat *mtproto.Chat) *mtproto.InputChannel {
	hash := int64(0)
	if v := chat.GetAccessHash_FLAGINT64(); v != nil {
		hash = v.GetValue()
	}
	return mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  chat.GetId(),
		AccessHash: hash,
	}).To_InputChannel()
}

func viewerID(ctx context.Context) int64 {
	md := metadata.RpcMetadataFromIncoming(ctx)
	if md == nil {
		return 0
	}
	return md.UserId
}

func internalHost(host string) bool {
	host = trimHost(host)
	switch host {
	case "tell.cc", "t.me", "telegram.me", "telegram.dog":
		return true
	}
	if base, err := url.Parse(brand.LinkBase()); err == nil && trimHost(base.Hostname()) == host && host != "" {
		return true
	}
	return false
}

func trimHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if i := strings.LastIndex(host, ":"); i >= 0 && !strings.Contains(host, "]") {
		host = host[:i]
	}
	return strings.TrimPrefix(host, "www.")
}

func segments(path string) []string {
	if strings.Contains(path, "\\") {
		return nil
	}
	var out []string
	for _, p := range strings.Split(path, "/") {
		if p == "" {
			continue
		}
		dec, err := url.PathUnescape(p)
		if err != nil || dec == "" || dec == "." || dec == ".." || strings.Contains(dec, "/") {
			return nil
		}
		out = append(out, dec)
	}
	return out
}

func validUsername(name string) bool {
	return usernameRE.MatchString(name) && !strings.HasSuffix(name, "_")
}

func parsePositive32(s string) (int32, error) {
	if s == "" || (len(s) > 1 && s[0] == '0') || strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+") {
		return 0, mtproto.ErrWebpageCurlFailed
	}
	n, err := strconv.ParseInt(s, 10, 32)
	if err != nil || n <= 0 {
		return 0, mtproto.ErrWebpageCurlFailed
	}
	return int32(n), nil
}

func parseChannelID(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "-100") && len(s) > 4 {
		s = s[4:]
	}
	if s == "" || strings.Contains(s, "-") || (len(s) > 1 && s[0] == '0') {
		return 0, mtproto.ErrWebpageCurlFailed
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, mtproto.ErrWebpageCurlFailed
	}
	return n, nil
}

func canonical(u *url.URL) string {
	cp := *u
	cp.Scheme = "https"
	cp.User = nil
	cp.RawQuery = ""
	cp.Fragment = ""
	return cp.String()
}

func displayURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	path := u.EscapedPath()
	if path == "" || path == "/" {
		return u.Host
	}
	return u.Host + path
}

func makePage(raw, typ, title, desc string, extra func(*mtproto.WebPage)) *mtproto.WebPage {
	title = clamp(title, maxTitleRunes)
	desc = clamp(desc, maxDescriptionRunes)
	sum := hash32(typ, raw, title, desc)
	page := mtproto.MakeTLWebPage(&mtproto.WebPage{
		Id:             int64(sum),
		Url_STRING:     raw,
		Url_FLAGSTRING: wrapperspb.String(raw),
		DisplayUrl:     displayURL(raw),
		Hash:           sum,
		Type:           wrapperspb.String(typ),
		SiteName:       wrapperspb.String(siteTellomi),
		Title:          wrapperspb.String(title),
		Date:           int32(time.Now().Unix()),
	}).To_WebPage()
	if desc != "" {
		page.Description = wrapperspb.String(desc)
	}
	if extra != nil {
		extra(page)
	}
	return page
}

func empty(raw string) *mtproto.WebPage {
	return mtproto.MakeTLWebPageEmpty(&mtproto.WebPage{
		Id:             int64(hash32("empty", raw)),
		Url_STRING:     raw,
		Url_FLAGSTRING: wrapperspb.String(raw),
	}).To_WebPage()
}

func hash32(parts ...string) int32 {
	h := fnv.New32a()
	for _, p := range parts {
		_, _ = h.Write([]byte(p))
		_, _ = h.Write([]byte{0})
	}
	sum := int32(h.Sum32() & 0x7fffffff)
	if sum == 0 {
		return 1
	}
	return sum
}

func attachPhoto(photo *mtproto.Photo) func(*mtproto.WebPage) {
	return func(p *mtproto.WebPage) {
		if usablePhoto(photo) != nil {
			p.Photo = photo
		}
	}
}

func usablePhoto(photo *mtproto.Photo) *mtproto.Photo {
	if photo == nil || photo.GetId() == 0 || photo.GetPredicateName() == mtproto.Predicate_photoEmpty {
		return nil
	}
	return photo
}

func chatPhoto(ctx context.Context, chat *mtproto.Chat) *mtproto.Photo {
	ph := chat.GetPhoto()
	if ph == nil || ph.GetPhotoId() == 0 {
		return nil
	}
	return storedPhoto(ctx, ph.GetPhotoId())
}

// audienceLine is the official member or subscriber line. The wording comes
// from the viewer's langpack (Conversation.StatusMembers / StatusSubscribers
// on iOS, Members / Subscribers on Android). English is the fallback.
func audienceLine(ctx context.Context, n int32, subscribers bool) string {
	if n < 0 {
		n = 0
	}
	if text := localizedCount(ctx, n, subscribers); text != "" {
		return text
	}
	if subscribers {
		if n == 1 {
			return "1 subscriber"
		}
		return fmt.Sprintf("%d subscribers", n)
	}
	if n == 1 {
		return "1 member"
	}
	return fmt.Sprintf("%d members", n)
}

func localizedCount(ctx context.Context, n int32, subscribers bool) string {
	md := metadata.RpcMetadataFromIncoming(ctx)
	if md == nil || strings.TrimSpace(md.GetLangCode()) == "" {
		return ""
	}
	lang := strings.TrimSpace(md.GetLangCode())
	platform := langpack.NormalizePlatform(md.GetLangpack())
	if platform == "" {
		platform = "ios"
	}
	keys := []string{"Conversation.StatusMembers"}
	if subscribers {
		keys = []string{"Conversation.StatusSubscribers"}
	}
	if platform == "android" {
		if subscribers {
			keys = append([]string{"Subscribers"}, keys...)
		} else {
			keys = append([]string{"Members"}, keys...)
		}
	}
	cat := langpack.DefaultCatalog()
	if cat == nil {
		return ""
	}
	code, _ := langpack.CanonicalLang(lang)
	if code == "" {
		code = lang
	}
	plural := "en"
	if info, err := cat.Language(platform, code); err == nil && info.GetPluralCode() != "" {
		plural = info.GetPluralCode()
	}
	form := pluralCategory(plural, n)
	for _, key := range keys {
		for _, pack := range []string{platform, "ios", "android"} {
			rows, err := cat.Strings(pack, code, []string{key})
			if err != nil || len(rows) == 0 {
				continue
			}
			text := pluralText(rows[0], form)
			if text == "" {
				continue
			}
			return fillCount(text, n)
		}
	}
	return ""
}

func pluralText(s *mtproto.LangPackString, form string) string {
	if s == nil || s.GetPredicateName() == mtproto.Predicate_langPackStringDeleted {
		return ""
	}
	if s.GetPredicateName() != mtproto.Predicate_langPackStringPluralized {
		return s.GetValue()
	}
	pick := func(v *wrapperspb.StringValue) string {
		if v == nil {
			return ""
		}
		return v.GetValue()
	}
	var text string
	switch form {
	case "zero":
		text = pick(s.GetZeroValue())
	case "one":
		text = pick(s.GetOneValue())
	case "two":
		text = pick(s.GetTwoValue())
	case "few":
		text = pick(s.GetFewValue())
	case "many":
		text = pick(s.GetManyValue())
	}
	if text == "" {
		text = s.GetOtherValue()
	}
	return text
}

func fillCount(format string, n int32) string {
	num := strconv.FormatInt(int64(n), 10)
	out := format
	replaced := false
	for _, token := range []string{"%1$@", "%@", "%1$d", "%d"} {
		if strings.Contains(out, token) {
			out = strings.ReplaceAll(out, token, num)
			replaced = true
		}
	}
	if !replaced {
		return strings.TrimSpace(format)
	}
	return strings.TrimSpace(out)
}

func pluralCategory(code string, n int32) string {
	if n < 0 {
		n = -n
	}
	switch code {
	case "zh", "ja", "ko", "vi", "th", "id", "ms", "lo", "km":
		return "other"
	case "fr":
		if n < 2 {
			return "one"
		}
		return "other"
	case "ru", "uk", "be", "sr", "hr", "bs":
		mod10 := n % 10
		mod100 := n % 100
		if mod10 == 1 && mod100 != 11 {
			return "one"
		}
		if mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14) {
			return "few"
		}
		return "many"
	case "pl":
		if n == 1 {
			return "one"
		}
		mod10 := n % 10
		mod100 := n % 100
		if mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14) {
			return "few"
		}
		return "many"
	case "ar":
		switch {
		case n == 0:
			return "zero"
		case n == 1:
			return "one"
		case n == 2:
			return "two"
		case n%100 >= 3 && n%100 <= 10:
			return "few"
		case n%100 >= 11:
			return "many"
		default:
			return "other"
		}
	default:
		if n == 1 {
			return "one"
		}
		return "other"
	}
}

func clamp(s string, max int) string {
	s = strings.TrimSpace(strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\t':
			return ' '
		}
		if r < 32 {
			return -1
		}
		return r
	}, s))
	if max > 0 && utf8.RuneCountInString(s) > max {
		s = string([]rune(s)[:max])
	}
	return strings.TrimSpace(s)
}
