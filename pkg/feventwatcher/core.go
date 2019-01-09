package feventwatcher

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
	yaml "gopkg.in/yaml.v2"
)

const (
	CREATE = "CREATE"
	WRITE  = "WRITE"
	REMOVE = "REMOVE"
	RENAME = "RENAME"
	CHMOD  = "CHMOD"
	PUSHED = "PUSHED"
)

var fsNotfyOpToStr = map[fsnotify.Op]string{
	fsnotify.Create: CREATE,
	fsnotify.Write:  WRITE,
	fsnotify.Remove: REMOVE,
	fsnotify.Rename: RENAME,
	fsnotify.Chmod:  CHMOD,
}

type File struct {
	Name string
	Meta interface{}
}

type EventCooldownConf struct {
	CounterMillis uint64 `default:"1000"`
}

type WatcherConf struct {
	BaseDir  string
	Cooldown EventCooldownConf `default:"EventCooldownConf{}"`
}

type WatcherEvent struct {
	Types   map[string]time.Time // Create, Write, Remove, Rename,  Chmod, Pushed
	File    *File
	Stat    FileInfo
	watcher *Watcher
	Time    time.Time
}

type Watcher struct {
	conf          WatcherConf
	watcher       *fsnotify.Watcher
	done          chan bool
	subs          []chan WatcherEvent
	pushed        chan File
	metaMap       map[string]interface{}
	coolingEvents map[string]CooldownTimer
	logger        *zap.Logger
	mutex         sync.Mutex
}

type FileInfo struct {
	Name    string      // base name of the file
	Size    int64       // length in bytes for regular files; system-dependent for others
	Mode    os.FileMode // file mode bits
	ModTime time.Time   // modification time
	IsDir   bool        // abbreviation for Mode().IsDir()
	Exists  bool        // file exists
	Sys     interface{} // underlying data source (can return nil)
}

type Watchable interface {
	Conf() WatcherConf
	SubscribeChan(ch chan WatcherEvent) error
	SubscribeFunc(f func(e WatcherEvent)) error
	AddFile(file File) error
	ForcePushFile(file File) error
	ForgetFile(filename string) (file File, err error)
	Start() error
	Done() error
}

func NewWatcherEventChan() chan WatcherEvent {
	return make(chan WatcherEvent)
}

func NewWatchable(conf WatcherConf) (wtb Watchable, err error) {
	w := &Watcher{conf: conf}
	w.watcher, err = fsnotify.NewWatcher()
	w.logger, _ = zap.NewDevelopment()
	defer w.logger.Sync()
	if err != nil {
		w.logger.Fatal(err.Error())

	}

	w.done = make(chan bool)
	w.pushed = make(chan File)
	w.subs = make([]chan WatcherEvent, 1)
	w.metaMap = make(map[string]interface{})
	w.coolingEvents = make(map[string]CooldownTimer)

	err = w.watcher.Add(w.conf.BaseDir)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	return w, nil
}

func (w *Watcher) Done() error {
	w.done <- true
	return nil
}

