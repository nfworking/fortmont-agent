package proxmox

import (
    "context"
    "crypto/tls"
    "encoding/json"
    "fmt"
    "net/http"
    "strings"
    "time"

    "github.com/nfworking/fortmont-agent/internal/pluginapi"
)

type Config struct { Endpoint string; TokenID string; TokenSecret string; VerifyTLS bool; NodeScope string }
type Plugin struct { config Config; client *http.Client; nodes []string }

func New(config Config) *Plugin {
    endpoint := strings.TrimRight(config.Endpoint, "/")
    transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: !config.VerifyTLS}} // #nosec G402 - explicitly controlled by the user's plugin profile.
    return &Plugin{config: Config{Endpoint: endpoint, TokenID: config.TokenID, TokenSecret: config.TokenSecret, VerifyTLS: config.VerifyTLS, NodeScope: config.NodeScope}, client: &http.Client{Transport: transport, Timeout: 15 * time.Second}}
}
func (p *Plugin) ID() string { return "00000000-0000-0000-0000-000000000001" }
func (p *Plugin) Slug() string { return "proxmoxv2" }
func (p *Plugin) Version() string { return "1.0.0" }
func (p *Plugin) Start(ctx context.Context) error { return p.request(ctx, "/api2/json/version", nil) }
func (p *Plugin) Stop() error { return nil }

func (p *Plugin) Collect(ctx context.Context) ([]pluginapi.TelemetryMeasurement, error) {
    var nodes struct { Data []struct { Node string `json:"node"`; Uptime int64 `json:"uptime"`; CPU float64 `json:"cpu"`; MaxMem uint64 `json:"maxmem"`; Mem uint64 `json:"mem"` } `json:"data"` }
    if err := p.request(ctx, "/api2/json/nodes", &nodes); err != nil { return nil, err }
    measurements := make([]pluginapi.TelemetryMeasurement, 0)
    for _, node := range nodes.Data {
        measurements = append(measurements, pluginapi.TelemetryMeasurement{Name: "proxmox_node", Tags: map[string]string{"node_id": node.Node, "node_name": node.Node}, Fields: map[string]any{"cpu_usage_percent": node.CPU * 100, "memory_total_bytes": node.MaxMem, "memory_used_bytes": node.Mem, "uptime_seconds": node.Uptime}})
        var guests struct { Data []struct { VMID int64 `json:"vmid"`; Name string `json:"name"`; Status string `json:"status"`; CPU float64 `json:"cpu"`; Mem uint64 `json:"mem"`; DiskRead uint64 `json:"diskread"`; DiskWrite uint64 `json:"diskwrite"` } `json:"data"` }
        if err := p.request(ctx, "/api2/json/nodes/"+node.Node+"/qemu", &guests); err == nil {
            for _, guest := range guests.Data {
                measurements = append(measurements, pluginapi.TelemetryMeasurement{Name: "proxmox_guest", Tags: map[string]string{"node_id": node.Node, "guest_id": fmt.Sprintf("%d", guest.VMID), "guest_name": guest.Name, "guest_type": "qemu"}, Fields: map[string]any{"status": guest.Status, "cpu_usage_percent": guest.CPU * 100, "memory_bytes": guest.Mem, "disk_read_bytes": guest.DiskRead, "disk_write_bytes": guest.DiskWrite}})
            }
        }
    }
    return measurements, nil
}

func (p *Plugin) request(ctx context.Context, path string, out any) error {
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.config.Endpoint+path, nil); if err != nil { return err }
    req.Header.Set("Authorization", "PVEAPIToken="+p.config.TokenID+"="+p.config.TokenSecret)
    resp, err := p.client.Do(req); if err != nil { return err }; defer resp.Body.Close()
    if resp.StatusCode < 200 || resp.StatusCode >= 300 { return fmt.Errorf("Proxmox API returned HTTP %d", resp.StatusCode) }
    if out == nil { return nil }
    return json.NewDecoder(resp.Body).Decode(out)
}
