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
	"time"

	"go.uber.org/zap"

	"github.com/gmrodrigues/feventwatcher/pkg/feventwatcher"
	"github.com/gmrodrigues/feventwatcher/pkg/modules/aws"
	"github.com/gmrodrigues/feventwatcher/pkg/modules/beanstalkd"
	"github.com/gmrodrigues/feventwatcher/pkg/modules/redis"
	"github.com/goinggo/tracelog"
	flags "github.com/jessevdk/go-flags"
	"github.com/jinzhu/configor"
	opentracing "github.com/opentracing/opentracing-go"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/opentracer"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

var logger *zap.Logger

type Options struct {
	LiveConf string `description:"Live config file path. Checked every second for updates. See file-conf.yaml.sample" long:"live-conf-file" short:"c"`
	Watch    struct {
		Basepaths []string `yaml:"Basepath" description:"Basepath on local filesystem to watch events from. Can be set multiple times" short:"w" long:"basepath" env:"BASEPATH"`
		Discover  struct {
			CmdLines      []string `yaml:"CmdLines" description:"A System Shell command line wich will return one basepath per line. Can be set multiple times" short:"f" long:"cmd-line" env:"WATCH_DISCOVER_CMD_LINE"`
			RefreshMillis int64    `yaml:"RefreshMillis" description:"Refresh milliseconds before running discovery command again to discover new basepaths. Disabled => 0" short:"z" long:"refresh-millis" env:"WATCH_DISCOVER_REFRESH_MILLIS" default:"0"`
		} `yaml:"Discover" group:"discover" namespace:"discover"`
		CooldownMillis        int64  `yaml:"CooldownMillis" description:"Cooldown milliseconds before notify watcher events" short:"t" long:"cooldown-millis" env:"COOLDOWN_MILLIS" default:"1000"`
		ResourceNameFileDepth int    `yaml:"ResourceNameFileDepth" description:"Use n levels of depth on file path as resource name" short:"r" long:"resource-name-depth" env:"RESOURCE_DEPTH" default:"4"`
		Meta                  string `yaml:"Meta" description:"Metadata to add to all event message body {\"Meta\":\"...\"} (use this to pass extra data about host, enviroment, etc)" short:"x" long:"meta" env:"WATCH_META"`
		Filter                struct {
			RegexBlackList []string `yaml:"RegexBlackList" description:"Can be set multiple times. Regex to (first priority) blacklist file events based on full normalized name." long:"blacklist" env:"WATCH_FILTER_BLACKLIST"`
			RegexWhiteList []string `yaml:"RegexWhiteList" description:"Can be set multiple times. Regex to (second priority) whitelist file events based on full normalized name." long:"whitelist" env:"WATCH_FILTER_WHITELIST"`
		} `yaml:"Filter" group:"filter" namespace:"filter"`
	} `yaml:"Watch" group:"watch" namespace:"watcher" env-namespace:"WATCH"`
	DataDogAgent struct {
		AgentAddr   string   `description:"DataDog agent address" long:"agent-address" env:"AGENT_ADDR" default:"127.0.0.1:8126"`
		ServiceName string   `description:"DataDog service name" long:"trace-service-name" default:"feventwatcher"`
		Env         string   `description:"DataDog application enviroment" long:"trace-env"`
		Tags        []string `description:"Additional multiple ddtrace tags Ex: --xtag aws_region=virginia" long:"xtag"`
	} `group:"ddagent" namespace:"dd" env-namespace:"DD"`
	Beanstalkd struct {
		Addr            string `description:"Beanstalkd (queue server) host:port (Ex: 127.0.0.1:11300)" long:"addr" env:"BEANSTALKD_ADDR"`
		Queue           string `description:"Beanstalkd queue name)" long:"queue" env:"BEANSTALKD_QUEUE" default:"default"`
		TimeToRunMillis int    `description:"Beanstalkd queue consumer's time to work before job return to queue)" long:"ttr" env:"BEANSTALKD_TTR" default:"600000"`
	} `group:"beanstalkd" namespace:"beanstalkd" env-namespace:"BEANSTALKD"`
	Redis struct {
		Addr     string `description:"Redis server host:port (Ex: localhost:6379)" long:"addr" env:"REDIS_ADDR"`
		Password string `description:"Redis server password" long:"password" env:"REDIS_PASSWORD"`
		QueueKey string `description:"Redis queue name" long:"queue-key" env:"REDIS_QUEUE" default:"fsevents:queue"`
		Db       int    `description:"Redis DB number" long:"db" env:"REDIS_DB" default:"0"`
	} `group:"redis" namespace:"redis" env-namespace:"REDIS"`
	Aws struct {
		AccessKeyId     string `description:"Specifies an AWS access key associated with an IAM user or role." long:"access-key-id" env:"AWS_ACCESS_KEY_ID"`
		SecredAccessKey string `description:"Specifies the secret key associated with the access key. This is essentially the 'password' for the access key." long:"secred-access-key" env:"AWS_SECRET_ACCESS_KEY"`
		Sns             struct {
			TopicArn       []string `description:"AWS SNS topic arn to send events to. Can be set multiple times." long:"topic-arn" env:"AWS_SNS_TOPIC_ARN"`
			PoolSize       int      `description:"AWS SNS concurrent connections for pushing" long:"pool-size" env:"AWS_SNS_POOL_SIZE" default:"25"`
			TimeoutSeconds int      `description:"AWS SNS publish timout" long:"timout-seconds" env:"AWS_SNS_TIMEOUT_SECONDS" default:"5"`
		} `group:"sns" namespace:"sns"`
	} `group:"aws" namespace:"aws" env-namespace:"AWS"`
	Health struct {
		Port int `description:"Listen on port for healh check status report (GET /health)" long:"port" short:"p" env:"HEALTH_PORT"`
	} `group:"health" namespace:"health" env-namespace:"HEALTH"`
	Debug []bool `description:"Debug mode, use multiple times to raise verbosity" short:"d" long:"debug"`
}

