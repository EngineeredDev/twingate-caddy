package twingate

import (
	"context"
	"fmt"

	"github.com/hasura/go-graphql-client"
	"go.uber.org/zap"
)

type TwingateClient struct {
	client *graphql.Client
	logger *zap.Logger
}

func (c *TwingateClient) TestConnection(ctx context.Context) error {
	var query struct {
		RemoteNetworks struct {
			Edges []struct {
				Node RemoteNetwork `json:"node"`
			} `json:"edges"`
		} `graphql:"remoteNetworks(first: 1)"`
	}

	err := c.client.Query(ctx, &query, nil)
	if err != nil {
		return fmt.Errorf("API connection test failed: %w", err)
	}

	c.logger.Debug("API connection test successful")
	return nil
}

func (c *TwingateClient) GetRemoteNetworks(ctx context.Context) ([]RemoteNetwork, error) {
	var query RemoteNetworksQuery
	variables := map[string]any{
		"first": 100,
	}

	err := c.client.Query(ctx, &query, variables)
	if err != nil {
		return nil, fmt.Errorf("failed to query remote networks: %w", err)
	}

	networks := make([]RemoteNetwork, len(query.RemoteNetworks.Edges))
	for i, edge := range query.RemoteNetworks.Edges {
		networks[i] = edge.Node
	}

	c.logger.Debug("Retrieved remote networks", zap.Int("count", len(networks)))
	return networks, nil
}

func (c *TwingateClient) GetRemoteNetworkByName(ctx context.Context, name string) (*RemoteNetwork, error) {
	networks, err := c.GetRemoteNetworks(ctx)
	if err != nil {
		return nil, err
	}

	for _, network := range networks {
		if network.Name == name {
			c.logger.Debug("Found remote network by name",
				zap.String("name", name),
				zap.String("id", network.ID))
			return &network, nil
		}
	}

	return nil, nil
}

func (c *TwingateClient) CreateRemoteNetwork(ctx context.Context, name string) (*RemoteNetwork, error) {
	var mutation RemoteNetworkCreateMutation

	variables := map[string]any{
		"name": name,
	}

	err := c.client.Mutate(ctx, &mutation, variables)
	if err != nil {
		return nil, fmt.Errorf("failed to create remote network: %w", err)
	}

	if !mutation.RemoteNetworkCreate.OK {
		errorMsg := "unknown error"
		if mutation.RemoteNetworkCreate.Error != nil {
			errorMsg = *mutation.RemoteNetworkCreate.Error
		}
		return nil, fmt.Errorf("remote network creation failed: %s", errorMsg)
	}

	if mutation.RemoteNetworkCreate.Entity == nil {
		return nil, fmt.Errorf("remote network creation succeeded but no entity returned")
	}

	c.logger.Info("Created remote network",
		zap.String("name", mutation.RemoteNetworkCreate.Entity.Name),
		zap.String("id", mutation.RemoteNetworkCreate.Entity.ID))

	return mutation.RemoteNetworkCreate.Entity, nil
}

func (c *TwingateClient) GetOrCreateRemoteNetwork(ctx context.Context, name string) (*RemoteNetwork, error) {
	network, err := c.GetRemoteNetworkByName(ctx, name)
	if err != nil {
		return nil, err
	}

	if network != nil {
		return network, nil
	}

	return c.CreateRemoteNetwork(ctx, name)
}

func (c *TwingateClient) GetResources(ctx context.Context, remoteNetworkID string) ([]Resource, error) {
	var query ResourcesQuery
	variables := map[string]any{
		"first": 100,
	}

	err := c.client.Query(ctx, &query, variables)
	if err != nil {
		return nil, fmt.Errorf("failed to query resources: %w", err)
	}

	resources := make([]Resource, 0)
	for _, edge := range query.Resources.Edges {
		if remoteNetworkID == "" || edge.Node.RemoteNetwork.ID == remoteNetworkID {
			resources = append(resources, edge.Node)
		}
	}

	c.logger.Debug("Retrieved resources",
		zap.Int("count", len(resources)),
		zap.String("remote_network_id", remoteNetworkID))

	return resources, nil
}

func (c *TwingateClient) GetResourceByAlias(ctx context.Context, alias string, remoteNetworkID string) (*Resource, error) {
	resources, err := c.GetResources(ctx, remoteNetworkID)
	if err != nil {
		return nil, err
	}

	for _, resource := range resources {
		if resource.Alias != nil && *resource.Alias == alias {
			c.logger.Debug("Found resource by alias",
				zap.String("alias", alias),
				zap.String("id", resource.ID))
			return &resource, nil
		}
	}

	return nil, nil
}

