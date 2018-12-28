package dw2s3up

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

type WatcherEvent struct {
	Filename string
	Type     string //  Create, Write, Remove, Rename,  Chmod
	Stat     Stat
	Watcher  *Watcher
	Meta     interface{}
}

type Watcher struct {
	dirName string
	watcher *fsnotify.Watcher
	done    chan bool
	subs    []chan WatcherEvent
	metaMap map[string]interface{}
}

type FileInfo struct {
	Name    string      // base name of the file
	Size    int64       // length in bytes for regular files; system-dependent for others
	Mode    os.FileMode // file mode bits
	ModTime time.Time   // modification time
	IsDir   bool        // abbreviation for Mode().IsDir()
	Sys     interface{} // underlying data source (can return nil)
}

type Watchable interface {
	Dirname() string
	SubscribeChan(ch chan WatcherEvent) error
	SubscribeFunc(f func(e WatcherEvent)) error
	AddFile(filename string) error
	AddFileWithMeta(filename string, meta interface{}) error
	Start() error
	Done() error
}

func NewWatcherEventChan() chan WatcherEvent {
	return make(chan WatcherEvent)
}

func NewWatchable(dirName string) (wtb Watchable, err error) {
	w := &Watcher{dirName: dirName}
	w.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)

	}

	w.done = make(chan bool)
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
	defer w.watcher.Close()
	matches, _ := filepath.Glob(fmt.Sprintf("%s/*", w.Dirname()))
	log.Printf("Glob matches %s: %#v", w.Dirname(), matches)

	for {
		select {
		case <-w.done:
			return nil
		case event, ok := <-w.watcher.Events:
			if ok {
				stat, _ = os.Stat(event.Name)
				e := WatcherEvent{
					Filename: event.Name,
					Type:     event.Op.String(),
					Watcher:  w,
					Meta:     w.metaMap[event.Name],
					Stat: Stat{
						Name    stat.Name(),      // base name of the file
						Size    stat.Size(),       // length in bytes for regular files; system-dependent for others
						Mode    os.FileMode // file mode bits
						ModTime time.Time   // modification time
						IsDir   bool        // abbreviation for Mode().IsDir()
						Sys     interface{} // underlying data source (can return nil)
					}
				}
				
				log.Printf("event: %#v %#v\n", e, event)

				for _, sub := range w.subs {
					go func() {
						sub <- e
					}()
				}
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				if err != nil {
					log.Printf("Err: %s\n", err.Error())
				}
			}
		case <-time.After(time.Minute):
			log.Printf("Timout\n")
		}
	}
}

func (w *Watcher) AddFileWithMeta(filename string, meta interface{}) (err error) {
	if w.metaMap[filename] == nil {
		err = w.watcher.Add(filename)
		if err != nil {
			w.metaMap[filename] = meta
		}
		return err
	}
	return nil
}

func (w *Watcher) AddFile(filename string) (err error) {
	return w.AddFileWithMeta(filename, true)
}

func (w *Watcher) Dirname() string {
	return w.dirName
}

func (w *Watcher) SubscribeChan(ch chan WatcherEvent) error {
	w.subs = append(w.subs, ch)
	return nil
}

func (w *Watcher) SubscribeFunc(f func(e WatcherEvent)) error {
	defer fmt.Println("Finish")

	go func() {
		defer fmt.Println("Yay")
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
	return we.Type == "Create"
}

func (we *WatcherEvent) isWrite() bool {
	return we.Type == "Write"
}

func (we *WatcherEvent) isRemove() bool {
	return we.Type == "Remove"
}

func (we *WatcherEvent) isRename() bool {
	return we.Type == "Rename"
}

func (we *WatcherEvent) isChmod() bool {
	return we.Type == "Chmod"
}
