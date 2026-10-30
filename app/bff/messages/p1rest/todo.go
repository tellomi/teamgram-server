package p1rest

import (
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
)

type todoKey struct {
	user int64
	kind string
	id   int64
	msg  int32
}

type todoItem struct {
	id    int32
	title string
	done  bool
	by    int64
	at    int32
}

type todoList struct {
	items []*todoItem
	next  int32
}

func (s *store) appendTodo(self int64, in *mtproto.TLMessagesAppendTodoList) (*mtproto.Updates, error) {
	ref, _, err := peerOf(self, in.GetPeer())
	if err != nil {
		return nil, err
	}
	if in.GetMsgId() <= 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	rows := in.GetList()
	if len(rows) == 0 {
		return nil, mtproto.ErrTodoNotModified
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k := todoKey{user: self, kind: ref.kind, id: ref.id, msg: in.GetMsgId()}
	list := s.todos[k]
	if list == nil {
		list = &todoList{}
		s.todos[k] = list
	}
	seen := map[int32]struct{}{}
	titles := map[string]struct{}{}
	for _, it := range list.items {
		seen[it.id] = struct{}{}
		if it.title != "" {
			titles[it.title] = struct{}{}
		}
	}
	var add []*todoItem
	for _, row := range rows {
		if row == nil {
			return nil, mtproto.ErrTodoNotModified
		}
		title := ""
		if row.GetTitle() != nil {
			title = strings.TrimSpace(row.GetTitle().GetText())
		}
		id := row.GetId()
		if id == 0 {
			list.next++
			if list.next <= 0 {
				list.next = 1
			}
			id = list.next
		}
		if _, ok := seen[id]; ok {
			return nil, mtproto.ErrTodoItemDuplicate
		}
		if title != "" {
			if _, ok := titles[title]; ok {
				return nil, mtproto.ErrTodoItemDuplicate
			}
			titles[title] = struct{}{}
		}
		seen[id] = struct{}{}
		if id > list.next {
			list.next = id
		}
		add = append(add, &todoItem{id: id, title: title})
	}
	list.items = append(list.items, add...)
	return mtproto.MakeEmptyUpdates(), nil
}

func (s *store) toggleTodo(self int64, in *mtproto.TLMessagesToggleTodoCompleted) (*mtproto.Updates, error) {
	ref, _, err := peerOf(self, in.GetPeer())
	if err != nil {
		return nil, err
	}
	if in.GetMsgId() <= 0 {
		return mtproto.MakeEmptyUpdates(), nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.todos[todoKey{user: self, kind: ref.kind, id: ref.id, msg: in.GetMsgId()}]
	if list == nil {
		return mtproto.MakeEmptyUpdates(), nil
	}
	now := int32(time.Now().Unix())
	byID := map[int32]*todoItem{}
	for _, it := range list.items {
		byID[it.id] = it
	}
	for _, id := range in.GetIncompleted() {
		if it := byID[id]; it != nil {
			it.done = false
			it.by = 0
			it.at = 0
		}
	}
	for _, id := range in.GetCompleted() {
		if it := byID[id]; it != nil {
			it.done = true
			it.by = self
			it.at = now
		}
	}
	return mtproto.MakeEmptyUpdates(), nil
}
