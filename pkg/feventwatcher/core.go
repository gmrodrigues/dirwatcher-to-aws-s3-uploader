package feventwatcher

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type File struct {
	Name     string
	NormName string
	IsDir    bool
	Exists   bool
	Meta     interface{}
}

type EventCooldownConf struct {
	CounterMillis uint64 `default:"1000"`
}

type WatcherConf struct {
	BaseDir  string
	DotFiles bool
	Cooldown EventCooldownConf `default:"EventCooldownConf{}"`
}

var EVENT_FORMAT_VERSION = "2019-01-10"

type WatcherEvent struct {
	Version       string
	UUID          string
	File          *File
	Stat          FileInfo
	watcher       *Watcher
	Time          time.Time
	FirstChange   time.Time
	ChangeCounter uint32
	Forced        bool
}

type Watcher struct {
	conf               WatcherConf
	watcher            *fsnotify.Watcher
	done               chan bool
	subs               []chan *WatcherEvent
	pushed             chan string
	coolingEvents      map[string]CooldownTimer
	logger             *zap.Logger
	lock               RWLock
	onCoolDownDone     func(data interface{}, timeCreated time.Time, timeUpdated time.Time)
	cooldownEventMerge func(newData interface{}, oldData interface{}) (mergedData interface{})
}

var FILEINFO_FORMAT_VERSION = "2019-01-10"

type FileInfo struct {
	Version string
	Name    string      // base name of the file
	Size    int64       // length in bytes for regular files; system-dependent for others
	Mode    os.FileMode // file mode bits
	ModTime time.Time   // modification time
	Sys     interface{} // underlying data source (can return nil)
}

type Watchable interface {
	Conf() WatcherConf
	SubscribeChan(ch chan *WatcherEvent) error
	SubscribeFunc(f func(e *WatcherEvent)) error
	Start() error
	Done() error
}

func NewWatcherEventChan() chan *WatcherEvent {
	return make(chan *WatcherEvent)
}

func NewWatchable(conf WatcherConf) (wtb Watchable, err error) {
	w := &Watcher{conf: conf}
	w.watcher, err = fsnotify.NewWatcher()
	w.logger, _ = zap.NewDevelopment()
	defer w.logger.Sync()
	if err != nil {
		// w.logger.Fatal(err.Error())

	}

	w.done = make(chan bool)
	w.pushed = make(chan string, 1000)
	w.subs = make([]chan *WatcherEvent, 1)
	w.coolingEvents = make(map[string]CooldownTimer)
	w.lock = NewRWLock(fmt.Sprintf("watch:%s", conf.BaseDir))
	w.onCoolDownDone = func(data interface{}, timeCreated time.Time, timeUpdated time.Time) {
		e := data.(*WatcherEvent)

		notify := func(s chan *WatcherEvent) {
			s <- e
		}

		for _, sub := range w.subs {
			go notify(sub)
		}
	}
	w.cooldownEventMerge = func(newData interface{}, oldData interface{}) (mergedData interface{}) {
		oldest := oldData.(*WatcherEvent)
		newest := newData.(*WatcherEvent)

		if oldest.Time.After(newest.Time) {
			oldest, newest = newest, oldest
		}

		newest.ChangeCounter = oldest.ChangeCounter + 1
		newest.FirstChange = oldest.FirstChange

		return newest
	}

	err = w.watcher.Add(w.conf.BaseDir)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	return w, nil
}

func (w *Watcher) Done() error {
	select {
	case w.done <- true:
		return nil
	default:
		return fmt.Errorf("Closed already")
	}
}

