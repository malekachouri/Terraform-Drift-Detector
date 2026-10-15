package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/malekachouri/terraform-drift-detector/internal/awsclient"
	"github.com/malekachouri/terraform-drift-detector/internal/model"
)

func fetchBuckets(ctx context.Context, homeRegion, endpoint string, regions []string) ([]model.Resource, error) {
	cfg, err := awsclient.LoadConfig(ctx, homeRegion, endpoint)
	if err != nil {
		return nil, err
	}
	client := awsclient.NewS3(cfg, endpoint)

	wanted := make(map[string]bool, len(regions))
	for _, r := range regions {
		wanted[r] = true
	}

	var resources []model.Resource
	pager := s3.NewListBucketsPaginator(client, &s3.ListBucketsInput{})
	for pager.HasMorePages() {
		out, err := pager.NextPage(ctx)
		if err != nil {
			return resources, fmt.Errorf("list buckets: %w", err)
		}
		for _, b := range out.Buckets {
			bucket := aws.ToString(b.Name)
			region, err := bucketRegion(ctx, client, bucket)
			if err != nil {
				return resources, err
			}
			if !wanted[region] {
				continue
			}

			tags := map[string]string{}
			// GetBucketTagging fails with NoSuchTagSet when there are no tags.
			if tagsOut, err := client.GetBucketTagging(ctx, &s3.GetBucketTaggingInput{Bucket: aws.String(bucket)}); err == nil {
				for _, t := range tagsOut.TagSet {
					tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
				}
			}

			verOut, err := client.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{Bucket: aws.String(bucket)})
			if err != nil {
				return resources, fmt.Errorf("bucket %s versioning: %w", bucket, err)
			}
			attrs := map[string]any{
				"versioning_enabled": verOut.Status == "Enabled",
			}

			name := tags["Name"]
			resources = append(resources, baseResource("aws_s3_bucket", bucket, name, region, attrs, tags))
		}
	}
	return resources, nil
}

func bucketRegion(ctx context.Context, client *s3.Client, bucket string) (string, error) {
	out, err := client.GetBucketLocation(ctx, &s3.GetBucketLocationInput{Bucket: aws.String(bucket)})
	if err != nil {
		return "", fmt.Errorf("bucket %s location: %w", bucket, err)
	}
	if out.LocationConstraint == "" {
		return "us-east-1", nil // legacy: empty means us-east-1
	}
	if out.LocationConstraint == "EU" {
		return "eu-west-1", nil
	}
	return string(out.LocationConstraint), nil
}
