package feventwatcher

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
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
	Forced   bool
}

type EventCooldownConf struct {
	CounterMillis uint64 `default:"1000"`
}

type WatcherConf struct {
	BaseDir        string
	DotFiles       bool
	RegexWhiteList []string
	RegexBlackList []string
	Cooldown       EventCooldownConf `default:"EventCooldownConf{}"`
}

var EVENT_FORMAT_VERSION = "2019-01-31"

type WatcherEvent struct {
	Version     string
	UUID        string
	File        *File
	Stat        FileInfo
	watcher     *Watcher
	Time        time.Time
	FirstChange time.Time
	Events      uint32
	Meta        interface{}
}

type CoolDownDone struct {
	file        *File
	timeCreated time.Time
	timeUpdated time.Time
	counter     uint32
}

type Watcher struct {
	conf    *WatcherConf
	watcher *fsnotify.Watcher
	filter  struct {
		regxpWL []*regexp.Regexp
		regxpBL []*regexp.Regexp
	}
	done           chan bool
	subs           []chan *WatcherEvent
	in             chan *File
	out            chan *CoolDownDone
	pushed         chan string
	coolingEvents  map[string]CooldownTimer
	logger         *zap.Logger
	onCoolDownDone func(data interface{}, counter uint32, timeCreated time.Time, timeUpdated time.Time)
}

var FILEINFO_FORMAT_VERSION = "2019-01-31"

type FileInfo struct {
	Version string
	Name    string // base name of the file
	Base    string
	Size    int64       // length in bytes for regular files; system-dependent for others
	Mode    os.FileMode // file mode bits
	ModTime time.Time   // modification time
	Sys     interface{} // underlying data source (can return nil)
}

type Watchable interface {
	Conf() WatcherConf
	UpdateConf(conf WatcherConf) error
	SubscribeChan(ch chan *WatcherEvent) error
	SubscribeFunc(f func(e *WatcherEvent)) error
	Start() error
	Done() error
}

func NewWatcherEventChan() chan *WatcherEvent {
	return make(chan *WatcherEvent)
}

func NewWatchable(conf WatcherConf) (wtb Watchable, err error) {
	w := &Watcher{conf: &conf}
	err = w.UpdateConf(*w.conf)
	if err != nil {
		return nil, err
	}

	// init watcher
	conf.BaseDir = filepath.ToSlash(filepath.Clean(conf.BaseDir))
	w.watcher, err = fsnotify.NewWatcher()
	w.logger, _ = zap.NewDevelopment()
	defer w.logger.Sync()
	if err != nil {
		// w.logger.Fatal(err.Error())

	}

	EVENT_BUFFER_SIZE := 1024 * 100
	w.subs = make([]chan *WatcherEvent, 0)
	w.in = make(chan *File, EVENT_BUFFER_SIZE)
	w.out = make(chan *CoolDownDone, EVENT_BUFFER_SIZE)

	w.done = make(chan bool)
	w.pushed = make(chan string, 1000)
	w.coolingEvents = make(map[string]CooldownTimer)
	w.onCoolDownDone = func(data interface{}, counter uint32, timeCreated time.Time, timeUpdated time.Time) {
		f := data.(*File)
		w.out <- &CoolDownDone{
			file:        f,
			timeCreated: timeCreated,
			timeUpdated: timeUpdated,
			counter:     counter,
		}
	}

	err = w.watcher.Add(w.conf.BaseDir)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	return w, nil
}

