package dw2s3up

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
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

type WatcherEvent struct {
	Type    string //  Create, Write, Remove, Rename,  Chmod, Pushed
	File    *File
	Stat    FileInfo
	Watcher *Watcher
}

type Watcher struct {
	dirName string
	watcher *fsnotify.Watcher
	done    chan bool
	subs    []chan WatcherEvent
	pushed  chan File
	metaMap map[string]interface{}
	logger  *zap.Logger
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
	Dirname() string
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

func NewWatchable(dirName string) (wtb Watchable, err error) {
	w := &Watcher{dirName: dirName}
	w.watcher, err = fsnotify.NewWatcher()
	w.logger, _ = zap.NewProduction()
	defer w.logger.Sync()
	if err != nil {
		w.logger.Fatal(err.Error())

	}

	w.done = make(chan bool)
	w.pushed = make(chan File)
	w.subs = make([]chan WatcherEvent, 1)
	w.metaMap = make(map[string]interface{})

	err = w.watcher.Add(dirName)
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
	dirGlobs, _ := filepath.Glob(fmt.Sprintf("%s/*", w.Dirname()))
	w.logger.Info(fmt.Sprintf("Glob matches %s: %#v\n", w.Dirname(), dirGlobs))

	for _, f := range dirGlobs {
		go func() { w.pushed <- File{Name: f} }()
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
		e := WatcherEvent{
			File:    &file,
			Type:    op,
			Watcher: w,
			Stat:    s,
		}
		e.tick()

		for _, sub := range w.subs {
			go func() {
				sub <- e
			}()
		}

		json, _ := json.MarshalIndent(e, "", "  ")
		w.logger.Info(fmt.Sprintf("\n%s\n", json))

		return e, nil
	}

	for {
		select {
		case <-w.done:
			return nil
		case event, ok := <-w.watcher.Events:
			if ok {
				e, _ := handle(File{Name: event.Name}, fsNotfyOpToStr[event.Op])
				w.logger.Info(fmt.Sprintf("\nevent: %#v %#v \n", e, event))
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				if err != nil {
					w.logger.Error(fmt.Sprintf("Err: %s\n", err.Error()))
				}
			}
		case file := <-w.pushed:
			e, _ := handle(file, PUSHED)
			w.logger.Info(fmt.Sprintf("\npushed: %#v %#v \n", e, file))
		case <-time.After(time.Minute):
			w.logger.Info(fmt.Sprintf("\nTimout\n"))
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

func (w *Watcher) Dirname() string {
	return w.dirName
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
func (we *WatcherEvent) isCreate() bool {
	return we.Type == CREATE
}

func (we *WatcherEvent) isWrite() bool {
	return we.Type == WRITE
}

func (we *WatcherEvent) isRemove() bool {
	return we.Type == REMOVE
}

func (we *WatcherEvent) isRename() bool {
	return we.Type == RENAME
}

func (we *WatcherEvent) isChmod() bool {
	return we.Type == CHMOD
}

func (we *WatcherEvent) isPushed() bool {
	return we.Type == PUSHED
}

func (we *WatcherEvent) tick() bool {
	if we.isCreate() || we.isPushed() {
		we.Watcher.AddFile(*we.File)
	} else if !we.Stat.Exists {
		we.Watcher.ForgetFile(we.File.Name)
	}
	return true
}
