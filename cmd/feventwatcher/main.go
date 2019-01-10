/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/gmrodrigues/feventwatcher/pkg/feventwatcher"
	"github.com/goinggo/tracelog"
	flags "github.com/jessevdk/go-flags"
	opentracing "github.com/opentracing/opentracing-go"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/opentracer"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

type Options struct {
	Watch struct {
		Basepath              string `description:"Basepath on local filesystem to watch events from" short:"w" long:"basepath" env:"BASEPATH" required:"true"`
		CooldownMillis        uint64 `description:"Cooldown milliseconds before notify watcher events" short:"t" long:"cooldown-millis" env:"COOLDOWN_MILLIS" default:"1000"`
		ResourceNameFileDepth uint8  `description:"Use n levels of depth on file path as resource name" short:"r" long:"resource-name-depth" env:"RESOURCE_DEPTH" default:"3"`
	} `group:"watch" namespace:"watcher" env-namespace:"WATCH"`
	DataDogAgent struct {
		AgentAddr   string   `description:"DataDog agent address" long:"agent-address" env:"AGENT_ADDR" default:"127.0.0.1:8126"`
		ServiceName string   `description:"DataDog service name" long:"trace-service-name" default:"feventwatcher"`
		Env         string   `description:"DataDog application enviroment" long:"trace-env"`
		Tags        []string `description:"Additional multiple ddtrace tags Ex: --xtag aws_region=virginia" long:"xtag"`
	} `group:"ddagent" namespace:"dd" env-namespace:"DD"`
	Debug []bool `description:"Debug mode, use multiple times to raise verbosity" short:"d" long:"debug"`
}

func main() {

	opts := Options{}
	_, err := flags.ParseArgs(&opts, os.Args)

	if err != nil {
		panic(err)
	}

	tstartops := []tracer.StartOption{
		tracer.WithAgentAddr(opts.DataDogAgent.AgentAddr),
		tracer.WithServiceName(opts.DataDogAgent.ServiceName),
		tracer.WithDebugMode(len(opts.Debug) > 0),
	}

	for _, tag := range opts.DataDogAgent.Tags {
		v := strings.SplitN(tag, "=", 0)
		tracer.WithGlobalTag(v[0], v[1])
	}

	t := opentracer.New(tstartops...)
	defer tracer.Stop()
	opentracing.SetGlobalTracer(t)

	// Start Watcher

	tracelog.Start(tracelog.LevelTrace)
	//fmt.Printf("File %s\n", dirname)
	conf := feventwatcher.WatcherConf{
		BaseDir: opts.Watch.Basepath,
		Cooldown: feventwatcher.EventCooldownConf{
			CounterMillis: opts.Watch.CooldownMillis,
		},
	}

	w, err := feventwatcher.NewWatchable(conf)
	if err != nil {
		fmt.Printf("Failed to start watcher: %s", err.Error())
		return
	}

	fmt.Println("Starting watcher with confs: %#v", conf)
	w.SubscribeFunc(func(e feventwatcher.WatcherEvent) {
		span := tracer.StartSpan("watch.event.notify")
		defer span.Finish()

		rname := ResourceName(e.File.Name, opts.Watch.Basepath, int(opts.Watch.ResourceNameFileDepth))
		span.SetTag(ext.ResourceName, rname)

		json, _ := json.MarshalIndent(e, "", "  ")
		span.SetTag("received.json", string(json))

		fmt.Printf("\nGot Jobe %s\n%s\n\n", rname, string(json))
	})

	fmt.Println("Starting ...")
	w.Start()
	fmt.Println("Done!")
	fmt.Println("Done!")
}

func ResourceName(fullpath string, basepath string, depth int) string {
	p := strings.Replace(path.Clean(fullpath), path.Clean(basepath), path.Base(basepath), 1)
	pathCrumbs := strings.Split(p, string(os.PathSeparator))
	maxRange := 3
	if len(pathCrumbs) < maxRange {
		maxRange = len(pathCrumbs)
	}

	return strings.Join(pathCrumbs[:maxRange], "/")
}
