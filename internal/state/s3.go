package state

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/malekachouri/terraform-drift-detector/internal/awsclient"
	"github.com/malekachouri/terraform-drift-detector/internal/model"
)

func readS3(ctx context.Context, cfg model.StateConfig) ([]byte, error) {
	if cfg.Bucket == "" || cfg.Key == "" {
		return nil, fmt.Errorf("s3 backend requires bucket and key")
	}

	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}

	awsCfg, err := awsclient.LoadConfig(ctx, region, cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	client := awsclient.NewS3(awsCfg, cfg.Endpoint)
	out, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(cfg.Bucket),
		Key:    aws.String(cfg.Key),
	})
	if err != nil {
		return nil, fmt.Errorf("get s3 object s3://%s/%s: %w", cfg.Bucket, cfg.Key, err)
	}
	defer out.Body.Close()

	return io.ReadAll(out.Body)
}
