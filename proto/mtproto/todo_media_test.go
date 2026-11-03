package mtproto

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/proto"
)

// layer232TodoIDs are the only constructors Android accepts when it re-parses
// a checklist after restart. messageMediaUnsupported#9f84f49e and the older
// todoCompletion#4cc120b7 both render as an unsupported message.
func layer232TodoIDs(withCompletion bool) []uint32 {
	ids := []uint32{
		0x8a53b014, // messageMediaToDo
		0x49b92a26, // todoList
		0xcba9a52f, // todoItem
		0x751f3146, // textWithEntities
	}
	if withCompletion {
		ids = append(ids, 0x221bb5e4) // todoCompletion, flag 0
	}
	return ids
}

func assertLayer232TodoWire(t *testing.T, raw []byte, withCompletion bool) {
	t.Helper()
	for _, id := range layer232TodoIDs(withCompletion) {
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], id)
		if !bytes.Contains(raw, b[:]) {
			t.Fatalf("encoded history is missing constructor 0x%08x", id)
		}
	}
	for _, banned := range []uint32{0x9f84f49e, 0x4cc120b7} {
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], banned)
		if withCompletion && banned == 0x4cc120b7 && bytes.Contains(raw, b[:]) {
			t.Fatal("layer 232 payload used todoCompletion#4cc120b7")
		}
		if banned == 0x9f84f49e && bytes.Contains(raw, b[:]) {
			t.Fatal("layer 232 payload used messageMediaUnsupported")
		}
	}
}

func assertLayer232TodoTree(t *testing.T, media *MessageMedia, withCompletion bool) {
	t.Helper()
	if media.GetPredicateName() != Predicate_messageMediaToDo || uint32(media.GetConstructor()) != 0x8a53b014 {
		t.Fatalf("media %s 0x%x", media.GetPredicateName(), uint32(media.GetConstructor()))
	}
	todo := media.GetTodo()
	if todo.GetPredicateName() != Predicate_todoList || uint32(todo.GetConstructor()) != 0x49b92a26 {
		t.Fatalf("todoList %s 0x%x", todo.GetPredicateName(), uint32(todo.GetConstructor()))
	}
	if uint32(todo.GetTitle().GetConstructor()) != 0x751f3146 {
		t.Fatalf("title textWithEntities 0x%x", uint32(todo.GetTitle().GetConstructor()))
	}
	if len(todo.GetList()) == 0 {
		t.Fatal("no items")
	}
	for _, it := range todo.GetList() {
		if uint32(it.GetConstructor()) != 0xcba9a52f {
			t.Fatalf("todoItem 0x%x", uint32(it.GetConstructor()))
		}
		if uint32(it.GetTitle().GetConstructor()) != 0x751f3146 {
			t.Fatalf("item textWithEntities 0x%x", uint32(it.GetTitle().GetConstructor()))
		}
	}
	if withCompletion {
		if len(media.GetCompletions()) == 0 || uint32(media.GetCompletions()[0].GetConstructor()) != 0x221bb5e4 {
			t.Fatalf("todoCompletion %+v", media.GetCompletions())
		}
	}
}

func sampleTodo(appendOK, completeOK bool) *TodoList {
	return MakeTLTodoList(&TodoList{
		OthersCanAppend:   appendOK,
		OthersCanComplete: completeOK,
		Title:             MakeTLTextWithEntities(&TextWithEntities{Text: "购物"}).To_TextWithEntities(),
		List: []*TodoItem{
			MakeTLTodoItem(&TodoItem{
				Id:    1,
				Title: MakeTLTextWithEntities(&TextWithEntities{Text: "牛奶"}).To_TextWithEntities(),
			}).To_TodoItem(),
			MakeTLTodoItem(&TodoItem{
				Id:    2,
				Title: MakeTLTextWithEntities(&TextWithEntities{Text: "面包"}).To_TextWithEntities(),
			}).To_TodoItem(),
		},
	}).To_TodoList()
}

