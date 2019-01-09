package feventwatcher_test

import (
	"fmt"
	"testing"

	"github.com/gmrodrigues/feventwatcher/pkg/feventwatcher"
	"github.com/goinggo/tracelog"
)

func TestTimeConsuming(t *testing.T) {
	tracelog.Start(tracelog.LevelTrace)
	dirname := "test"
	//fmt.Printf("File %s\n", dirname)
	conf := feventwatcher.WatcherConf{
		BaseDir: dirname,
		Cooldown: feventwatcher.EventCooldownConf{
			CounterMillis: 1000,
		},
	}
	w, err := feventwatcher.NewWatchable(conf)
	if err != nil {
		fmt.Printf("Err: %s", err.Error())
		return
	}

	fmt.Println("Subscribing ...")
	w.SubscribeFunc(func(e feventwatcher.WatcherEvent) {
		//fmt.Printf("File %s: %s", e.File.Name, e.Types)
	})

	fmt.Println("Starting ...")
	w.Start()
	fmt.Println("Done!")
}
