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
	"sync"

	"github.com/gmrodrigues/feventwatcher/pkg/feventwatcher"
	"github.com/gmrodrigues/feventwatcher/pkg/modules/beanstalkd"
	"github.com/gmrodrigues/feventwatcher/pkg/modules/redis"
	"github.com/goinggo/tracelog"
	flags "github.com/jessevdk/go-flags"
	opentracing "github.com/opentracing/opentracing-go"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/opentracer"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

type Options struct {
	Watch struct {
		Basepath              []string `description:"Basepath on local filesystem to watch events from. Can be set multiple times" short:"w" long:"basepath" env:"BASEPATH" required:"true"`
		CooldownMillis        uint64   `description:"Cooldown milliseconds before notify watcher events" short:"t" long:"cooldown-millis" env:"COOLDOWN_MILLIS" default:"1000"`
		ResourceNameFileDepth uint8    `description:"Use n levels of depth on file path as resource name" short:"r" long:"resource-name-depth" env:"RESOURCE_DEPTH" default:"4"`
		Meta                  string   `description:"Metadata to add to all event message body {\"Meta\":\"...\"} (use this to pass extra data about host, enviroment, etc)" short:"x" long:"meta" env:"WATCH_META"`
	} `group:"watch" namespace:"watcher" env-namespace:"WATCH"`
	DataDogAgent struct {
		AgentAddr   string   `description:"DataDog agent address" long:"agent-address" env:"AGENT_ADDR" default:"127.0.0.1:8126"`
		ServiceName string   `description:"DataDog service name" long:"trace-service-name" default:"feventwatcher"`
		Env         string   `description:"DataDog application enviroment" long:"trace-env"`
		Tags        []string `description:"Additional multiple ddtrace tags Ex: --xtag aws_region=virginia" long:"xtag"`
	} `group:"ddagent" namespace:"dd" env-namespace:"DD"`
	Beanstalkd struct {
		Addr            string `description:"Beanstalkd (queue server) host:port (Ex: 127.0.0.1:11300)" long:"addr" env:"BEANSTALKD_ADDR"`
		Queue           string `description:"Beanstalkd queue name)" long:"queue" env:"BEANSTALKD_QUEUE" default:"default"`
		TimeToRunMillis uint32 `description:"Beanstalkd queue consumer's time to work before job return to queue)" long:"ttr" env:"BEANSTALKD_TTR" default:"600000"`
	} `group:"beanstalkd" namespace:"beanstalkd" env-namespace:"BEANSTALKD"`
	Redis struct {
		Addr     string `description:"Redis server host:port (Ex: localhost:6379)" long:"addr" env:"REDIS_ADDR"`
		Password string `description:"Redis server password" long:"password" env:"REDIS_PASSWORD"`
		QueueKey string `description:"Redis queue name" long:"queue-key" env:"REDIS_QUEUE" default:"fsevents:queue"`
		Db       int    `description:"Redis DB number" long:"db" env:"REDIS_DB" default:"0"`
	} `group:"redis" namespace:"redis" env-namespace:"REDIS"`
	Health struct {
		Port int `description:"Listen on port for healh check status report (GET /health)" long:"port" short:"p" env:"HEALTH_PORT"`
	} `group:"health" namespace:"health" env-namespace:"HEALTH"`
	Debug []bool `description:"Debug mode, use multiple times to raise verbosity" short:"d" long:"debug"`
}