func TestMessageMediaToDoLayer232RoundTrip(t *testing.T) {
	media, err := MessageMediaFromTodo(sampleTodo(true, true))
	if err != nil {
		t.Fatal(err)
	}
	if media.GetPredicateName() != Predicate_messageMediaToDo {
		t.Fatalf("predicate %s", media.GetPredicateName())
	}
	if media.GetCompletions() != nil {
		t.Fatal("fresh checklist must omit completions so flag 0 stays clear")
	}
	if !media.GetTodo().GetOthersCanAppend() || !media.GetTodo().GetOthersCanComplete() {
		t.Fatal("flags dropped")
	}

	msg := MakeTLMessage(nil)
	msg.SetId(8)
	msg.SetOut(true)
	msg.SetPeerId(MakePeerUser(2))
	msg.SetDate(1_700_000_000)
	msg.SetMessage("清单")
	msg.SetMedia(media)

	raw := encodeAt(t, msg.To_Message(), Layer)
	if ctorOf(raw) != 0x7600b9d3 {
		t.Fatalf("message ctor 0x%x", ctorOf(raw))
	}
	assertLayer232TodoWire(t, raw, false)
	got := &Message{}
	if err := got.Decode(NewDecodeBuf(raw)); err != nil {
		t.Fatal(err)
	}
	assertLayer232TodoTree(t, got.GetMedia(), false)
	if len(got.GetMedia().GetCompletions()) != 0 {
		t.Fatalf("completions flag leaked: %+v", got.GetMedia().GetCompletions())
	}
	todo := got.GetMedia().GetTodo()
	if todo.GetTitle().GetText() != "购物" || len(todo.GetList()) != 2 || todo.GetList()[0].GetTitle().GetText() != "牛奶" {
		t.Fatalf("todo %+v", todo)
	}
	if !todo.GetOthersCanAppend() || !todo.GetOthersCanComplete() {
		t.Fatal("todo flags")
	}
	if uint32(todo.GetConstructor()) != 0x49b92a26 {
		t.Fatalf("todoList ctor 0x%x", uint32(todo.GetConstructor()))
	}

	// Below layer 205 the constructor is absent, so the message encoder
	// substitutes messageMediaUnsupported. Layer 232 must not take that path.
	old := proto.Clone(msg.To_Message()).(*Message)
	oldRaw := encodeAt(t, old, 204)
	oldGot := &Message{}
	if err := oldGot.Decode(NewDecodeBuf(oldRaw)); err != nil {
		t.Fatal(err)
	}
	if oldGot.GetMedia().GetPredicateName() != Predicate_messageMediaUnsupported {
		t.Fatalf("layer 204 media %s", oldGot.GetMedia().GetPredicateName())
	}
}

func TestTodoPersistJSONAndProtoThenToggle(t *testing.T) {
	media, err := MessageMediaFromTodo(sampleTodo(false, true))
	if err != nil {
		t.Fatal(err)
	}
	msg := MakeTLMessage(&Message{
		Id:      4,
		PeerId:  MakePeerUser(9),
		Date:    10,
		Message: "",
		Media:   media,
	}).To_Message()

	js, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var fromJSON Message
	if err := json.Unmarshal(js, &fromJSON); err != nil {
		t.Fatal(err)
	}
	if fromJSON.GetMedia().GetPredicateName() != Predicate_messageMediaToDo {
		t.Fatalf("json media %s", fromJSON.GetMedia().GetPredicateName())
	}

	pb, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var fromProto Message
	if err := proto.Unmarshal(pb, &fromProto); err != nil {
		t.Fatal(err)
	}

	done, undone, err := ToggleTodoItems(fromJSON.GetMedia(), 7, []int32{1}, nil, 99)
	if err != nil || len(done) != 1 || done[0] != 1 || len(undone) != 0 {
		t.Fatalf("toggle %v %v %v", done, undone, err)
	}
	comp := fromJSON.GetMedia().GetCompletions()
	if len(comp) != 1 || comp[0].GetCompletedBy_INT64() != 7 || comp[0].GetCompletedBy_PEER().GetUserId() != 7 || comp[0].GetDate() != 99 {
		t.Fatalf("completion %+v", comp)
	}

	raw := encodeAt(t, fromJSON.FixData(), Layer)
	assertLayer232TodoWire(t, raw, true)
	got := &Message{}
	if err := got.Decode(NewDecodeBuf(raw)); err != nil {
		t.Fatal(err)
	}
	assertLayer232TodoTree(t, got.GetMedia(), true)
	if got.GetMedia().GetCompletions()[0].GetCompletedBy_PEER().GetUserId() != 7 {
		t.Fatalf("completed_by %+v", got.GetMedia().GetCompletions()[0])
	}

	// Layer 216 still has the checklist, but completed_by is a long.
	raw216 := encodeAt(t, proto.Clone(&fromJSON).(*Message), 216)
	got216 := &Message{}
	if err := got216.Decode(NewDecodeBuf(raw216)); err != nil {
		t.Fatal(err)
	}
	if got216.GetMedia().GetPredicateName() != Predicate_messageMediaToDo {
		t.Fatalf("layer 216 media %s", got216.GetMedia().GetPredicateName())
	}
	if uint32(got216.GetMedia().GetCompletions()[0].GetConstructor()) != 0x4cc120b7 {
		t.Fatalf("layer 216 todoCompletion ctor 0x%x", uint32(got216.GetMedia().GetCompletions()[0].GetConstructor()))
	}
	if got216.GetMedia().GetCompletions()[0].GetCompletedBy_INT64() != 7 {
		t.Fatalf("layer 216 completed_by %d", got216.GetMedia().GetCompletions()[0].GetCompletedBy_INT64())
	}

	done, undone, err = ToggleTodoItems(fromProto.GetMedia(), 7, nil, []int32{1}, 100)
	if err == nil {
		t.Fatal("proto copy was never completed")
	}
	done, undone, err = ToggleTodoItems(fromJSON.GetMedia(), 7, nil, []int32{1}, 100)
	if err != nil || len(undone) != 1 || undone[0] != 1 || len(done) != 0 {
		t.Fatalf("untoggle %v %v %v", done, undone, err)
	}
	if fromJSON.GetMedia().GetCompletions() != nil {
		t.Fatalf("flag should clear: %+v", fromJSON.GetMedia().GetCompletions())
	}
}

