package mtproto

import (
	"strings"

	"google.golang.org/protobuf/proto"
)

// Checklist limits published by official clients as help.getAppConfig
// todo_title_length_max, todo_item_length_max, and todo_items_max.
const (
	TodoTitleLengthMax = 32
	TodoItemLengthMax  = 200
	TodoItemsMax       = 30
)

// MessageMediaFromTodo builds messageMediaToDo from inputMediaTodo.todo.
// Completions stay unset (flag 0 clear) until messages.toggleTodoCompleted.
// Layer 232 constructor: messageMediaToDo#8a53b014.
func MessageMediaFromTodo(list *TodoList) (*MessageMedia, error) {
	norm, err := normalizeTodoList(list)
	if err != nil {
		return nil, err
	}
	media := MakeTLMessageMediaToDo(&MessageMedia{Todo: norm}).To_MessageMedia()
	// Android re-reads these constructor ids from the history bytes. Layer 232
	// is messageMediaToDo#8a53b014. Nested objects are stamped the same way.
	stampTodoMedia(media)
	return media, nil
}

// AppendTodoItems adds items to a stored checklist.
// The returned slice is what messageActionTodoAppendTasks carries.
func AppendTodoItems(media *MessageMedia, rows []*TodoItem) ([]*TodoItem, error) {
	list := todoListOf(media)
	if list == nil {
		return nil, ErrMediaInvalid
	}
	if len(rows) == 0 {
		return nil, ErrTodoNotModified
	}
	if len(list.GetList())+len(rows) > TodoItemsMax {
		return nil, ErrMediaInvalid
	}
	seen, titles := indexTodo(list.GetList())
	var added []*TodoItem
	for _, row := range rows {
		item, err := normalizeTodoItem(row)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[item.GetId()]; ok {
			return nil, ErrTodoItemDuplicate
		}
		if _, ok := titles[item.GetTitle().GetText()]; ok {
			return nil, ErrTodoItemDuplicate
		}
		seen[item.GetId()] = struct{}{}
		titles[item.GetTitle().GetText()] = struct{}{}
		added = append(added, item)
	}
	list.List = append(list.GetList(), added...)
	stampTodoList(list)
	return added, nil
}

// ToggleTodoItems marks the listed item ids completed or not.
// completed wins when an id is in both vectors. Unknown ids are ignored.
// done and undone are the ids whose state actually changed, in list order,
// for messageActionTodoCompletions.
func ToggleTodoItems(media *MessageMedia, actor int64, completed, incompleted []int32, now int32) (done, undone []int32, err error) {
	list := todoListOf(media)
	if list == nil {
		return nil, nil, ErrMediaInvalid
	}
	if len(completed) == 0 && len(incompleted) == 0 {
		return nil, nil, ErrTodoNotModified
	}
	known := map[int32]struct{}{}
	for _, it := range list.GetList() {
		if it != nil {
			known[it.GetId()] = struct{}{}
		}
	}
	before := map[int32]bool{}
	for _, c := range media.GetCompletions() {
		if c != nil {
			before[c.GetId()] = true
		}
	}
	wantOff := map[int32]struct{}{}
	wantOn := map[int32]struct{}{}
	for _, id := range incompleted {
		if _, ok := known[id]; ok {
			wantOff[id] = struct{}{}
		}
	}
	for _, id := range completed {
		if _, ok := known[id]; ok {
			delete(wantOff, id)
			wantOn[id] = struct{}{}
		}
	}
	var next []*TodoCompletion
	for _, c := range media.GetCompletions() {
		if c == nil {
			continue
		}
		if _, off := wantOff[c.GetId()]; off {
			continue
		}
		if _, on := wantOn[c.GetId()]; on {
			delete(wantOn, c.GetId())
		}
		next = append(next, c)
	}
	for _, it := range list.GetList() {
		if it == nil {
			continue
		}
		if _, on := wantOn[it.GetId()]; !on {
			continue
		}
		next = append(next, todoCompletion(it.GetId(), actor, now))
	}
	if len(next) == 0 {
		media.Completions = nil
	} else {
		media.Completions = next
		stampTodoMedia(media)
	}
	after := map[int32]bool{}
	for _, c := range media.GetCompletions() {
		after[c.GetId()] = true
	}
	for _, it := range list.GetList() {
		if it == nil {
			continue
		}
		id := it.GetId()
		if after[id] && !before[id] {
			done = append(done, id)
		}
		if !after[id] && before[id] {
			undone = append(undone, id)
		}
	}
	if len(done) == 0 && len(undone) == 0 {
		return nil, nil, ErrTodoNotModified
	}
	return done, undone, nil
}

// KeepTodoCompletions copies completions whose item ids survive an edit.
// messages.editMessage with a new inputMediaTodo must not clear checkmarks
// for items that keep their id.
func KeepTodoCompletions(prev, next *MessageMedia) *MessageMedia {
	if next == nil || next.GetPredicateName() != Predicate_messageMediaToDo || next.GetTodo() == nil {
		return next
	}
	if prev.GetPredicateName() != Predicate_messageMediaToDo {
		next.Completions = nil
		return next
	}
	keep := map[int32]*TodoCompletion{}
	for _, c := range prev.GetCompletions() {
		if c != nil {
			keep[c.GetId()] = c
		}
	}
	var out []*TodoCompletion
	for _, it := range next.GetTodo().GetList() {
		if it == nil {
			continue
		}
		if c, ok := keep[it.GetId()]; ok {
			out = append(out, proto.Clone(c).(*TodoCompletion))
		}
	}
	if len(out) == 0 {
		next.Completions = nil
	} else {
		next.Completions = out
	}
	return next
}

