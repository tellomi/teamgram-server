package chatbox

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// qrKey is a document key minted inside this test. It is not a phone number
// and it is not inserted into users.
func qrKey(slot int) int64 {
	return 8_801_000_000_000 + int64(os.Getpid()%1_000_000)*100 + int64(slot)
}

func fixedNow() time.Time { return time.Unix(1_700_000_100, 0) }

type flakyDocs struct {
	inner    *memDocs
	fail     bool
	attempts int
	commits  int
}

func (f *flakyDocs) Load(ctx context.Context, key string) ([]byte, bool, error) {
	return f.inner.Load(ctx, key)
}

func (f *flakyDocs) Save(ctx context.Context, key string, raw []byte) error {
	f.attempts++
	if f.fail {
		return errors.New("injected quick-reply save failure")
	}
	f.commits++
	return f.inner.Save(ctx, key, raw)
}

func captureOwned(t *testing.T, b *Book, owner, from int64) (sid, first, second int32) {
	t.Helper()
	ctx := context.Background()
	self := mtproto.MakePeerUtil(mtproto.PEER_SELF, owner)
	shortcut := mtproto.MakeTLInputQuickReplyShortcut(&mtproto.InputQuickReplyShortcut{
		Shortcut: "owned",
	}).To_InputQuickReplyShortcut()
	ups, err := b.CaptureQuickReply(ctx, owner, self, shortcut, []*mtproto.Message{
		{
			Message: "first",
			FromId:  mtproto.MakePeerUser(from),
			Media:   mtproto.MakeTLMessageMediaGeo(nil).To_MessageMedia(),
		},
		{Message: "second"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range ups.GetUpdates() {
		msg := u.GetMessage_MESSAGE()
		switch msg.GetMessage() {
		case "first":
			first = msg.GetId()
		case "second":
			second = msg.GetId()
		}
	}
	if first != 1 || second == 0 {
		t.Fatalf("ids first=%d second=%d", first, second)
	}
	page, err := b.QuickReplies(ctx, owner, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.GetQuickReplies()) != 1 {
		t.Fatalf("shortcuts %+v", page)
	}
	return page.GetQuickReplies()[0].GetShortcutId(), first, second
}

func storedText(t *testing.T, b *Book, owner int64, sid, id int32) *mtproto.Message {
	t.Helper()
	msgs, err := b.QuickReplyMessages(context.Background(), owner, sid, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range msgs.GetMessages() {
		if m.GetId() == id {
			return m
		}
	}
	t.Fatalf("message %d missing in %+v", id, msgs.GetMessages())
	return nil
}

func TestEditQuickReplyOwnership(t *testing.T) {
	if os.Getenv("TEAMGRAM_MYSQL_DSN") != "" {
		t.Fatal("TEAMGRAM_MYSQL_DSN is set; refusing to touch a configured database")
	}
	ctx := context.Background()
	owner := qrKey(1)
	from := qrKey(2)
	other := qrKey(3)
	stores := [3]*memDocs{newMemDocs(), newMemDocs(), newMemDocs()}
	books := [3]*Book{newBook(stores[0]), newBook(stores[1]), newBook(stores[2])}
	for _, b := range books {
		b.now = fixedNow
	}
	sid, first, second := captureOwned(t, books[0], owner, from)

	// The same key on the other stores is a different document.
	for i := 1; i < len(books); i++ {
		page, err := books[i].QuickReplies(ctx, owner, 0)
		if err != nil || len(page.GetQuickReplies()) != 0 {
			t.Fatalf("store %d leaked %+v %v", i, page, err)
		}
	}

	text := "rewritten"
	if _, err := books[0].EditQuickReplyMessage(ctx, other, sid, first, &text, nil); err != mtproto.ErrShortcutInvalid {
		t.Fatalf("other account: %v", err)
	}
	got := storedText(t, books[0], owner, sid, first)
	if got.GetMessage() != "first" || got.GetFromId().GetUserId() != from {
		t.Fatalf("other account wrote %+v", got)
	}
	if storedText(t, books[0], owner, sid, second).GetMessage() != "second" {
		t.Fatal("sibling changed")
	}

	edited, err := books[0].EditQuickReplyMessage(ctx, owner, sid, first, &text, nil)
	if err != nil {
		t.Fatal(err)
	}
	msg := edited.GetUpdates()[0].GetMessage_MESSAGE()
	if edited.GetUpdates()[0].GetPredicateName() != mtproto.Predicate_updateQuickReplyMessage || msg.GetMessage() != "rewritten" {
		t.Fatalf("update %+v", edited.GetUpdates()[0])
	}
	if msg.GetFromId().GetUserId() != from || msg.GetEditDate().GetValue() != int32(fixedNow().Unix()) {
		t.Fatalf("owner edit lost from_id or date %+v", msg)
	}
	reloaded := newBook(stores[0])
	again := storedText(t, reloaded, owner, sid, first)
	if again.GetMessage() != "rewritten" || again.GetFromId().GetUserId() != from {
		t.Fatalf("reload %+v", again)
	}
	if storedText(t, reloaded, owner, sid, second).GetMessage() != "second" {
		t.Fatal("reload sibling")
	}
	for i := 1; i < len(books); i++ {
		page, err := books[i].QuickReplies(ctx, owner, 0)
		if err != nil || len(page.GetQuickReplies()) != 0 {
			t.Fatalf("store %d saw the edit %+v %v", i, page, err)
		}
	}
}

func TestEditQuickReplyCorrectness(t *testing.T) {
	if os.Getenv("TEAMGRAM_MYSQL_DSN") != "" {
		t.Fatal("TEAMGRAM_MYSQL_DSN is set; refusing to touch a configured database")
	}
	ctx := context.Background()
	owner := qrKey(11)
	docs := &flakyDocs{inner: newMemDocs()}
	b := newBook(docs)
	b.now = fixedNow
	sid, first, second := captureOwned(t, b, owner, qrKey(12))
	afterCapture := docs.commits

	blank := ""
	if _, err := b.EditQuickReplyMessage(ctx, owner, sid, second, &blank, nil); err != mtproto.ErrMessageEmpty {
		t.Fatalf("empty text: %v", err)
	}
	if storedText(t, b, owner, sid, second).GetMessage() != "second" || docs.commits != afterCapture {
		t.Fatal("empty text was stored")
	}
	emptyMedia := mtproto.MakeTLMessageMediaEmpty(nil).To_MessageMedia()
	if _, err := b.CaptureQuickReply(ctx, owner, mtproto.MakePeerUtil(mtproto.PEER_SELF, owner), mtproto.MakeTLInputQuickReplyShortcutId(&mtproto.InputQuickReplyShortcut{ShortcutId: sid}).To_InputQuickReplyShortcut(), []*mtproto.Message{{Message: "holder", Media: emptyMedia}}); err != nil {
		t.Fatal(err)
	}
	var holderID int32
	var holder *mtproto.Message
	msgs, err := b.QuickReplyMessages(ctx, owner, sid, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range msgs.GetMessages() {
		if m.GetMessage() == "holder" {
			holderID = m.GetId()
			holder = m
		}
	}
	if holderID == 0 || holder.GetMedia().GetPredicateName() != mtproto.Predicate_messageMediaEmpty {
		t.Fatalf("holder %+v", holder)
	}
	if _, err := b.EditQuickReplyMessage(ctx, owner, sid, holderID, &blank, nil); err != mtproto.ErrMessageEmpty {
		t.Fatalf("empty media: %v", err)
	}

	text := "rewritten"
	if _, err := b.EditQuickReplyMessage(ctx, owner, 0, first, &text, nil); err != mtproto.ErrMessageIdInvalid {
		t.Fatalf("zero shortcut: %v", err)
	}
	if _, err := b.EditQuickReplyMessage(ctx, owner, sid, 0, &text, nil); err != mtproto.ErrMessageIdInvalid {
		t.Fatalf("zero message: %v", err)
	}
	if _, err := b.EditQuickReplyMessage(ctx, owner, sid+99, first, &text, nil); err != mtproto.ErrShortcutInvalid {
		t.Fatalf("unknown shortcut: %v", err)
	}
	if _, err := b.EditQuickReplyMessage(ctx, owner, sid, 99, &text, nil); err != mtproto.ErrMessageIdInvalid {
		t.Fatalf("unknown message: %v", err)
	}
	if storedText(t, b, owner, sid, first).GetMessage() != "first" {
		t.Fatal("rejected edit changed the message")
	}

	entity := mtproto.MakeTLMessageEntityBold(&mtproto.MessageEntity{Offset: 0, Length: 9}).To_MessageEntity()
	edited, err := b.EditQuickReplyMessage(ctx, owner, sid, first, &text, []*mtproto.MessageEntity{entity})
	if err != nil {
		t.Fatal(err)
	}
	if edited.GetUpdates()[0].GetMessage_MESSAGE().GetEntities()[0].GetLength() != 9 {
		t.Fatalf("entities %+v", edited)
	}
	commits := docs.commits
	date := storedText(t, b, owner, sid, first).GetEditDate().GetValue()
	if _, err := b.EditQuickReplyMessage(ctx, owner, sid, first, &text, []*mtproto.MessageEntity{entity}); err != mtproto.ErrMessageNotModified {
		t.Fatalf("duplicate: %v", err)
	}
	if docs.commits != commits || docs.attempts != commits {
		t.Fatalf("duplicate wrote attempts=%d commits=%d", docs.attempts, docs.commits)
	}
	if storedText(t, b, owner, sid, first).GetEditDate().GetValue() != date {
		t.Fatal("duplicate bumped edit_date")
	}
	if _, err := b.EditQuickReplyMessage(ctx, owner, sid, first, &blank, nil); err != nil {
		t.Fatal(err)
	}
	cleared := storedText(t, b, owner, sid, first)
	if cleared.GetMessage() != "" || cleared.GetMedia().GetPredicateName() != mtproto.Predicate_messageMediaGeo {
		t.Fatalf("caption clear %+v", cleared)
	}
	if storedText(t, b, owner, sid, second).GetMessage() != "second" {
		t.Fatal("sibling changed")
	}
}

func TestEditQuickReplySaveFailureThenRetry(t *testing.T) {
	if os.Getenv("TEAMGRAM_MYSQL_DSN") != "" {
		t.Fatal("TEAMGRAM_MYSQL_DSN is set; refusing to touch a configured database")
	}
	ctx := context.Background()
	owner := qrKey(21)
	docs := &flakyDocs{inner: newMemDocs()}
	b := newBook(docs)
	b.now = fixedNow
	sid, first, second := captureOwned(t, b, owner, qrKey(22))
	docs.fail = true
	text := "after-failure"
	_, err := b.EditQuickReplyMessage(ctx, owner, sid, first, &text, nil)
	if err == nil || err.Error() != "injected quick-reply save failure" {
		t.Fatalf("failure: %v", err)
	}
	if storedText(t, b, owner, sid, first).GetMessage() != "first" || storedText(t, b, owner, sid, first).GetEditDate() != nil {
		t.Fatal("failed save left the new text in memory")
	}
	if storedText(t, newBook(docs.inner), owner, sid, first).GetMessage() != "first" {
		t.Fatal("failed save committed")
	}
	if storedText(t, b, owner, sid, second).GetMessage() != "second" {
		t.Fatal("sibling changed")
	}
	docs.fail = false
	if _, err := b.EditQuickReplyMessage(ctx, owner, sid, first, &text, nil); err != nil {
		t.Fatal(err)
	}
	if storedText(t, newBook(docs.inner), owner, sid, first).GetMessage() != "after-failure" {
		t.Fatal("retry did not commit")
	}
	if _, err := b.EditQuickReplyMessage(ctx, owner, sid, first, &text, nil); err != mtproto.ErrMessageNotModified {
		t.Fatalf("retry duplicate: %v", err)
	}
}

func TestEditQuickReplyConcurrentSubmit(t *testing.T) {
	if os.Getenv("TEAMGRAM_MYSQL_DSN") != "" {
		t.Fatal("TEAMGRAM_MYSQL_DSN is set; refusing to touch a configured database")
	}
	ctx := context.Background()
	owner := qrKey(31)
	docs := newMemDocs()
	b := newBook(docs)
	b.now = fixedNow
	sid, first, second := captureOwned(t, b, owner, qrKey(32))
	const n = 8
	text := "same-submit"
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			_, err := b.EditQuickReplyMessage(ctx, owner, sid, first, &text, nil)
			errs <- err
		}()
	}
	ok, same := 0, 0
	for i := 0; i < n; i++ {
		err := <-errs
		switch err {
		case nil:
			ok++
		case mtproto.ErrMessageNotModified:
			same++
		default:
			t.Fatalf("concurrent: %v", err)
		}
	}
	if ok != 1 || same != n-1 {
		t.Fatalf("successes %d duplicates %d", ok, same)
	}
	if storedText(t, newBook(docs), owner, sid, first).GetMessage() != text {
		t.Fatal("concurrent duplicate did not keep one write")
	}
	if storedText(t, b, owner, sid, second).GetMessage() != "second" {
		t.Fatal("sibling changed")
	}

	alts := []string{"alpha", "beta", "gamma", "delta"}
	altErrs := make(chan error, len(alts))
	for _, alt := range alts {
		alt := alt
		go func() {
			_, err := b.EditQuickReplyMessage(ctx, owner, sid, first, &alt, nil)
			altErrs <- err
		}()
	}
	for range alts {
		if err := <-altErrs; err != nil {
			t.Fatal(err)
		}
	}
	final := storedText(t, newBook(docs), owner, sid, first).GetMessage()
	seen := false
	for _, alt := range alts {
		if final == alt {
			seen = true
		}
	}
	if !seen || storedText(t, b, owner, sid, second).GetMessage() != "second" {
		t.Fatalf("last write %q", final)
	}
}

func TestEditQuickReplyHashFollowsEditDate(t *testing.T) {
	if os.Getenv("TEAMGRAM_MYSQL_DSN") != "" {
		t.Fatal("TEAMGRAM_MYSQL_DSN is set; refusing to touch a configured database")
	}
	ctx := context.Background()
	owner := qrKey(41)
	b := newBook(newMemDocs())
	b.now = fixedNow
	sid, first, second := captureOwned(t, b, owner, qrKey(42))
	page, err := b.QuickReplyMessages(ctx, owner, sid, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	old := hashQuickMessages(page.GetMessages())
	cached, err := b.QuickReplyMessages(ctx, owner, sid, nil, old)
	if err != nil || cached.GetPredicateName() != mtproto.Predicate_messages_messagesNotModified {
		t.Fatalf("fresh hash %+v %v", cached, err)
	}
	text := "rewritten"
	if _, err := b.EditQuickReplyMessage(ctx, owner, sid, first, &text, nil); err != nil {
		t.Fatal(err)
	}
	stale, err := b.QuickReplyMessages(ctx, owner, sid, nil, old)
	if err != nil || stale.GetPredicateName() == mtproto.Predicate_messages_messagesNotModified {
		t.Fatal("edit left the previous message hash valid")
	}
	var saw bool
	for _, m := range stale.GetMessages() {
		if m.GetId() == first && m.GetMessage() == "rewritten" && m.GetEditDate().GetValue() != 0 {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("stale fetch %+v", stale.GetMessages())
	}
	fresh := hashQuickMessages(stale.GetMessages())
	if fresh == old {
		t.Fatal("edit did not change the hash")
	}
	hit, err := b.QuickReplyMessages(ctx, owner, sid, nil, fresh)
	if err != nil || hit.GetPredicateName() != mtproto.Predicate_messages_messagesNotModified {
		t.Fatalf("new hash %+v %v", hit, err)
	}
	date := storedText(t, b, owner, sid, first).GetEditDate().GetValue()
	if _, err := b.EditQuickReplyMessage(ctx, owner, sid, first, &text, nil); err != mtproto.ErrMessageNotModified {
		t.Fatal(err)
	}
	if storedText(t, b, owner, sid, first).GetEditDate().GetValue() != date {
		t.Fatal("duplicate changed edit_date")
	}
	if still, err := b.QuickReplyMessages(ctx, owner, sid, nil, fresh); err != nil || still.GetPredicateName() != mtproto.Predicate_messages_messagesNotModified {
		t.Fatalf("duplicate hash %+v %v", still, err)
	}

	list, err := b.QuickReplies(ctx, owner, 0)
	if err != nil {
		t.Fatal(err)
	}
	listHash := hashQuick(list.GetQuickReplies(), list.GetMessages())
	if again, err := b.QuickReplies(ctx, owner, listHash); err != nil || again.GetPredicateName() != mtproto.Predicate_messages_quickRepliesNotModified {
		t.Fatalf("list hash %+v %v", again, err)
	}
	top := "top-edited"
	if _, err := b.EditQuickReplyMessage(ctx, owner, sid, second, &top, nil); err != nil {
		t.Fatal(err)
	}
	changed, err := b.QuickReplies(ctx, owner, listHash)
	if err != nil || changed.GetPredicateName() != mtproto.Predicate_messages_quickReplies {
		t.Fatalf("top edit %+v %v", changed, err)
	}
	if changed.GetMessages()[0].GetMessage() != "top-edited" {
		t.Fatalf("top preview %+v", changed.GetMessages())
	}
}

func TestEditQuickReplyReplacesMediaAndClearsCaption(t *testing.T) {
	if os.Getenv("TEAMGRAM_MYSQL_DSN") != "" {
		t.Fatal("TEAMGRAM_MYSQL_DSN is set; refusing to touch a configured database")
	}
	ctx := context.Background()
	owner := qrKey(51)
	b := newBook(newMemDocs())
	b.now = fixedNow
	sid, first, second := captureOwned(t, b, owner, qrKey(52))
	markup := mtproto.MakeTLReplyKeyboardMarkup(&mtproto.ReplyMarkup{
		Rows_VECTORKEYBOARDBUTTONROW: []*mtproto.KeyboardButtonRow{
			mtproto.MakeTLKeyboardButtonRow(&mtproto.KeyboardButtonRow{
				Buttons: []*mtproto.KeyboardButton{
					mtproto.MakeTLKeyboardButton(&mtproto.KeyboardButton{Text: "go"}).To_KeyboardButton(),
				},
			}).To_KeyboardButtonRow(),
		},
	}).To_ReplyMarkup()
	text := "moved"
	media := mtproto.MakeTLMessageMediaGeo(&mtproto.MessageMedia{
		Geo: mtproto.MakeTLGeoPoint(&mtproto.GeoPoint{Lat: 9, Long: 8}).To_GeoPoint(),
	}).To_MessageMedia()
	edited, err := b.EditQuickReply(ctx, owner, sid, first, QuickReplyEdit{Text: &text, Markup: markup, Media: media})
	if err != nil {
		t.Fatal(err)
	}
	msg := edited.GetUpdates()[0].GetMessage_MESSAGE()
	if msg.GetMessage() != "moved" || msg.GetMedia().GetGeo().GetLong() != 8 || msg.GetReplyMarkup().GetRows_VECTORKEYBOARDBUTTONROW()[0].GetButtons()[0].GetText() != "go" {
		t.Fatalf("replace %+v", msg)
	}
	blank := ""
	if _, err := b.EditQuickReply(ctx, owner, sid, first, QuickReplyEdit{Text: &blank}); err != nil {
		t.Fatal(err)
	}
	cleared := storedText(t, b, owner, sid, first)
	if cleared.GetMessage() != "" || cleared.GetMedia().GetGeo().GetLong() != 8 || cleared.GetReplyMarkup() == nil {
		t.Fatalf("caption clear %+v", cleared)
	}
	empty := mtproto.MakeTLMessageMediaEmpty(nil).To_MessageMedia()
	if _, err := b.EditQuickReply(ctx, owner, sid, first, QuickReplyEdit{Text: &blank, Media: empty}); err != mtproto.ErrMessageEmpty {
		t.Fatalf("empty media: %v", err)
	}
	if kept := storedText(t, b, owner, sid, first); kept.GetMedia().GetGeo().GetLong() != 8 || storedText(t, b, owner, sid, second).GetMessage() != "second" {
		t.Fatalf("empty media wrote %+v", kept)
	}
}

func TestChatboxDocSaveDropsAStaleSnapshot(t *testing.T) {
	if os.Getenv("TEAMGRAM_MYSQL_DSN") != "" {
		t.Fatal("TEAMGRAM_MYSQL_DSN is set; refusing to touch a configured database")
	}
	ctx := context.Background()
	owner := qrKey(61)
	docs := newMemDocs()
	b := newBook(docs)
	b.now = fixedNow
	sid, first, _ := captureOwned(t, b, owner, qrKey(62))
	prev, ok, err := docs.Load(ctx, quickKey(owner))
	if err != nil || !ok {
		t.Fatal(err)
	}
	stale := append([]byte(nil), prev...)
	text := "kept-by-edit"
	if _, err := b.EditQuickReplyMessage(ctx, owner, sid, first, &text, nil); err != nil {
		t.Fatal(err)
	}
	// Save has no version. Putting the pre-edit bytes back drops the edit.
	// This is the store limitation. The edit path below does not use this call.
	if err := docs.Save(ctx, quickKey(owner), stale); err != nil {
		t.Fatal(err)
	}
	if got := storedText(t, newBook(docs), owner, sid, first).GetMessage(); got != "first" {
		t.Fatalf("stale Save kept %q; the unconditional write no longer drops a snapshot", got)
	}
}

type freezeDocs struct {
	inner  *memDocs
	mu     sync.Mutex
	armed  bool
	loads  int
	freeze []byte
}

func (f *freezeDocs) Load(ctx context.Context, key string) ([]byte, bool, error) {
	f.mu.Lock()
	f.loads++
	n := f.loads
	snap := append([]byte(nil), f.freeze...)
	armed := f.armed
	f.mu.Unlock()
	if armed && n <= 2 {
		return snap, true, nil
	}
	return f.inner.Load(ctx, key)
}

func (f *freezeDocs) Save(ctx context.Context, key string, raw []byte) error {
	return f.inner.Save(ctx, key, raw)
}

func (f *freezeDocs) SaveIf(ctx context.Context, key string, prev, next []byte) (bool, error) {
	return f.inner.SaveIf(ctx, key, prev, next)
}

func TestEditQuickReplyCrossBookKeepsBothEdits(t *testing.T) {
	if os.Getenv("TEAMGRAM_MYSQL_DSN") != "" {
		t.Fatal("TEAMGRAM_MYSQL_DSN is set; refusing to touch a configured database")
	}
	ctx := context.Background()
	owner := qrKey(71)
	store := &freezeDocs{inner: newMemDocs()}
	a := newBook(store)
	a.now = fixedNow
	sid, first, second := captureOwned(t, a, owner, qrKey(72))
	snap, ok, err := store.inner.Load(ctx, quickKey(owner))
	if err != nil || !ok {
		t.Fatal(err)
	}
	store.mu.Lock()
	store.freeze = append([]byte(nil), snap...)
	store.armed = true
	store.loads = 0
	store.mu.Unlock()
	b := newBook(store)
	b.now = fixedNow
	leftText := "left-edit"
	rightText := "right-edit"
	errs := make(chan error, 2)
	go func() {
		_, err := a.EditQuickReplyMessage(ctx, owner, sid, first, &leftText, nil)
		errs <- err
	}()
	go func() {
		_, err := b.EditQuickReplyMessage(ctx, owner, sid, second, &rightText, nil)
		errs <- err
	}()
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	got := map[int32]string{}
	for _, m := range mustMessages(t, newBook(store.inner), owner, sid) {
		got[m.GetId()] = m.GetMessage()
	}
	if got[first] != leftText || got[second] != rightText {
		t.Fatalf("cross-book edits %#v", got)
	}
}

func mustMessages(t *testing.T, b *Book, owner int64, sid int32) []*mtproto.Message {
	t.Helper()
	page, err := b.QuickReplyMessages(context.Background(), owner, sid, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	return page.GetMessages()
}
