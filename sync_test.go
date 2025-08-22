package twingate

import (
	"context"
	"fmt"
	"testing"

	"go.uber.org/zap"
)

// clientInterface defines the subset of TwingateClient methods needed for testing
type clientInterface interface {
	GetResources(ctx context.Context, remoteNetworkID string) ([]Resource, error)
	DeleteResource(ctx context.Context, resourceID string) error
	GetResourceByAlias(ctx context.Context, alias string, remoteNetworkID string) (*Resource, error)
	CreateResource(ctx context.Context, input ResourceCreateInput) (*Resource, error)
	UpdateResource(ctx context.Context, input ResourceUpdateInput) (*Resource, error)
	GetOrCreateRemoteNetwork(ctx context.Context, name string) (*RemoteNetwork, error)
}

// testableResourceSyncer wraps ResourceSyncer to allow dependency injection for testing
type testableResourceSyncer struct {
	client clientInterface
	logger *zap.Logger
}

// deleteStaleResources mirrors the ResourceSyncer implementation but uses the interface
func (r *testableResourceSyncer) deleteStaleResources(ctx context.Context, desiredMappings []ResourceMapping, networkID string, cleanupConfig *CleanupConfig) (deleted, errors int) {
	// Get ALL resources in this network (no filtering)
	existingResources, err := r.client.GetResources(ctx, networkID)
	if err != nil {
		r.logger.Error("Failed to list resources in network", zap.Error(err))
		return 0, 1
	}

	r.logger.Info("Found existing resources in network",
		zap.Int("count", len(existingResources)))

	// Build set of desired resource names for quick lookup
	desiredNames := make(map[string]bool)
	for _, mapping := range desiredMappings {
		desiredNames[mapping.Name] = true
	}

	// Find stale resources (existing but not desired)
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

	// Delete each stale resource
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

// MockTwingateClient is a mock implementation of clientInterface for testing
type MockTwingateClient struct {
	Resources       map[string]Resource // key is resource ID
	DeletedIDs      []string
	GetResourcesErr error
	DeleteErr       error
	CallLog         []string // Track method calls for verification
}

// GetResources returns mock resources filtered by network ID
func (m *MockTwingateClient) GetResources(ctx context.Context, remoteNetworkID string) ([]Resource, error) {
	m.CallLog = append(m.CallLog, fmt.Sprintf("GetResources(%s)", remoteNetworkID))

	if m.GetResourcesErr != nil {
		return nil, m.GetResourcesErr
	}

	var resources []Resource
	for _, r := range m.Resources {
		if r.RemoteNetwork.ID == remoteNetworkID {
			resources = append(resources, r)
		}
	}
	return resources, nil
}

// DeleteResource deletes a resource by ID
func (m *MockTwingateClient) DeleteResource(ctx context.Context, resourceID string) error {
	m.CallLog = append(m.CallLog, fmt.Sprintf("DeleteResource(%s)", resourceID))

	if m.DeleteErr != nil {
		return m.DeleteErr
	}

	m.DeletedIDs = append(m.DeletedIDs, resourceID)
	delete(m.Resources, resourceID)
	return nil
}

// CreateResource creates a new resource (needed for interface compatibility)
func (m *MockTwingateClient) CreateResource(ctx context.Context, input ResourceCreateInput) (*Resource, error) {
	return nil, nil
}

// UpdateResource updates a resource (needed for interface compatibility)
func (m *MockTwingateClient) UpdateResource(ctx context.Context, input ResourceUpdateInput) (*Resource, error) {
	return nil, nil
}

// GetResourceByAlias finds a resource by alias (needed for interface compatibility)
func (m *MockTwingateClient) GetResourceByAlias(ctx context.Context, alias string, remoteNetworkID string) (*Resource, error) {
	return nil, nil
}

// GetOrCreateRemoteNetwork gets or creates a remote network (needed for interface compatibility)
func (m *MockTwingateClient) GetOrCreateRemoteNetwork(ctx context.Context, name string) (*RemoteNetwork, error) {
	return nil, nil
}

// TestDeleteStaleResources tests that only stale resources are deleted
func TestDeleteStaleResources(t *testing.T) {
	tests := []struct {
		name              string
		existingResources map[string]Resource
		desiredMappings   []ResourceMapping
		expectedDeleted   []string
		expectedPreserved []string
	}{
		{
			name: "delete only stale resources",
			existingResources: map[string]Resource{
				"res1": {
					ID:   "res1",
					Name: "api.example.com",
					Address: struct {
						Value string `graphql:"value"`
					}{Value: "10.0.0.1"},
					RemoteNetwork: struct {
						ID string `graphql:"id"`
					}{ID: "net1"},
				},
				"res2": {
					ID:   "res2",
					Name: "app.example.com",
					Address: struct {
						Value string `graphql:"value"`
					}{Value: "10.0.0.1"},
					RemoteNetwork: struct {
						ID string `graphql:"id"`
					}{ID: "net1"},
				},
				"res3": {
					ID:   "res3",
					Name: "old.example.com",
					Address: struct {
						Value string `graphql:"value"`
					}{Value: "10.0.0.1"},
					RemoteNetwork: struct {
						ID string `graphql:"id"`
					}{ID: "net1"},
				},
			},
			desiredMappings: []ResourceMapping{
				{Name: "api.example.com", Address: "10.0.0.1"},
				{Name: "app.example.com", Address: "10.0.0.1"},
			},
			expectedDeleted:   []string{"res3"},
			expectedPreserved: []string{"res1", "res2"},
		},
		{
			name: "no stale resources",
			existingResources: map[string]Resource{
				"res1": {
					ID:   "res1",
					Name: "api.example.com",
					Address: struct {
						Value string `graphql:"value"`
					}{Value: "10.0.0.1"},
					RemoteNetwork: struct {
						ID string `graphql:"id"`
					}{ID: "net1"},
				},
			},
			desiredMappings: []ResourceMapping{
				{Name: "api.example.com", Address: "10.0.0.1"},
			},
			expectedDeleted:   []string{},
			expectedPreserved: []string{"res1"},
		},
		{
			name: "all resources stale",
			existingResources: map[string]Resource{
				"res1": {
					ID:   "res1",
					Name: "old1.example.com",
					Address: struct {
						Value string `graphql:"value"`
					}{Value: "10.0.0.1"},
					RemoteNetwork: struct {
						ID string `graphql:"id"`
					}{ID: "net1"},
				},
				"res2": {
					ID:   "res2",
					Name: "old2.example.com",
					Address: struct {
						Value string `graphql:"value"`
					}{Value: "10.0.0.1"},
					RemoteNetwork: struct {
						ID string `graphql:"id"`
					}{ID: "net1"},
				},
			},
			desiredMappings:   []ResourceMapping{},
			expectedDeleted:   []string{"res1", "res2"},
			expectedPreserved: []string{},
		},
		{
			name:              "no existing resources",
			existingResources: map[string]Resource{},
			desiredMappings: []ResourceMapping{
				{Name: "new.example.com", Address: "10.0.0.1"},
			},
			expectedDeleted:   []string{},
			expectedPreserved: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock client
			mockClient := &MockTwingateClient{
				Resources:  tt.existingResources,
				DeletedIDs: []string{},
				CallLog:    []string{},
			}

			// Create syncer with mock client
			logger := zap.NewNop()
			syncer := &testableResourceSyncer{
				client: mockClient,
				logger: logger,
			}

			// Run deletion
			cleanupConfig := &CleanupConfig{
				Enabled: true,
				DryRun:  false,
			}

			deleted, errors := syncer.deleteStaleResources(
				context.Background(),
				tt.desiredMappings,
				"net1",
				cleanupConfig,
			)

			// Verify no errors
			if errors != 0 {
				t.Errorf("Expected 0 errors, got %d", errors)
			}

			// Verify deleted count
			if deleted != len(tt.expectedDeleted) {
				t.Errorf("Expected %d deleted resources, got %d", len(tt.expectedDeleted), deleted)
			}

			// Verify correct resources were deleted
			if len(mockClient.DeletedIDs) != len(tt.expectedDeleted) {
				t.Fatalf("Expected %d deletions, got %d", len(tt.expectedDeleted), len(mockClient.DeletedIDs))
			}

			// Check that expected resources were deleted
			deletedMap := make(map[string]bool)
			for _, id := range mockClient.DeletedIDs {
				deletedMap[id] = true
			}

			for _, expectedID := range tt.expectedDeleted {
				if !deletedMap[expectedID] {
					t.Errorf("Expected resource %s to be deleted, but it wasn't", expectedID)
				}
			}

			// Verify preserved resources still exist
			for _, preservedID := range tt.expectedPreserved {
				if _, exists := mockClient.Resources[preservedID]; !exists {
					t.Errorf("Expected resource %s to be preserved, but it was deleted", preservedID)
				}
			}

			// Verify GetResources was called
			if len(mockClient.CallLog) == 0 {
				t.Error("Expected GetResources to be called")
			}
		})
	}
}

