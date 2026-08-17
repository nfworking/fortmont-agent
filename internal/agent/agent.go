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
    "strings"
    "sync"
    "time"

    "github.com/gorilla/websocket"

    "github.com/nfworking/fortmont-agent/internal/config"
    "github.com/nfworking/fortmont-agent/internal/gateway"
    "github.com/nfworking/fortmont-agent/internal/identity"
    "github.com/nfworking/fortmont-agent/internal/protocol"
)

type Agent struct {
    cfg     config.Config
    store   identity.Store
    creds   identity.Credentials
    private ed25519.PrivateKey
    public  ed25519.PublicKey
    log     *slog.Logger
    writeMu sync.Mutex
}

func New(cfg config.Config, log *slog.Logger) (*Agent, error) {
    store := identity.Store{Path: cfg.CredentialsPath}
    creds, privateKey, publicKey, err := store.LoadOrCreate()
    if err != nil { return nil, err }
    return &Agent{cfg: cfg, store: store, creds: creds, private: privateKey, public: publicKey, log: log}, nil
}

func (a *Agent) Run(ctx context.Context, enrollmentToken string) error {
    backoff := time.Second
    for {
        if ctx.Err() != nil { return nil }

        selected, candidates, err := gateway.SelectFastest(ctx, a.cfg.WSNodes, a.cfg.ConnectTimeout)
        if err != nil {
            a.log.Warn("no gateway reachable", "error", err)
            if !sleepContext(ctx, backoff) { return nil }
            backoff = minDuration(backoff*2, a.cfg.ReconnectMax)
            continue
        }
        backoff = time.Second
        a.logGatewaySelection(selected, candidates)

        err = a.connectAndRun(ctx, selected, enrollmentToken)
        if err == nil || ctx.Err() != nil { return nil }
        a.log.Warn("gateway connection ended; reconnecting", "error", err)
        enrollmentToken = ""
        if !sleepContext(ctx, backoff) { return nil }
        backoff = minDuration(backoff*2, a.cfg.ReconnectMax)
    }
}

func (a *Agent) connectAndRun(ctx context.Context, wsURL, enrollmentToken string) error {
    parsed, err := url.Parse(wsURL)
    if err != nil { return err }
    dialer := websocket.Dialer{HandshakeTimeout: a.cfg.ConnectTimeout}
    conn, _, err := dialer.DialContext(ctx, parsed.String(), nil)
    if err != nil { return fmt.Errorf("connect %s: %w", wsURL, err) }
    defer conn.Close()

    conn.SetReadLimit(1024 * 1024)
    _ = conn.SetReadDeadline(time.Now().Add(a.cfg.ConnectTimeout))
    conn.SetPongHandler(func(string) error {
        return conn.SetReadDeadline(time.Now().Add(a.cfg.PingInterval * 2))
    })

    if a.creds.AgentID != "" && a.creds.KeyID != "" {
        if err := a.send(conn, "authenticate", protocol.AuthenticateRequest{AgentID: a.creds.AgentID, KeyID: a.creds.KeyID}); err != nil { return err }
    } else {
        if enrollmentToken == "" { return errors.New("agent is not enrolled; supply an enrollment token with --token or FORTMONT_ENROLLMENT_TOKEN") }
        if err := a.send(conn, "register", a.registration(enrollmentToken)); err != nil { return err }
    }

    if err := a.finishHandshake(conn); err != nil { return err }
    a.log.Info("agent authenticated", "agent_id", a.creds.AgentID, "key_id", a.creds.KeyID, "gateway", wsURL)

    if err := a.send(conn, "heartbeat", a.heartbeat(time.Now().UTC())); err != nil { return err }

    heartbeat := time.NewTicker(a.cfg.HeartbeatInterval)
    defer heartbeat.Stop()
    ping := time.NewTicker(a.cfg.PingInterval)
    defer ping.Stop()

    readCh := make(chan readResult, 1)
    go readLoop(conn, readCh)

    for {
        select {
        case <-ctx.Done():
            return nil
        case result := <-readCh:
            if result.err != nil { return result.err }
            if err := handleServerMessage(result.raw); err != nil { return err }
        case <-heartbeat.C:
            if err := a.send(conn, "heartbeat", a.heartbeat(time.Now().UTC())); err != nil { return err }
        case <-ping.C:
            a.writeMu.Lock()
            _ = conn.SetWriteDeadline(time.Now().Add(a.cfg.ConnectTimeout))
            err := conn.WriteMessage(websocket.PingMessage, nil)
            a.writeMu.Unlock()
            if err != nil { return err }
        }
    }
}

type readResult struct { raw []byte; err error }