func checkMandatoryArgs(o Options) error {
	hasBasePaths := false
	var errMsgs []string
	for _, basepath := range o.Watch.Basepaths {
		_, err := os.Stat(basepath)
		if err != nil {
			errMsgs = append(errMsgs, err.Error())
			continue
		} else {
			hasBasePaths = true
		}
	}
	if len(o.LiveConf) > 0 {
		_, err := loadOptions(o.LiveConf)
		if err != nil {
			errMsgs = append(errMsgs, err.Error())
		} else {
			hasBasePaths = true
		}
	}
	if len(o.Watch.Discover.CmdLines) > 0 {
		hasBasePaths = true
	}
	if !hasBasePaths {
		errMsgs = append(errMsgs, "No basepaths found")
	}
	if len(errMsgs) > 0 {
		return fmt.Errorf("Error checking mandatory args: [%s]", strings.Join(errMsgs, "] ["))
	}
	return nil
}

func loadOptions(confFile string) (*Options, error) {
	opts := &Options{}
	err := configor.Load(opts, confFile)

	if len(opts.Watch.Basepaths) > 0 || len(opts.Watch.Discover.CmdLines) > 0 {
		return opts, nil
	}

	if err != nil {
		return nil, fmt.Errorf("Cannot use conf file %s. Error: %#v", confFile, err.Error())
	}

	return nil, fmt.Errorf("Cannot use conf file %s: no watch diretories found. %#v", confFile, opts)
}

type handleFunc func(pspan opentracing.Span, json []byte)

