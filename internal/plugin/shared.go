// Package plugin implements scouter-server's external plugin system on top
// of hashicorp/go-plugin. Each plugin is a separate OS process that
// implements one or more gRPC services defined in internal/plugin/proto.
//
// Two hooks are currently supported:
//
//   - IAlert: called once per AlertPack
//   - ICounter: called once per PerfCounterPack
//
// Plugins are loaded from the directory returned by config.PluginDir().
// Any file directly inside that directory that is executable and whose
// name does not start with "." is treated as a plugin. Subdirectories
// are ignored. See docs/plugin.md for the plugin authoring guide.
package plugin

import (
	"context"

	hplugin "github.com/hashicorp/go-plugin"
	pluginpb "github.com/scouter-project/scouter-server-go/internal/plugin/proto"
	"google.golang.org/grpc"
)

// Handshake is the shared handshake between scouter-server and its
// plugins. Changing any field invalidates previously built plugins, so
// treat bumps to ProtocolVersion as a breaking change.
var Handshake = hplugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "SCOUTER_PLUGIN_COOKIE",
	MagicCookieValue: "scouter-server-go-plugin-v1",
}

// Plugin type keys used by hashicorp/go-plugin. A single plugin binary
// MAY serve both IAlert and ICounter; scouter-server requests whichever
// ones it needs and silently skips the ones the binary does not export.
const (
	PluginKeyAlert   = "alert"
	PluginKeyCounter = "counter"
)

// PluginMap is advertised by the host (scouter-server) and by plugin
// implementations. It enumerates every service scouter knows about.
var PluginMap = map[string]hplugin.Plugin{
	PluginKeyAlert:   &AlertGRPCPlugin{},
	PluginKeyCounter: &CounterGRPCPlugin{},
}

// ---------------------------------------------------------------------------
// Host-side interfaces
// ---------------------------------------------------------------------------

// AlertHook is the Go-side view of a loaded IAlert plugin.
type AlertHook interface {
	Process(ctx context.Context, p *pluginpb.AlertPayload) error
}

// CounterHook is the Go-side view of a loaded ICounter plugin.
type CounterHook interface {
	Process(ctx context.Context, p *pluginpb.CounterPayload) error
}

// ---------------------------------------------------------------------------
// AlertGRPCPlugin — hashicorp/go-plugin adapter for the IAlert service.
// ---------------------------------------------------------------------------

type AlertGRPCPlugin struct {
	hplugin.Plugin
	// Impl is only set on the plugin side (the external process). Hosts
	// leave this nil — GRPCClient is what matters on the host.
	Impl AlertHook
}

func (p *AlertGRPCPlugin) GRPCServer(_ *hplugin.GRPCBroker, s *grpc.Server) error {
	pluginpb.RegisterIAlertServer(s, &alertServerAdapter{impl: p.Impl})
	return nil
}

func (p *AlertGRPCPlugin) GRPCClient(_ context.Context, _ *hplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &alertClientAdapter{client: pluginpb.NewIAlertClient(c)}, nil
}

type alertClientAdapter struct {
	client pluginpb.IAlertClient
}

func (a *alertClientAdapter) Process(ctx context.Context, p *pluginpb.AlertPayload) error {
	_, err := a.client.Process(ctx, p)
	return err
}

type alertServerAdapter struct {
	pluginpb.UnimplementedIAlertServer
	impl AlertHook
}

func (a *alertServerAdapter) Process(ctx context.Context, p *pluginpb.AlertPayload) (*pluginpb.Ack, error) {
	if a.impl == nil {
		return &pluginpb.Ack{}, nil
	}
	if err := a.impl.Process(ctx, p); err != nil {
		return nil, err
	}
	return &pluginpb.Ack{}, nil
}

// ---------------------------------------------------------------------------
// CounterGRPCPlugin — hashicorp/go-plugin adapter for the ICounter service.
// ---------------------------------------------------------------------------

type CounterGRPCPlugin struct {
	hplugin.Plugin
	Impl CounterHook
}

func (p *CounterGRPCPlugin) GRPCServer(_ *hplugin.GRPCBroker, s *grpc.Server) error {
	pluginpb.RegisterICounterServer(s, &counterServerAdapter{impl: p.Impl})
	return nil
}

func (p *CounterGRPCPlugin) GRPCClient(_ context.Context, _ *hplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &counterClientAdapter{client: pluginpb.NewICounterClient(c)}, nil
}

type counterClientAdapter struct {
	client pluginpb.ICounterClient
}

func (c *counterClientAdapter) Process(ctx context.Context, p *pluginpb.CounterPayload) error {
	_, err := c.client.Process(ctx, p)
	return err
}

type counterServerAdapter struct {
	pluginpb.UnimplementedICounterServer
	impl CounterHook
}

func (c *counterServerAdapter) Process(ctx context.Context, p *pluginpb.CounterPayload) (*pluginpb.Ack, error) {
	if c.impl == nil {
		return &pluginpb.Ack{}, nil
	}
	if err := c.impl.Process(ctx, p); err != nil {
		return nil, err
	}
	return &pluginpb.Ack{}, nil
}

// Serve is a convenience for Go-based plugin authors. Pass any combination
// of AlertHook/CounterHook; nil entries are skipped.
func Serve(alert AlertHook, counter CounterHook) {
	pmap := map[string]hplugin.Plugin{
		PluginKeyAlert:   &AlertGRPCPlugin{Impl: alert},
		PluginKeyCounter: &CounterGRPCPlugin{Impl: counter},
	}
	hplugin.Serve(&hplugin.ServeConfig{
		HandshakeConfig: Handshake,
		Plugins:         pmap,
		GRPCServer:      hplugin.DefaultGRPCServer,
	})
}