package aws

import (
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go/aws/session"
)

type Credentials struct {
	aws_access_key_id     string
	aws_secret_access_key string
	mCreds                *credentials.Credentials
}

type Session struct {
	creds    *Credentials
	mSession *session.Session
}

func NewCredentials(aws_access_key_id string, aws_secret_access_key string) *Credentials {
	mCreds := credentials.NewChainCredentials(
		[]credentials.Provider{
			&credentials.StaticProvider{credentials.Value{
				AccessKeyID:     aws_access_key_id,
				SecretAccessKey: aws_secret_access_key,
			}},
			&credentials.EnvProvider{},
			&credentials.SharedCredentialsProvider{},
			&ec2rolecreds.EC2RoleProvider{},
		})
	c := &Credentials{
		aws_access_key_id:     aws_access_key_id,
		aws_secret_access_key: aws_secret_access_key,
		mCreds:                mCreds,
	}
	return c
}

func NewSession(creds Credentials, region string) (*Session, error) {
	s, err := session.NewSessionWithOptions(session.Options{
		Config: *aws.NewConfig().WithRegion(region).WithCredentials(creds.mCreds),
	})
	if err != nil {
		return nil, err
	}
	return &Session{
		creds:    &creds,
		mSession: s,
	}, nil
}

func RegionFromArn(arn string) (region string) {
	return strings.Split(arn, ":")[3]
}
