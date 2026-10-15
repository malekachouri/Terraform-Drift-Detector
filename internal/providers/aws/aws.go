package aws

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/malekachouri/terraform-drift-detector/internal/awsclient"
	"github.com/malekachouri/terraform-drift-detector/internal/model"
)

// Provider implements AWS (and LocalStack) cloud resource fetching.
type Provider struct{}

// Request is the AWS-specific fetch request.
type Request struct {
	Expected []model.Resource
	Regions  []string
	Endpoint string
	AllTypes bool
}

func NewProvider() *Provider {
	return &Provider{}
}

func (p *Provider) Name() string { return "aws" }

func (p *Provider) SupportedTypes() []string {
	return []string{
		"aws_instance",
		"aws_s3_bucket",
		"aws_security_group",
		"aws_subnet",
		"aws_vpc",
	}
}

type regionalFetcher func(ctx context.Context, c *awsclient.Clients, region string) ([]model.Resource, error)

var regionalFetchers = map[string]regionalFetcher{
	"aws_instance":       fetchInstances,
	"aws_vpc":            fetchVPCs,
	"aws_subnet":         fetchSubnets,
	"aws_security_group": fetchSecurityGroups,
}

// FetchResources lists every live resource of the relevant types in each
// region. Listing (instead of describing state IDs) means deleted resources
// simply don't come back -> "missing", and hand-made ones do -> "extra".
func (p *Provider) FetchResources(ctx context.Context, req Request) ([]model.Resource, []error) {
	regions := regionSet(req)
	types := p.typesToFetch(req)

	var (
		mu        sync.Mutex
		wg        sync.WaitGroup
		resources []model.Resource
		errs      []error
	)
	collect := func(r []model.Resource, err error) {
		mu.Lock()
		defer mu.Unlock()
		resources = append(resources, r...)
		if err != nil {
			errs = append(errs, err)
		}
	}

	for _, region := range regions {
		clients, err := awsclient.New(ctx, region, req.Endpoint)
		if err != nil {
			collect(nil, fmt.Errorf("region %s: %w", region, err))
			continue
		}
		for t, fetch := range regionalFetchers {
			if !types[t] {
				continue
			}
			wg.Add(1)
			go func(t string, fetch regionalFetcher, region string, c *awsclient.Clients) {
				defer wg.Done()
				r, err := fetch(ctx, c, region)
				if err != nil {
					err = fmt.Errorf("%s in %s: %w", t, region, err)
				}
				collect(r, err)
			}(t, fetch, region, clients)
		}
	}

	// S3 is global: list buckets once and keep those in our regions.
	if types["aws_s3_bucket"] {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := fetchBuckets(ctx, regions[0], req.Endpoint, regions)
			if err != nil {
				err = fmt.Errorf("aws_s3_bucket: %w", err)
			}
			collect(r, err)
		}()
	}

	wg.Wait()
	sort.Slice(resources, func(i, j int) bool { return resources[i].ID < resources[j].ID })
	return resources, errs
}

func (p *Provider) typesToFetch(req Request) map[string]bool {
	out := make(map[string]bool)
	if req.AllTypes {
		for _, t := range p.SupportedTypes() {
			out[t] = true
		}
		return out
	}
	for _, e := range req.Expected {
		out[e.Type] = true
	}
	return out
}

func regionSet(req Request) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(r string) {
		if r != "" && !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	for _, r := range req.Regions {
		add(r)
	}
	for _, e := range req.Expected {
		add(e.Region)
	}
	if len(out) == 0 {
		add("us-east-1")
	}
	return out
}

func baseResource(resType, cloudID, name, region string, attrs map[string]any, tags map[string]string) model.Resource {
	if name == "" {
		name = cloudID
	}
	return model.Resource{
		ID:         fmt.Sprintf("aws/%s/%s", resType, cloudID),
		Provider:   "aws",
		Type:       resType,
		CloudID:    cloudID,
		Name:       name,
		Attributes: attrs,
		Tags:       tags,
		Region:     region,
		Source:     "cloud",
	}
}
