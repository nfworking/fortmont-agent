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

type Manager struct { mu sync.RWMutex; store *SecretStore; plugins map[string]Plugin; reportStatus StatusReporter; reportTelemetry TelemetryReporter }

func NewManager(store *SecretStore, status StatusReporter, telemetry TelemetryReporter) *Manager { return &Manager{store: store, plugins: make(map[string]Plugin), reportStatus: status, reportTelemetry: telemetry} }

func (m *Manager) Install(ctx context.Context, pluginID, slug, version string, config map[string]any) error {
    _ = m.report(ctx, pluginID, StatusInstalling, version, "")
    var p Plugin
    switch slug {
    case "proxmox":
        p = proxmox.New(proxmox.Config{Endpoint: stringValue(config["endpoint"]), TokenID: stringValue(config["tokenId"]), TokenSecret: stringValue(config["tokenSecret"]), VerifyTLS: boolValue(config["verifyTls"], true), NodeScope: stringValue(config["nodeScope"])})
    default:
        err := fmt.Errorf("unsupported plugin: %s", slug)
        _ = m.report(ctx, pluginID, StatusError, version, err.Error())
        return err
    }
    if err := m.store.Save(pluginID, config); err != nil { _ = m.report(ctx, pluginID, StatusError, version, err.Error()); return err }
    if err := p.Start(ctx); err != nil { _ = m.report(ctx, pluginID, StatusError, version, err.Error()); return err }
    m.mu.Lock(); m.plugins[pluginID] = p; m.mu.Unlock()
    return m.report(ctx, pluginID, StatusRunning, version, "")
}

func (m *Manager) Run(ctx context.Context, interval time.Duration) {
    ticker := time.NewTicker(interval); defer ticker.Stop()
    collect := func() {
        m.mu.RLock(); snapshot := make([]struct { id string; plugin Plugin }, 0, len(m.plugins)); for id, p := range m.plugins { snapshot = append(snapshot, struct { id string; plugin Plugin }{id: id, plugin: p}) }; m.mu.RUnlock()
        for _, entry := range snapshot {
            collectCtx, cancel := context.WithTimeout(ctx, interval)
            measurements, err := entry.plugin.Collect(collectCtx); cancel()
            if err != nil { _ = m.report(ctx, entry.id, StatusDegraded, entry.plugin.Version(), err.Error()); continue }
            if len(measurements) == 0 { continue }
            // The control-plane plugin ID is the installation key. Plugin
            // implementations should not invent their own database IDs.
            _ = m.sendTelemetry(ctx, Telemetry{PluginID: entry.id, PluginSlug: entry.plugin.Slug(), SchemaVersion: 1, Timestamp: time.Now().Unix(), Measurements: measurements})
            _ = m.report(ctx, entry.id, StatusRunning, entry.plugin.Version(), "")
        }
    }
    collect()
    for { select { case <-ctx.Done(): return; case <-ticker.C: collect() } }
}

func (m *Manager) report(ctx context.Context, pluginID string, status Status, version, message string) error { if m.reportStatus == nil { return nil }; return m.reportStatus(ctx, pluginID, status, version, message) }
func (m *Manager) sendTelemetry(ctx context.Context, telemetry Telemetry) error { if m.reportTelemetry == nil { return nil }; return m.reportTelemetry(ctx, telemetry) }
func stringValue(value any) string { if value, ok := value.(string); ok { return value }; return "" }
func boolValue(value any, fallback bool) bool { if value, ok := value.(bool); ok { return value }; return fallback }
