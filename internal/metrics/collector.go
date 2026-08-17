package metrics

import (
    "context"
    "os"
    "runtime"
    "time"

    "github.com/shirou/gopsutil/v3/cpu"
    "github.com/shirou/gopsutil/v3/disk"
    "github.com/shirou/gopsutil/v3/mem"
)

type CPUStats struct {
    UsagePercent float64 `json:"usage_percent"`
    Cores        int     `json:"cores"`
}

type MemoryStats struct {
    TotalBytes   uint64  `json:"total_bytes"`
    UsedBytes    uint64  `json:"used_bytes"`
    FreeBytes    uint64  `json:"free_bytes"`
    UsagePercent float64 `json:"usage_percent"`
}

type StorageStats struct {
    Path         string  `json:"path"`
    TotalBytes   uint64  `json:"total_bytes"`
    UsedBytes    uint64  `json:"used_bytes"`
    FreeBytes    uint64  `json:"free_bytes"`
    UsagePercent float64 `json:"usage_percent"`
}

type SystemMetrics struct {
    Timestamp int64        `json:"timestamp"`
    CPU       CPUStats     `json:"cpu"`
    Memory    MemoryStats  `json:"memory"`
    Storage   StorageStats `json:"storage"`
}

func Collect(ctx context.Context) (*SystemMetrics, error) {
    cpuUsage := float64(0)
    if values, err := cpu.PercentWithContext(ctx, 500*time.Millisecond, false); err == nil && len(values) > 0 {
        cpuUsage = values[0]
    }

    memory := MemoryStats{}
    if value, err := mem.VirtualMemoryWithContext(ctx); err == nil {
        memory = MemoryStats{TotalBytes: value.Total, UsedBytes: value.Used, FreeBytes: value.Free, UsagePercent: value.UsedPercent}
    }

    path := "/"
    if runtime.GOOS == "windows" {
        drive := os.Getenv("SystemDrive")
        if drive == "" { drive = "C:" }
        path = drive + "\\"
    }

    storage := StorageStats{Path: path}
    if value, err := disk.UsageWithContext(ctx, path); err == nil {
        storage = StorageStats{Path: path, TotalBytes: value.Total, UsedBytes: value.Used, FreeBytes: value.Free, UsagePercent: value.UsedPercent}
    }

    return &SystemMetrics{
        Timestamp: time.Now().Unix(),
        CPU: CPUStats{UsagePercent: cpuUsage, Cores: runtime.NumCPU()},
        Memory: memory,
        Storage: storage,
    }, nil
}
