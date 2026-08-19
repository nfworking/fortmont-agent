package agent

import (
    "context"
    "crypto/ed25519"
    "encoding/base64"
    "encoding/json"
    "errors"
    "fmt"
    "log/slog"
    "net"
    "net/url"
    "os"
    "runtime"
    "strconv"
    "strings"
    "sync"
    "time"

    "github.com/gorilla/websocket"
    "github.com/nfworking/fortmont-agent/internal/config"
    "github.com/nfworking/fortmont-agent/internal/gateway"
    "github.com/nfworking/fortmont-agent/internal/identity"
    "github.com/nfworking/fortmont-agent/internal/plugins"
    "github.com/nfworking/fortmont-agent/internal/protocol"
)

type Agent struct { cfg config.Config; store identity.Store; creds identity.Credentials; private ed25519.PrivateKey; public ed25519.PublicKey; log *slog.Logger; writeMu sync.Mutex; pluginMu sync.RWMutex; pluginConn *websocket.Conn; plugins *plugins.Manager }

func New(cfg config.Config, log *slog.Logger) (*Agent, error) {
    store := identity.Store{Path: cfg.CredentialsPath}
    creds, privateKey, publicKey, err := store.LoadOrCreate(); if err != nil { return nil, err }
    a := &Agent{cfg: cfg, store: store, creds: creds, private: privateKey, public: publicKey, log: log}
    secretStore := plugins.NewSecretStore(cfg.CredentialsPath+".plugins", privateKey)
    a.plugins = plugins.NewManager(secretStore, a.reportPluginStatus, a.reportPluginTelemetry)
    return a, nil
}

func (a *Agent) Run(ctx context.Context, enrollmentToken string) error {
    go a.plugins.Run(ctx, 60*time.Second)
    backoff := time.Second
    failedGateways := make(map[string]struct{})
    a.log.Info("agent connection manager started", "gateway_count", len(a.cfg.WSNodes), "agent_id", a.creds.AgentID, "device_id", a.creds.DeviceID)
    for {
        if ctx.Err() != nil { return nil }
        selected, candidates, err := gateway.SelectFastestExcluding(ctx, a.cfg.WSNodes, failedGateways, a.cfg.ConnectTimeout)
        a.logGatewaySelection(selected, candidates)
        if err != nil { a.log.Warn("no WebSocket gateway reachable", "error", err); if !sleepContext(ctx, backoff) { return nil }; backoff = minDuration(backoff*2, a.cfg.ReconnectMax); continue }
        a.log.Info("attempting WebSocket connection", "gateway", selected)
        err = a.connectAndRun(ctx, selected, enrollmentToken)
        if err == nil || ctx.Err() != nil { return nil }
        failedGateways[selected] = struct{}{}
        a.log.Warn("gateway connection ended; failing over", "gateway", selected, "error", err, "failed_gateway_count", len(failedGateways))
        enrollmentToken = ""
        if !sleepContext(ctx, backoff) { return nil }
        backoff = minDuration(backoff*2, a.cfg.ReconnectMax)
        if len(failedGateways) >= len(a.cfg.WSNodes) { a.log.Info("all configured gateways failed; retrying the full gateway pool"); failedGateways = make(map[string]struct{}) }
    }
}

