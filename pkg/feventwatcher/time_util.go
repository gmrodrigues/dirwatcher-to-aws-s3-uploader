package feventwatcher

import (
	"fmt"
	"time"
)

type cooldownTimer struct {
	id              string
	timeCreated     time.Time
	timeUpdated     time.Time
	countdownMillis uint64
	data            interface{}
	newData         chan interface{}
	notify          chan bool
	stop            chan bool
	mergeData       func(newData interface{}, oldData interface{}) (mergedData interface{})
	onDone          func(data interface{}, timeCreated time.Time, timeUpdated time.Time)
	onClose         func()
}

type CooldownTimer interface {
	NewData() chan interface{}
	Stop() error
}

func NewCooldownTime(
	id string, countdownMillis uint64,
	mergeData func(newData interface{}, oldData interface{}) (mergedData interface{}),
	onDone func(data interface{}, timeCreated time.Time, timeUpdated time.Time),
	onClose func()) (CooldownTimer, error) {

	now := time.Now()
	t := &cooldownTimer{
		id:              id,
		countdownMillis: countdownMillis,
		timeCreated:     now,
		timeUpdated:     now,
		onDone:          onDone,
		onClose:         onClose,
		mergeData:       mergeData,
		newData:         make(chan interface{}),
		notify:          make(chan bool),
		stop:            make(chan bool),
	}

	go t.collingLoop()
	go t.dataLoop()

	return t, nil
}

func (t *cooldownTimer) collingLoop() {
	fmt.Printf("Start timerLoop countdown [%s]", t.id)
StartLoop:
	for {
		select {
		case <-t.notify:
		CollingLoop:
			for {
				select {
				case <-time.After(time.Duration(t.countdownMillis) * time.Millisecond):
					// Cool down enough
					t.stop <- true
					if t.data != nil {
						fmt.Printf("Cooled countdown [%s]", t.id)
						d, c, u := t.data, t.timeCreated, t.timeUpdated
						t.data = nil
						t.onDone(d, c, u)
					}
					break StartLoop
				case <-t.notify:
					// Notified about new data...
					// hot again, let's coolin' wait
					continue CollingLoop
				}
			}
		}
	}
}

func (t *cooldownTimer) dataLoop() {
	fmt.Printf("Start dataLoop countdown [%s]", t.id)
	for {
		select {
		case <-t.stop:
			fmt.Printf("Stoping countdown [%s]", t.id)
			t.onClose()
			close(t.stop)
			close(t.notify)
			close(t.newData)
			return
		case newData := <-t.newData:
			fmt.Printf("New data on countdown [%s]", t.id)
			t.timeUpdated = time.Now()
			if t.mergeData != nil {
				if t.data != nil && newData != nil {
					t.data = t.mergeData(newData, t.data)
				} else {
					t.data = newData
				}
			}
			t.notify <- true
			continue
		}
	}
}

func (t *cooldownTimer) NewData() chan interface{} {
	return t.newData
}

func (t *cooldownTimer) Stop() error {
	select {
	case t.stop <- true:
		return nil
	case <-time.After(time.Duration(5) * time.Second):
		return fmt.Errorf("Countdowns %s already stopped", t.id)
	}
}
