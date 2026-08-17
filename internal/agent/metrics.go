package agent

import (
    "context"
    "time"

    "github.com/gorilla/websocket"
    agentmetrics "github.com/nfworking/fortmont-agent/internal/metrics"
    "github.com/nfworking/fortmont-agent/internal/protocol"
)

func (a *Agent) runMetricsLoop(ctx context.Context, conn *websocket.Conn) {
    ticker := time.NewTicker(a.cfg.MetricsInterval)
    defer ticker.Stop()

    sendSnapshot := func() bool {
        collectCtx, cancel := context.WithTimeout(ctx, a.cfg.MetricsInterval)
        defer cancel()
        snapshot, err := agentmetrics.Collect(collectCtx)
        if err != nil {
            a.log.Warn("failed to collect system metrics", "error", err)
            return true
        }

        payload := protocol.SystemMetrics{
            Timestamp: snapshot.Timestamp,
            CPU: struct { UsagePercent float64 `json:"usage_percent"`; Cores int `json:"cores"` }{UsagePercent: snapshot.CPU.UsagePercent, Cores: snapshot.CPU.Cores},
            Memory: struct { TotalBytes uint64 `json:"total_bytes"`; UsedBytes uint64 `json:"used_bytes"`; FreeBytes uint64 `json:"free_bytes"`; UsagePercent float64 `json:"usage_percent"` }{TotalBytes: snapshot.Memory.TotalBytes, UsedBytes: snapshot.Memory.UsedBytes, FreeBytes: snapshot.Memory.FreeBytes, UsagePercent: snapshot.Memory.UsagePercent},
            Storage: struct { Path string `json:"path"`; TotalBytes uint64 `json:"total_bytes"`; UsedBytes uint64 `json:"used_bytes"`; FreeBytes uint64 `json:"free_bytes"`; UsagePercent float64 `json:"usage_percent"` }{Path: snapshot.Storage.Path, TotalBytes: snapshot.Storage.TotalBytes, UsedBytes: snapshot.Storage.UsedBytes, FreeBytes: snapshot.Storage.FreeBytes, UsagePercent: snapshot.Storage.UsagePercent},
        }

        if err := a.send(conn, "metrics", payload); err != nil {
            a.log.Warn("failed to send system metrics; closing gateway connection", "error", err)
            _ = conn.Close()
            return false
        }
        a.log.Debug("system metrics sent", "cpu", payload.CPU.UsagePercent, "memory", payload.Memory.UsagePercent, "storage", payload.Storage.UsagePercent)
        return true
    }

    if !sendSnapshot() { return }
    for {
        select {
        case <-ctx.Done(): return
        case <-ticker.C:
            if !sendSnapshot() { return }
        }
    }
}
