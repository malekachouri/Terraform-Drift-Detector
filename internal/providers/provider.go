package providers

import (
	"context"

	"github.com/malekachouri/terraform-drift-detector/internal/model"
	"github.com/malekachouri/terraform-drift-detector/internal/providers/aws"
)

// FetchRequest describes what to read from the cloud.
type FetchRequest struct {
	Expected []model.Resource
	Regions  []string
	Endpoint string // custom API endpoint (LocalStack); empty for the real cloud
	// AllTypes fetches every supported type, not only the ones in the state,
	// so unmanaged resources of other types are also discovered.
	AllTypes bool
}

// CloudProvider fetches live cloud resources.
type CloudProvider interface {
	Name() string
	FetchResources(ctx context.Context, req FetchRequest) ([]model.Resource, []error)
	SupportedTypes() []string
}

// Registry holds registered cloud providers.
type Registry struct {
	providers map[string]CloudProvider
}

func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]CloudProvider)}
}

func (r *Registry) Register(p CloudProvider) {
	r.providers[p.Name()] = p
}

func (r *Registry) Get(name string) (CloudProvider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

// awsAdapter maps the generic request onto the aws package.
type awsAdapter struct{ p *aws.Provider }

func (a awsAdapter) Name() string             { return a.p.Name() }
func (a awsAdapter) SupportedTypes() []string { return a.p.SupportedTypes() }
func (a awsAdapter) FetchResources(ctx context.Context, req FetchRequest) ([]model.Resource, []error) {
	return a.p.FetchResources(ctx, aws.Request{
		Expected: req.Expected,
		Regions:  req.Regions,
		Endpoint: req.Endpoint,
		AllTypes: req.AllTypes,
	})
}

func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(awsAdapter{aws.NewProvider()})
	return r
}
