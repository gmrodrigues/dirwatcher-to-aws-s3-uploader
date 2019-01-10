package feventwatcher

import (
	"fmt"
	"time"
)

type cooldownTimer struct {
	timeCreated     time.Time
	timeUpdated     time.Time
	countdownMillis uint64
	done            bool
	renew           chan bool
	data            interface{}
	onDone          func(t CooldownTimer)
	lock            Lockable
}

type CooldownTimer interface {
	NewData(newData interface{}, mergeDataFunc func(newData interface{}, oldData interface{}) (mergedData interface{})) error
	OnDone(callback func(t CooldownTimer)) error
	IsDone() bool
	Data() interface{}
	TimeCreated() time.Time
	TimeUpdated() time.Time
	CountdownMillis() uint64
}

func NewCooldownTime(id string, countdownMillis uint64, data interface{}) (CooldownTimer, error) {
	now := time.Now()
	t := &cooldownTimer{
		countdownMillis: countdownMillis,
		data:            data,
		timeCreated:     now,
		timeUpdated:     now,
		done:            countdownMillis == 0,
		renew:           make(chan bool),
		lock:            NewLockable(id),
	}

	fmt.Printf("countdownDuration %v", countdownMillis)

	return t, nil
}

func (t *cooldownTimer) NewData(newData interface{}, mergeDataFunc func(newData interface{}, oldData interface{}) (mergedData interface{})) error {

	if !t.done {
		t.lock.Lock("NewData")
		if mergeDataFunc != nil {
			t.data = mergeDataFunc(newData, t.data)
		}
		t.timeUpdated = time.Now()
		t.lock.Unlock("NewData")
		select {
		case t.renew <- true:
			break
		default:
			break
		}
	}
	return nil
}

func (t *cooldownTimer) OnDone(callback func(t CooldownTimer)) error {

	if t.onDone == nil {
		t.lock.Lock("OnDone")
		t.onDone = callback
		t.lock.Unlock("OnDone")

		go func() {
			for {
				// fmt.Println("\nLoop OnDone")
				select {
				case <-time.After(time.Duration(t.countdownMillis) * time.Millisecond):
					// fmt.Println("\nAfter")
					t.onDone(t)
					return
				case done := <-t.renew:
					if done {
						// fmt.Println("\nDone")
						t.onDone(t)
						return
					}
				}
			}
		}()
		return nil
	} else {
		return fmt.Errorf("OnDone callback already defined for cooldownTimer data %#v", t.data)
	}
}

func (t *cooldownTimer) IsDone() bool {
	return t.done
}

func (t *cooldownTimer) Data() interface{} {
	return t.data
}

func (t *cooldownTimer) TimeCreated() time.Time {
	return t.timeCreated
}

func (t *cooldownTimer) TimeUpdated() time.Time {
	return t.timeUpdated
}

func (t *cooldownTimer) CountdownMillis() uint64 {
	return t.countdownMillis
}
