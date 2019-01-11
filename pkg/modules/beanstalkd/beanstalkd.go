package beanstalkd

import (
	"fmt"
	"sync"
	"time"

	beanstalk "github.com/beanstalkd/go-beanstalk"
)

// https://github.com/beanstalkd/beanstalkd/blob/master/doc/protocol.txt#L129
//
// - <pri> is an integer < 2**32. Jobs with smaller priority values will be
//    scheduled before jobs with larger priorities. The most urgent priority is 0;
//    the least urgent priority is 4,294,967,295.

//  - <delay> is an integer number of seconds to wait before putting the job in
//    the ready queue. The job will be in the "delayed" state during this time.

//  - <ttr> -- time to run -- is an integer number of seconds to allow a worker
//    to run this job. This time is counted from the moment a worker reserves
//    this job. If the worker does not delete, release, or bury the job within
//    <ttr> seconds, the job will time out and the server will release the job.
//    The minimum ttr is 1. If the client sends 0, the server will silently
//    increase the ttr to 1.
type QueueConf struct {
	Name            string
	Priority        uint32
	DelayMillis     uint32
	TimeToRunMillis uint32
}

type conn struct {
	bConn *beanstalk.Conn
	addr  string
}

type queue struct {
	bTube    *beanstalk.Tube
	bTubeSet *beanstalk.TubeSet
	conn     *conn
	conf     *QueueConf
	mutex    sync.Mutex
}

type Connection interface {
	Addr() string
	Close() error
	ListQueues() ([]string, error)
	NewQueueHandler(conf QueueConf) (QueueHandler, error)
}

type QueueHandler interface {
	Name() string
	Conn() Connection
	Conf() QueueConf
	Put(body []byte) (id uint64, err error)
	Reserve() (id uint64, body []byte, err error)
	Delete(id uint64) error
	Peek(id uint64) (body []byte, err error)
	PeekReady() (id uint64, body []byte, err error)
	Pause(duration time.Duration) error
	Stats() (map[string]string, error)
	StatsJob(id uint64) (map[string]string, error)
}

func NewConnection(addr string) (c Connection, err error) {
	return NewConnectionTimeout(addr, time.Duration(10)*time.Second)
}

func NewConnectionTimeout(addr string, timeout time.Duration) (c Connection, err error) {
	conn := &conn{addr: addr}
	conn.bConn, err = beanstalk.DialTimeout("tcp", conn.addr, timeout)
	if err != err {
		return nil, err
	}

	return conn, nil
}

func (c *conn) NewQueueHandler(conf QueueConf) (QueueHandler, error) {
	copy := QueueConf{
		Name:            conf.Name,
		Priority:        10000000,
		DelayMillis:     conf.DelayMillis,
		TimeToRunMillis: 10 * 60 * 1000,
	}

	if conf.Priority > 0 {
		copy.Priority = conf.Priority
	}
	if conf.DelayMillis > 0 {
		copy.DelayMillis = conf.DelayMillis
	}
	if conf.TimeToRunMillis > 0 {
		copy.TimeToRunMillis = conf.TimeToRunMillis
	}

	q := &queue{
		conn: c,
		conf: &copy,
	}

	return q, nil
}

func (c *conn) Addr() string {
	return c.addr
}

func (c *conn) ListQueues() ([]string, error) {
	return c.bConn.ListTubes()
}

func (c *conn) Close() error {
	return c.bConn.Close()
}

func (q *queue) Name() string {
	return q.conf.Name
}

func (q *queue) Conn() Connection {
	return q.conn
}

func (q *queue) Conf() QueueConf {
	return *q.conf
}

func (q *queue) Put(body []byte) (id uint64, err error) {
	fmt.Printf("\nPut queue\n")
	defer fmt.Printf("\nPut queue ok\n")

	q.mutex.Lock()
	if q.bTube == nil {
		q.bTube = &beanstalk.Tube{
			Conn: q.conn.bConn,
			Name: q.conf.Name,
		}
	}
	q.mutex.Unlock()

	return q.bTube.Put(body,
		q.conf.Priority,
		time.Duration(q.conf.DelayMillis)*time.Millisecond,
		time.Duration(q.conf.TimeToRunMillis)*time.Millisecond)
}

func (q *queue) Reserve() (id uint64, body []byte, err error) {
	q.mutex.Lock()
	if q.bTube == nil {
		q.bTubeSet = &beanstalk.TubeSet{
			Conn: q.conn.bConn,
			Name: map[string]bool{q.conf.Name: true},
		}
	}
	q.mutex.Unlock()

	return q.bTubeSet.Reserve(time.Duration(q.conf.TimeToRunMillis) * time.Millisecond)
}

func (q *queue) Peek(id uint64) (body []byte, err error) {
	return q.conn.bConn.Peek(id)
}

func (q *queue) PeekReady() (id uint64, body []byte, err error) {
	return q.bTube.PeekReady()
}

func (q *queue) Pause(duration time.Duration) error {
	return q.bTube.Pause(duration)
}

func (q *queue) Delete(id uint64) error {
	return q.conn.bConn.Delete(id)
}

func (q *queue) Stats() (map[string]string, error) {
	return q.bTube.Stats()
}

func (q *queue) StatsJob(id uint64) (map[string]string, error) {
	return q.conn.bConn.StatsJob(id)
}
