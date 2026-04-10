package plugin

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	hclog "github.com/hashicorp/go-hclog"
	hplugin "github.com/hashicorp/go-plugin"
	pluginpb "github.com/scouter-project/scouter-server-go/internal/plugin/proto"
	"github.com/scouter-project/scouter-server-go/internal/protocol/pack"
)

// Default per-call timeout. Plugin RPCs must not block scouter's alert
// or counter processing goroutines for longer than this.
const defaultCallTimeout = 2 * time.Second

// loaded tracks a single running plugin process and the hooks it exposes.
type loaded struct {
	path    string
	name    string
	client  *hplugin.Client
	alert   AlertHook
	counter CounterHook
}

// Manager discovers, launches, and dispatches events to plugins. It is
// safe for concurrent use. Construct with NewManager; call Start once on
// server boot and Stop on shutdown.
type Manager struct {
	dir     string
	enabled bool
	timeout time.Duration

	mu      sync.RWMutex
	plugins []*loaded
}

// NewManager returns a Manager configured to scan dir. If enabled is
// false, the manager becomes a no-op (Start/Dispatch calls return
// immediately). Passing an empty dir disables the manager too.
func NewManager(dir string, enabled bool) *Manager {
	if dir == "" {
		enabled = false
	}
	return &Manager{
		dir:     dir,
		enabled: enabled,
		timeout: defaultCallTimeout,
	}
}

// Start scans the plugin directory and launches every executable file it
// finds. Errors for individual plugins are logged, never fatal — a bad
// plugin must not prevent scouter from starting.
func (m *Manager) Start(ctx context.Context) {
	if !m.enabled {
		return
	}
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("plugin: scan failed", "dir", m.dir, "error", err)
		}
		return
	}
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		// Skip non-plugin files (config, docs).
		name := e.Name()
		if strings.HasSuffix(name, ".alert") || strings.HasSuffix(name, ".conf") ||
			strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".txt") {
			continue
		}
		path := filepath.Join(m.dir, name)
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.Mode()&0o111 == 0 {
			// Not executable — silently skip; users often drop READMEs
			// or example sources in the plugin dir.
			continue
		}
		if l, err := m.launch(path); err != nil {
			slog.Warn("plugin: launch failed", "path", path, "error", err)
		} else {
			m.mu.Lock()
			m.plugins = append(m.plugins, l)
			m.mu.Unlock()
			slog.Info("plugin: loaded",
				"name", l.name,
				"hasAlert", l.alert != nil,
				"hasCounter", l.counter != nil)
		}
	}

	// Shut everything down when ctx is cancelled.
	go func() {
		<-ctx.Done()
		m.Stop()
	}()
}

// launch spawns a single plugin binary and dispenses whichever hooks it
// advertises. Missing hooks are tolerated; a binary that exports only
// IAlert (no ICounter) is perfectly valid.
func (m *Manager) launch(path string) (*loaded, error) {
	cmd := exec.Command(path)
	// Route plugin stderr into our log stream so users see panics.
	logger := hclog.New(&hclog.LoggerOptions{
		Name:   "plugin." + filepath.Base(path),
		Output: io.Discard, // avoid duplicate lines; scouter logs events itself
		Level:  hclog.Info,
	})
	client := hplugin.NewClient(&hplugin.ClientConfig{
		HandshakeConfig:  Handshake,
		Plugins:          PluginMap,
		Cmd:              cmd,
		AllowedProtocols: []hplugin.Protocol{hplugin.ProtocolGRPC},
		Logger:           logger,
	})
	rpc, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("rpc handshake: %w", err)
	}
	l := &loaded{
		path:   path,
		name:   filepath.Base(path),
		client: client,
	}
	if raw, err := rpc.Dispense(PluginKeyAlert); err == nil {
		if hook, ok := raw.(AlertHook); ok {
			l.alert = hook
		}
	}
	if raw, err := rpc.Dispense(PluginKeyCounter); err == nil {
		if hook, ok := raw.(CounterHook); ok {
			l.counter = hook
		}
	}
	if l.alert == nil && l.counter == nil {
		client.Kill()
		return nil, fmt.Errorf("plugin exports neither alert nor counter hook")
	}
	return l, nil
}

// Stop kills every loaded plugin. Safe to call more than once.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.plugins {
		if p.client != nil {
			p.client.Kill()
			p.client = nil
		}
	}
	m.plugins = nil
}

// DispatchAlert fans an AlertPack out to every plugin that exported an
// IAlert hook. Errors and panics are logged and swallowed — a crashing
// plugin must never take down scouter's alert pipeline.
func (m *Manager) DispatchAlert(ap *pack.AlertPack) {
	if !m.enabled || ap == nil {
		return
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.plugins) == 0 {
		return
	}
	payload := alertToProto(ap)
	for _, p := range m.plugins {
		if p.alert == nil {
			continue
		}
		m.callAlert(p, payload)
	}
}

// DispatchCounter fans a PerfCounterPack out to every plugin that
// exported an ICounter hook. Same error/panic isolation as DispatchAlert.
func (m *Manager) DispatchCounter(cp *pack.PerfCounterPack) {
	if !m.enabled || cp == nil {
		return
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.plugins) == 0 {
		return
	}
	payload := counterToProto(cp)
	for _, p := range m.plugins {
		if p.counter == nil {
			continue
		}
		m.callCounter(p, payload)
	}
}

func (m *Manager) callAlert(p *loaded, payload *pluginpb.AlertPayload) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("plugin: alert hook panic", "plugin", p.name, "panic", r)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()
	if err := p.alert.Process(ctx, payload); err != nil {
		slog.Warn("plugin: alert hook error", "plugin", p.name, "error", err)
	}
}

func (m *Manager) callCounter(p *loaded, payload *pluginpb.CounterPayload) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("plugin: counter hook panic", "plugin", p.name, "panic", r)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()
	if err := p.counter.Process(ctx, payload); err != nil {
		slog.Warn("plugin: counter hook error", "plugin", p.name, "error", err)
	}
}