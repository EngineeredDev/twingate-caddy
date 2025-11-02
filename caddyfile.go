package twingate

import (
	"encoding/json"
	"net"
	"strconv"

	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
)

func parseTwingateApp(d *caddyfile.Dispenser, _ any) (any, error) {
	app := &TwingateApp{}

	for d.Next() {
		for d.NextBlock(0) {
			switch d.Val() {
			case "tenant":
				if !d.NextArg() {
					return nil, d.ArgErr()
				}
				app.Tenant = d.Val()

			case "remote_network":
				if !d.NextArg() {
					return nil, d.ArgErr()
				}
				app.RemoteNetwork = d.Val()

			case "caddy_address":
				if !d.NextArg() {
					return nil, d.ArgErr()
				}
				addr := d.Val()

				if net.ParseIP(addr) == nil {
					return nil, d.Errf("caddy_address must be a valid IP address, got: %s", addr)
				}
				app.CaddyAddress = addr

			case "resource_cleanup":
				cleanup := &CleanupConfig{}
				for d.NextBlock(1) {
					switch d.Val() {
					case "enabled":
						if !d.NextArg() {
							return nil, d.ArgErr()
						}
						enabled, err := strconv.ParseBool(d.Val())
						if err != nil {
							return nil, d.Errf("enabled must be true or false, got: %s", d.Val())
						}
						cleanup.Enabled = enabled

					case "dry_run":
						if !d.NextArg() {
							return nil, d.ArgErr()
						}
						dryRun, err := strconv.ParseBool(d.Val())
						if err != nil {
							return nil, d.Errf("dry_run must be true or false, got: %s", d.Val())
						}
						cleanup.DryRun = dryRun

					default:
						return nil, d.Errf("unrecognized resource_cleanup directive: %s", d.Val())
					}
				}
				app.ResourceCleanup = cleanup

			default:
				return nil, d.Errf("unrecognized directive: %s", d.Val())
			}
		}
	}

	if app.Tenant == "" {
		return nil, d.Err("tenant is required")
	}

	// Marshal to JSON for httpcaddyfile.App wrapper
	appJSON, err := json.Marshal(app)
	if err != nil {
		return nil, d.Errf("failed to marshal twingate app config: %v", err)
	}

	return httpcaddyfile.App{
		Name:  "twingate",
		Value: appJSON,
	}, nil
}

// UnmarshalCaddyfile implements caddyfile.Unmarshaler for direct JSON config
func (t *TwingateApp) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		for d.NextBlock(0) {
			switch d.Val() {
			case "tenant":
				if !d.NextArg() {
					return d.ArgErr()
				}
				t.Tenant = d.Val()

			case "remote_network":
				if !d.NextArg() {
					return d.ArgErr()
				}
				t.RemoteNetwork = d.Val()

			case "caddy_address":
				if !d.NextArg() {
					return d.ArgErr()
				}
				addr := d.Val()

				if net.ParseIP(addr) == nil {
					return d.Errf("caddy_address must be a valid IP address, got: %s", addr)
				}
				t.CaddyAddress = addr

			case "resource_cleanup":
				cleanup := &CleanupConfig{}
				for d.NextBlock(1) {
					switch d.Val() {
					case "enabled":
						if !d.NextArg() {
							return d.ArgErr()
						}
						enabled, err := strconv.ParseBool(d.Val())
						if err != nil {
							return d.Errf("enabled must be true or false, got: %s", d.Val())
						}
						cleanup.Enabled = enabled

					case "dry_run":
						if !d.NextArg() {
							return d.ArgErr()
						}
						dryRun, err := strconv.ParseBool(d.Val())
						if err != nil {
							return d.Errf("dry_run must be true or false, got: %s", d.Val())
						}
						cleanup.DryRun = dryRun

					default:
						return d.Errf("unrecognized resource_cleanup directive: %s", d.Val())
					}
				}
				t.ResourceCleanup = cleanup

			default:
				return d.Errf("unrecognized directive: %s", d.Val())
			}
		}
	}

	return nil
}

var _ caddyfile.Unmarshaler = (*TwingateApp)(nil)
