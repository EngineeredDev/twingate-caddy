package main

import (
	caddycmd "github.com/caddyserver/caddy/v2/cmd"

	// Import Caddy's standard modules
	_ "github.com/caddyserver/caddy/v2/modules/standard"

	// Import the Twingate plugin
	_ "github.com/twingate/twingate-caddy-plugin"
)

func main() {
	caddycmd.Main()
}