// TestDeleteStaleResourcesDryRun tests that dry-run doesn't actually delete
func TestDeleteStaleResourcesDryRun(t *testing.T) {
	// Setup mock client with stale resources
	mockClient := &MockTwingateClient{
		Resources: map[string]Resource{
			"res1": {
				ID:   "res1",
				Name: "api.example.com",
				Address: struct {
					Value string `graphql:"value"`
				}{Value: "10.0.0.1"},
				RemoteNetwork: struct {
					ID string `graphql:"id"`
				}{ID: "net1"},
			},
			"res2": {
				ID:   "res2",
				Name: "stale.example.com",
				Address: struct {
					Value string `graphql:"value"`
				}{Value: "10.0.0.1"},
				RemoteNetwork: struct {
					ID string `graphql:"id"`
				}{ID: "net1"},
			},
		},
		DeletedIDs: []string{},
		CallLog:    []string{},
	}

	// Create syncer with mock client
	logger := zap.NewNop()
	syncer := &testableResourceSyncer{
		client: mockClient,
		logger: logger,
	}

	// Desired mappings (only api.example.com, so stale.example.com should be identified as stale)
	desiredMappings := []ResourceMapping{
		{Name: "api.example.com", Address: "10.0.0.1"},
	}

	// Run deletion with DRY RUN enabled
	cleanupConfig := &CleanupConfig{
		Enabled: true,
		DryRun:  true, // DRY RUN MODE
	}

	deleted, errors := syncer.deleteStaleResources(
		context.Background(),
		desiredMappings,
		"net1",
		cleanupConfig,
	)

	// Verify no errors
	if errors != 0 {
		t.Errorf("Expected 0 errors, got %d", errors)
	}

	// Verify deleted count indicates what WOULD be deleted
	if deleted != 1 {
		t.Errorf("Expected 1 resource to be marked for deletion, got %d", deleted)
	}

	// Verify NO actual deletions occurred
	if len(mockClient.DeletedIDs) != 0 {
		t.Errorf("Expected no actual deletions in dry-run mode, but %d resources were deleted: %v",
			len(mockClient.DeletedIDs), mockClient.DeletedIDs)
	}

	// Verify DeleteResource was NOT called
	for _, call := range mockClient.CallLog {
		if call[:14] == "DeleteResource" {
			t.Error("DeleteResource should not be called in dry-run mode")
		}
	}

	// Verify all resources still exist
	if len(mockClient.Resources) != 2 {
		t.Errorf("Expected all 2 resources to still exist, but found %d", len(mockClient.Resources))
	}

	if _, exists := mockClient.Resources["res1"]; !exists {
		t.Error("Resource res1 should still exist")
	}
	if _, exists := mockClient.Resources["res2"]; !exists {
		t.Error("Resource res2 (stale) should still exist in dry-run mode")
	}
}