func (e *WatcherEvent) Watcher() *Watcher {
	return e.watcher
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

	handle := func(filename string, forced bool) error {
		normName := filepath.ToSlash(filepath.Clean(filename))

		if len(filename) == 0 {
			return fmt.Errorf("Empty file name received")
		}

		baseFile := filepath.Base(normName)
		if !w.conf.DotFiles && strings.HasPrefix(baseFile, ".") {
			return fmt.Errorf("Dotfile ignored: %s", normName)
		}

		w.logger.Info(fmt.Sprintf("Handling %s", normName))

		file := &File{
			Name:     filename,
			NormName: normName,
			Forced:   forced,
		}

		w.in <- file
		w.logger.Info(fmt.Sprintf("Handled %s", filename))

		return nil
	}

	go w.cooldownNotifyLoop()
	go w.walkPush()

	for {
		w.logger.Debug("Main Loop")
		select {
		case event, ok := <-w.watcher.Events:
			forced := false
			if ok {
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
			go handle(filename, forced)
			continue
		case <-time.After(time.Minute):
			w.logger.Debug(fmt.Sprintf("Timout"))
			continue
		case <-w.done:
			defer w.Done()
			return nil
		}
	}
}

func (w *Watcher) walkPush() {
	walkVisit := func(path string, f os.FileInfo, err error) error {
		w.logger.Info(fmt.Sprintf("Walk push %s", path))
		if f.IsDir() || w.acceptedByFilters(filepath.Clean(path)) {
			w.pushed <- path
		}
		return nil
	}

	err := filepath.Walk(w.conf.BaseDir, walkVisit)
	if err != nil {
		w.logger.Error(fmt.Sprintf("Error on Walking and Pushing files on %s: %s", w.conf.BaseDir, err.Error()))
	}
}

func (w *Watcher) acceptedByFilters(normName string) bool {

	anyWhiteListFilter := len(w.filter.regxpWL) > 0
	anyFilter := anyWhiteListFilter || len(w.filter.regxpBL) > 0
	accepted := true
	if anyFilter {
		for _, m := range w.filter.regxpBL {
			if m.MatchString(normName) {
				w.logger.Info(fmt.Sprintf("Blacklisted %s", normName))
				return false
			}
		}
		for _, m := range w.filter.regxpWL {
			accepted = false
			if m.MatchString(normName) {
				w.logger.Info(fmt.Sprintf("Whitelisted %s", normName))
				return true
			}
		}
		return accepted
	}
	return accepted
}

func (w *Watcher) cooldownNotifyLoop() {

	handleIn := func(file *File) {
		w.watcher.Add(file.NormName)

		ce := w.coolingEvents[file.NormName]
		if ce == nil {
			cc := w.conf.Cooldown
			ce, _ = NewCooldownTime(
				fmt.Sprintf("cooldown:%s", file.Name),
				cc.CounterMillis,
				w.onCoolDownDone)
			w.coolingEvents[file.NormName] = ce
		}
		ce.NewData() <- file
	}

	notify := func(we *WatcherEvent, s chan *WatcherEvent) {
		s <- we
	}
	handleOut := func(cdd *CoolDownDone) {
		file := cdd.file
		delete(w.coolingEvents, file.NormName)

		stat, stat_err := os.Stat(file.NormName)
		file.Exists = !os.IsNotExist(stat_err)
		rel, _ := filepath.Rel(w.conf.BaseDir, file.NormName)
		s := FileInfo{
			Version: FILEINFO_FORMAT_VERSION,
			Name:    rel,
			Base:    w.conf.BaseDir,
		}
		if file.Exists && stat != nil {
			file.IsDir = stat.IsDir()
			s = FileInfo{
				Version: FILEINFO_FORMAT_VERSION,
				Name:    rel, // relative name of the file
				Base:    w.conf.BaseDir,
				Size:    stat.Size(),    // length in bytes for regular files; system-dependent for others
				Mode:    stat.Mode(),    // file mode bits
				ModTime: stat.ModTime(), // modification time
				Sys:     stat.Sys(),     // underlying data source (can return nil)
			}
		}

		if !w.acceptedByFilters(file.NormName) {
			w.logger.Info(fmt.Sprintf("Not accepted by filters %s", file.NormName))
			if !file.IsDir || !file.Exists {
				w.watcher.Remove(file.NormName)
			}
			return
		}

		e := &WatcherEvent{
			Version:     EVENT_FORMAT_VERSION,
			UUID:        uuid.New().String(),
			File:        file,
			watcher:     w,
			FirstChange: cdd.timeCreated,
			Time:        cdd.timeUpdated,
			Events:      cdd.counter,
			Stat:        s,
		}

		for i, sub := range w.subs {
			w.logger.Debug(fmt.Sprintf("Nofify Loop %v", i))
			go notify(e, sub)
		}
	}

	for {
		w.logger.Debug("Cooldown Loop")
		select {
		case cdd := <-w.out:
			w.logger.Info(fmt.Sprintf("OUT file: %s", cdd.file.NormName))
			handleOut(cdd)
			continue
		case <-w.done:
			defer w.Done()
			return
		case f := <-w.in:
			w.logger.Info(fmt.Sprintf("IN file: %s", f.NormName))
			handleIn(f)
			continue
		}
	}
}

func (w *Watcher) ForcePushFile(filename string) (err error) {
	w.pushed <- filename
	return nil
}

func (w *Watcher) Conf() WatcherConf {
	return *w.conf
}

func (w *Watcher) UpdateConf(conf WatcherConf) error {
	w.conf.Cooldown.CounterMillis = conf.Cooldown.CounterMillis
	w.conf.RegexWhiteList = conf.RegexWhiteList
	w.conf.RegexBlackList = conf.RegexBlackList

	//init black and white lists
	for _, s := range w.conf.RegexWhiteList {
		r, err := regexp.Compile(s)
		if err != nil {
			return fmt.Errorf("Regex [%s] for white list [%s]: %s", s, conf.BaseDir, err.Error())
		}
		w.filter.regxpWL = append(w.filter.regxpWL, r)
	}

	for _, s := range w.conf.RegexBlackList {
		r, err := regexp.Compile(s)
		if err != nil {
			return fmt.Errorf("Regex [%s] for black list [%s]: %s", s, conf.BaseDir, err.Error())
		}
		w.filter.regxpBL = append(w.filter.regxpBL, r)
	}

	return nil
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

		err := w.SubscribeChan(ch)
		if err != nil {
			fmt.Println(err.Error())
			return
		}

		for {
			w.logger.Debug("Subscription Loop")
			e := <-ch
			t <- true
			f(e)
			<-t
		}
	}()

	return nil
}
