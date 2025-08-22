package twingate

import (
	"net"
	"os"
	"testing"
)

func TestGetOutboundIP(t *testing.T) {
	ip, err := GetOutboundIP()
	if err != nil {
		t.Fatalf("GetOutboundIP() failed: %v", err)
	}

	if net.ParseIP(ip) == nil {
		t.Errorf("GetOutboundIP() returned invalid IP: %s", ip)
	}

	// Should be IPv4
	if net.ParseIP(ip).To4() == nil {
		t.Errorf("GetOutboundIP() returned non-IPv4 address: %s", ip)
	}
}

func TestValidate_WithAPIKey(t *testing.T) {
	// Set the environment variable
	t.Setenv("TWINGATE_API_KEY", "test-api-key-123")

	app := &TwingateApp{
		Tenant: "test-tenant",
	}

	err := app.Validate()
	if err != nil {
		t.Errorf("Validate() should succeed with valid API key, got error: %v", err)
	}
}

func TestValidate_MissingAPIKey(t *testing.T) {
	// Ensure the environment variable is not set
	os.Unsetenv("TWINGATE_API_KEY")

	app := &TwingateApp{
		Tenant: "test-tenant",
	}

	err := app.Validate()
	if err == nil {
		t.Error("Validate() should fail when TWINGATE_API_KEY is not set")
	}
	if err != nil && err.Error() != "TWINGATE_API_KEY environment variable is required" {
		t.Errorf("Validate() returned unexpected error message: %v", err)
	}
}

func TestValidate_EmptyAPIKey(t *testing.T) {
	// Set the environment variable to empty string
	t.Setenv("TWINGATE_API_KEY", "")

	app := &TwingateApp{
		Tenant: "test-tenant",
	}

	err := app.Validate()
	if err == nil {
		t.Error("Validate() should fail when TWINGATE_API_KEY is empty")
	}
	if err != nil && err.Error() != "TWINGATE_API_KEY environment variable is required" {
		t.Errorf("Validate() returned unexpected error message: %v", err)
	}
}

func TestValidate_MissingTenant(t *testing.T) {
	// Set valid API key but no tenant
	t.Setenv("TWINGATE_API_KEY", "test-api-key-123")

	app := &TwingateApp{
		Tenant: "",
	}

	err := app.Validate()
	if err == nil {
		t.Error("Validate() should fail when tenant is not set")
	}
	if err != nil && err.Error() != "tenant is required" {
		t.Errorf("Validate() returned unexpected error message: %v", err)
	}
}