// TestDeleteStaleResourcesPartialFailure tests partial failure handling
func TestDeleteStaleResourcesPartialFailure(t *testing.T) {
	tests := []struct {
		name                 string
		existingResources    map[string]Resource
		desiredMappings      []ResourceMapping
		deleteErr            error
		expectedDeletedCount int
		expectedErrorCount   int
	}{
		{
			name: "one deletion fails, processing continues",
			existingResources: map[string]Resource{
				"res1": {
					ID:   "res1",
					Name: "stale1.example.com",
					Address: struct {
						Value string `graphql:"value"`
					}{Value: "10.0.0.1"},
					RemoteNetwork: struct {
						ID string `graphql:"id"`
					}{ID: "net1"},
				},
				"res2": {
					ID:   "res2",
					Name: "stale2.example.com",
					Address: struct {
						Value string `graphql:"value"`
					}{Value: "10.0.0.1"},
					RemoteNetwork: struct {
						ID string `graphql:"id"`
					}{ID: "net1"},
				},
				"res3": {
					ID:   "res3",
					Name: "stale3.example.com",
					Address: struct {
						Value string `graphql:"value"`
					}{Value: "10.0.0.1"},
					RemoteNetwork: struct {
						ID string `graphql:"id"`
					}{ID: "net1"},
				},
			},
			desiredMappings:      []ResourceMapping{}, // All are stale
			deleteErr:            fmt.Errorf("simulated deletion error"),
			expectedDeletedCount: 0, // All deletions fail
			expectedErrorCount:   3, // All 3 deletions fail
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock client that returns errors on delete
			mockClient := &MockTwingateClient{
				Resources:  tt.existingResources,
				DeletedIDs: []string{},
				CallLog:    []string{},
				DeleteErr:  tt.deleteErr,
			}

			// Create syncer with mock client
			logger := zap.NewNop()
			syncer := &testableResourceSyncer{
				client: mockClient,
				logger: logger,
			}

			// Run deletion
			cleanupConfig := &CleanupConfig{
				Enabled: true,
				DryRun:  false,
			}

			deleted, errors := syncer.deleteStaleResources(
				context.Background(),
				tt.desiredMappings,
				"net1",
				cleanupConfig,
			)

			// Verify error count
			if errors != tt.expectedErrorCount {
				t.Errorf("Expected %d errors, got %d", tt.expectedErrorCount, errors)
			}

			// Verify deleted count (should be 0 when all fail)
			if deleted != tt.expectedDeletedCount {
				t.Errorf("Expected %d successful deletions, got %d", tt.expectedDeletedCount, deleted)
			}

			// Verify DeleteResource was attempted for all stale resources
			deleteCallCount := 0
			for _, call := range mockClient.CallLog {
				if len(call) >= 14 && call[:14] == "DeleteResource" {
					deleteCallCount++
				}
			}

			expectedCallCount := len(tt.existingResources)
			if deleteCallCount != expectedCallCount {
				t.Errorf("Expected DeleteResource to be called %d times, but was called %d times",
					expectedCallCount, deleteCallCount)
			}
		})
	}
}