func readLoop(conn *websocket.Conn, resultCh chan<- readResult) {
    for {
        _, raw, err := conn.ReadMessage()
        resultCh <- readResult{raw: raw, err: err}
        if err != nil { return }
    }
}

func (a *Agent) finishHandshake(conn *websocket.Conn) error {
    for {
        _, raw, err := conn.ReadMessage()
        if err != nil { return err }
        var env protocol.Envelope
        if err := json.Unmarshal(raw, &env); err != nil { return errors.New("invalid gateway message") }
        switch env.Type {
        case "registration_complete":
            var complete protocol.RegistrationComplete
            if err := json.Unmarshal(env.Data, &complete); err != nil { return err }
            if complete.AgentID == "" || complete.KeyID == "" { return errors.New("gateway returned incomplete registration") }
            a.creds.AgentID = complete.AgentID
            a.creds.KeyID = complete.KeyID
            if err := a.store.Save(a.creds); err != nil { return err }
            a.log.Info("agent enrollment completed", "agent_id", complete.AgentID, "key_id", complete.KeyID)
        case "challenge":
            var challenge protocol.Challenge
            if err := json.Unmarshal(env.Data, &challenge); err != nil { return err }
            signature := ed25519.Sign(a.private, []byte(challenge.Challenge))
            encoded := base64.RawURLEncoding.EncodeToString(signature)
            if err := a.send(conn, "challenge_response", protocol.ChallengeResponse{KeyID: a.creds.KeyID, Signature: encoded}); err != nil { return err }
        case "authenticated":
            return nil
        case "error":
            var gatewayErr protocol.Error
            _ = json.Unmarshal(env.Data, &gatewayErr)
            if gatewayErr.Message == "" { gatewayErr.Message = "gateway rejected connection" }
            return fmt.Errorf("gateway error %s: %s", gatewayErr.Code, gatewayErr.Message)
        }
    }
}

func handleServerMessage(raw []byte) error {
    var env protocol.Envelope
    if err := json.Unmarshal(raw, &env); err != nil { return errors.New("invalid gateway message") }
    if env.Type == "error" {
        var gatewayErr protocol.Error
        _ = json.Unmarshal(env.Data, &gatewayErr)
        return fmt.Errorf("gateway error %s: %s", gatewayErr.Code, gatewayErr.Message)
    }
    return nil
}

func (a *Agent) registration(token string) protocol.RegisterRequest {
    hostname, _ := os.Hostname()
    return protocol.RegisterRequest{
        EnrollmentToken: token,
        PublicKey: base64.RawURLEncoding.EncodeToString(a.public),
        DeviceID: a.creds.DeviceID,
        Hostname: hostname,
        LocalIP: localIP(),
        PublicIP: strings.TrimSpace(os.Getenv("FORTMONT_PUBLIC_IP")),
        Platform: runtime.GOOS,
        Architecture: runtime.GOARCH,
        Version: a.cfg.Version,
    }
}

func (a *Agent) heartbeat(now time.Time) protocol.Heartbeat {
    hostname, _ := os.Hostname()
    return protocol.Heartbeat{
        Timestamp: now.Format(time.RFC3339Nano),
        Hostname: hostname,
        LocalIP: localIP(),
        PublicIP: strings.TrimSpace(os.Getenv("FORTMONT_PUBLIC_IP")),
        Platform: runtime.GOOS,
        Architecture: runtime.GOARCH,
        Version: a.cfg.Version,
    }
}

func (a *Agent) send(conn *websocket.Conn, typ string, data any) error {
    message, err := protocol.MarshalMessage(typ, data)
    if err != nil { return err }
    a.writeMu.Lock()
    defer a.writeMu.Unlock()
    _ = conn.SetWriteDeadline(time.Now().Add(a.cfg.ConnectTimeout))
    return conn.WriteMessage(websocket.TextMessage, message)
}

func (a *Agent) logGatewaySelection(selected string, candidates []gateway.Candidate) {
    fields := make([]any, 0, len(candidates)*2)
    for _, candidate := range candidates {
        fields = append(fields, candidate.URL, candidate.Latency.String())
    }
    a.log.Info("selected fastest WebSocket gateway", "url", selected, "candidates", fields)
}

func localIP() string {
    conn, err := net.Dial("udp", "1.1.1.1:53")
    if err != nil { return "" }
    defer conn.Close()
    if address, ok := conn.LocalAddr().(*net.UDPAddr); ok { return address.IP.String() }
    return ""
}

func sleepContext(ctx context.Context, duration time.Duration) bool {
    timer := time.NewTimer(duration)
    defer timer.Stop()
    select { case <-ctx.Done(): return false; case <-timer.C: return true }
}

func minDuration(a, b time.Duration) time.Duration { if a < b { return a }; return b }
