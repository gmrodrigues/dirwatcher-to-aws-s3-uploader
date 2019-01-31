package redis

import (
	"github.com/go-redis/redis"
)

type ClientConf struct {
	Addr     string `default:"localhost:6379"`
	Password string
	DB       int
}

type client struct {
	mClient *redis.Client
	conf *ClientConf
}

type queue struct {
	client *client
	key    string
}

type Client interface {
	NewQueue(key string) (Queue, error)
}

type Queue interface {
	PushMessage(message []byte) error
	Key() string
	Conf() *ClientConf
}

func NewClient(conf ClientConf) (Client, error){
	c := redis.NewClient(&redis.Options{
		Addr:     conf.Addr,
		Password: conf.Password,
		DB:       conf.DB,
	})

	_, err := c.Ping().Result()
	if err != nil {
		return nil, err
	}
	return &client{mClient: c, conf: &conf}, err
}

func (c *client) NewQueue(key string) (Queue, error) {
	return &queue{client: c, key: key}, nil
}

func (q *queue) Conf() *ClientConf {
	return q.client.conf
}

func (q *queue) Key() string {
	return q.key
}

func (q *queue) PushMessage(message []byte) error {
	return q.client.mClient.LPush(q.key, message).Err()
}