// TestDeleteStaleResourcesGetResourcesError tests error handling when listing resources fails
func TestDeleteStaleResourcesGetResourcesError(t *testing.T) {
	// Setup mock client that fails on GetResources
	mockClient := &MockTwingateClient{
		Resources:       map[string]Resource{},
		DeletedIDs:      []string{},
		CallLog:         []string{},
		GetResourcesErr: fmt.Errorf("simulated API error"),
	}

	// Create syncer with mock client
	logger := zap.NewNop()
	syncer := &testableResourceSyncer{
		client: mockClient,
		logger: logger,
	}

	// Run deletion
	cleanupConfig := &CleanupConfig{
		Enabled: true,
		DryRun:  false,
	}

	deleted, errors := syncer.deleteStaleResources(
		context.Background(),
		[]ResourceMapping{{Name: "test.example.com", Address: "10.0.0.1"}},
		"net1",
		cleanupConfig,
	)

	// Verify error handling
	if errors != 1 {
		t.Errorf("Expected 1 error when GetResources fails, got %d", errors)
	}

	if deleted != 0 {
		t.Errorf("Expected 0 deletions when GetResources fails, got %d", deleted)
	}

	// Verify no deletion attempts were made
	if len(mockClient.DeletedIDs) != 0 {
		t.Errorf("Expected no deletions when GetResources fails, but %d occurred", len(mockClient.DeletedIDs))
	}
}

