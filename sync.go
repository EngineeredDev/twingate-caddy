package twingate

import (
	"context"
	"fmt"
	"net"

	"go.uber.org/zap"
)

const (
	DefaultRemoteNetworkName = "Caddy-Managed"
)

type ResourceSyncer struct {
	client *TwingateClient
	logger *zap.Logger
}

func (r *ResourceSyncer) SyncResources(ctx context.Context, mappings []ResourceMapping, remoteNetworkName string, cleanupConfig *CleanupConfig) error {
	if len(mappings) == 0 {
		r.logger.Info("No resource mappings to sync")
		return nil
	}

	networkName := remoteNetworkName
	if networkName == "" {
		networkName = DefaultRemoteNetworkName
	}

	r.logger.Info("Starting resource synchronization",
		zap.String("remote_network", networkName),
		zap.Int("mappings_count", len(mappings)))

	network, err := r.client.GetOrCreateRemoteNetwork(ctx, networkName)
	if err != nil {
		return fmt.Errorf("failed to get or create remote network: %w", err)
	}

	r.logger.Info("Using remote network",
		zap.String("name", network.Name),
		zap.String("id", network.ID))

	successCount, errorCount := r.upsertResources(ctx, mappings, network.ID)

	r.logger.Info("Resource upsert completed",
		zap.Int("success", successCount),
		zap.Int("errors", errorCount))

	var deleteCount, deleteErrors int
	if cleanupConfig != nil && cleanupConfig.Enabled {
		deleteCount, deleteErrors = r.deleteStaleResources(ctx, mappings, network.ID, cleanupConfig)

		r.logger.Info("Resource cleanup completed",
			zap.Int("deleted", deleteCount),
			zap.Int("errors", deleteErrors))
	} else {
		r.logger.Debug("Resource cleanup disabled, skipping deletion phase")
	}

	totalErrors := errorCount + deleteErrors
	if totalErrors > 0 {
		return fmt.Errorf("sync completed with %d errors (upsert: %d, delete: %d)",
			totalErrors, errorCount, deleteErrors)
	}

	return nil
}

func (r *ResourceSyncer) upsertResources(ctx context.Context, mappings []ResourceMapping, networkID string) (success, errors int) {
	for i, mapping := range mappings {
		r.logger.Debug("Upserting resource",
			zap.Int("index", i+1),
			zap.Int("total", len(mappings)),
			zap.String("name", mapping.Name))

		if err := r.syncSingleResource(ctx, mapping, networkID); err != nil {
			r.logger.Error("Failed to upsert resource",
				zap.String("name", mapping.Name),
				zap.Error(err))
			errors++
		} else {
			success++
		}
	}

	return success, errors
}

func (r *ResourceSyncer) deleteStaleResources(ctx context.Context, desiredMappings []ResourceMapping, networkID string, cleanupConfig *CleanupConfig) (deleted, errors int) {
	existingResources, err := r.client.GetResources(ctx, networkID)
	if err != nil {
		r.logger.Error("Failed to list resources in network", zap.Error(err))
		return 0, 1
	}

	r.logger.Info("Found existing resources in network",
		zap.Int("count", len(existingResources)))

	desiredNames := make(map[string]bool)
	for _, mapping := range desiredMappings {
		desiredNames[mapping.Name] = true
	}

	var staleResources []Resource
	for _, resource := range existingResources {
		if !desiredNames[resource.Name] {
			staleResources = append(staleResources, resource)
		}
	}

	if len(staleResources) == 0 {
		r.logger.Info("No stale resources to delete")
		return 0, 0
	}

	r.logger.Info("Found stale resources",
		zap.Int("count", len(staleResources)),
		zap.Bool("dry_run", cleanupConfig.DryRun))

	for _, resource := range staleResources {
		if cleanupConfig.DryRun {
			r.logger.Info("[DRY RUN] Would delete resource",
				zap.String("id", resource.ID),
				zap.String("name", resource.Name),
				zap.String("address", resource.Address.Value))
			deleted++
			continue
		}

		r.logger.Info("Deleting stale resource",
			zap.String("id", resource.ID),
			zap.String("name", resource.Name))

		if err := r.client.DeleteResource(ctx, resource.ID); err != nil {
			r.logger.Error("Failed to delete resource",
				zap.String("id", resource.ID),
				zap.String("name", resource.Name),
				zap.Error(err))
			errors++
		} else {
			deleted++
		}
	}

	return deleted, errors
}

