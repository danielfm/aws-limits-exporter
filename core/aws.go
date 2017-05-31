package core

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
)

// NewSession returns a new client session for interacting with the AWS API.
// The credentials are retrieved from environment variables or from the
// `~/.aws/credentials` file.
func NewSession(region string) *session.Session {
	creds := credentials.NewChainCredentials(
		[]credentials.Provider{
			&credentials.EnvProvider{},
			&credentials.SharedCredentialsProvider{},
		})

	awsConfig := aws.NewConfig()
	awsConfig.WithCredentials(creds)
	awsConfig.WithRegion(region)

	return session.New(awsConfig)
}
