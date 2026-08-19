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

type proxmoxNode struct {
    Node string `json:"node"`
    ID string `json:"id"`
    Status string `json:"status"`
    Type string `json:"type"`
    Level string `json:"level"`
    Uptime int64 `json:"uptime"`
    CPU float64 `json:"cpu"`
    MaxCPU int64 `json:"maxcpu"`
    MaxMem uint64 `json:"maxmem"`
    Mem uint64 `json:"mem"`
    Disk uint64 `json:"disk"`
    MaxDisk uint64 `json:"maxdisk"`
    SSLCertFingerprint string `json:"ssl_fingerprint"`
}

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
    nodes, err := p.listNodes(ctx)
    if err != nil { return nil, err }

    measurements := make([]pluginapi.TelemetryMeasurement, 0, len(nodes))
    for _, node := range nodes {
        nodeID := node.ID
        if nodeID == "" { nodeID = "node/" + node.Node }

        measurements = append(measurements, pluginapi.TelemetryMeasurement{
            Name: "proxmox_node",
            Tags: map[string]string{
                "node_id": nodeID,
                "node_name": node.Node,
                "status": node.Status,
                "type": node.Type,
            },
            Fields: map[string]any{
                "cpu_usage_percent": node.CPU * 100,
                "cpu_count": node.MaxCPU,
                "memory_total_bytes": node.MaxMem,
                "memory_used_bytes": node.Mem,
                "disk_total_bytes": node.MaxDisk,
                "disk_used_bytes": node.Disk,
                "uptime_seconds": node.Uptime,
            },
        })
    }

    return measurements, nil
}

// listNodes intentionally uses only Proxmox's cluster node endpoint. The
// plugin's node telemetry must represent Proxmox nodes, not guests/VMs.
// Do not replace this with /nodes/{node}/qemu or another guest inventory API.
func (p *Plugin) listNodes(ctx context.Context) ([]proxmoxNode, error) {
    var response struct { Data []proxmoxNode `json:"data"` }
    if err := p.request(ctx, "/api2/json/nodes/", &response); err != nil { return nil, err }
    if p.config.NodeScope == "" || p.config.NodeScope == "*" { return response.Data, nil }

    wanted := map[string]struct{}{}
    for _, value := range strings.Split(p.config.NodeScope, ",") {
        if name := strings.TrimSpace(value); name != "" { wanted[name] = struct{}{} }
    }
    if len(wanted) == 0 { return response.Data, nil }

    filtered := make([]proxmoxNode, 0, len(response.Data))
    for _, node := range response.Data {
        if _, ok := wanted[node.Node]; ok { filtered = append(filtered, node) }
    }
    return filtered, nil
}

func (p *Plugin) request(ctx context.Context, path string, out any) error {
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.config.Endpoint+path, nil)
    if err != nil { return err }
    req.Header.Set("Authorization", "PVEAPIToken="+p.config.TokenID+"="+p.config.TokenSecret)

    resp, err := p.client.Do(req)
    if err != nil { return err }
    defer resp.Body.Close()

    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        return fmt.Errorf("Proxmox API returned HTTP %d", resp.StatusCode)
    }
    if out == nil { return nil }
    return json.NewDecoder(resp.Body).Decode(out)
}
