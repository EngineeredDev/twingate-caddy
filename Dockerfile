# Dockerfile for Twingate Caddy Plugin
# Uses official Caddy builder image to create a custom Caddy binary with Twingate plugin

# Build stage: Use official Caddy builder image
FROM caddy:2-builder AS builder

# Set working directory
WORKDIR /build

# Copy Go module files first for better caching
COPY go.mod go.sum ./

# Download dependencies (cached layer)
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy the entire source code
COPY . .

# Build Caddy with the Twingate plugin from local source
# This builds from cmd/caddy/main.go which imports the plugin
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 \
    go build \
        -ldflags="-s -w" \
        -o /usr/bin/caddy \
        ./cmd/caddy

# Runtime stage: Use official Caddy image
FROM caddy:2

# Copy the custom Caddy binary from builder stage
COPY --from=builder /usr/bin/caddy /usr/bin/caddy

# The official Caddy image already sets up:
# - Non-root user (caddy)
# - Exposed ports (80, 443, 2019)
# - Volume for /data
# - Volume for /config
# - Working directory /srv
# - Default command: caddy run --config /etc/caddy/Caddyfile --adapter caddyfile

# Labels for metadata
LABEL org.opencontainers.image.source="https://github.com/EngineeredDev/twingate-caddy"
LABEL org.opencontainers.image.description="Caddy web server with Twingate Zero Trust Network plugin"
LABEL org.opencontainers.image.licenses="MIT"