func (a *Agent) connectAndRun(ctx context.Context, wsURL, enrollmentToken string) error {
    parsed, err := url.Parse(wsURL); if err != nil { return err }
    a.log.Info("connecting to WebSocket gateway", "gateway", wsURL)
    dialer := websocket.Dialer{HandshakeTimeout: a.cfg.ConnectTimeout}
    conn, _, err := dialer.DialContext(ctx, parsed.String(), nil); if err != nil { return fmt.Errorf("connect %s: %w", wsURL, err) }
    defer conn.Close()
    a.pluginMu.Lock(); a.pluginConn = conn; a.pluginMu.Unlock(); defer func() { a.pluginMu.Lock(); if a.pluginConn == conn { a.pluginConn = nil }; a.pluginMu.Unlock() }()
    a.log.Info("WebSocket connection established", "gateway", wsURL)
    conn.SetReadLimit(1024 * 1024); _ = conn.SetReadDeadline(time.Now().Add(a.cfg.ConnectTimeout))

    var latencyMu sync.RWMutex
    var latencyMs float64
    conn.SetPongHandler(func(payload string) error { _ = conn.SetReadDeadline(time.Now().Add(a.cfg.PingInterval * 2)); sentAt, parseErr := strconv.ParseInt(payload, 10, 64); if parseErr != nil { return nil }; latency := time.Since(time.Unix(0, sentAt)); if latency < 0 || latency > time.Minute { return nil }; value := float64(latency.Microseconds()) / 1000; latencyMu.Lock(); latencyMs = value; latencyMu.Unlock(); a.log.Debug("WebSocket gateway latency measured", "gateway", wsURL, "latency_ms", value); return nil })

    if a.creds.AgentID != "" && a.creds.KeyID != "" { if err := a.send(conn, "authenticate", protocol.AuthenticateRequest{AgentID: a.creds.AgentID, KeyID: a.creds.KeyID}); err != nil { return err } } else { if enrollmentToken == "" { return errors.New("agent is not enrolled; supply an enrollment token with --token or FORTMONT_ENROLLMENT_TOKEN") }; if err := a.send(conn, "register", a.registration(enrollmentToken)); err != nil { return err } }
    if err := a.finishHandshake(conn); err != nil { a.log.Warn("gateway authentication failed", "gateway", wsURL, "error", err); return err }
    a.log.Info("agent authenticated", "agent_id", a.creds.AgentID, "key_id", a.creds.KeyID, "gateway", wsURL)
    _ = conn.SetReadDeadline(time.Now().Add(a.cfg.PingInterval * 2)); if err := a.send(conn, "heartbeat", a.heartbeat(time.Now().UTC(), latencyMs)); err != nil { return err }

    heartbeat := time.NewTicker(a.cfg.HeartbeatInterval); defer heartbeat.Stop(); ping := time.NewTicker(a.cfg.PingInterval); defer ping.Stop(); health := time.NewTicker(a.cfg.GatewayHealthInterval); defer health.Stop()
    metricsCtx, metricsCancel := context.WithCancel(ctx); defer metricsCancel(); go a.runMetricsLoop(metricsCtx, conn)
    readCh := make(chan readResult, 1); go readLoop(conn, readCh, a.cfg.PingInterval*2)
    for {
        select {
        case <-ctx.Done(): return nil
        case result := <-readCh:
            if result.err != nil { return fmt.Errorf("gateway connection lost: %w", result.err) }
            if err := a.handleServerMessage(ctx, result.raw); err != nil {
                // Plugin/configuration failures are application errors. Keep
                // the authenticated socket alive so the gateway does not
                // incorrectly fail over to another node.
                a.log.Warn("failed to handle gateway message", "gateway", wsURL, "error", err)
            }
        case <-heartbeat.C:
            latencyMu.RLock(); currentLatency := latencyMs; latencyMu.RUnlock(); if err := a.send(conn, "heartbeat", a.heartbeat(time.Now().UTC(), currentLatency)); err != nil { return fmt.Errorf("heartbeat failed: %w", err) }
        case <-ping.C:
            sentAt := strconv.FormatInt(time.Now().UnixNano(), 10); a.writeMu.Lock(); _ = conn.SetWriteDeadline(time.Now().Add(a.cfg.ConnectTimeout)); err := conn.WriteMessage(websocket.PingMessage, []byte(sentAt)); a.writeMu.Unlock(); if err != nil { return fmt.Errorf("gateway ping failed: %w", err) }
        case <-health.C:
            probeCtx, cancel := context.WithTimeout(ctx, a.cfg.ConnectTimeout); probeErr := gateway.Probe(probeCtx, wsURL, a.cfg.ConnectTimeout); cancel(); if probeErr != nil { return fmt.Errorf("gateway health check failed: %w", probeErr) }
        }
    }
}

type readResult struct { raw []byte; err error }
func readLoop(conn *websocket.Conn, resultCh chan<- readResult, timeout time.Duration) { for { _ = conn.SetReadDeadline(time.Now().Add(timeout)); _, raw, err := conn.ReadMessage(); resultCh <- readResult{raw: raw, err: err}; if err != nil { return } } }

func (a *Agent) finishHandshake(conn *websocket.Conn) error { for { _, raw, err := conn.ReadMessage(); if err != nil { return err }; var env protocol.Envelope; if err := json.Unmarshal(raw, &env); err != nil { return errors.New("invalid gateway message") }; switch env.Type { case "registration_complete": var complete protocol.RegistrationComplete; if err := json.Unmarshal(env.Data, &complete); err != nil { return err }; if complete.AgentID == "" || complete.KeyID == "" { return errors.New("gateway returned incomplete registration") }; a.creds.AgentID, a.creds.KeyID = complete.AgentID, complete.KeyID; if err := a.store.Save(a.creds); err != nil { return err }; _ = os.Remove(a.cfg.EnrollmentTokenPath); a.log.Info("agent enrollment completed", "agent_id", complete.AgentID, "key_id", complete.KeyID); case "challenge": var challenge protocol.Challenge; if err := json.Unmarshal(env.Data, &challenge); err != nil { return err }; signature := ed25519.Sign(a.private, []byte(challenge.Challenge)); encoded := base64.RawURLEncoding.EncodeToString(signature); if err := a.send(conn, "challenge_response", protocol.ChallengeResponse{KeyID: a.creds.KeyID, Signature: encoded}); err != nil { return err }; case "authenticated": return nil; case "error": var gatewayErr struct { Code string `json:"code"`; Message string `json:"message"` }; _ = json.Unmarshal(env.Data, &gatewayErr); if gatewayErr.Message == "" { gatewayErr.Message = "gateway rejected connection" }; return fmt.Errorf("gateway error %s: %s", gatewayErr.Code, gatewayErr.Message) } } }