func TestTodoAppendAndEditKeepsCompletions(t *testing.T) {
	media, err := MessageMediaFromTodo(sampleTodo(true, false))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ToggleTodoItems(media, 3, []int32{2}, nil, 5); err != nil {
		t.Fatal(err)
	}
	extra := MakeTLTodoItem(&TodoItem{
		Id:    3,
		Title: MakeTLTextWithEntities(&TextWithEntities{Text: "鸡蛋"}).To_TextWithEntities(),
	}).To_TodoItem()
	added, err := AppendTodoItems(media, []*TodoItem{extra})
	if err != nil || len(added) != 1 || added[0].GetTitle().GetText() != "鸡蛋" {
		t.Fatalf("append %v %v", added, err)
	}
	if _, err := AppendTodoItems(media, []*TodoItem{extra}); err != ErrTodoItemDuplicate {
		t.Fatalf("dup %v", err)
	}
	if _, err := AppendTodoItems(media, nil); err != ErrTodoNotModified {
		t.Fatalf("empty %v", err)
	}
	if len(media.GetCompletions()) != 1 || media.GetCompletions()[0].GetId() != 2 {
		t.Fatalf("append cleared a check: %+v", media.GetCompletions())
	}

	edited, err := MessageMediaFromTodo(MakeTLTodoList(&TodoList{
		Title: MakeTLTextWithEntities(&TextWithEntities{Text: "购物"}).To_TextWithEntities(),
		List: []*TodoItem{
			MakeTLTodoItem(&TodoItem{
				Id:    2,
				Title: MakeTLTextWithEntities(&TextWithEntities{Text: "面包"}).To_TextWithEntities(),
			}).To_TodoItem(),
		},
	}).To_TodoList())
	if err != nil {
		t.Fatal(err)
	}
	kept := KeepTodoCompletions(media, edited)
	if len(kept.GetCompletions()) != 1 || kept.GetCompletions()[0].GetId() != 2 {
		t.Fatalf("edit dropped the surviving check: %+v", kept.GetCompletions())
	}
	if len(kept.GetTodo().GetList()) != 1 {
		t.Fatal("removed item came back")
	}
}

func TestTodoRejectsBadLists(t *testing.T) {
	if _, err := MessageMediaFromTodo(nil); err != ErrMediaInvalid {
		t.Fatal(err)
	}
	blank := sampleTodo(false, false)
	blank.Title = MakeTLTextWithEntities(&TextWithEntities{Text: "  "}).To_TextWithEntities()
	if _, err := MessageMediaFromTodo(blank); err != ErrMediaInvalid {
		t.Fatal(err)
	}
	dup := sampleTodo(false, false)
	dup.List[1].Id = 1
	if _, err := MessageMediaFromTodo(dup); err != ErrTodoItemDuplicate {
		t.Fatal(err)
	}
	if !TodoActorAllowed(4, 4, false) || TodoActorAllowed(4, 5, false) || !TodoActorAllowed(4, 5, true) {
		t.Fatal("actor check")
	}
}
