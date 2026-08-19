package pluginapi

type TelemetryMeasurement struct {
    Name string `json:"name"`
    Tags map[string]string `json:"tags,omitempty"`
    Fields map[string]any `json:"fields"`
}
