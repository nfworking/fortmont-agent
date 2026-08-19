package plugins

import (
    "context"
    "github.com/nfworking/fortmont-agent/internal/pluginapi"
)

type Status string

const (
    StatusInstalling Status = "installing"
    StatusConfigured Status = "configured"
    StatusRunning Status = "running"
    StatusDegraded Status = "degraded"
    StatusError Status = "error"
    StatusStopped Status = "stopped"
)

type TelemetryMeasurement = pluginapi.TelemetryMeasurement

type Telemetry struct {
    PluginID string
    PluginSlug string
    SchemaVersion int
    Timestamp int64
    Measurements []TelemetryMeasurement
}

type Plugin interface {
    ID() string
    Slug() string
    Version() string
    Start(context.Context) error
    Stop() error
    Collect(context.Context) ([]TelemetryMeasurement, error)
}
