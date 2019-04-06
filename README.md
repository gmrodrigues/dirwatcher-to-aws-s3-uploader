## File watcher event dispatcher

Watches for file system events on a given directory and sends to multiple destinations (topics, queues, etc).

## Download

Check binary multiplataform releases:
   - [Downloads](https://github.com/gmrodrigues/feventwatcher/releases)

## Health check

You can set a tcp port and use it for health checking:

```Shell

feventwatcher [OPTIONS] -p 9123 # run command with options then set health check listening on host port 9123
curl -XGET http://localhost:9123/health -I # check if its running
> HTTP/1.1 200 OK
> Content-Type: application/json
> Date: Thu, 31 Jan 2019 09:52:08 GMT
> Content-Length: 693
```

## Usage

```Shell
$ bin/amd64/feventwatcher -h

Usage:
  feventwatcher [OPTIONS]

Application Options:
  -c, --live-conf-file=              Live config file path. Checked every second for updates. See file-conf.yaml.sample
  -d, --debug                        Debug mode, use multiple times to raise verbosity

watch:
  -w, --watcher.basepath=            Basepath on local filesystem to watch events from. Can be set multiple times [$BASEPATH]
  -l, --watcher.sublevels-depth=     Watch subdirectories n levels of depth from Basepath (default: 0) [$SUBLEVELS_DEPTH]
  -t, --watcher.cooldown-millis=     Cooldown milliseconds before notify watcher events (default: 1000) [$COOLDOWN_MILLIS]
  -r, --watcher.resource-name-depth= Use n levels of depth on file path as resource name (default: 4) [$RESOURCE_DEPTH]
  -x, --watcher.meta=                Metadata to add to all event message body {"Meta":"..."} (use this to pass extra data about host, enviroment,
                                     etc) [$WATCH_META]

filter:
      --watcher.filter.blacklist=    Can be set multiple times. Regex to (first priority) blacklist file events based on full normalized name.
                                     [$WATCH_FILTER_BLACKLIST]
      --watcher.filter.whitelist=    Can be set multiple times. Regex to (second priority) whitelist file events based on full normalized name.
                                     [$WATCH_FILTER_WHITELIST]

ddagent:
      --dd.agent-address=            DataDog agent address (default: 127.0.0.1:8126) [$AGENT_ADDR]
      --dd.trace-service-name=       DataDog service name (default: feventwatcher)
      --dd.trace-env=                DataDog application enviroment
      --dd.xtag=                     Additional multiple ddtrace tags Ex: --xtag aws_region=virginia

beanstalkd:
      --beanstalkd.addr=             Beanstalkd (queue server) host:port (Ex: 127.0.0.1:11300) [$BEANSTALKD_ADDR]
      --beanstalkd.queue=            Beanstalkd queue name) (default: default) [$BEANSTALKD_QUEUE]
      --beanstalkd.ttr=              Beanstalkd queue consumer\'s time to work before job return to queue) (default: 600000) [$BEANSTALKD_TTR]

redis:
      --redis.addr=                  Redis server host:port (Ex: localhost:6379) [$REDIS_ADDR]
      --redis.password=              Redis server password [$REDIS_PASSWORD]
      --redis.queue-key=             Redis queue name (default: fsevents:queue) [$REDIS_QUEUE]
      --redis.db=                    Redis DB number (default: 0) [$REDIS_DB]

aws:
      --aws.access-key-id=           Specifies an AWS access key associated with an IAM user or role. [$AWS_ACCESS_KEY_ID]
      --aws.secred-access-key=       Specifies the secret key associated with the access key. This is essentially the 'password' for the access key.
                                     [$AWS_SECRET_ACCESS_KEY]

sns:
      --aws.sns.topic-arn=           AWS SNS topic arn to send events to. Can be set multiple times. [$AWS_SNS_TOPIC_ARN]
      --aws.sns.pool-size=           AWS SNS concurrent connections for pushing (default: 25) [$AWS_SNS_POOL_SIZE]
      --aws.sns.timout-seconds=      AWS SNS publish timout (default: 5) [$AWS_SNS_TIMEOUT_SECONDS]

health:
  -p, --health.port=                 Listen on port for healh check status report (GET /health) [$HEALTH_PORT]

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

## Create Windows Service

We recoment using https://nssm.cc/
 - [Download](https://nssm.cc/download)
 - [Usage](https://nssm.cc/usage)
