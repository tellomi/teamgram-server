// Package aicompose stores AI composer tones for the current process.
// A restart drops custom tones. Built-in tones are always available.
// Examples are local templates. There is no composer model.
package aicompose

import (
	"encoding/binary"
	"hash/fnv"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	maxTitle  = 64
	maxPrompt = 1024
)

type builtin struct {
	key, title string
	samples    [3][2]string
}

var builtins = []builtin{
	{
		key: "friendly", title: "Friendly",
		samples: [3][2]string{
			{"Can we move the meeting to Friday?", "Hey, could we shift the meeting to Friday?"},
			{"Please send the file.", "Could you send the file when you have a moment?"},
			{"I do not agree with this.", "I see it differently, and I would love to talk it through."},
		},
	},
	{
		key: "professional", title: "Professional",
		samples: [3][2]string{
			{"Can we move the meeting to Friday?", "Could we reschedule the meeting to Friday?"},
			{"Please send the file.", "Please send the file at your earliest convenience."},
			{"I do not agree with this.", "I disagree with this proposal and would like to discuss it."},
		},
	},
	{
		key: "concise", title: "Concise",
		samples: [3][2]string{
			{"Can we move the meeting to Friday?", "Move the meeting to Friday?"},
			{"Please send the file.", "Please send the file."},
			{"I do not agree with this.", "I disagree."},
		},
	},
	{
		key: "witty", title: "Witty",
		samples: [3][2]string{
			{"Can we move the meeting to Friday?", "Friday has a better calendar outfit. Can we meet then?"},
			{"Please send the file.", "The file and I should really meet."},
			{"I do not agree with this.", "My disagreement has entered the chat."},
		},
	},
}

type custom struct {
	id       int64
	owner    int64
	access   int64
	slug     string
	title    string
	prompt   string
	emoji    int64
	showAuth bool
	installs int32
}

type savedRef struct {
	custom bool
	id     int64
	name   string
}

type box struct {
	ready bool
	saved []savedRef
}

var (
	mu      sync.Mutex
	nextID  atomic.Int64
	customs = map[int64]*custom{}
	bySlug  = map[string]int64{}
	users   = map[int64]*box{}
)

// Create stores a custom tone and saves it for the caller.
func Create(self int64, in *mtproto.TLAicomposeCreateTone) (*mtproto.AiComposeTone, error) {
	if self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrAiComposeToneInvalid
	}
	title, prompt, err := clean(in.GetTitle(), in.GetPrompt())
	if err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	id := nextID.Add(1)
	row := &custom{
		id: id, owner: self, access: accessOf(self, id),
		slug: slugify(title, id), title: title, prompt: prompt,
		emoji: in.GetEmojiId(), showAuth: in.GetDisplayAuthor(), installs: 1,
	}
	customs[id] = row
	bySlug[row.slug] = id
	b := ensure(self)
	b.saved = append([]savedRef{{custom: true, id: id}}, b.saved...)
	return presentCustom(self, row), nil
}

// Update edits a custom tone the caller created.
func Update(self int64, in *mtproto.TLAicomposeUpdateTone) (*mtproto.AiComposeTone, error) {
	if self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrAiComposeToneInvalid
	}
	mu.Lock()
	defer mu.Unlock()
	row, err := owned(self, in.GetTone())
	if err != nil {
		return nil, err
	}
	if in.GetDisplayAuthor() != nil {
		row.showAuth = mtproto.FromBool(in.GetDisplayAuthor())
	}
	if in.GetEmojiId() != nil {
		row.emoji = in.GetEmojiId().GetValue()
	}
	title, prompt := row.title, row.prompt
	if in.GetTitle() != nil {
		title = in.GetTitle().GetValue()
	}
	if in.GetPrompt() != nil {
		prompt = in.GetPrompt().GetValue()
	}
	if in.GetTitle() != nil || in.GetPrompt() != nil {
		title, prompt, err = clean(title, prompt)
		if err != nil {
			return nil, err
		}
		row.title, row.prompt = title, prompt
	}
	return presentCustom(self, row), nil
}

// Save installs or removes a tone from the caller's saved list.
func Save(self int64, in *mtproto.TLAicomposeSaveTone) (*mtproto.Bool, error) {
	if self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetTone() == nil {
		return nil, mtproto.ErrAiComposeToneInvalid
	}
	mu.Lock()
	defer mu.Unlock()
	ref, row, err := resolve(in.GetTone(), false)
	if err != nil {
		return nil, err
	}
	b := ensure(self)
	if mtproto.FromBool(in.GetUnsave()) {
		if has(b.saved, ref) {
			b.saved = drop(b.saved, ref)
			if row != nil && row.installs > 0 {
				row.installs--
			}
		}
		return mtproto.BoolTrue, nil
	}
	if !has(b.saved, ref) {
		b.saved = append([]savedRef{ref}, b.saved...)
		if row != nil {
			row.installs++
		}
	}
	return mtproto.BoolTrue, nil
}

