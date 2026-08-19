package unifi

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nfworking/fortmont-agent/internal/pluginapi"
)

type Config struct {
	Endpoint        string
	APIKey          string
	SiteID          string
	VerifyTLS       bool
	LegacySiteName  string
}

type Plugin struct {
	config Config
	client *http.Client
}

type page[T any] struct {
	Data []T `json:"data"`
}

type appInfo struct {
	ApplicationVersion string `json:"applicationVersion"`
}

type site struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type device struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Model            string `json:"model"`
	Supported        bool   `json:"supported"`
	MACAddress       string `json:"macAddress"`
	IPAddress        string `json:"ipAddress"`
	State            string `json:"state"`
	FirmwareVersion  string `json:"firmwareVersion"`
}

type deviceStats struct {
	UptimeSec              int64   `json:"uptimeSec"`
	CPUUtilizationPct      float64 `json:"cpuUtilizationPct"`
	MemoryUtilizationPct   float64 `json:"memoryUtilizationPct"`
	Uplink                 struct {
		TxRateBps float64 `json:"txRateBps"`
		RxRateBps float64 `json:"rxRateBps"`
	} `json:"uplink"`
}

type client struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Type           string `json:"type"`
	IPAddress      string `json:"ipAddress"`
	MACAddress     string `json:"macAddress"`
	UplinkDeviceID string `json:"uplinkDeviceId"`
}

type legacyUplink struct {
	RxBytes  uint64  `json:"rx_bytes"`
	TxBytes  uint64  `json:"tx_bytes"`
	RxRate   float64 `json:"rx_bytes-r"`
	TxRate   float64 `json:"tx_bytes-r"`
}

type legacyDevice struct {
	ID       string         `json:"_id"`
	Name     string         `json:"name"`
	Model    string         `json:"model"`
	Type     string         `json:"type"`
	Uplink   *legacyUplink  `json:"uplink"`
	RxBytes  uint64         `json:"rx_bytes"`
	TxBytes  uint64         `json:"tx_bytes"`
}

type legacyResponse struct {
	Data []legacyDevice `json:"data"`
}

func New(config Config) *Plugin {
	endpoint := strings.TrimRight(strings.TrimSpace(config.Endpoint), "/")
	legacySiteName := strings.TrimSpace(config.LegacySiteName)
	if legacySiteName == "" {
		legacySiteName = "default"
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: !config.VerifyTLS}, // #nosec G402 - explicitly controlled by the user's plugin profile.
	}

	return &Plugin{
		config: Config{
			Endpoint:       endpoint,
			APIKey:         config.APIKey,
			SiteID:         config.SiteID,
			VerifyTLS:      config.VerifyTLS,
			LegacySiteName: legacySiteName,
		},
		client: &http.Client{Transport: transport, Timeout: 20 * time.Second},
	}
}

func (p *Plugin) ID() string      { return "00000000-0000-0000-0000-000000000002" }
func (p *Plugin) Slug() string   { return "unifi" }
func (p *Plugin) Version() string { return "1.0.0" }

func (p *Plugin) Start(ctx context.Context) error {
	if _, err := p.appInfo(ctx); err != nil {
		return err
	}
	_, err := p.resolveSiteID(ctx)
	return err
}

func (p *Plugin) Stop() error { return nil }

