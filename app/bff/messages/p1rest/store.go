package p1rest

import "sync"

type dialogKey struct {
	user int64
	kind string
	id   int64
}

type store struct {
	mu        sync.Mutex
	themes    map[dialogKey]themeRec
	walls     map[dialogKey]wallState
	imports   map[int64]*importJob
	importSeq int64
	todos     map[todoKey]*todoList
	shares    map[dialogKey]string
	suggested map[todoKey]bool
}

var (
	gate    sync.Mutex
	current = newStore()
)

func newStore() *store {
	return &store{
		themes:    map[dialogKey]themeRec{},
		walls:     map[dialogKey]wallState{},
		imports:   map[int64]*importJob{},
		todos:     map[todoKey]*todoList{},
		shares:    map[dialogKey]string{},
		suggested: map[todoKey]bool{},
	}
}

// Reset clears process state. Tests call it.
func Reset() {
	gate.Lock()
	current = newStore()
	gate.Unlock()
}

func shared() *store {
	gate.Lock()
	s := current
	gate.Unlock()
	return s
}