// Delete removes a custom tone the caller created.
func Delete(self int64, in *mtproto.TLAicomposeDeleteTone) (*mtproto.Bool, error) {
	if self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrAiComposeToneInvalid
	}
	mu.Lock()
	defer mu.Unlock()
	row, err := owned(self, in.GetTone())
	if err != nil {
		return nil, err
	}
	delete(customs, row.id)
	delete(bySlug, row.slug)
	gone := savedRef{custom: true, id: row.id}
	for _, b := range users {
		b.saved = drop(b.saved, gone)
	}
	return mtproto.BoolTrue, nil
}

// Get returns one tone.
func Get(self int64, in *mtproto.TLAicomposeGetTone) (*mtproto.Aicompose_Tones, error) {
	if self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetTone() == nil {
		return nil, mtproto.ErrAiComposeToneInvalid
	}
	mu.Lock()
	defer mu.Unlock()
	ref, row, err := resolve(in.GetTone(), false)
	if err != nil {
		return nil, err
	}
	tone := present(self, ref, row)
	return tonesOf(toneHash(ref), []*mtproto.AiComposeTone{tone}, authors(tone)), nil
}

// List returns the caller's saved tones. A matching hash returns tonesNotModified.
func List(self, hash int64) (*mtproto.Aicompose_Tones, error) {
	if self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	mu.Lock()
	defer mu.Unlock()
	b := ensure(self)
	sum := listHash(b.saved)
	if hash != 0 && hash == sum {
		return mtproto.MakeTLAicomposeTonesNotModified(&mtproto.Aicompose_Tones{}).To_Aicompose_Tones(), nil
	}
	out := make([]*mtproto.AiComposeTone, 0, len(b.saved))
	var people []*mtproto.User
	for _, item := range b.saved {
		tone := present(self, item, customs[item.id])
		out = append(out, tone)
		people = append(people, authors(tone)...)
	}
	return tonesOf(sum, out, people), nil
}

// Example returns a template rewrite for the tone picker.
func Example(self int64, in *mtproto.TLAicomposeGetToneExample) (*mtproto.AiComposeToneExample, error) {
	if self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetTone() == nil || in.GetNum() < 0 {
		return nil, mtproto.ErrAiComposeToneInvalid
	}
	mu.Lock()
	defer mu.Unlock()
	from, to, err := sample(in.GetTone(), int(in.GetNum()))
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLAiComposeToneExample(&mtproto.AiComposeToneExample{
		From: text(from),
		To:   text(to),
	}).To_AiComposeToneExample(), nil
}

func clean(title, prompt string) (string, string, error) {
	title = strings.TrimSpace(title)
	prompt = strings.TrimSpace(prompt)
	if title == "" || utf8.RuneCountInString(title) > maxTitle {
		return "", "", mtproto.ErrAiComposeToneTitleInvalid
	}
	if prompt == "" || utf8.RuneCountInString(prompt) > maxPrompt {
		return "", "", mtproto.ErrAiComposeToneInvalid
	}
	return title, prompt, nil
}

func ensure(self int64) *box {
	b := users[self]
	if b == nil {
		b = &box{}
		users[self] = b
	}
	if !b.ready {
		for _, item := range builtins {
			b.saved = append(b.saved, savedRef{name: item.key})
		}
		b.ready = true
	}
	return b
}

func owned(self int64, tone *mtproto.InputAiComposeTone) (*custom, error) {
	if tone == nil || tone.GetPredicateName() != mtproto.Predicate_inputAiComposeToneID {
		return nil, mtproto.ErrAiComposeToneInvalid
	}
	row := customs[tone.GetId()]
	if row == nil || row.access != tone.GetAccessHash() || row.owner != self {
		return nil, mtproto.ErrAiComposeToneInvalid
	}
	return row, nil
}

func resolve(tone *mtproto.InputAiComposeTone, allowSingle bool) (savedRef, *custom, error) {
	if tone == nil {
		return savedRef{}, nil, mtproto.ErrAiComposeToneInvalid
	}
	switch tone.GetPredicateName() {
	case mtproto.Predicate_inputAiComposeToneDefault:
		if builtinBy(tone.GetTone()) == nil {
			return savedRef{}, nil, mtproto.ErrAiComposeToneInvalid
		}
		return savedRef{name: tone.GetTone()}, nil, nil
	case mtproto.Predicate_inputAiComposeToneID:
		row := customs[tone.GetId()]
		if row == nil || row.access != tone.GetAccessHash() {
			return savedRef{}, nil, mtproto.ErrAiComposeToneInvalid
		}
		return savedRef{custom: true, id: row.id}, row, nil
	case mtproto.Predicate_inputAiComposeToneSlug:
		id, ok := bySlug[tone.GetSlug()]
		if !ok {
			return savedRef{}, nil, mtproto.ErrAiComposeToneInvalid
		}
		return savedRef{custom: true, id: id}, customs[id], nil
	case mtproto.Predicate_inputAiComposeToneSingleUse:
		if !allowSingle || strings.TrimSpace(tone.GetCustomPrompt()) == "" {
			return savedRef{}, nil, mtproto.ErrAiComposeToneInvalid
		}
		return savedRef{name: "single"}, nil, nil
	default:
		return savedRef{}, nil, mtproto.ErrAiComposeToneInvalid
	}
}