func (r *ResourceSyncer) syncSingleResource(ctx context.Context, mapping ResourceMapping, remoteNetworkID string) error {
	if err := r.validateMapping(mapping); err != nil {
		return fmt.Errorf("invalid mapping: %w", err)
	}

	var existingResource *Resource
	var err error

	if mapping.Alias != nil {
		r.logger.Info("Checking for existing resource by alias",
			zap.String("alias", *mapping.Alias),
			zap.String("remote_network_id", remoteNetworkID))

		existingResource, err = r.client.GetResourceByAlias(ctx, *mapping.Alias, remoteNetworkID)
		if err != nil {
			return fmt.Errorf("failed to check for existing resource: %w", err)
		}

		if existingResource != nil {
			r.logger.Info("Found existing resource by alias",
				zap.String("resource_id", existingResource.ID),
				zap.String("name", existingResource.Name))
		} else {
			r.logger.Info("No existing resource found by alias",
				zap.String("alias", *mapping.Alias))
		}
	} else {
		r.logger.Info("Checking for existing resource by name",
			zap.String("name", mapping.Name),
			zap.String("remote_network_id", remoteNetworkID))

		resources, err := r.client.GetResources(ctx, remoteNetworkID)
		if err != nil {
			return fmt.Errorf("failed to list resources: %w", err)
		}

		r.logger.Info("Fetched resources from API",
			zap.Int("count", len(resources)),
			zap.String("remote_network_id", remoteNetworkID))

		for _, res := range resources {
			if res.Name == mapping.Name {
				existingResource = &res
				r.logger.Info("Found existing resource by name",
					zap.String("resource_id", res.ID),
					zap.String("name", res.Name))
				break
			}
		}

		if existingResource == nil {
			r.logger.Info("No existing resource found by name",
				zap.String("name", mapping.Name))
		}
	}

	if existingResource != nil {
		return r.updateExistingResource(ctx, mapping, existingResource)
	} else {
		return r.createNewResource(ctx, mapping, remoteNetworkID)
	}
}

func (r *ResourceSyncer) validateMapping(mapping ResourceMapping) error {
	if mapping.Name == "" {
		return fmt.Errorf("resource name cannot be empty")
	}
	if mapping.Address == "" {
		return fmt.Errorf("resource address cannot be empty")
	}

	// Twingate API currently only supports IPv4 addresses
	ip := net.ParseIP(mapping.Address)
	if ip == nil {
		return fmt.Errorf("address '%s' is not a valid IP address", mapping.Address)
	}
	if ip.To4() == nil {
		return fmt.Errorf("address '%s' is IPv6, but only IPv4 is currently supported", mapping.Address)
	}

	return nil
}

func (r *ResourceSyncer) createNewResource(ctx context.Context, mapping ResourceMapping, remoteNetworkID string) error {
	aliasStr := "<none>"
	if mapping.Alias != nil {
		aliasStr = *mapping.Alias
	}

	r.logger.Debug("Creating new resource",
		zap.String("name", mapping.Name),
		zap.String("address", mapping.Address),
		zap.String("alias", aliasStr))

	input := ResourceCreateInput{
		Name:            mapping.Name,
		Address:         mapping.Address,
		RemoteNetworkID: remoteNetworkID,
	}

	if mapping.Alias != nil {
		input.Alias = *mapping.Alias
	}

	resource, err := r.client.CreateResource(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create resource: %w", err)
	}

	r.logger.Info("Successfully created resource",
		zap.String("id", resource.ID),
		zap.String("name", resource.Name),
		zap.String("address", resource.Address.Value))

	return nil
}