func (p *Plugin) Collect(ctx context.Context) ([]pluginapi.TelemetryMeasurement, error) {
	info, err := p.appInfo(ctx)
	if err != nil {
		return nil, err
	}
	siteID, err := p.resolveSiteID(ctx)
	if err != nil {
		return nil, err
	}

	devicesPage, err := p.listDevices(ctx, siteID)
	if err != nil {
		return nil, err
	}
	clientsPage, err := p.listClients(ctx, siteID)
	if err != nil {
		return nil, err
	}

	legacy, _ := p.legacyDevices(ctx)
	bandwidthDownRate, bandwidthUpRate, totalDownBytes, totalUpBytes := bandwidth(legacy)

	measurements := make([]pluginapi.TelemetryMeasurement, 0, len(devicesPage.Data)+len(clientsPage.Data)+1)
	onlineDevices := 0
	wiredClients := 0
	wirelessClients := 0
	var cpuTotal, memTotal float64
	var cpuSamples, memSamples int

	measurements = append(measurements, pluginapi.TelemetryMeasurement{
		Name: "unifi_summary",
		Tags: map[string]string{"site_id": siteID},
		Fields: map[string]any{
			"app_version":       info.ApplicationVersion,
			"total_devices":     int64(len(devicesPage.Data)),
			"total_clients":     int64(len(clientsPage.Data)),
			"down_rate_mbps":    bandwidthDownRate,
			"up_rate_mbps":      bandwidthUpRate,
			"total_down_bytes":  totalDownBytes,
			"total_up_bytes":    totalUpBytes,
		},
	})

	for _, d := range devicesPage.Data {
		if d.State == "ONLINE" {
			onlineDevices++
		}

		stats, statsErr := p.deviceStats(ctx, siteID, d.ID)
		fields := map[string]any{
			"supported":         boolToInt(d.Supported),
			"cpu_utilization_pct": float64(0),
			"memory_utilization_pct": float64(0),
			"uptime_seconds":    int64(0),
			"rx_rate_bps":       float64(0),
			"tx_rate_bps":       float64(0),
		}
		if statsErr == nil {
			fields["cpu_utilization_pct"] = stats.CPUUtilizationPct
			fields["memory_utilization_pct"] = stats.MemoryUtilizationPct
			fields["uptime_seconds"] = stats.UptimeSec
			fields["rx_rate_bps"] = stats.Uplink.RxRateBps
			fields["tx_rate_bps"] = stats.Uplink.TxRateBps
			cpuTotal += stats.CPUUtilizationPct
			memTotal += stats.MemoryUtilizationPct
			cpuSamples++
			memSamples++
		}

		measurements = append(measurements, pluginapi.TelemetryMeasurement{
			Name: "unifi_device",
			Tags: map[string]string{
				"site_id":    siteID,
				"device_id":  d.ID,
				"name":       d.Name,
				"model":      d.Model,
				"state":      d.State,
				"ip_address": d.IPAddress,
			},
			Fields: fields,
		})
	}

	for _, c := range clientsPage.Data {
		switch c.Type {
		case "WIRED":
			wiredClients++
		case "WIRELESS":
			wirelessClients++
		}
		measurements = append(measurements, pluginapi.TelemetryMeasurement{
			Name: "unifi_client",
			Tags: map[string]string{
				"site_id":         siteID,
				"client_id":       c.ID,
				"name":            c.Name,
				"type":            c.Type,
				"ip_address":      c.IPAddress,
				"mac_address":     c.MACAddress,
				"uplink_device_id": c.UplinkDeviceID,
			},
			Fields: map[string]any{"connected": int64(1)},
		})
	}

	// Update the summary with the values that require the complete device/client
	// inventory. Keep it as a single measurement so dashboards can query it cheaply.
	summary := measurements[0].Fields
	summary["online_devices"] = int64(onlineDevices)
	summary["offline_devices"] = int64(len(devicesPage.Data) - onlineDevices)
	summary["wired_clients"] = int64(wiredClients)
	summary["wireless_clients"] = int64(wirelessClients)
	if cpuSamples > 0 {
		summary["avg_cpu_pct"] = cpuTotal / float64(cpuSamples)
	} else {
		summary["avg_cpu_pct"] = float64(0)
	}
	if memSamples > 0 {
		summary["avg_mem_pct"] = memTotal / float64(memSamples)
	} else {
		summary["avg_mem_pct"] = float64(0)
	}

	return measurements, nil
}

func (p *Plugin) resolveSiteID(ctx context.Context) (string, error) {
	if value := strings.TrimSpace(p.config.SiteID); value != "" {
		return value, nil
	}
	var response page[site]
	if err := p.integrationGet(ctx, "/sites", &response); err != nil {
		return "", err
	}
	if len(response.Data) == 0 {
		return "", fmt.Errorf("no UniFi sites available for this API key")
	}
	return response.Data[0].ID, nil
}

func (p *Plugin) appInfo(ctx context.Context) (appInfo, error) {
	var response appInfo
	err := p.integrationGet(ctx, "/info", &response)
	return response, err
}

func (p *Plugin) listDevices(ctx context.Context, siteID string) (page[device], error) {
	var response page[device]
	path := "/sites/" + url.PathEscape(siteID) + "/devices?offset=0&limit=200"
	return response, p.integrationGet(ctx, path, &response)
}

func (p *Plugin) listClients(ctx context.Context, siteID string) (page[client], error) {
	var response page[client]
	path := "/sites/" + url.PathEscape(siteID) + "/clients?offset=0&limit=200"
	return response, p.integrationGet(ctx, path, &response)
}

func (p *Plugin) deviceStats(ctx context.Context, siteID, deviceID string) (deviceStats, error) {
	var response deviceStats
	path := "/sites/" + url.PathEscape(siteID) + "/devices/" + url.PathEscape(deviceID) + "/statistics/latest"
	return response, p.integrationGet(ctx, path, &response)
}

func (p *Plugin) legacyDevices(ctx context.Context) ([]legacyDevice, error) {
	var response legacyResponse
	path := "/s/" + url.PathEscape(p.config.LegacySiteName) + "/stat/device"
	if err := p.legacyGet(ctx, path, &response); err != nil {
		return nil, err
	}
	return response.Data, nil
}

func (p *Plugin) integrationGet(ctx context.Context, path string, out any) error {
	return p.get(ctx, "/proxy/network/integration/v1"+path, out)
}

func (p *Plugin) legacyGet(ctx context.Context, path string, out any) error {
	return p.get(ctx, "/proxy/network/api"+path, out)
}

func (p *Plugin) get(ctx context.Context, path string, out any) error {
	if p.config.Endpoint == "" || p.config.APIKey == "" {
		return fmt.Errorf("UniFi integration is not configured: endpoint and apiKey are required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.config.Endpoint+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-KEY", p.config.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("UniFi API returned HTTP %d for %s", resp.StatusCode, path)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func bandwidth(devices []legacyDevice) (downRate, upRate float64, totalDown, totalUp uint64) {
	for _, d := range devices {
		if d.Uplink != nil {
			downRate += d.Uplink.RxRate
			upRate += d.Uplink.TxRate
			totalDown += d.Uplink.RxBytes
			totalUp += d.Uplink.TxBytes
		} else {
			totalDown += d.RxBytes
			totalUp += d.TxBytes
		}
	}
	return downRate * 8 / 1_000_000, upRate * 8 / 1_000_000, totalDown, totalUp
}

func boolToInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}
