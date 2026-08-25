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
type Plugin struct { config Config; client *http.Client }

type clusterResource struct {
    ID string `json:"id"`
    Node string `json:"node"`
    Name string `json:"name"`
    Type string `json:"type"`
    Status string `json:"status"`
    Level string `json:"level"`
    VMID int64 `json:"vmid"`
    Uptime int64 `json:"uptime"`
    CPU float64 `json:"cpu"`
    MaxCPU int64 `json:"maxcpu"`
    MaxMem uint64 `json:"maxmem"`
    Mem uint64 `json:"mem"`
    MemHost uint64 `json:"memhost"`
    Disk uint64 `json:"disk"`
    MaxDisk uint64 `json:"maxdisk"`
    DiskRead uint64 `json:"diskread"`
    DiskWrite uint64 `json:"diskwrite"`
    NetIn uint64 `json:"netin"`
    NetOut uint64 `json:"netout"`
    Template int64 `json:"template"`
    Tags string `json:"tags"`
    CgroupMode int64 `json:"cgroup-mode"`

    Storage string `json:"storage"`
    Content string `json:"content"`
    PluginType string `json:"plugintype"`
    Shared int64 `json:"shared"`

    Network string `json:"network"`
    NetworkType string `json:"network-type"`
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
    resources, err := p.listClusterResources(ctx)
    if err != nil { return nil, err }

    measurements := make([]pluginapi.TelemetryMeasurement, 0, len(resources))
    for _, resource := range resources {
        if !p.nodeAllowed(resource.Node) {
            continue
        }

        switch resource.Type {
        case "node":
            measurements = append(measurements, nodeMeasurement(resource))
        case "lxc", "qemu":
            measurements = append(measurements, guestMeasurement(resource))
        case "storage":
            measurements = append(measurements, storageMeasurement(resource))
        case "network":
            measurements = append(measurements, networkMeasurement(resource))
        }
    }

    return measurements, nil
}

func (p *Plugin) nodeAllowed(node string) bool {
    scope := strings.TrimSpace(p.config.NodeScope)
    if scope == "" || scope == "*" { return true }
    for _, value := range strings.Split(scope, ",") {
        if strings.TrimSpace(value) == node { return true }
    }
    return false
}

func nodeMeasurement(resource clusterResource) pluginapi.TelemetryMeasurement {
    nodeID := resource.ID
    if nodeID == "" { nodeID = "node/" + resource.Node }
    return pluginapi.TelemetryMeasurement{
        Name: "proxmox_node",
        Tags: map[string]string{
            "node_id": nodeID,
            "node_name": resource.Node,
            "status": resource.Status,
            "type": resource.Type,
        },
        Fields: map[string]any{
            "cpu_usage_percent": resource.CPU * 100,
            "cpu_count": resource.MaxCPU,
            "memory_total_bytes": resource.MaxMem,
            "memory_used_bytes": resource.Mem,
            "disk_total_bytes": resource.MaxDisk,
            "disk_used_bytes": resource.Disk,
            "uptime_seconds": resource.Uptime,
        },
    }
}

func guestMeasurement(resource clusterResource) pluginapi.TelemetryMeasurement {
    guestID := resource.ID
    if guestID == "" && resource.VMID > 0 {
        guestID = fmt.Sprintf("%s/%d", resource.Type, resource.VMID)
    }
    return pluginapi.TelemetryMeasurement{
        Name: "proxmox_guest",
        Tags: map[string]string{
            "guest_id": guestID,
            "node_name": resource.Node,
            "name": resource.Name,
            "status": resource.Status,
            "type": resource.Type,
            "resource_tags": resource.Tags,
        },
        Fields: map[string]any{
            "vmid": resource.VMID,
            "cpu_usage_percent": resource.CPU * 100,
            "cpu_count": resource.MaxCPU,
            "memory_total_bytes": resource.MaxMem,
            "memory_used_bytes": resource.Mem,
            "memory_host_bytes": resource.MemHost,
            "disk_total_bytes": resource.MaxDisk,
            "disk_used_bytes": resource.Disk,
            "disk_read_bytes": resource.DiskRead,
            "disk_write_bytes": resource.DiskWrite,
            "network_in_bytes": resource.NetIn,
            "network_out_bytes": resource.NetOut,
            "uptime_seconds": resource.Uptime,
            "template": resource.Template,
        },
    }
}

func storageMeasurement(resource clusterResource) pluginapi.TelemetryMeasurement {
    storageID := resource.ID
    if storageID == "" { storageID = "storage/" + resource.Node + "/" + resource.Storage }
    return pluginapi.TelemetryMeasurement{
        Name: "proxmox_storage",
        Tags: map[string]string{
            "storage_id": storageID,
            "node_name": resource.Node,
            "storage_name": resource.Storage,
            "status": resource.Status,
            "type": resource.Type,
            "plugin_type": resource.PluginType,
            "content": resource.Content,
        },
        Fields: map[string]any{
            "total_bytes": resource.MaxDisk,
            "used_bytes": resource.Disk,
            "shared": resource.Shared,
        },
    }
}

func networkMeasurement(resource clusterResource) pluginapi.TelemetryMeasurement {
    networkID := resource.ID
    if networkID == "" { networkID = "network/" + resource.Node + "/" + resource.Network }
    return pluginapi.TelemetryMeasurement{
        Name: "proxmox_network",
        Tags: map[string]string{
            "network_id": networkID,
            "node_name": resource.Node,
            "network_name": resource.Network,
            "status": resource.Status,
            "network_type": resource.NetworkType,
        },
        Fields: map[string]any{
            "available": resource.Status == "ok",
        },
    }
}

func (p *Plugin) listClusterResources(ctx context.Context) ([]clusterResource, error) {
    var response struct { Data []clusterResource `json:"data"` }
    if err := p.request(ctx, "/api2/json/cluster/resources", &response); err != nil { return nil, err }
    return response.Data, nil
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
