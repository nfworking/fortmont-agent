package plugins

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nfworking/fortmont-agent/internal/plugins/proxmox"
	"github.com/nfworking/fortmont-agent/internal/plugins/unifi"
)

type StatusReporter func(context.Context, string, Status, string, string) error
type TelemetryReporter func(context.Context, Telemetry) error

type Manager struct {
	mu              sync.RWMutex
	store           *SecretStore
	plugins         map[string]Plugin
	reportStatus    StatusReporter
	reportTelemetry TelemetryReporter
}

func NewManager(store *SecretStore, status StatusReporter, telemetry TelemetryReporter) *Manager {
	return &Manager{store: store, plugins: make(map[string]Plugin), reportStatus: status, reportTelemetry: telemetry}
}

func (m *Manager) Install(ctx context.Context, pluginID, slug, version string, config map[string]any) error {
	_ = m.report(ctx, pluginID, StatusInstalling, version, "")
	p, err := m.newPlugin(slug, config)
	if err != nil {
		_ = m.report(ctx, pluginID, StatusError, version, err.Error())
		return err
	}
	if err := p.Start(ctx); err != nil {
		_ = m.report(ctx, pluginID, StatusError, version, err.Error())
		return err
	}
	if err := m.store.Save(pluginID, config); err != nil {
		_ = m.report(ctx, pluginID, StatusError, version, err.Error())
		_ = p.Stop()
		return err
	}
	if err := m.store.SaveInstallation(Installation{PluginID: pluginID, Slug: slug, Version: version}); err != nil {
		_ = m.report(ctx, pluginID, StatusError, version, err.Error())
		_ = p.Stop()
		return err
	}
	m.mu.Lock()
	if previous, exists := m.plugins[pluginID]; exists {
		_ = previous.Stop()
	}
	m.plugins[pluginID] = p
	m.mu.Unlock()
	return m.report(ctx, pluginID, StatusRunning, version, "")
}

func (m *Manager) Remove(ctx context.Context, pluginID string) error {
	m.mu.Lock()
	p, exists := m.plugins[pluginID]
	if exists {
		delete(m.plugins, pluginID)
	}
	m.mu.Unlock()

	if exists {
		if err := p.Stop(); err != nil {
			return err
		}
	}
	if err := m.store.Delete(pluginID); err != nil {
		return err
	}
	if err := m.store.DeleteInstallation(pluginID); err != nil {
		return err
	}
	return nil
}

// Restore loads encrypted plugin configuration and installation metadata from
// disk and starts every previously installed plugin. Secrets never leave the
// local SecretStore in plaintext; they are decrypted only for plugin startup.
// Restore is intentionally called after the WebSocket handshake so status
// updates can be reported to the control plane.
func (m *Manager) Restore(ctx context.Context) error {
	installations, err := m.store.ListInstallations()
	if err != nil {
		return err
	}
	for _, installation := range installations {
		if installation.PluginID == "" || installation.Slug == "" {
			continue
		}
		config, err := m.store.Load(installation.PluginID)
		if err != nil {
			_ = m.report(ctx, installation.PluginID, StatusError, installation.Version, fmt.Sprintf("load persisted plugin configuration: %v", err))
			continue
		}
		_ = m.report(ctx, installation.PluginID, StatusInstalling, installation.Version, "restoring persisted plugin")
		p, err := m.newPlugin(installation.Slug, config)
		if err != nil {
			_ = m.report(ctx, installation.PluginID, StatusError, installation.Version, err.Error())
			continue
		}
		if err := p.Start(ctx); err != nil {
			_ = m.report(ctx, installation.PluginID, StatusError, installation.Version, err.Error())
			_ = p.Stop()
			continue
		}
		m.mu.Lock()
		if previous, exists := m.plugins[installation.PluginID]; exists {
			_ = previous.Stop()
		}
		m.plugins[installation.PluginID] = p
		m.mu.Unlock()
		_ = m.report(ctx, installation.PluginID, StatusRunning, installation.Version, "restored after agent restart")
	}
	return nil
}

func (m *Manager) newPlugin(slug string, config map[string]any) (Plugin, error) {
	switch slug {
	case "proxmox", "proxmoxv2":
		return proxmox.New(proxmox.Config{
			Endpoint:   stringValue(config["endpoint"]),
			TokenID:    stringValue(config["tokenId"]),
			TokenSecret: stringValue(config["tokenSecret"]),
			VerifyTLS:  boolValue(config["verifyTls"], true),
			NodeScope:  stringValue(config["nodeScope"]),
		}), nil
	case "unifi":
		return unifi.New(unifi.Config{
			Endpoint:       stringValue(config["endpoint"]),
			APIKey:         stringValue(config["apiKey"]),
			SiteID:         stringValue(config["siteId"]),
			VerifyTLS:      boolValue(config["verifyTls"], false),
			LegacySiteName: stringValue(config["legacySiteName"]),
		}), nil
	default:
		return nil, fmt.Errorf("unsupported plugin: %s", slug)
	}
}

func (m *Manager) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	collect := func() {
		m.mu.RLock()
		snapshot := make([]struct {
			id     string
			plugin Plugin
		}, 0, len(m.plugins))
		for id, p := range m.plugins {
			snapshot = append(snapshot, struct {
				id     string
				plugin Plugin
			}{id: id, plugin: p})
		}
		m.mu.RUnlock()
		for _, entry := range snapshot {
			collectCtx, cancel := context.WithTimeout(ctx, interval)
			measurements, err := entry.plugin.Collect(collectCtx)
			cancel()
			if err != nil {
				_ = m.report(ctx, entry.id, StatusDegraded, entry.plugin.Version(), err.Error())
				continue
			}
			if len(measurements) == 0 {
				continue
			}
			_ = m.sendTelemetry(ctx, Telemetry{PluginID: entry.id, PluginSlug: entry.plugin.Slug(), SchemaVersion: 1, Timestamp: time.Now().Unix(), Measurements: measurements})
			_ = m.report(ctx, entry.id, StatusRunning, entry.plugin.Version(), "")
		}
	}
	collect()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			collect()
		}
	}
}

func (m *Manager) report(ctx context.Context, pluginID string, status Status, version, message string) error {
	if m.reportStatus == nil {
		return nil
	}
	return m.reportStatus(ctx, pluginID, status, version, message)
}

func (m *Manager) sendTelemetry(ctx context.Context, telemetry Telemetry) error {
	if m.reportTelemetry == nil {
		return nil
	}
	return m.reportTelemetry(ctx, telemetry)
}

func stringValue(value any) string {
	if value, ok := value.(string); ok {
		return value
	}
	return ""
}

func boolValue(value any, fallback bool) bool {
	if value, ok := value.(bool); ok {
		return value
	}
	return fallback
}