// TodoActorAllowed reports whether actor may change a checklist owned by author.
// The author always can. Everyone else needs the matching todoList flag
// (others_can_append or others_can_complete).
func TodoActorAllowed(author, actor int64, others bool) bool {
	if actor != 0 && actor == author {
		return true
	}
	return others
}

func todoListOf(media *MessageMedia) *TodoList {
	if media.GetPredicateName() != Predicate_messageMediaToDo {
		return nil
	}
	return media.GetTodo()
}

func normalizeTodoList(list *TodoList) (*TodoList, error) {
	if list == nil {
		return nil, ErrMediaInvalid
	}
	title, err := normalizeTodoText(list.GetTitle(), TodoTitleLengthMax)
	if err != nil {
		return nil, err
	}
	items := list.GetList()
	if len(items) == 0 || len(items) > TodoItemsMax {
		return nil, ErrMediaInvalid
	}
	seen := map[int32]struct{}{}
	titles := map[string]struct{}{}
	norm := make([]*TodoItem, 0, len(items))
	for _, row := range items {
		item, err := normalizeTodoItem(row)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[item.GetId()]; ok {
			return nil, ErrTodoItemDuplicate
		}
		if _, ok := titles[item.GetTitle().GetText()]; ok {
			return nil, ErrTodoItemDuplicate
		}
		seen[item.GetId()] = struct{}{}
		titles[item.GetTitle().GetText()] = struct{}{}
		norm = append(norm, item)
	}
	out := MakeTLTodoList(&TodoList{
		OthersCanAppend:   list.GetOthersCanAppend(),
		OthersCanComplete: list.GetOthersCanComplete(),
		Title:             title,
		List:              norm,
	}).To_TodoList()
	stampTodoList(out)
	return out, nil
}

func normalizeTodoItem(row *TodoItem) (*TodoItem, error) {
	if row == nil || row.GetId() <= 0 {
		return nil, ErrMediaInvalid
	}
	title, err := normalizeTodoText(row.GetTitle(), TodoItemLengthMax)
	if err != nil {
		return nil, err
	}
	item := MakeTLTodoItem(&TodoItem{Id: row.GetId(), Title: title}).To_TodoItem()
	item.Constructor = CRC32_todoItem
	stampText(item.Title)
	return item, nil
}

func normalizeTodoText(in *TextWithEntities, maxLen int) (*TextWithEntities, error) {
	if in == nil {
		return nil, ErrMediaInvalid
	}
	text := in.GetText()
	if strings.TrimSpace(text) == "" || UTF16Len(text) > maxLen {
		return nil, ErrMediaInvalid
	}
	out := MakeTLTextWithEntities(&TextWithEntities{
		Text:     text,
		Entities: in.GetEntities(),
	}).To_TextWithEntities()
	stampText(out)
	return out, nil
}

func indexTodo(items []*TodoItem) (map[int32]struct{}, map[string]struct{}) {
	seen := map[int32]struct{}{}
	titles := map[string]struct{}{}
	for _, it := range items {
		if it == nil {
			continue
		}
		seen[it.GetId()] = struct{}{}
		if it.GetTitle() != nil {
			titles[it.GetTitle().GetText()] = struct{}{}
		}
	}
	return seen, titles
}

func todoCompletion(id int32, actor int64, now int32) *TodoCompletion {
	// Layer 232 completed_by is a Peer (todoCompletion#221bb5e4). The long form
	// todoCompletion#4cc120b7 is only for layers 205–216; Android rejects it.
	c := MakeTLTodoCompletion(&TodoCompletion{
		Id:                id,
		CompletedBy_PEER:  MakePeerUser(actor),
		CompletedBy_INT64: actor,
		Date:              now,
	}).To_TodoCompletion()
	c.Constructor = CRC32_todoCompletion_221bb5e4
	if c.CompletedBy_PEER != nil {
		c.CompletedBy_PEER.Constructor = CRC32_peerUser
	}
	return c
}

// Layer-232 constructors Android accepts when it re-parses a stored message.
// Any other id for these types is drawn as an unsupported message.
const (
	todoCtorMedia      = CRC32_messageMediaToDo        // messageMediaToDo#8a53b014
	todoCtorList       = CRC32_todoList                // todoList#49b92a26
	todoCtorItem       = CRC32_todoItem                // todoItem#cba9a52f
	todoCtorText       = CRC32_textWithEntities        // textWithEntities#751f3146
	todoCtorCompletion = CRC32_todoCompletion_221bb5e4 // todoCompletion#221bb5e4
)

func stampTodoMedia(media *MessageMedia) {
	if media == nil {
		return
	}
	media.Constructor = todoCtorMedia
	stampTodoList(media.Todo)
	for _, c := range media.Completions {
		if c == nil {
			continue
		}
		c.Constructor = todoCtorCompletion
	}
}

func stampTodoList(list *TodoList) {
	if list == nil {
		return
	}
	list.Constructor = todoCtorList
	stampText(list.Title)
	for _, it := range list.List {
		if it == nil {
			continue
		}
		it.Constructor = todoCtorItem
		stampText(it.Title)
	}
}

func stampText(t *TextWithEntities) {
	if t == nil {
		return
	}
	t.Constructor = todoCtorText
}