func (r *ResourceSyncer) updateExistingResource(ctx context.Context, mapping ResourceMapping, existing *Resource) error {
	needsUpdate := false

	// Initialize all mutation fields with existing values, then update as needed
	updateInput := ResourceUpdateInput{
		ID:      existing.ID,
		Name:    &existing.Name,
		Address: &existing.Address.Value,
		Alias:   existing.Alias,
	}

	if existing.Name != mapping.Name {
		needsUpdate = true
		updateInput.Name = &mapping.Name
		r.logger.Debug("Resource name needs update",
			zap.String("resource_id", existing.ID),
			zap.String("current", existing.Name),
			zap.String("new", mapping.Name))
	}

	if existing.Address.Value != mapping.Address {
		needsUpdate = true
		updateInput.Address = &mapping.Address
		r.logger.Debug("Resource address needs update",
			zap.String("resource_id", existing.ID),
			zap.String("current", existing.Address.Value),
			zap.String("new", mapping.Address))
	}

	currentAlias := ""
	if existing.Alias != nil {
		currentAlias = *existing.Alias
	}
	desiredAlias := ""
	if mapping.Alias != nil {
		desiredAlias = *mapping.Alias
	}
	if currentAlias != desiredAlias {
		needsUpdate = true
		updateInput.Alias = mapping.Alias
		r.logger.Debug("Resource alias needs update",
			zap.String("resource_id", existing.ID),
			zap.String("current", currentAlias),
			zap.String("new", desiredAlias))
	}

	if !needsUpdate {
		r.logger.Debug("Resource is already up to date",
			zap.String("resource_id", existing.ID),
			zap.String("name", existing.Name))
		return nil
	}

	r.logger.Debug("Updating existing resource",
		zap.String("id", existing.ID),
		zap.String("name", existing.Name))

	resource, err := r.client.UpdateResource(ctx, updateInput)
	if err != nil {
		return fmt.Errorf("failed to update resource: %w", err)
	}

	r.logger.Info("Successfully updated resource",
		zap.String("id", resource.ID),
		zap.String("name", resource.Name),
		zap.String("address", resource.Address.Value))

	return nil
}

func (r *ResourceSyncer) GetSyncSummary(ctx context.Context, mappings []ResourceMapping, remoteNetworkName string) (*SyncSummary, error) {
	summary := &SyncSummary{
		TotalMappings: len(mappings),
	}

	if len(mappings) == 0 {
		return summary, nil
	}

	networkName := remoteNetworkName
	if networkName == "" {
		networkName = DefaultRemoteNetworkName
	}

	network, err := r.client.GetRemoteNetworkByName(ctx, networkName)
	if err != nil {
		return nil, fmt.Errorf("failed to check remote network: %w", err)
	}

	if network == nil {
		summary.RemoteNetworkAction = "create"
		summary.RemoteNetworkName = networkName
	} else {
		summary.RemoteNetworkAction = "use_existing"
		summary.RemoteNetworkName = network.Name
		summary.RemoteNetworkID = network.ID
	}

	for _, mapping := range mappings {
		if network != nil {
			var existing *Resource
			var err error

			if mapping.Alias != nil {
				existing, err = r.client.GetResourceByAlias(ctx, *mapping.Alias, network.ID)
				if err != nil {
					r.logger.Warn("Failed to check existing resource during summary",
						zap.String("name", mapping.Name),
						zap.Error(err))
					continue
				}
			} else {
				resources, err := r.client.GetResources(ctx, network.ID)
				if err != nil {
					r.logger.Warn("Failed to list resources during summary",
						zap.String("name", mapping.Name),
						zap.Error(err))
					continue
				}
				for _, res := range resources {
					if res.Name == mapping.Name {
						existing = &res
						break
					}
				}
			}

			if existing != nil {
				summary.ResourcesToUpdate++
			} else {
				summary.ResourcesToCreate++
			}
		} else {
			summary.ResourcesToCreate++
		}
	}

	return summary, nil
}

type SyncSummary struct {
	TotalMappings       int    `json:"total_mappings"`
	RemoteNetworkAction string `json:"remote_network_action"` // "create" or "use_existing"
	RemoteNetworkName   string `json:"remote_network_name"`
	RemoteNetworkID     string `json:"remote_network_id,omitempty"`
	ResourcesToCreate   int    `json:"resources_to_create"`
	ResourcesToUpdate   int    `json:"resources_to_update"`
}
