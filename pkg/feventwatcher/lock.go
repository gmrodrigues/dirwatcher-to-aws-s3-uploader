package feventwatcher

import (
	"sync"
)

type Lock struct {
	id    string
	mutex sync.Mutex
}

type Lockable interface {
	Lock(op string)
	Unlock(op string)
	Id() string
}

func NewLockable(id string) Lockable {
	return &Lock{id: id}
}

func (l *Lock) Lock(op string) {
	// fmt.Printf("\n[> ] %s %p Lock id: [%s] %s\n", op, l, l.id, time.Now())
	l.mutex.Lock()
	// fmt.Printf("\n[>>] %s %p Lock id: [%s] %s\n", op, l, l.id, time.Now())
}

func (l *Lock) Unlock(op string) {
	// fmt.Printf("\n[ <] %s %p Unlock id: [%s] %s\n", op, l, l.id, time.Now())
	l.mutex.Unlock()
	// fmt.Printf("\n[<<] %s %p Unlock id: [%s] %s\n", op, l, l.id, time.Now())
}

func (l *Lock) Id() string {
	return l.id
}
