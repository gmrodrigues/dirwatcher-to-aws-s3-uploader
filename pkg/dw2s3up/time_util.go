package dw2s3up

import "time"

type cooldownTimer struct {
	timeCreated   time.Time
	timeUpdated   time.Time
	countdownTime time.Duration
	done          bool
	notifyDone    chan bool
	data          interface{}
}

type CooldownTimer interface {
	Renew(data interface{}) error
	OnDone(callback func(t CooldownTimer)) error
	IsDone() bool
	Data() interface{}
	TimeCreated() time.Time
	TimeUpdated() time.Time
	CountdownTime() time.Duration
}

func NewCooldownTime(countdownTime time.Duration, data interface{}) (CooldownTimer, error) {
	now := time.Now()
	t := &cooldownTimer{
		countdownTime: countdownTime,
		data:          data,
		timeCreated:   now,
		timeUpdated:   now,
		done:          false,
		notifyDone:    make(chan bool),
	}

	return t, nil
}

func (t *cooldownTimer) Renew(data interface{}) error {
	if !t.done {
		t.data = data
		t.done = false
		t.timeUpdated = time.Now()
		t.notifyDone <- t.done
	}
	return nil
}

func (t *cooldownTimer) OnDone(callback func(t CooldownTimer)) error {
	go func() {
		for {
			select {
			case <-time.After(t.countdownTime):
				callback(t)
				return
			case done := <-t.notifyDone:
				if done {
					callback(t)
					return
				}
			}
		}
	}()
	return nil
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

func (t *cooldownTimer) CountdownTime() time.Duration {
	return t.countdownTime
}