func main() {

	opts := Options{}
	_, err := flags.ParseArgs(&opts, os.Args)

	if err != nil {
		fmt.Println(err.Error())
		os.Exit(-1)
	}

	go Health(&opts)

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

	//////////////////////////////////
	// Beanstalkd

	bqueues, err := InitBeanstalkd(opts)
	if err != nil {
		//fmt.Fprintf(os.Stderr, "Beanstalkd Client error: %s", err.Error())
		panic(err)
	}

	rqueues, err := InitRedis(opts)
	if err != nil {
		//fmt.Fprintf(os.Stderr, "Beanstalkd Client error: %s", err.Error())
		panic(err)
	}

	bstalckHandle := func(pspan opentracing.Span, q beanstalkd.QueueHandler, json []byte) {
		span := t.StartSpan("beanstalk.producer", opentracing.ChildOf(pspan.Context()))
		defer span.Finish()

		span.SetTag(ext.ResourceName, q.Conf().Name)
		span.SetTag("queue", q.Conf().Name)
		span.SetTag("host", q.Conn().Addr())

		id, err := q.Put(json)
		if err != nil {
			span.LogKV("error", err)
			return
		}

		span.LogKV("id", id)
		span.LogKV("payload", json)
	}

	redisHandle := func(pspan opentracing.Span, q redis.Queue, json []byte) {
		span := t.StartSpan("redis.producer", opentracing.ChildOf(pspan.Context()))
		defer span.Finish()

		span.SetTag(ext.ResourceName, q.Key())
		span.SetTag("queue", q.Key())
		span.SetTag("host", q.Conf().Addr)

		err := q.PushMessage(json)
		if err != nil {
			span.LogKV("error", err)
			return
		}

		span.LogKV("payload", json)
	}

	/////////////////////////////////
	// Start Watcher

	runtime := NewHealthStatus(&opts).Runtime
	handler := func(e *feventwatcher.WatcherEvent) {
		span := t.StartSpan("watch.event.notify")
		defer span.Finish()
		rname := ResourceName(e.File.NormName, e.Watcher().Conf().BaseDir, int(opts.Watch.ResourceNameFileDepth))
		span.SetTag(ext.ResourceName, rname)
		span.SetTag("event.uuid", e.UUID)
		span.SetTag("event.version", e.Version)
		span.SetTag("event.timestamp", e.Time)
		span.SetTag("file.name", e.File.Name)
		span.SetTag("file.norm", e.File.NormName)
		span.SetTag("file.size", e.Stat.Size)
		span.SetTag("file.exists", e.File.Exists)
		span.SetTag("file.is_dir", e.File.IsDir)
		span.SetTag("file.forced", e.File.Forced)
		span.SetTag("file.watch.base", e.Watcher().Conf().BaseDir)
		span.SetTag("runtime.pid", runtime.Pid)
		span.SetTag("runtime.executable", runtime.Executable)
		span.SetTag("runtime.Wd", runtime.Wd)
		span.SetTag("runtime.hostname", runtime.Hostname)
		span.SetTag("runtime.uptime", uptime())

		e.Meta = opts.Watch.Meta
		payload, _ := json.MarshalIndent(e, "", "  ")
		span.SetTag("payload", string(payload))

		fmt.Printf("\nGot Jobe %s\n%s\n\n", rname, string(payload))

		for _, rqueue := range rqueues {
			redisHandle(span, rqueue, payload)
		}

		for _, bqueue := range bqueues {
			bstalckHandle(span, bqueue, payload)
		}
	}

	fmt.Print("Starting ...")
	tracelog.Start(tracelog.LevelTrace)
	var wg sync.WaitGroup
	for _, basepath := range opts.Watch.Basepath {
		//fmt.Printf("File %s\n", dirname)
		conf := feventwatcher.WatcherConf{
			BaseDir: basepath,
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
		w.SubscribeFunc(handler)
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.Start()
		}()
	}

	wg.Wait()
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

func InitBeanstalkd(opts Options) ([]beanstalkd.QueueHandler, error) {
	opt := opts.Beanstalkd
	if len(opt.Addr) > 0 {
		conn, err := beanstalkd.NewConnection(opt.Addr)
		if err != nil {
			return nil, err
		}
		qconf := beanstalkd.QueueConf{
			Name:            opt.Queue,
			TimeToRunMillis: opt.TimeToRunMillis,
		}
		qhand, err := conn.NewQueueHandler(qconf)
		if err != nil {
			return nil, err
		}
		return []beanstalkd.QueueHandler{qhand}, nil
	}

	return []beanstalkd.QueueHandler{}, nil
}

func InitRedis(opts Options) ([]redis.Queue, error) {
	opt := opts.Redis
	if len(opt.Addr) > 0 {
		conf := redis.ClientConf{
			Addr:     opt.Addr,
			Password: opt.Password,
			DB:       opt.Db,
		}
		c, err := redis.NewClient(conf)
		if err != nil {
			return nil, err
		}
		q, err := c.NewQueue(opt.QueueKey)
		if err != nil {
			return nil, err
		}
		return []redis.Queue{q}, nil
	}

	return []redis.Queue{}, nil
}
