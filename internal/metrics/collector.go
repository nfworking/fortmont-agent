package metrics

import (
    "context"
    "fmt"
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
    Timestamp int64       `json:"timestamp"`
    CPU       CPUStats    `json:"cpu"`
    Memory    MemoryStats `json:"memory"`
    Storage   StorageStats `json:"storage"`
}

func Collect(ctx context.Context) (*SystemMetrics, error) {
    values, err := cpu.PercentWithContext(ctx, 500*time.Millisecond, false)
    if err != nil {
        return nil, fmt.Errorf("collect CPU metrics: %w", err)
    }
    if len(values) == 0 {
        return nil, fmt.Errorf("collect CPU metrics: no CPU sample returned")
    }

    value, err := mem.VirtualMemoryWithContext(ctx)
    if err != nil {
        return nil, fmt.Errorf("collect memory metrics: %w", err)
    }
    memory := MemoryStats{
        TotalBytes:   value.Total,
        UsedBytes:    value.Used,
        FreeBytes:    value.Free,
        UsagePercent: value.UsedPercent,
    }

    path := "/"
    if runtime.GOOS == "windows" {
        drive := os.Getenv("SystemDrive")
        if drive == "" {
            drive = "C:"
        }
        path = drive + "\\"
    }

    storageValue, err := disk.UsageWithContext(ctx, path)
    if err != nil {
        return nil, fmt.Errorf("collect storage metrics for %q: %w", path, err)
    }
    storage := StorageStats{
        Path:         path,
        TotalBytes:   storageValue.Total,
        UsedBytes:    storageValue.Used,
        FreeBytes:    storageValue.Free,
        UsagePercent: storageValue.UsedPercent,
    }

    return &SystemMetrics{
        Timestamp: time.Now().Unix(),
        CPU: CPUStats{
            UsagePercent: values[0],
            Cores:        runtime.NumCPU(),
        },
        Memory:  memory,
        Storage: storage,
    }, nil
}