func (w *Watcher) Start() error {
	defer w.logger.Sync()
	defer w.watcher.Close()
	dirGlobs, _ := filepath.Glob(fmt.Sprintf("%s/*", w.conf.BaseDir))
	w.logger.Info(fmt.Sprintf("Glob matches %s: %#v\n", w.conf.BaseDir, dirGlobs))

	pushGlob := func(fname string) {
		w.pushed <- File{Name: fname}
	}
	for _, f := range dirGlobs {
		go pushGlob(f)
	}

	cooldownEventMerge := func(newData interface{}, oldData interface{}) (mergedData interface{}) {
		oldest := oldData.(WatcherEvent)
		newest := newData.(WatcherEvent)

		if oldest.Time.After(newest.Time) {
			oldest, newest = newest, oldest
		}

		for ty, time := range oldest.Types {
			if _, ok := newest.Types[ty]; !ok || time.After(newest.Types[ty]) {
				newest.Types[ty] = time
			}
		}

		return newest
	}

	onCoolDownDone := func(ct CooldownTimer) {
		e := ct.Data().(WatcherEvent)
		delete(e.watcher.coolingEvents, e.File.Name)
		notify := func(s chan WatcherEvent) {
			s <- e
		}

		yaml, _ := yaml.Marshal(e)
		w.logger.Info(string(yaml))

		for _, sub := range w.subs {
			go notify(sub)
		}
	}

	handle := func(file File, op string) (WatcherEvent, error) {
		defer w.logger.Sync()

		if file.Meta == nil {
			file.Meta = w.metaMap[file.Name]
		}
		stat, stat_err := os.Stat(file.Name)
		exists := !os.IsNotExist(stat_err)
		s := FileInfo{
			Name:   filepath.Base(file.Name),
			Exists: exists,
		}
		if exists {
			s = FileInfo{
				Name:    stat.Name(),    // base name of the file
				Size:    stat.Size(),    // length in bytes for regular files; system-dependent for others
				Mode:    stat.Mode(),    // file mode bits
				ModTime: stat.ModTime(), // modification time
				IsDir:   stat.IsDir(),   // abbreviation for Mode().IsDir()
				Exists:  exists,         // file exists
				Sys:     stat.Sys(),     // underlying data source (can return nil)
			}
		}
		now := time.Now()
		e := WatcherEvent{
			File:    &file,
			Types:   map[string]time.Time{op: now},
			watcher: w,
			Stat:    s,
			Time:    now,
		}
		e.tick()
		//w.logger.Debug(fmt.Sprintf("event: %#v", e))
		c := e.instanceOfCooldown()
		c.NewData(e, cooldownEventMerge)
		c.OnDone(onCoolDownDone)

		return e, nil
	}

	for {
		select {
		case <-w.done:
			return nil
		case event, ok := <-w.watcher.Events:
			if ok {
				go handle(File{Name: event.Name}, fsNotfyOpToStr[event.Op])
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				if err != nil {
					w.logger.Error(fmt.Sprintf("Err: %s", err.Error()))
				}
			}
		case file := <-w.pushed:
			go handle(file, PUSHED)
		case <-time.After(time.Minute):
			w.logger.Debug(fmt.Sprintf("Timout"))
		}
	}
}

func (w *Watcher) AddFile(file File) (err error) {
	defer w.logger.Sync()

	w.logger.Debug(fmt.Sprintf("AddFileWithMeta: %s\n", file.Name))
	if w.metaMap[file.Name] == nil {
		err = w.watcher.Add(file.Name)
		if err != nil {
			if file.Meta != nil {
				w.metaMap[file.Name] = file.Meta
			} else {
				w.metaMap[file.Name] = true
			}
		}
		return err
	}
	return nil
}

func (w *Watcher) ForcePushFile(file File) (err error) {
	w.pushed <- file
	return nil
}

func (w *Watcher) ForgetFile(filename string) (file File, err error) {
	defer w.logger.Sync()

	w.logger.Debug(fmt.Sprintf("ForgetFile: %s\n", filename))
	err = w.watcher.Remove(filename)
	meta := w.metaMap[filename]
	if meta != nil {
		delete(w.metaMap, filename)
	}
	return File{Name: filename, Meta: meta}, err
}

func (w *Watcher) contains(filename string) bool {
	return w.metaMap[filename] != nil
}

func (w *Watcher) Conf() WatcherConf {
	return w.conf
}

func (w *Watcher) SubscribeChan(ch chan WatcherEvent) error {
	w.subs = append(w.subs, ch)
	return nil
}

func (w *Watcher) SubscribeFunc(f func(e WatcherEvent)) error {
	defer w.logger.Info("Finish")
	defer w.logger.Sync()

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

// Create, Write, Remove, Rename,  Chmod
func (we *WatcherEvent) isType(eType string) bool {
	_, ok := we.Types[eType]
	return ok
}

func (we *WatcherEvent) isCreate() bool {
	return we.isType(CREATE)
}

func (we *WatcherEvent) isWrite() bool {
	return we.isType(WRITE)
}

func (we *WatcherEvent) isRemove() bool {
	return we.isType(REMOVE)
}

func (we *WatcherEvent) isRename() bool {
	return we.isType(RENAME)
}

func (we *WatcherEvent) isChmod() bool {
	return we.isType(CHMOD)
}

func (we *WatcherEvent) isPushed() bool {
	return we.isType(PUSHED)
}

func (we *WatcherEvent) instanceOfCooldown() CooldownTimer {
	we.watcher.mutex.Lock()
	defer we.watcher.mutex.Unlock()

	ce := we.watcher.coolingEvents[we.File.Name]
	cc := we.watcher.conf.Cooldown
	fmt.Printf("\n\nCOUNTER %v\n", cc.CounterMillis)
	if ce == nil {
		ce, _ = NewCooldownTime(cc.CounterMillis, *we)
		we.watcher.coolingEvents[we.File.Name] = ce
	}
	return ce
}

func (we *WatcherEvent) tick() bool {
	if we.Stat.Exists {
		we.watcher.AddFile(*we.File)
	} else {
		we.watcher.ForgetFile(we.File.Name)
	}
	return true
}