func (w *Watcher) Start() error {
	defer w.logger.Sync()
	defer w.watcher.Close()
	dirGlobs, _ := filepath.Glob(path.Join(w.conf.BaseDir, "**", "*"))
	// w.logger.Info(fmt.Sprintf("Glob matches %s: %#v\n", w.conf.BaseDir, dirGlobs))

	pushGlob := func(fname string) {
		w.pushed <- fname
	}
	for _, f := range dirGlobs {
		go pushGlob(f)
	}

	handle := func(filename string, forced bool) error {
		fmt.Printf("\n\nHandle: %s\n\n", filename)

		if len(filename) == 0 {
			return fmt.Errorf("Empty file name received")
		}

		baseFile := filepath.Base(filename)
		if !w.conf.DotFiles && strings.HasPrefix(baseFile, ".") {
			return fmt.Errorf("Dotfile ignored: %s", filename)
		}

		w.logger.Info(fmt.Sprintf("Handling %s", filename))

		stat, stat_err := os.Stat(filename)
		file := File{
			Name:     filename,
			NormName: filepath.Clean(filename),
			Exists:   !os.IsNotExist(stat_err),
		}
		s := FileInfo{
			Version: FILEINFO_FORMAT_VERSION,
			Name:    baseFile,
		}
		if file.Exists {
			file.IsDir = stat.IsDir()
			s = FileInfo{
				Version: FILEINFO_FORMAT_VERSION,
				Name:    stat.Name(),    // base name of the file
				Size:    stat.Size(),    // length in bytes for regular files; system-dependent for others
				Mode:    stat.Mode(),    // file mode bits
				ModTime: stat.ModTime(), // modification time
				Sys:     stat.Sys(),     // underlying data source (can return nil)
			}
		}
		now := time.Now()
		e := WatcherEvent{
			Version:     EVENT_FORMAT_VERSION,
			UUID:        uuid.New().String(),
			File:        &file,
			watcher:     w,
			Stat:        s,
			Forced:      forced,
			FirstChange: now,
			Time:        now,
		}
		e.tick()

		return nil
	}

	for {
		select {
		case <-w.done:
			return nil
		case event, ok := <-w.watcher.Events:
			forced := false
			if ok {
				fmt.Printf("Ahew: [%s] %#v", event.Name, event)
				go handle(event.Name, forced)
				continue
			}
		case err, ok := <-w.watcher.Errors:
			if err != nil {
				w.logger.Error(fmt.Sprintf("Err: %s", err.Error()))
			} else {
				w.logger.Error(fmt.Sprintf("Erro locao ok: %v", ok))
			}
			continue
		case filename := <-w.pushed:
			forced := true
			fmt.Printf("Woooo: [%s]", filename)
			go handle(filename, forced)
			continue
		case <-time.After(time.Minute):
			w.logger.Debug(fmt.Sprintf("Timout"))
			continue
		}
	}
}

func (we *WatcherEvent) keep() (err error) {
	file := we.File
	w := we.watcher
	err = w.watcher.Add(file.NormName)

	if file.IsDir || !file.Exists {
		w.onCoolDownDone(we, we.Time, we.Time)
	} else {
		lock_id := fmt.Sprintf("AddFile [%s]", file.NormName)
		w.lock.WLock(lock_id)
		defer w.lock.WUnlock(lock_id)

		ce := w.coolingEvents[file.NormName]
		if err != nil {
			if ce != nil {
				ce.Stop()
				delete(w.coolingEvents, file.NormName)
			}
			w.logger.Error(fmt.Sprintf("Error adding [%s]: %s", file.Name, err.Error()))
		} else {
			if ce == nil {
				cc := w.conf.Cooldown
				fmt.Printf("\n\nCOUNTER %v\n", cc.CounterMillis)
				ce, _ = NewCooldownTime(
					fmt.Sprintf("cooldown:%s", file.Name),
					cc.CounterMillis,
					w.cooldownEventMerge,
					w.onCoolDownDone,
					func() {
						// onStop
						defer w.logger.Debug(fmt.Sprintf("Success cooling [%s]", file.Name))
						w.lock.WLock("remove cooldown")
						defer w.lock.WUnlock("remove cooldown")

						delete(w.coolingEvents, file.NormName)
					})
				w.coolingEvents[file.NormName] = ce
			}
			w.logger.Debug(fmt.Sprintf("Success adding [%s]", file.Name))
		}
	}

	return err
}

func (w *Watcher) ForcePushFile(filename string) (err error) {
	w.pushed <- filename
	return nil
}

func (w *Watcher) Conf() WatcherConf {
	return w.conf
}

func (w *Watcher) SubscribeChan(ch chan *WatcherEvent) error {
	w.subs = append(w.subs, ch)
	return nil
}

func (w *Watcher) SubscribeFunc(f func(e *WatcherEvent)) error {
	// defer w.logger.Info("Finish")
	// defer w.logger.Sync()

	go func() {
		ch := NewWatcherEventChan()
		t := make(chan bool, 100) // 100 func threads
		defer close(t)

		err := w.SubscribeChan(ch)
		if err != nil {
			fmt.Println(err.Error())
			return
		}

		for {
			e := <-ch
			t <- true
			f(e)
			<-t
		}
	}()

	return nil
}

func (we *WatcherEvent) tick() error {
	we.keep()

	if we.File.IsDir {
		return nil
	}

	we.watcher.lock.RLock("tick")
	defer we.watcher.lock.RUnlock("tick")

	cdt := we.watcher.coolingEvents[we.File.NormName]
	if cdt != nil {
		select {
		case cdt.NewData() <- we:
			we.watcher.logger.Debug(fmt.Sprintf("Sent data about %s", we.File.Name))
			return nil
		case <-time.After(time.Duration(15) * time.Second):
			return fmt.Errorf("Failed to tick event after 15 seconds: %#v", we)
		}
	}
	return nil
}
