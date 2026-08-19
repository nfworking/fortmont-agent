package plugins

import (
    "context"
    "fmt"
    "sync"
    "time"

    "github.com/nfworking/fortmont-agent/internal/plugins/proxmox"
)

type StatusReporter func(context.Context, string, Status, string, string) error
type TelemetryReporter func(context.Context, Telemetry) error

type Manager struct { mu sync.RWMutex; store *SecretStore; plugins map[string]Plugin; configs map[string]map[string]any; status StatusReporter; telemetry TelemetryReporter }

func NewManager(store *SecretStore, status StatusReporter, telemetry TelemetryReporter) *Manager { return &Manager{store: store, plugins: make(map[string]Plugin), configs: make(map[string]map[string]any), status: status, telemetry: telemetry} }

func (m *Manager) Install(ctx context.Context, pluginID, slug, version string, config map[string]any) error {
    _ = m.status(ctx, pluginID, StatusInstalling, version, "")
    var p Plugin
    switch slug {
    case "proxmox":
        p = proxmox.New(proxmox.Config{Endpoint: stringValue(config["endpoint"]), TokenID: stringValue(config["tokenId"]), TokenSecret: stringValue(config["tokenSecret"]), VerifyTLS: boolValue(config["verifyTls"], true), NodeScope: stringValue(config["nodeScope"])})
    default:
        return fmt.Errorf("unsupported plugin: %s", slug)
    }
    if err := m.store.Save(pluginID, config); err != nil { _ = m.status(ctx, pluginID, StatusError, version, err.Error()); return err }
    if err := p.Start(ctx); err != nil { _ = m.status(ctx, pluginID, StatusError, version, err.Error()); return err }
    m.mu.Lock(); m.plugins[pluginID] = p; m.configs[pluginID] = config; m.mu.Unlock()
    if err := m.status(ctx, pluginID, StatusRunning, version, ""); err != nil { return err }
    return nil
}

func (m *Manager) Run(ctx context.Context, interval time.Duration) {
    ticker := time.NewTicker(interval); defer ticker.Stop()
    collect := func() {
        m.mu.RLock(); snapshot := make([]struct { id string; plugin Plugin } , 0, len(m.plugins)); for id, p := range m.plugins { snapshot = append(snapshot, struct { id string; plugin Plugin }{id: id, plugin: p}) }; m.mu.RUnlock()
        for _, entry := range snapshot {
            collectCtx, cancel := context.WithTimeout(ctx, interval)
            measurements, err := entry.plugin.Collect(collectCtx); cancel()
            if err != nil { _ = m.status(ctx, entry.id, StatusDegraded, entry.plugin.Version(), err.Error()); continue }
            if len(measurements) == 0 { continue }
            _ = m.telemetry(ctx, Telemetry{PluginID: entry.plugin.ID(), PluginSlug: entry.plugin.Slug(), SchemaVersion: 1, Timestamp: time.Now().Unix(), Measurements: measurements})
            _ = m.status(ctx, entry.id, StatusRunning, entry.plugin.Version(), "")
        }
    }
    collect()
    for { select { case <-ctx.Done(): return; case <-ticker.C: collect() } }
}

func (m *Manager) status(ctx context.Context, pluginID string, status Status, version, message string) error { if m.status == nil { return nil }; return m.status(ctx, pluginID, status, version, message) }
func (m *Manager) telemetry(ctx context.Context, telemetry Telemetry) error { if m.telemetry == nil { return nil }; return m.telemetry(ctx, telemetry) }
func stringValue(value any) string { if value, ok := value.(string); ok { return value }; return "" }
func boolValue(value any, fallback bool) bool { if value, ok := value.(bool); ok { return value }; return fallback }