func (a *Agent) handleServerMessage(ctx context.Context, raw []byte) error {
    var env protocol.Envelope; if err := json.Unmarshal(raw, &env); err != nil { return errors.New("invalid gateway message") }
    if env.Type == "error" { var gatewayErr struct { Code string `json:"code"`; Message string `json:"message"` }; _ = json.Unmarshal(env.Data, &gatewayErr); return fmt.Errorf("gateway error %s: %s", gatewayErr.Code, gatewayErr.Message) }
    switch env.Type {
    case "plugin.install":
        var install protocol.PluginInstall; if err := json.Unmarshal(env.Data, &install); err != nil { return err }
        if install.PluginID == "" || install.PluginSlug == "" || install.ProfileID == "" { return errors.New("invalid plugin installation command") }
        if err := a.plugins.Install(ctx, install.PluginID, install.PluginSlug, install.PluginVersion, install.Config); err != nil { return err }
    }
    return nil
}

func (a *Agent) reportPluginStatus(ctx context.Context, pluginID string, status plugins.Status, version, message string) error {
    a.pluginMu.RLock(); conn := a.pluginConn; a.pluginMu.RUnlock(); if conn == nil { return errors.New("agent is not connected to a gateway") }
    return a.send(conn, "plugin_status", protocol.PluginStatus{PluginID: pluginID, Status: string(status), Version: version, Error: message})
}

func (a *Agent) reportPluginTelemetry(ctx context.Context, telemetry plugins.Telemetry) error {
    a.pluginMu.RLock(); conn := a.pluginConn; a.pluginMu.RUnlock(); if conn == nil { return errors.New("agent is not connected to a gateway") }
    measurements := make([]protocol.PluginMeasurement, 0, len(telemetry.Measurements)); for _, measurement := range telemetry.Measurements { measurements = append(measurements, protocol.PluginMeasurement{Name: measurement.Name, Tags: measurement.Tags, Fields: measurement.Fields}) }
    return a.send(conn, "plugin_data", protocol.PluginData{PluginID: telemetry.PluginID, PluginSlug: telemetry.PluginSlug, SchemaVersion: telemetry.SchemaVersion, Timestamp: telemetry.Timestamp, Measurements: measurements})
}

func (a *Agent) registration(token string) protocol.RegisterRequest { hostname, _ := os.Hostname(); return protocol.RegisterRequest{EnrollmentToken: token, PublicKey: base64.RawURLEncoding.EncodeToString(a.public), DeviceID: a.creds.DeviceID, Hostname: hostname, LocalIP: localIP(), PublicIP: strings.TrimSpace(os.Getenv("FORTMONT_PUBLIC_IP")), Platform: runtime.GOOS, Architecture: runtime.GOARCH, Version: a.cfg.Version} }
func (a *Agent) heartbeat(now time.Time, latencyMs float64) protocol.Heartbeat { hostname, _ := os.Hostname(); return protocol.Heartbeat{Timestamp: now.Format(time.RFC3339Nano), Hostname: hostname, LocalIP: localIP(), PublicIP: strings.TrimSpace(os.Getenv("FORTMONT_PUBLIC_IP")), Platform: runtime.GOOS, Architecture: runtime.GOARCH, Version: a.cfg.Version, LatencyMs: latencyMs} }
func (a *Agent) send(conn *websocket.Conn, typ string, data any) error { message, err := protocol.MarshalMessage(typ, data); if err != nil { return err }; a.writeMu.Lock(); defer a.writeMu.Unlock(); _ = conn.SetWriteDeadline(time.Now().Add(a.cfg.ConnectTimeout)); return conn.WriteMessage(websocket.TextMessage, message) }
func (a *Agent) logGatewaySelection(selected string, candidates []gateway.Candidate) { fields := make([]string, 0, len(candidates)); for _, candidate := range candidates { if candidate.Err != nil { fields = append(fields, fmt.Sprintf("%s failed: %v", candidate.URL, candidate.Err)); continue }; fields = append(fields, fmt.Sprintf("%s %s", candidate.URL, candidate.Latency)) }; if selected == "" { a.log.Warn("no usable WebSocket gateway selected", "candidates", fields); return }; a.log.Info("selected fastest WebSocket gateway", "url", selected, "candidates", fields) }
func localIP() string { conn, err := net.Dial("udp", "1.1.1.1:53"); if err != nil { return "" }; defer conn.Close(); if address, ok := conn.LocalAddr().(*net.UDPAddr); ok { return address.IP.String() }; return "" }
func sleepContext(ctx context.Context, duration time.Duration) bool { timer := time.NewTimer(duration); defer timer.Stop(); select { case <-ctx.Done(): return false; case <-timer.C: return true } }
func minDuration(a, b time.Duration) time.Duration { if a < b { return a }; return b }
