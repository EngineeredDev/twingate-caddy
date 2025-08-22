package twingate

import (
	"testing"
)

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/api/*", "/api/"},
		{"/api", "/api/"},
		{"/", ""},
		{"/admin/*", "/admin/"},
		{"/*", ""},
	}

	d := &RouteDiscoverer{}
	for _, tt := range tests {
		result := d.normalizePath(tt.input)
		if result != tt.expected {
			t.Errorf("normalizePath(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestCanonicalKey(t *testing.T) {
	tests := []struct {
		ep1  Endpoint
		ep2  Endpoint
		same bool
	}{
		{
			Endpoint{Host: "api.example.com", Path: ""},
			Endpoint{Host: "api.example.com", Path: ""},
			true,
		},
		{
			Endpoint{Host: "api.example.com", Path: "/v1/"},
			Endpoint{Host: "api.example.com", Path: "/v1/"},
			true,
		},
		{
			Endpoint{Host: "api.example.com", Path: "/v1/"},
			Endpoint{Host: "api.example.com", Path: "/v2/"},
			false,
		},
	}

	for _, tt := range tests {
		key1 := tt.ep1.CanonicalKey()
		key2 := tt.ep2.CanonicalKey()

		if tt.same && key1 != key2 {
			t.Errorf("Expected same keys for %+v and %+v, got %q and %q",
				tt.ep1, tt.ep2, key1, key2)
		}
		if !tt.same && key1 == key2 {
			t.Errorf("Expected different keys for %+v and %+v, got %q",
				tt.ep1, tt.ep2, key1)
		}
	}
}

func TestResourceAlias(t *testing.T) {
	tests := []struct {
		host          string
		path          string
		expectedAlias *string
	}{
		{"api.example.com", "", strPtr("api.example.com")},
		{"*.dev.example.com", "", nil},
		{"*.example.com", "/api/", nil},
		{"app.example.com", "/v1/", strPtr("app.example.com")}, // Alias is hostname only, not hostname+path
	}

	for _, tt := range tests {
		ep := Endpoint{Host: tt.host, Path: tt.path}
		alias := ep.ResourceAlias()

		if tt.expectedAlias == nil && alias != nil {
			t.Errorf("Expected no alias for %+v, got %q", ep, *alias)
		}
		if tt.expectedAlias != nil && alias == nil {
			t.Errorf("Expected alias for %+v, got nil", ep)
		}
		if tt.expectedAlias != nil && alias != nil && *alias != *tt.expectedAlias {
			t.Errorf("Expected alias %q for %+v, got %q", *tt.expectedAlias, ep, *alias)
		}
	}
}

func TestResourceName(t *testing.T) {
	tests := []struct {
		host         string
		path         string
		expectedName string
	}{
		{"api.example.com", "", "api.example.com"},
		{"app.example.com", "/v1/", "app.example.com"}, // Name is hostname only, path is ignored
		{"*.dev.example.com", "", "*.dev.example.com"},
		{"*.example.com", "/api/", "*.example.com"},
	}

	for _, tt := range tests {
		ep := Endpoint{Host: tt.host, Path: tt.path}
		name := ep.ResourceName()

		if name != tt.expectedName {
			t.Errorf("Expected ResourceName() = %q for %+v, got %q", tt.expectedName, ep, name)
		}
	}
}

// Helper function to create string pointers
func strPtr(s string) *string {
	return &s
}