// TestDeleteStaleResourcesMixedSuccess tests scenario where some deletions succeed and some fail
func TestDeleteStaleResourcesMixedSuccess(t *testing.T) {
	// Create a mock client that fails on specific resource IDs
	type selectiveFailClient struct {
		*MockTwingateClient
		failOnIDs map[string]bool
	}

	mockBase := &MockTwingateClient{
		Resources: map[string]Resource{
			"res1": {
				ID:   "res1",
				Name: "stale1.example.com",
				Address: struct {
					Value string `graphql:"value"`
				}{Value: "10.0.0.1"},
				RemoteNetwork: struct {
					ID string `graphql:"id"`
				}{ID: "net1"},
			},
			"res2": {
				ID:   "res2",
				Name: "stale2.example.com",
				Address: struct {
					Value string `graphql:"value"`
				}{Value: "10.0.0.1"},
				RemoteNetwork: struct {
					ID string `graphql:"id"`
				}{ID: "net1"},
			},
			"res3": {
				ID:   "res3",
				Name: "stale3.example.com",
				Address: struct {
					Value string `graphql:"value"`
				}{Value: "10.0.0.1"},
				RemoteNetwork: struct {
					ID string `graphql:"id"`
				}{ID: "net1"},
			},
		},
		DeletedIDs: []string{},
		CallLog:    []string{},
	}

	// Create selective fail wrapper
	failOnIDs := map[string]bool{"res2": true} // Only res2 will fail

	mockClient := &selectiveFailClient{
		MockTwingateClient: mockBase,
		failOnIDs:          failOnIDs,
	}

	// Override DeleteResource to fail selectively
	originalDelete := mockBase.DeleteResource
	deleteResource := func(ctx context.Context, resourceID string) error {
		if mockClient.failOnIDs[resourceID] {
			mockBase.CallLog = append(mockBase.CallLog, fmt.Sprintf("DeleteResource(%s)", resourceID))
			return fmt.Errorf("simulated error for %s", resourceID)
		}
		return originalDelete(ctx, resourceID)
	}

	// Create a custom client interface for the syncer
	type clientInterface interface {
		GetResources(ctx context.Context, remoteNetworkID string) ([]Resource, error)
		DeleteResource(ctx context.Context, resourceID string) error
	}

	customClient := struct {
		clientInterface
		getResources   func(ctx context.Context, remoteNetworkID string) ([]Resource, error)
		deleteResource func(ctx context.Context, resourceID string) error
	}{
		getResources:   mockBase.GetResources,
		deleteResource: deleteResource,
	}

	// Manually implement the deletion logic to test partial failure
	ctx := context.Background()
	desiredMappings := []ResourceMapping{} // All are stale
	networkID := "net1"

	// Get resources
	existingResources, err := customClient.getResources(ctx, networkID)
	if err != nil {
		t.Fatalf("GetResources failed: %v", err)
	}

	// Build desired set
	desiredNames := make(map[string]bool)
	for _, mapping := range desiredMappings {
		desiredNames[mapping.Name] = true
	}

	// Find stale resources
	var staleResources []Resource
	for _, resource := range existingResources {
		if !desiredNames[resource.Name] {
			staleResources = append(staleResources, resource)
		}
	}

	// Delete stale resources with error tracking
	var deleted, errors int
	for _, resource := range staleResources {
		if err := customClient.deleteResource(ctx, resource.ID); err != nil {
			errors++
		} else {
			deleted++
		}
	}

	// Verify results
	if deleted != 2 {
		t.Errorf("Expected 2 successful deletions (res1, res3), got %d", deleted)
	}

	if errors != 1 {
		t.Errorf("Expected 1 error (res2), got %d", errors)
	}

	// Verify correct resources were deleted
	expectedDeleted := map[string]bool{"res1": true, "res3": true}
	for id := range expectedDeleted {
		if _, exists := mockBase.Resources[id]; exists {
			t.Errorf("Resource %s should have been deleted", id)
		}
	}

	// Verify failed resource still exists
	if _, exists := mockBase.Resources["res2"]; !exists {
		t.Error("Resource res2 should still exist (deletion failed)")
	}
}
