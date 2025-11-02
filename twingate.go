package twingate

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/hasura/go-graphql-client"
	"go.uber.org/zap"
)

func init() {
	caddy.RegisterModule((*TwingateApp)(nil))
	httpcaddyfile.RegisterGlobalOption("twingate", parseTwingateApp)
}

type CleanupConfig struct {
	Enabled bool `json:"enabled"`
	DryRun  bool `json:"dry_run,omitempty"`
}

type TwingateApp struct {
	Tenant          string         `json:"tenant,omitempty"`
	RemoteNetwork   string         `json:"remote_network,omitempty"`
	CaddyAddress    string         `json:"caddy_address,omitempty"`
	ResourceCleanup *CleanupConfig `json:"resource_cleanup,omitempty"`

	client    *TwingateClient
	ctx       caddy.Context
	logger    *zap.Logger
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	lastSync  time.Time
	syncMutex sync.RWMutex
}

func (*TwingateApp) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "twingate",
		New: func() caddy.Module { return new(TwingateApp) },
	}
}

func (t *TwingateApp) Provision(ctx caddy.Context) error {
	t.ctx = ctx
	t.logger = ctx.Logger(t)

	if t.Tenant == "" {
		return fmt.Errorf("tenant is required")
	}

	apiKey := os.Getenv("TWINGATE_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("TWINGATE_API_KEY environment variable is required")
	}

	endpoint := fmt.Sprintf("https://%s.twingate.com/api/graphql/", t.Tenant)
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	graphqlClient := graphql.NewClient(endpoint, httpClient).
		WithRequestModifier(func(r *http.Request) {
			r.Header.Set("X-API-KEY", apiKey)
			r.Header.Set("Content-Type", "application/json")
		})

	t.client = &TwingateClient{
		client: graphqlClient,
		logger: t.logger,
	}

	if err := t.client.TestConnection(context.Background()); err != nil {
		return fmt.Errorf("failed to connect to Twingate API: %w", err)
	}

	t.logger.Info("Twingate module provisioned successfully",
		zap.String("tenant", t.Tenant),
		zap.String("endpoint", endpoint))

	if t.ResourceCleanup != nil && t.ResourceCleanup.Enabled {
		t.logger.Warn("Resource cleanup ENABLED - ALL resources in network will be managed",
			zap.Bool("dry_run", t.ResourceCleanup.DryRun),
			zap.String("remote_network", t.RemoteNetwork),
			zap.String("warning", "Ensure this is a dedicated remote network - all resources not in Caddyfile will be deleted"))
	}

	if err := t.performSync(context.Background()); err != nil {
		return fmt.Errorf("initial sync failed: %w", err)
	}

	// NOTE: Config reload handling is automatic via the App lifecycle.
	// When config is reloaded, Caddy creates a new instance of this app,
	// calls Provision() (which performs initial sync above), then Start().
	// The old instance is then stopped and cleaned up. This ensures
	// Twingate resources are re-synced on every config reload.

	return nil
}

func (t *TwingateApp) Validate() error {
	if t.Tenant == "" {
		return fmt.Errorf("tenant is required")
	}
	if os.Getenv("TWINGATE_API_KEY") == "" {
		return fmt.Errorf("TWINGATE_API_KEY environment variable is required")
	}
	return nil
}

func (t *TwingateApp) Start() error {
	t.logger.Info("Starting Twingate app")

	_, cancel := context.WithCancel(context.Background())
	t.cancel = cancel

	// NOTE: No need to perform sync here - Provision() already performed
	// the initial sync synchronously. This avoids duplicate resource creation
	// and ensures proper error handling (Provision fails if sync fails).
	// Config reloads automatically create a new app instance which will
	// call Provision() again, triggering a fresh sync.

	return nil
}

func (t *TwingateApp) Stop() error {
	t.logger.Info("Stopping Twingate app")

	if t.cancel != nil {
		t.cancel()
	}

	done := make(chan struct{})
	go func() {
		t.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.logger.Info("Twingate app stopped gracefully")
	case <-time.After(10 * time.Second):
		t.logger.Warn("Twingate app stop timeout, some operations may not have completed")
	}

	return nil
}

