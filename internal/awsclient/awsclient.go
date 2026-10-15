// Package awsclient builds AWS SDK clients that can target either real AWS
// or an AWS emulator such as LocalStack.
package awsclient

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Clients bundles the service clients used by driftctl for one region.
type Clients struct {
	EC2 *ec2.Client
	S3  *s3.Client
}

// ResolveEndpoint returns the endpoint to use: the explicit value, or the
// standard AWS_ENDPOINT_URL environment variable, or "" for real AWS.
func ResolveEndpoint(endpoint string) string {
	if endpoint != "" {
		return endpoint
	}
	return os.Getenv("AWS_ENDPOINT_URL")
}

// LoadConfig loads an aws.Config for region. When endpoint is set (LocalStack)
// and no credentials are present in the environment, LocalStack's dummy
// credentials ("test"/"test") are used so no AWS account is required.
func LoadConfig(ctx context.Context, region, endpoint string) (aws.Config, error) {
	endpoint = ResolveEndpoint(endpoint)
	opts := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(region)}
	if endpoint != "" {
		opts = append(opts, awsconfig.WithBaseEndpoint(endpoint))
		if os.Getenv("AWS_ACCESS_KEY_ID") == "" && os.Getenv("AWS_PROFILE") == "" {
			opts = append(opts, awsconfig.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider("test", "test", "")))
		}
	}
	return awsconfig.LoadDefaultConfig(ctx, opts...)
}

// New returns EC2 and S3 clients for region.
func New(ctx context.Context, region, endpoint string) (*Clients, error) {
	cfg, err := LoadConfig(ctx, region, endpoint)
	if err != nil {
		return nil, err
	}
	return &Clients{
		EC2: ec2.NewFromConfig(cfg),
		S3:  NewS3(cfg, endpoint),
	}, nil
}

// NewS3 returns an S3 client. Emulators need path-style addressing
// (http://host:4566/bucket/key) because virtual-host DNS names don't resolve.
func NewS3(cfg aws.Config, endpoint string) *s3.Client {
	custom := ResolveEndpoint(endpoint) != ""
	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		if custom {
			o.UsePathStyle = true
		}
	})
}
