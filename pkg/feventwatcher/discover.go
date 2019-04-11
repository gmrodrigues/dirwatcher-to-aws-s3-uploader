package feventwatcher

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type DiscoverConf struct {
	CmdLines      []string
	RefreshMillis int64
}

type DiscoverCmdOutput struct {
	linesout []string
	md5sum   string
}

func exe_cmd(cmd string) (DiscoverCmdOutput, error) {
	parts := strings.Fields(cmd)
	head := parts[0]
	parts = parts[1:len(parts)]

	fmt.Printf("command is %s [%s] %#v\n", cmd, head, parts)
	c := exec.Command(head, parts...)
	var out bytes.Buffer
	var stderr bytes.Buffer
	c.Stdout = &out
	c.Stderr = &stderr
	err := c.Run()
	if err != nil {
		fmt.Println(fmt.Sprint(err) + ": " + stderr.String())
		return DiscoverCmdOutput{}, err
	}
	fmt.Printf("Out is: %s", out)

	scanner := bufio.NewScanner(bytes.NewReader(out.Bytes()))
	list := make([]string, 0)
	for scanner.Scan() {
		list = append(list, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return DiscoverCmdOutput{}, err
	}
	return DiscoverCmdOutput{
		linesout: list,
		md5sum:   fmt.Sprintf("%x", md5.Sum(out.Bytes())),
	}, nil
}

func (w *Watcher) addCmdOuputToBasefiles() {

	runCmd := func(cmd string, wg *sync.WaitGroup) {
		wg.Add(1)
		defer wg.Done()

		w.logger.Info(fmt.Sprintf("[FindCmd] Start Running CMD: %s", cmd))
		defer w.logger.Info(fmt.Sprintf("[FindCmd] Finish Running CMD: %s", cmd))

		out, err := exe_cmd(cmd)
		if err == nil {
			for _, basepath := range out.linesout {
				w.AddBaseFile(basepath)
			}
		} else {
			w.logger.Error(fmt.Sprintf("[FindCmd] Error Running CMD $(%s): %s", cmd, err.Error()))
		}
	}

	wg := &sync.WaitGroup{}

	for _, cmd := range w.conf.Discover.CmdLines {
		go runCmd(cmd, wg)
	}

	wg.Wait()
}

func (w *Watcher) discoverCmdLoop() {

	w.addCmdOuputToBasefiles()

	if w.conf.Discover.RefreshMillis > 0 {
		for {
			select {
			case <-w.done:
				defer w.Done()
				return
			case <-time.After(time.Duration(w.conf.Discover.RefreshMillis) * time.Millisecond):
				w.addCmdOuputToBasefiles()
			}
		}
	}
}
