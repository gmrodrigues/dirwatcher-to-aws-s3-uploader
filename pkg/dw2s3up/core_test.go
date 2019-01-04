package dw2s3up_test

import (
	"fmt"
	"testing"

	"github.com/gmrodrigues/dirwatcher-to-aws-s3-uploader/pkg/dw2s3up"
	"github.com/goinggo/tracelog"
)

func TestTimeConsuming(t *testing.T) {
	tracelog.Start(tracelog.LevelTrace)
	dirname := "test"
	fmt.Printf("File %s\n", dirname)
	w, err := dw2s3up.NewWatchable(dirname)
	if err != nil {
		fmt.Printf("Err: %s", err.Error())
		return
	}

	fmt.Println("Subscribing ...")
	w.SubscribeFunc(func(e dw2s3up.WatcherEvent) {
		fmt.Printf("File %s: %s", e.File.Name, e.Type)
	})

	fmt.Println("Starting ...")
	w.Start()
	fmt.Println("Done!")
}