func (t *TwingateApp) performSync(ctx context.Context) error {
	t.syncMutex.Lock()
	defer t.syncMutex.Unlock()

	t.logger.Info("Starting Twingate sync")

	caddyAddress, err := t.resolveCaddyAddress()
	if err != nil {
		return err
	}

	httpAppIface, err := t.ctx.App("http")
	if err != nil {
		return fmt.Errorf("failed to get HTTP app: %w", err)
	}

	httpApp, ok := httpAppIface.(*caddyhttp.App)
	if !ok {
		return fmt.Errorf("HTTP app is not of expected type")
	}

	discoverer := &RouteDiscoverer{
		logger:       t.logger,
		caddyAddress: caddyAddress,
	}

	endpoints, err := discoverer.DiscoverEndpoints(httpApp)
	if err != nil {
		return fmt.Errorf("failed to discover endpoints: %w", err)
	}

	if len(endpoints) == 0 {
		t.logger.Info("No reverse_proxy endpoints found, skipping sync")
		return nil
	}

	mappings := make([]ResourceMapping, len(endpoints))
	for i, ep := range endpoints {
		mappings[i] = ep.ToResourceMapping(caddyAddress)
	}

	t.logger.Info("Discovered reverse_proxy endpoints",
		zap.Int("count", len(mappings)))

	syncer := &ResourceSyncer{
		client: t.client,
		logger: t.logger,
	}

	if err := syncer.SyncResources(ctx, mappings, t.RemoteNetwork, t.ResourceCleanup); err != nil {
		return fmt.Errorf("failed to sync resources: %w", err)
	}

	t.lastSync = time.Now()
	t.logger.Info("Twingate sync completed successfully",
		zap.Time("last_sync", t.lastSync))

	return nil
}

func (t *TwingateApp) GetLastSyncTime() time.Time {
	t.syncMutex.RLock()
	defer t.syncMutex.RUnlock()
	return t.lastSync
}

func (t *TwingateApp) TriggerSync() error {
	t.logger.Info("Manual sync triggered")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	return t.performSync(ctx)
}

// NOTE: onConfigReload is no longer needed because Caddy's App lifecycle
// automatically handles config reloads by creating new app instances.
// When config is reloaded, a new TwingateApp instance is provisioned and
// started, which triggers a fresh sync. The old instance is then stopped.
// This method is kept for reference but is unused.
/*
func (t *TwingateApp) onConfigReload(event caddyevents.Event) error {
	t.logger.Info("Config reload detected, triggering Twingate re-sync")

	// Run sync in background to avoid blocking reload
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		if err := t.performSync(ctx); err != nil {
			t.logger.Error("Re-sync after config reload failed", zap.Error(err))
		} else {
			t.logger.Info("Re-sync after config reload completed successfully")
		}
	}()

	return nil
}
*/

// GetOutboundIP uses a UDP dial to 8.8.8.8:80 to determine the local outbound IP
// without actually sending any data over the network
func GetOutboundIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", fmt.Errorf("failed to detect outbound IP: %w", err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}

func (t *TwingateApp) resolveCaddyAddress() (string, error) {
	if t.CaddyAddress != "" {
		t.logger.Info("Using explicitly configured Caddy address",
			zap.String("address", t.CaddyAddress))
		return t.CaddyAddress, nil
	}

	ip, err := GetOutboundIP()
	if err != nil {
		return "", fmt.Errorf("failed to resolve Caddy address: %w. Consider setting caddy_address explicitly in Twingate config", err)
	}

	t.logger.Info("Auto-detected Caddy address from outbound interface",
		zap.String("address", ip),
		zap.String("method", "udp_dial"))

	return ip, nil
}

var (
	_ caddy.Module      = (*TwingateApp)(nil)
	_ caddy.App         = (*TwingateApp)(nil)
	_ caddy.Provisioner = (*TwingateApp)(nil)
	_ caddy.Validator   = (*TwingateApp)(nil)
)
