package twingate

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/reverseproxy"
	"go.uber.org/zap"
)

type RouteDiscoverer struct {
	logger       *zap.Logger
	caddyAddress string
}

type RouteContext struct {
	Hosts []string
	Path  string
}

type Endpoint struct {
	Host string
	Path string
}

func (e *Endpoint) CanonicalKey() string {
	return e.Host + "\x00" + e.Path
}

// ResourceName consolidates all paths on a host into one resource, returning only the hostname
func (e *Endpoint) ResourceName() string {
	return e.Host
}

// ResourceAlias returns nil for wildcard hosts, otherwise returns the hostname as a valid DNS name
func (e *Endpoint) ResourceAlias() *string {
	if strings.Contains(e.Host, "*") {
		return nil
	}

	return &e.Host
}

func (e *Endpoint) ToResourceMapping(caddyAddress string) ResourceMapping {
	return ResourceMapping{
		Name:    e.ResourceName(),
		Alias:   e.ResourceAlias(),
		Address: caddyAddress,
	}
}

func (d *RouteDiscoverer) DiscoverEndpoints(httpApp *caddyhttp.App) ([]Endpoint, error) {
	endpointMap := make(map[string]Endpoint)

	for serverName, server := range httpApp.Servers {
		d.logger.Debug("Scanning server", zap.String("server", serverName))

		ctx := RouteContext{
			Hosts: []string{},
			Path:  "",
		}

		if serverName != "" && serverName != "srv0" {
			ctx.Hosts = []string{serverName}
		}

		for i, route := range server.Routes {
			d.logger.Debug("Scanning route",
				zap.String("server", serverName),
				zap.Int("route_index", i))

			d.traverseRoute(route, ctx, endpointMap)
		}
	}

	endpoints := make([]Endpoint, 0, len(endpointMap))
	for _, ep := range endpointMap {
		endpoints = append(endpoints, ep)
	}

	// Deduplicate by host - Twingate works at the host level, not per-path
	hostMap := make(map[string]Endpoint)
	for _, ep := range endpoints {
		if _, exists := hostMap[ep.Host]; !exists {
			hostMap[ep.Host] = Endpoint{
				Host: ep.Host,
				Path: "",
			}
		}
	}

	endpoints = make([]Endpoint, 0, len(hostMap))
	for _, ep := range hostMap {
		endpoints = append(endpoints, ep)
	}

	d.logger.Info("Route discovery complete",
		zap.Int("endpoints_found", len(endpoints)))

	return endpoints, nil
}

func (d *RouteDiscoverer) traverseRoute(route caddyhttp.Route, parentCtx RouteContext, endpointMap map[string]Endpoint) {
	ctx := d.mergeMatchers(route, parentCtx)

	for _, handler := range route.Handlers {
		d.traverseHandler(handler, ctx, endpointMap)
	}

	if len(route.Handlers) == 0 && len(route.HandlersRaw) > 0 {
		for _, handlerRaw := range route.HandlersRaw {
			d.traverseHandlerRaw(handlerRaw, ctx, endpointMap)
		}
	}
}

func (d *RouteDiscoverer) traverseHandler(handler caddyhttp.MiddlewareHandler, ctx RouteContext, endpointMap map[string]Endpoint) {
	switch h := handler.(type) {
	case *reverseproxy.Handler:
		d.emitEndpoints(ctx, endpointMap)

	case *caddyhttp.Subroute:
		for _, route := range h.Routes {
			d.traverseRoute(route, ctx, endpointMap)
		}

	default:
		d.logger.Debug("Skipping handler type",
			zap.String("type", fmt.Sprintf("%T", handler)))
	}
}

func (d *RouteDiscoverer) traverseHandlerRaw(handlerRaw json.RawMessage, ctx RouteContext, endpointMap map[string]Endpoint) {
	var handlerConfig map[string]any
	if err := json.Unmarshal(handlerRaw, &handlerConfig); err != nil {
		d.logger.Warn("Failed to unmarshal handler", zap.Error(err))
		return
	}

	handlerType, ok := handlerConfig["handler"].(string)
	if !ok {
		return
	}

	switch handlerType {
	case "reverse_proxy":
		d.emitEndpoints(ctx, endpointMap)

	case "subroute":
		if routes, ok := handlerConfig["routes"].([]any); ok {
			for _, routeAny := range routes {
				if routeBytes, err := json.Marshal(routeAny); err == nil {
					var route caddyhttp.Route
					if err := json.Unmarshal(routeBytes, &route); err == nil {
						d.traverseRoute(route, ctx, endpointMap)
					}
				}
			}
		}
	}
}

func (d *RouteDiscoverer) mergeMatchers(route caddyhttp.Route, parentCtx RouteContext) RouteContext {
	ctx := RouteContext{
		Hosts: parentCtx.Hosts,
		Path:  parentCtx.Path,
	}

	for _, matcherSet := range route.MatcherSets {
		for _, matcher := range matcherSet {
			switch m := matcher.(type) {
			case *caddyhttp.MatchHost:
				ctx.Hosts = []string(*m)

			case *caddyhttp.MatchPath:
				paths := []string(*m)
				if len(paths) > 0 {
					ctx.Path = d.normalizePath(paths[0])
				}
			}
		}
	}

	return ctx
}

// normalizePath converts path matchers to consistent format:
// "/api/*" -> "/api/", "/api" -> "/api/", "/" -> ""
func (d *RouteDiscoverer) normalizePath(path string) string {
	path = strings.TrimSuffix(path, "*")

	if path != "" && path != "/" && !strings.HasSuffix(path, "/") {
		path = path + "/"
	}

	if path == "/" {
		path = ""
	}

	return path
}

func (d *RouteDiscoverer) emitEndpoints(ctx RouteContext, endpointMap map[string]Endpoint) {
	hosts := ctx.Hosts
	if len(hosts) == 0 {
		hosts = []string{"localhost"}
		d.logger.Warn("No host matchers found for reverse_proxy, using localhost",
			zap.String("path", ctx.Path))
	}

	for _, host := range hosts {
		ep := Endpoint{
			Host: host,
			Path: ctx.Path,
		}

		key := ep.CanonicalKey()

		if _, exists := endpointMap[key]; !exists {
			endpointMap[key] = ep
			d.logger.Debug("Discovered endpoint",
				zap.String("host", ep.Host),
				zap.String("path", ep.Path),
				zap.String("resource_name", ep.ResourceName()))
		}
	}
}
