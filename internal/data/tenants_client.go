package data

import (
	"context"

	tenants_v1 "gitlab.calendaria.team/services/tenants/api/tenants/v1"
	"gitlab.calendaria.team/services/platform-billing/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TenantsClient provides access to the tenants service for revenue share lookups.
type TenantsClient interface {
	GetTenant(ctx context.Context, tenantID int64) (*tenants_v1.Tenant, error)
}

type tenantsClient struct {
	client tenants_v1.TenantsClient
	log    *log.Helper
}

type stubTenantsClient struct {
	log *log.Helper
}

// NewTenantsClientFromConf creates a TenantsClient from config.
func NewTenantsClientFromConf(bc *conf.Bootstrap, logger log.Logger) (TenantsClient, func(), error) {
	endpoint := ""
	if bc.GetDiscovery() != nil {
		endpoint = bc.GetDiscovery().GetTenantsEndpoint()
	}
	return NewTenantsClient(endpoint, logger)
}

// NewTenantsClient creates a gRPC client for the tenants service.
// If endpoint is empty, returns a stub that returns nil referred_by.
func NewTenantsClient(endpoint string, logger log.Logger) (TenantsClient, func(), error) {
	l := log.NewHelper(logger)

	if endpoint == "" {
		l.Info("Tenants endpoint not configured, using stub client")
		return &stubTenantsClient{log: l}, func() {}, nil
	}

	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, err
	}

	client := tenants_v1.NewTenantsClient(conn)
	l.Infof("Connected to tenants service at %s", endpoint)

	cleanup := func() {
		conn.Close()
	}

	return &tenantsClient{client: client, log: l}, cleanup, nil
}

func (c *tenantsClient) GetTenant(ctx context.Context, tenantID int64) (*tenants_v1.Tenant, error) {
	reply, err := c.client.GetTenant(ctx, &tenants_v1.TenantRequest{TenantId: tenantID})
	if err != nil {
		return nil, err
	}
	return reply.GetTenant(), nil
}

func (c *stubTenantsClient) GetTenant(_ context.Context, _ int64) (*tenants_v1.Tenant, error) {
	// Stub: returns a tenant with no referred_by
	return &tenants_v1.Tenant{}, nil
}
