package feventwatcher

import (
	"fmt"
	"time"
)

type cooldownTimer struct {
	id              string
	timeCreated     time.Time
	timeUpdated     time.Time
	countdownMillis int64
	counter         uint32
	data            interface{}
	newData         chan interface{}
	notify          chan bool
	stop            chan bool
	onDone          func(data interface{}, counter uint32, timeCreated time.Time, timeUpdated time.Time)
}

type CooldownTimer interface {
	NewData() chan interface{}
	Stop() error
}

func NewCooldownTime(
	id string, countdownMillis int64,
	onDone func(data interface{}, counter uint32, timeCreated time.Time, timeUpdated time.Time)) (CooldownTimer, error) {

	now := time.Now()
	t := &cooldownTimer{
		id:              id,
		countdownMillis: countdownMillis,
		timeCreated:     now,
		timeUpdated:     now,
		onDone:          onDone,
		newData:         make(chan interface{}),
		notify:          make(chan bool),
		stop:            make(chan bool),
	}

	go t.coolingLoop()
	go t.dataLoop()

	return t, nil
}

func (t *cooldownTimer) coolingLoop() {
	// fmt.Printf("Start timerLoop countdown [%s]", t.id)
	for {
		// fmt.Println("Cooling Outter Loop")
		select {
		case <-t.notify:
		CollingLoop:
			for {
				// fmt.Println("Cooling Inner Loop")
				select {
				case <-time.After(time.Duration(t.countdownMillis) * time.Millisecond):
					// Cool down enough
					t.stop <- true
					if t.data != nil {
						// fmt.Printf("Cooled countdown [%s]", t.id)
						t.onDone(t.data, t.counter, t.timeCreated, t.timeUpdated)
					}
					defer t.Stop()
					return
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
	// fmt.Printf("Start dataLoop countdown [%s]", t.id)
	for {
		// fmt.Println("Data Loop")
		select {
		case <-t.stop:
			// fmt.Printf("Stoping countdown [%s]", t.id)
			return
		case newData := <-t.newData:
			// fmt.Printf("New data on countdown [%s]", t.id)
			t.timeUpdated = time.Now()
			t.counter = t.counter + 1
			t.data = newData
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
