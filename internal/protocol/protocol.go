package protocol

import "encoding/json"

type Envelope struct { Type string `json:"type"`; ID string `json:"id,omitempty"`; Data json.RawMessage `json:"data,omitempty"` }
type RegisterRequest struct { EnrollmentToken string `json:"enrollment_token"`; PublicKey string `json:"public_key"`; DeviceID string `json:"device_id"`; Hostname string `json:"hostname"`; LocalIP string `json:"local_ip,omitempty"`; PublicIP string `json:"public_ip,omitempty"`; Platform string `json:"platform"`; Architecture string `json:"architecture"`; Version string `json:"version"` }
type AuthenticateRequest struct { AgentID string `json:"agent_id"`; KeyID string `json:"key_id"` }
type Challenge struct { Challenge string `json:"challenge"`; ExpiresAt string `json:"expires_at"` }
type ChallengeResponse struct { KeyID string `json:"key_id"`; Signature string `json:"signature"` }
type RegistrationComplete struct { AgentID string `json:"agent_id"`; KeyID string `json:"key_id"` }
type Heartbeat struct { Timestamp string `json:"timestamp"`; Hostname string `json:"hostname,omitempty"`; LocalIP string `json:"local_ip,omitempty"`; PublicIP string `json:"public_ip,omitempty"`; Platform string `json:"platform,omitempty"`; Architecture string `json:"architecture,omitempty"`; Version string `json:"version,omitempty"`; LatencyMs float64 `json:"latency_ms,omitempty"` }
type SystemMetrics struct { Timestamp int64 `json:"timestamp"`; CPU struct { UsagePercent float64 `json:"usage_percent"`; Cores int `json:"cores"` } `json:"cpu"`; Memory struct { TotalBytes uint64 `json:"total_bytes"`; UsedBytes uint64 `json:"used_bytes"`; FreeBytes uint64 `json:"free_bytes"`; UsagePercent float64 `json:"usage_percent"` } `json:"memory"`; Storage struct { Path string `json:"path"`; TotalBytes uint64 `json:"total_bytes"`; UsedBytes uint64 `json:"used_bytes"`; FreeBytes uint64 `json:"free_bytes"`; UsagePercent float64 `json:"usage_percent"` } `json:"storage"` }

type PluginInstall struct { CommandID string `json:"commandId"`; AgentID string `json:"agentId"`; PluginID string `json:"pluginId"`; PluginSlug string `json:"pluginSlug"`; PluginVersion string `json:"pluginVersion"`; ProfileID string `json:"profileId"`; ConfigVersion string `json:"configVersion"`; Config map[string]any `json:"config"`; DataSchema map[string]any `json:"dataSchema,omitempty"` }
type PluginRemove struct { CommandID string `json:"commandId"`; AgentID string `json:"agentId"`; PluginID string `json:"pluginId"`; PluginSlug string `json:"pluginSlug,omitempty"` }
type PluginStatus struct { AgentID string `json:"agentId"`; PluginID string `json:"pluginId"`; Status string `json:"status"`; Version string `json:"version,omitempty"`; Error string `json:"error,omitempty"` }
type PluginMeasurement struct { Name string `json:"name"`; Tags map[string]string `json:"tags,omitempty"`; Fields map[string]any `json:"fields"` }
type PluginData struct { AgentID string `json:"agentId"`; PluginID string `json:"pluginId"`; PluginSlug string `json:"pluginSlug"`; SchemaVersion int `json:"schemaVersion"`; Timestamp int64 `json:"timestamp"`; Measurements []PluginMeasurement `json:"measurements"` }

type Error struct { Code string `json:"code"`; Message string `json:"message"` }
func MarshalMessage(typ string, data any) ([]byte, error) { payload, err := json.Marshal(data); if err != nil { return nil, err }; return json.Marshal(Envelope{Type: typ, Data: payload}) }
