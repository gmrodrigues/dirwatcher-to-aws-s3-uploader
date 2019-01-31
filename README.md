## File watcher event dispatcher

Watches for file system events on a given directory and sends to multiple destinations (topics, queues, etc).

See inotify for implementation details, Tested in windows and linux.

## Usage

```Shell
feventwatcher [OPTIONS]

Application Options:
  -d, --debug                        Debug mode, use multiple times to raise verbosity

watch:
  -w, --watcher.basepath=            Basepath on local filesystem to watch events from [$BASEPATH]
  -t, --watcher.cooldown-millis=     Cooldown milliseconds before notify watcher events (default: 1000) [$COOLDOWN_MILLIS]
  -r, --watcher.resource-name-depth= Use n levels of depth on file path as resource name (default: 3) [$RESOURCE_DEPTH]
  -x, --watcher.meta=                Metadata to add to all event message body {"Meta":"..."} (use this to pass extra data about host, enviroment, etc) [$WATCH_META]

ddagent:
      --dd.agent-address=            DataDog agent address (default: 127.0.0.1:8126) [$AGENT_ADDR]
      --dd.trace-service-name=       DataDog service name (default: feventwatcher)
      --dd.trace-env=                DataDog application enviroment
      --dd.xtag=                     Additional multiple ddtrace tags Ex: --xtag aws_region=virginia

beanstalkd:
      --beanstalkd.addr=             Beanstalkd (queue server) host:port (Ex: 127.0.0.1:11300) [$BEANSTALKD_ADDR]
      --beanstalkd.queue=            Beanstalkd queue name) (default: default) [$BEANSTALKD_QUEUE]
      --beanstalkd.ttr=              Beanstalkd queue consumer's time to work before job return to queue) (default: 600000) [$BEANSTALKD_TTR]

redis:
      --redis.addr=                  Redis server host:port (Ex: localhost:6379) [$REDIS_ADDR]
      --redis.password=              Redis server password [$REDIS_PASSWORD]
      --redis.queue-key=             Redis queue name (default: fsevents:queue) [$REDIS_QUEUE]
      --redis.db=                    Redis DB number (default: 0) [$REDIS_DB]

Help Options:
  -h, --help                         Show this help message

```

## Building

Requires docker and automake to build

Run `make` or `make build` to compile your app.  This will use a Docker image
to build your app, with the current directory volume-mounted into place.  This
will store incremental state for the fastest possible build.  Run `make
all-build` to build for all architectures.

To cross compile set GOOS (go operation system) env var with desired target OS. Ex:
```Shell
GOOS=windows make
GOOS=linux make
... etc ...
```

Run `make container` to build the container image.  It will calculate the image
tag based on the most recent git tag, and whether the repo is "dirty" since
that tag (see `make version`).  Run `make all-container` to build containers
for all architectures.

Run `make push` to push the container image to `REGISTRY`.  Run `make all-push`
to push the container images for all architectures.

Run `make clean` to clean up.
