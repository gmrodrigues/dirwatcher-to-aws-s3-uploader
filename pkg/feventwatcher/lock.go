package feventwatcher

import (
	"fmt"
	"sync"
	"time"
)

type Lock struct {
	id    string
	mutex sync.RWMutex
}

type RWLock interface {
	RLock(op string)
	RUnlock(op string)
	WLock(op string)
	WUnlock(op string)
	Id() string
}

func NewRWLock(id string) RWLock {
	return &Lock{id: id, mutex: sync.RWMutex{}}
}

func (l *Lock) RLock(op string) {
	fmt.Printf("\n[> ] %s %p RLock id: [%s] %s\n", op, l, l.id, time.Now())
	l.mutex.RLock()
	fmt.Printf("\n[>>] %s %p RLock id: [%s] %s\n", op, l, l.id, time.Now())
}

func (l *Lock) RUnlock(op string) {
	fmt.Printf("\n[ <] %s %p RUnlock id: [%s] %s\n", op, l, l.id, time.Now())
	l.mutex.RUnlock()
	fmt.Printf("\n[<<] %s %p RUnlock id: [%s] %s\n", op, l, l.id, time.Now())
}

func (l *Lock) WLock(op string) {
	fmt.Printf("\n[> ] %s %p WLock id: [%s] %s\n", op, l, l.id, time.Now())
	l.mutex.Lock()
	fmt.Printf("\n[>>] %s %p WLock id: [%s] %s\n", op, l, l.id, time.Now())
}

func (l *Lock) WUnlock(op string) {
	fmt.Printf("\n[ <] %s %p WUnlock id: [%s] %s\n", op, l, l.id, time.Now())
	l.mutex.Unlock()
	fmt.Printf("\n[<<] %s %p WUnlock id: [%s] %s\n", op, l, l.id, time.Now())
}

func (l *Lock) Id() string {
	return l.id
}
