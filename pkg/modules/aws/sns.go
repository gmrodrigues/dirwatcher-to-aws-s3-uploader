package aws

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sns"
)

type Sns struct {
	topicArn string
	session  *Session
	mSvc     *sns.SNS
}

type SnsPublishPool struct {
	sns           *Sns
	poolSize      int
	timeoutSecond int
	pool          chan bool
	jobs          chan []byte
	stop          chan bool
}

func NewSns(s *Session, topicArn string) (*Sns, error) {
	svc := sns.New(s.mSession)
	return &Sns{
		topicArn: topicArn,
		session:  s,
		mSvc:     svc,
	}, nil
}

func NewSnsPublishPool(sns *Sns, poolSize int, timeoutSecond int) *SnsPublishPool {
	return &SnsPublishPool{
		timeoutSecond: timeoutSecond,
		sns:           sns,
		poolSize:      poolSize,
		pool:          make(chan bool, poolSize),
		jobs:          make(chan []byte, 1024),
		stop:          make(chan bool),
	}
}

func BuildSnsPool(
	aws_access_key_id string,
	aws_secret_access_key string,
	topicArn string,
	poolSize int,
	timeoutSecond int) (*SnsPublishPool, error) {
	region := RegionFromArn(topicArn)
	cred := NewCredentials(aws_access_key_id, aws_secret_access_key)
	sess, err := NewSession(*cred, region)
	if err != nil {
		return nil, err
	}
	sns, err := NewSns(sess, topicArn)
	if err != nil {
		return nil, err
	}
	pool := NewSnsPublishPool(sns, poolSize, timeoutSecond)
	return pool, nil
}

func (s *Sns) Publish(message []byte, timeoutSecond int) error {
	if timeoutSecond == 0 {
		timeoutSecond = 30
	}
	ctx, cancelFn := context.WithTimeout(context.Background(), time.Duration(timeoutSecond)*time.Second)
	defer cancelFn()

	params := &sns.PublishInput{
		Message:  aws.String(string(message)),
		TopicArn: aws.String(s.topicArn),
	}
	_, err := s.mSvc.PublishWithContext(ctx, params)
	return err
}

func (p *SnsPublishPool) Publish(message []byte) {
	p.jobs <- message
}

func (s *SnsPublishPool) TopicArn() string {
	return s.sns.topicArn
}

func (p *SnsPublishPool) PublishSync(message []byte) error {
	return p.sns.Publish(message, p.timeoutSecond)
}

func (p *SnsPublishPool) Start() {
	pub := func(message []byte) {
		p.pool <- true
		for i := 0; i < 5; i++ {
			err := p.PublishSync(message)
			if err == nil {
				break
			}
			time.Sleep(time.Duration(p.timeoutSecond) * time.Second)
		}
		<-p.pool
	}

	for {
		select {
		case <-p.stop:
			break
		case message := <-p.jobs:
			pub(message)
			continue
		}
	}
}
