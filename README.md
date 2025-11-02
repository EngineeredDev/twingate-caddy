# Twingate Caddy Plugin

A Caddy module that automatically synchronizes your reverse proxy configurations with Twingate, enabling secure Zero Trust access to your services.

## What It Does

This module scans your Caddy reverse proxy routes and automatically creates corresponding Twingate resources. When you add or update reverse proxy configurations in your Caddyfile, the module syncs them to Twingate so they're accessible through your Zero Trust network.

## Installation

### Option 1: Download Pre-built Binary

Download the latest release for your platform from the [Releases](https://github.com/EngineeredDev/twingate-caddy/releases) page.

**Supported Platforms:**
- **Linux**: amd64, arm64, armv6, armv7
- **FreeBSD**: amd64

```bash
# Make it executable (Linux/macOS/FreeBSD)
chmod +x caddy

# Run it
./caddy run --config Caddyfile
```

### Option 2: Build from Source

Requires Go 1.25+ and [xcaddy](https://github.com/caddyserver/xcaddy).

```bash
# Install xcaddy
go install github.com/caddyserver/xcaddy/cmd/xcaddy@latest

# Build Caddy with this module
xcaddy build --with github.com/EngineeredDev/twingate-caddy

# Run
./caddy run --config Caddyfile
```

### Option 3: Docker

Docker images are available from GitHub Container Registry.

**Supported Platforms:**
- **linux/amd64**: x86_64 64-bit
- **linux/arm64**: ARM 64-bit (including Apple Silicon via Docker Desktop)

```bash
# Pull the latest image
docker pull ghcr.io/engineereddev/twingate-caddy:latest

# Or pull a specific version
docker pull ghcr.io/engineereddev/twingate-caddy:v0.0.1

# Run with a Caddyfile from your host
docker run -d \
  --name twingate-caddy \
  -p 80:80 \
  -p 443:443 \
  -e TWINGATE_API_KEY=your_api_key_here \
  -v $(pwd)/Caddyfile:/etc/caddy/Caddyfile \
  -v caddy_data:/data \
  -v caddy_config:/config \
  ghcr.io/engineereddev/twingate-caddy:latest
```

**Docker Compose Example:**

```yaml
version: '3.8'

services:
  twingate-caddy:
    image: ghcr.io/engineereddev/twingate-caddy:latest
    container_name: twingate-caddy
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
      - "2019:2019"  # Admin API (optional)
    environment:
      - TWINGATE_API_KEY=${TWINGATE_API_KEY}
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile
      - caddy_data:/data
      - caddy_config:/config

volumes:
  caddy_data:
  caddy_config:
```

## Configuration

### Get Twingate API Credentials

1. Log in to your Twingate Admin Console
2. Navigate to Settings → API
3. Generate a new API token
4. Set the API token as the `TWINGATE_API_KEY` environment variable
5. Note your tenant name (subdomain in your Twingate URL)

### Configure Your Caddyfile

Add the Twingate configuration block to your Caddyfile:

```caddyfile
{
    twingate {
        tenant "your-company"               # Required: Your Twingate tenant name
        remote_network "Caddy-Resources"    # Optional: Remote network (defaults to "Caddy-Managed")
        caddy_address "192.168.1.100"       # Optional: Caddy server address for Twingate
        resource_cleanup {
            enabled true                     # Optional: Auto-delete resources not in Caddyfile
        }
    }
}

# Your reverse proxy configurations
api.example.com {
    reverse_proxy localhost:8080
}

app.example.com {
    reverse_proxy /api/* localhost:3000
    reverse_proxy /admin/* localhost:3001
}
```

## How It Works

1. Scans your Caddy configuration for `reverse_proxy` directives
2. Extracts route patterns (hosts, paths) and upstream destinations
3. Creates or updates Twingate resources in your specified remote network
4. Resources are named after the route pattern and point to the upstream address
5. Re-syncs automatically when Caddy configuration is reloaded

### What Gets Created

For each reverse proxy route, a Twingate resource is created:

- **Name**: The source route (e.g., `api.example.com`, `app.example.com/api/`)
- **Address**: The Caddy server's outbound address (auto-detected or manually set via `caddy_address`)

Example:
```caddyfile
{
    twingate {
        tenant "your-company"
        caddy_address "192.168.1.100"  # Optional: Set manually, otherwise auto-detected
    }
}

api.example.com {
    reverse_proxy http://localhost:3000
}
```
Creates a Twingate resource:
- Name: `api.example.com`
- Address: `192.168.1.100` (the Caddy server's address, not the upstream)

### Resource Cleanup

If `resource_cleanup.enabled` is `true`, the module will **delete** any resources in the remote network that aren't defined in your Caddyfile. Use a dedicated remote network for Caddy-managed resources to avoid accidentally deleting manually created resources.

## Supported Routing Patterns

- Host-based: `api.example.com { reverse_proxy localhost:8080 }`
- Path-based: `reverse_proxy /api/* localhost:3000`
- Wildcards: `*.dev.example.com { reverse_proxy localhost:9000 }`
- Load balancing: `reverse_proxy localhost:8001 localhost:8002`
- Handle blocks: `handle /v1/* { reverse_proxy localhost:8100 }`

See [examples/Caddyfile](examples/Caddyfile) for more patterns.

## Troubleshooting

### Enable Debug Logging

```bash
caddy run --config Caddyfile --log-level debug
```

### Common Issues

**API Connection Failed**
- Verify the `TWINGATE_API_KEY` environment variable is set
- Verify tenant name is correct
- Check network access to `https://{tenant}.twingate.com/api/graphql/`

**No Resources Created**
- Ensure `reverse_proxy` directives exist in your Caddyfile
- Verify the `TWINGATE_API_KEY` environment variable is set and has permissions to create resources and networks

**Resources Not Updating**
- Reload Caddy config: `caddy reload --config Caddyfile`
- Check logs for sync errors

## Security Notes

- The API key is read from the `TWINGATE_API_KEY` environment variable for security
- Never commit API keys to version control
- Use dedicated remote networks to isolate Caddy-managed resources
- Review and apply appropriate Twingate access policies to created resources
- The `resource_cleanup` feature will delete resources - use with caution

## License

MIT License - see [LICENSE](LICENSE) for details.

## Support

- [Report Issues](https://github.com/EngineeredDev/twingate-caddy/issues)
- [Twingate Documentation](https://www.twingate.com/docs)
- [Caddy Documentation](https://caddyserver.com/docs)