func sample(tone *mtproto.InputAiComposeTone, num int) (string, string, error) {
	num = num % 3
	switch tone.GetPredicateName() {
	case mtproto.Predicate_inputAiComposeToneDefault:
		item := builtinBy(tone.GetTone())
		if item == nil {
			return "", "", mtproto.ErrAiComposeToneInvalid
		}
		return item.samples[num][0], item.samples[num][1], nil
	case mtproto.Predicate_inputAiComposeToneSingleUse:
		prompt := strings.TrimSpace(tone.GetCustomPrompt())
		if prompt == "" || utf8.RuneCountInString(prompt) > maxPrompt {
			return "", "", mtproto.ErrAiComposeToneInvalid
		}
		from := builtins[0].samples[num][0]
		return from, prompt + " " + from, nil
	default:
		_, row, err := resolve(tone, false)
		if err != nil {
			return "", "", err
		}
		from := builtins[0].samples[num][0]
		if row == nil {
			return from, from, nil
		}
		return from, row.title + ": " + from, nil
	}
}

func present(self int64, item savedRef, row *custom) *mtproto.AiComposeTone {
	if !item.custom {
		itemBuilt := builtinBy(item.name)
		title := item.name
		if itemBuilt != nil {
			title = itemBuilt.title
		}
		return mtproto.MakeTLAiComposeToneDefault(&mtproto.AiComposeTone{
			Tone:          item.name,
			Title:         title,
			EmojiId_INT64: 0,
		}).To_AiComposeTone()
	}
	if row == nil {
		row = customs[item.id]
	}
	return presentCustom(self, row)
}

func presentCustom(self int64, row *custom) *mtproto.AiComposeTone {
	out := &mtproto.AiComposeTone{
		Creator:    row.owner == self,
		Id:         row.id,
		AccessHash: row.access,
		Slug:       row.slug,
		Title:      row.title,
	}
	if row.emoji != 0 {
		out.EmojiId_FLAGINT64 = wrapperspb.Int64(row.emoji)
	}
	if row.owner == self && row.prompt != "" {
		out.Prompt = wrapperspb.String(row.prompt)
	}
	if row.installs > 0 {
		out.InstallsCount = wrapperspb.Int32(row.installs)
	}
	if row.showAuth {
		out.AuthorId = wrapperspb.Int64(row.owner)
	}
	return mtproto.MakeTLAiComposeTone(out).To_AiComposeTone()
}

func authors(tone *mtproto.AiComposeTone) []*mtproto.User {
	if tone.GetAuthorId() == nil || tone.GetAuthorId().GetValue() == 0 {
		return nil
	}
	return []*mtproto.User{mtproto.MakeTLUserEmpty(&mtproto.User{Id: tone.GetAuthorId().GetValue()}).To_User()}
}

func tonesOf(hash int64, rows []*mtproto.AiComposeTone, users []*mtproto.User) *mtproto.Aicompose_Tones {
	if rows == nil {
		rows = []*mtproto.AiComposeTone{}
	}
	if users == nil {
		users = []*mtproto.User{}
	}
	return mtproto.MakeTLAicomposeTones(&mtproto.Aicompose_Tones{
		Hash:  hash,
		Tones: rows,
		Users: users,
	}).To_Aicompose_Tones()
}

func toneHash(item savedRef) int64 {
	return listHash([]savedRef{item})
}

func listHash(rows []savedRef) int64 {
	h := fnv.New64a()
	for _, item := range rows {
		if item.custom {
			_ = binary.Write(h, binary.LittleEndian, int64(1))
			_ = binary.Write(h, binary.LittleEndian, item.id)
			continue
		}
		_, _ = h.Write([]byte(item.name))
		_, _ = h.Write([]byte{0})
	}
	sum := int64(h.Sum64())
	if sum == 0 {
		return 1
	}
	return sum
}

func builtinBy(name string) *builtin {
	for i := range builtins {
		if builtins[i].key == name {
			return &builtins[i]
		}
	}
	return nil
}

func has(rows []savedRef, item savedRef) bool {
	for _, row := range rows {
		if same(row, item) {
			return true
		}
	}
	return false
}

func drop(rows []savedRef, item savedRef) []savedRef {
	out := rows[:0]
	for _, row := range rows {
		if !same(row, item) {
			out = append(out, row)
		}
	}
	return out
}

func same(a, b savedRef) bool {
	if a.custom || b.custom {
		return a.custom && b.custom && a.id == b.id
	}
	return a.name == b.name
}

func accessOf(owner, id int64) int64 {
	h := owner ^ (id * 0x9E3779B1)
	if h == 0 {
		return 1
	}
	return h
}

func slugify(title string, id int64) string {
	var b strings.Builder
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-':
			if b.Len() > 0 && b.String()[b.Len()-1] != '-' {
				b.WriteByte('-')
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		s = "tone"
	}
	return s + "-" + itoa(id)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func text(s string) *mtproto.TextWithEntities {
	return mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{Text: s}).To_TextWithEntities()
}