func main() {
	// runtime.GOMAXPROCS(runtime.NumCPU())
	opts := &Options{}
	_, err := flags.ParseArgs(opts, os.Args)

	if err != nil {
		if _, ok := err.(*flags.Error); !ok {
			fmt.Println(err.Error())
		}
		os.Exit(-1)
	} else if err = checkMandatoryArgs(*opts); err != nil {
		fmt.Println(err.Error())
		fmt.Println("\nPlease refer to --help for help")
		os.Exit(-1)
	}

	logger, _ = zap.NewProduction()
	debug := len(opts.Debug) > 0
	if debug {
		logger, _ = zap.NewDevelopment()
	}

	logger.Info("Args accepted")

	go Health(opts)

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

	bqueues, err := InitBeanstalkd(*opts)
	if err != nil {
		panic(err)
	}

	rqueues, err := InitRedis(*opts)
	if err != nil {
		panic(err)
	}

	squeues, err := InitSns(*opts)
	if err != nil {
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

	snsHandle := func(pspan opentracing.Span, s *aws.SnsPublishPool, json []byte) {
		span := t.StartSpan("aws_sns.producer", opentracing.ChildOf(pspan.Context()))
		defer span.Finish()

		span.SetTag(ext.ResourceName, s.TopicArn())
		span.SetTag("topic.arn", s.TopicArn())

		s.Publish(json)

		span.LogKV("payload", json)
	}

	handleFuncs := make([]handleFunc, 0)
	if len(bqueues) > 0 {
		handleFuncs = append(handleFuncs, func(pspan opentracing.Span, json []byte) {
			for _, bqueue := range bqueues {
				bstalckHandle(pspan, bqueue, json)
			}
		})
	}
	if len(rqueues) > 0 {
		handleFuncs = append(handleFuncs, func(pspan opentracing.Span, json []byte) {
			for _, rqueue := range rqueues {
				redisHandle(pspan, rqueue, json)
			}
		})
	}
	if len(squeues) > 0 {
		handleFuncs = append(handleFuncs, func(pspan opentracing.Span, json []byte) {
			for _, squeue := range squeues {
				snsHandle(pspan, squeue, json)
			}
		})
	}

	/////////////////////////////////
	// Start Watcher

	runtime := NewHealthStatus(opts).Runtime
	handler := func(e *feventwatcher.WatcherEvent) {
		span := t.StartSpan("watch.event.notify")
		defer span.Finish()
		rname := ResourceName(e.Stat.Name, e.Stat.Base, int(opts.Watch.ResourceNameFileDepth))
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
		span.SetTag("stat.Name", e.Stat.Name)
		span.SetTag("stat.Base", e.Stat.Base)
		span.SetTag("stat.ModTime", e.Stat.ModTime)
		span.SetTag("file.watch.base", e.Watcher().Conf().BaseDir)
		span.SetTag("runtime.pid", runtime.Pid)
		span.SetTag("runtime.executable", runtime.Executable)
		span.SetTag("runtime.Wd", runtime.Wd)
		span.SetTag("runtime.hostname", runtime.Hostname)
		span.SetTag("runtime.uptime", uptime())

		e.Meta = opts.Watch.Meta
		payload, _ := json.MarshalIndent(e, "", "  ")
		span.SetTag("payload", string(payload))

		if debug {
			fmt.Printf("\nGot Jobe %s\n%s\n\n", rname, string(payload))
		}

		for _, hf := range handleFuncs {
			hf(span, payload)
		}
	}

	fmt.Print("Starting ...")
	tracelog.Start(tracelog.LevelTrace)

	wConf := NewWatcherConf(opts)
	watcher, err := feventwatcher.NewWatchable(*wConf, logger)
	if err != nil {
		panic(err)
	}
	watcher.SubscribeFunc(handler)

	if opts.LiveConf != "" {
		go func() {
			logger.Info(fmt.Sprintf("Start liconf config from %s", opts.LiveConf))
			var lastStats os.FileInfo
			for {

				if stats, err := os.Stat(opts.LiveConf); err != nil || (stats != nil && lastStats != nil && stats.ModTime() == lastStats.ModTime()) {
					continue
				} else {
					lastStats = stats
				}

				newOpts, err := loadOptions(opts.LiveConf)
				if err != nil {
					fmt.Printf("Error loading LiveConf file [%s]: %s\n", opts.LiveConf, err.Error())
					continue
				}

				logger.Info(fmt.Sprintf("[LiveConf] loading config from %s", opts.LiveConf))
				wConf := NewWatcherConf(newOpts)
				watcher.UpdateConf(*wConf)
				time.Sleep(1 * time.Second)
			}
		}()
	}

	watcher.Start()
	fmt.Println("Done!")
}

func ResourceName(fullpath string, basepath string, depth int) string {
	p := strings.Replace(path.Clean(fullpath), path.Clean(basepath), "", 1)
	pathCrumbs := strings.Split(p, "/")
	maxRange := depth
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
			TimeToRunMillis: uint32(opt.TimeToRunMillis),
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

func InitSns(opts Options) ([]*aws.SnsPublishPool, error) {
	pools := make([]*aws.SnsPublishPool, 0)
	Aws := opts.Aws
	for _, arn := range Aws.Sns.TopicArn {
		pool, err := aws.BuildSnsPool(
			Aws.AccessKeyId, Aws.SecredAccessKey,
			arn, Aws.Sns.PoolSize, Aws.Sns.TimeoutSeconds)
		if err != nil {
			return nil, err
		}
		go pool.Start()
		pools = append(pools, pool)
	}
	return pools, nil
}

func NewWatcherConf(opts *Options) *feventwatcher.WatcherConf {
	return &feventwatcher.WatcherConf{
		BaseDir: opts.Watch.Basepaths,
		Discover: feventwatcher.DiscoverConf{
			CmdLines:      opts.Watch.Discover.CmdLines,
			RefreshMillis: opts.Watch.Discover.RefreshMillis,
		},
		Cooldown: feventwatcher.EventCooldownConf{
			CounterMillis: opts.Watch.CooldownMillis,
		},
		RegexWhiteList: opts.Watch.Filter.RegexWhiteList,
		RegexBlackList: opts.Watch.Filter.RegexBlackList,
	}
}