func (c *TwingateClient) CreateResource(ctx context.Context, input ResourceCreateInput) (*Resource, error) {
	var mutation struct {
		ResourceCreate struct {
			OK     bool    `graphql:"ok"`
			Error  *string `graphql:"error"`
			Entity *struct {
				ID      string `graphql:"id"`
				Name    string `graphql:"name"`
				Address struct {
					Value string `graphql:"value"`
				} `graphql:"address"`
				Alias         *string `graphql:"alias"`
				RemoteNetwork struct {
					ID string `graphql:"id"`
				} `graphql:"remoteNetwork"`
			} `graphql:"entity"`
		} `graphql:"resourceCreate(address: $address, name: $name, remoteNetworkId: $remoteNetworkId, alias: $alias)"`
	}

	variables := map[string]any{
		"name":            input.Name,
		"address":         input.Address,
		"remoteNetworkId": graphql.ID(input.RemoteNetworkID),
		"alias":           input.Alias,
	}

	c.logger.Info("Creating resource with variables",
		zap.String("name", input.Name),
		zap.String("address", input.Address),
		zap.String("remoteNetworkId", input.RemoteNetworkID))

	err := c.client.Mutate(ctx, &mutation, variables)
	if err != nil {
		c.logger.Error("GraphQL mutation failed",
			zap.Error(err),
			zap.String("error_type", fmt.Sprintf("%T", err)),
			zap.String("error_string", err.Error()))
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	c.logger.Info("GraphQL mutation response",
		zap.Bool("ok", mutation.ResourceCreate.OK),
		zap.Any("error", mutation.ResourceCreate.Error),
		zap.Any("entity", mutation.ResourceCreate.Entity))

	if !mutation.ResourceCreate.OK {
		errorMsg := "unknown error"
		if mutation.ResourceCreate.Error != nil {
			errorMsg = *mutation.ResourceCreate.Error
		}
		c.logger.Error("Resource creation returned error",
			zap.String("error_message", errorMsg))
		return nil, fmt.Errorf("resource creation failed: %s", errorMsg)
	}

	if mutation.ResourceCreate.Entity == nil {
		return nil, fmt.Errorf("resource creation succeeded but no entity returned")
	}

	resource := &Resource{
		ID:   mutation.ResourceCreate.Entity.ID,
		Name: mutation.ResourceCreate.Entity.Name,
		Address: struct {
			Value string `graphql:"value"`
		}{Value: mutation.ResourceCreate.Entity.Address.Value},
		Alias: mutation.ResourceCreate.Entity.Alias,
		RemoteNetwork: struct {
			ID string `graphql:"id"`
		}{ID: mutation.ResourceCreate.Entity.RemoteNetwork.ID},
	}

	c.logger.Info("Created resource",
		zap.String("name", resource.Name),
		zap.String("id", resource.ID),
		zap.String("address", resource.Address.Value))

	return resource, nil
}

func (c *TwingateClient) UpdateResource(ctx context.Context, input ResourceUpdateInput) (*Resource, error) {
	var mutation ResourceUpdateMutation

	// All parameters must be provided to match the mutation signature
	variables := map[string]any{
		"id":      graphql.ID(input.ID),
		"name":    "",
		"address": "",
		"alias":   "",
	}

	if input.Name != nil {
		variables["name"] = *input.Name
	}
	if input.Address != nil {
		variables["address"] = *input.Address
	}
	if input.Alias != nil {
		variables["alias"] = *input.Alias
	}

	err := c.client.Mutate(ctx, &mutation, variables)
	if err != nil {
		return nil, fmt.Errorf("failed to update resource: %w", err)
	}

	if !mutation.ResourceUpdate.OK {
		errorMsg := "unknown error"
		if mutation.ResourceUpdate.Error != nil {
			errorMsg = *mutation.ResourceUpdate.Error
		}
		return nil, fmt.Errorf("resource update failed: %s", errorMsg)
	}

	if mutation.ResourceUpdate.Entity == nil {
		return nil, fmt.Errorf("resource update succeeded but no entity returned")
	}

	c.logger.Info("Updated resource",
		zap.String("name", mutation.ResourceUpdate.Entity.Name),
		zap.String("id", mutation.ResourceUpdate.Entity.ID),
		zap.String("address", mutation.ResourceUpdate.Entity.Address.Value))

	return mutation.ResourceUpdate.Entity, nil
}

func (c *TwingateClient) DeleteResource(ctx context.Context, resourceID string) error {
	var mutation struct {
		ResourceDelete struct {
			OK    bool    `graphql:"ok"`
			Error *string `graphql:"error"`
		} `graphql:"resourceDelete(id: $id)"`
	}

	variables := map[string]any{
		"id": graphql.ID(resourceID),
	}

	err := c.client.Mutate(ctx, &mutation, variables)
	if err != nil {
		return fmt.Errorf("failed to delete resource: %w", err)
	}

	if !mutation.ResourceDelete.OK {
		errorMsg := "unknown error"
		if mutation.ResourceDelete.Error != nil {
			errorMsg = *mutation.ResourceDelete.Error
		}
		return fmt.Errorf("resource deletion failed: %s", errorMsg)
	}

	c.logger.Debug("Successfully deleted resource", zap.String("id", resourceID))
	return nil
}

func (c *TwingateClient) CreateOrUpdateResource(ctx context.Context, mapping ResourceMapping, remoteNetworkID string) (*Resource, error) {
	c.logger.Info("Processing resource mapping",
		zap.String("name", mapping.Name),
		zap.Any("alias", mapping.Alias),
		zap.String("address", mapping.Address),
		zap.String("remote_network_id", remoteNetworkID))

	lookupAlias := mapping.Name
	if mapping.Alias != nil {
		lookupAlias = *mapping.Alias
	}

	existing, err := c.GetResourceByAlias(ctx, lookupAlias, remoteNetworkID)
	if err != nil {
		return nil, fmt.Errorf("failed to check for existing resource: %w", err)
	}

	if existing != nil {
		input := ResourceUpdateInput{
			ID:      existing.ID,
			Name:    &mapping.Name,
			Address: &mapping.Address,
			Alias:   mapping.Alias,
		}
		return c.UpdateResource(ctx, input)
	}

	input := ResourceCreateInput{
		Name:            mapping.Name,
		Address:         mapping.Address,
		RemoteNetworkID: remoteNetworkID,
	}

	if mapping.Alias != nil {
		input.Alias = *mapping.Alias
	}

	return c.CreateResource(ctx, input)
}
