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
	stop            []chan bool
	onDone          func(data interface{}, timeCreated time.Time, timeUpdated time.Time)
	mergeData       func(newData interface{}, oldData interface{}) (mergedData interface{})
}

type CooldownTimer interface {
	NewData() chan interface{}
	Stop() error
}

func NewCooldownTime(
	id string, countdownMillis uint64,
	mergeData func(newData interface{}, oldData interface{}) (mergedData interface{}),
	onDone func(data interface{}, timeCreated time.Time, timeUpdated time.Time)) (CooldownTimer, error) {

	now := time.Now()
	t := &cooldownTimer{
		id:              id,
		countdownMillis: countdownMillis,
		timeCreated:     now,
		timeUpdated:     now,
		onDone:          onDone,
		mergeData:       mergeData,
		newData:         make(chan interface{}),
		notify:          make(chan bool),
		stop:            make([]chan bool, 0),
	}

	go t.timerLoop()
	go t.dataLoop()

	return t, nil
}

func (t *cooldownTimer) makeStopChan() chan bool {
	ch := make(chan bool)
	t.stop = append(t.stop, ch)
	return ch
}

func (t *cooldownTimer) timerLoop() {
	fmt.Printf("Start timerLoop countdown [%s]", t.id)
	stop := t.makeStopChan()
StartLoop:
	for {
		select {
		case <-stop:
			fmt.Printf("Stoping countdown [%s]", t.id)
			break StartLoop
		case <-t.notify:
		TimerLoop:
			for {
				select {
				case <-time.After(time.Duration(t.countdownMillis) * time.Millisecond):
					if t.data != nil {
						fmt.Printf("Cooled countdown [%s]", t.id)
						d, c, u := t.data, t.timeCreated, t.timeUpdated
						t.data = nil
						t.onDone(d, c, u)
					}
					continue StartLoop
				case <-t.notify:
					continue TimerLoop
				}
			}
		}
	}
}

func (t *cooldownTimer) dataLoop() {
	fmt.Printf("Start dataLoop countdown [%s]", t.id)
	stop := t.makeStopChan()
	for {
		select {
		case <-stop:
			fmt.Printf("Stoping countdown [%s]", t.id)
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
	var err error = nil
	for _, stop := range t.stop {
		select {
		case stop <- true:
			continue
		default:
			err = fmt.Errorf("Countdowns %s already stopped", t.id)
		}
	}
	return err
}
