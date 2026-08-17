package gateway

import (
    "context"
    "fmt"
    "net/http"
    "net/url"
    "sort"
    "strings"
    "sync"
    "time"

    "github.com/gorilla/websocket"
)

type Candidate struct {
    URL     string
    Latency time.Duration
    Err     error
}

func SelectFastest(ctx context.Context, nodes []string, timeout time.Duration) (string, []Candidate, error) {
    return SelectFastestExcluding(ctx, nodes, nil, timeout)
}

func SelectFastestExcluding(ctx context.Context, nodes []string, excluded map[string]struct{}, timeout time.Duration) (string, []Candidate, error) {
    candidates := make([]string, 0, len(nodes))
    for _, node := range nodes {
        if _, skip := excluded[node]; skip { continue }
        candidates = append(candidates, node)
    }
    if len(candidates) == 0 { candidates = append(candidates, nodes...) }

    results := make([]Candidate, len(candidates))
    var wg sync.WaitGroup
    for i, node := range candidates {
        wg.Add(1)
        go func(i int, node string) {
            defer wg.Done()
            start := time.Now()
            err := Probe(ctx, node, timeout)
            results[i] = Candidate{URL: node, Latency: time.Since(start), Err: err}
        }(i, node)
    }
    wg.Wait()

    reachable := make([]Candidate, 0, len(results))
    for _, result := range results {
        if result.Err == nil { reachable = append(reachable, result) }
    }
    sort.SliceStable(reachable, func(i, j int) bool { return reachable[i].Latency < reachable[j].Latency })
    if len(reachable) == 0 { return "", results, fmt.Errorf("no WebSocket gateways are reachable: %s", summarize(results)) }
    return reachable[0].URL, results, nil
}

// Probe performs a real WebSocket handshake against the gateway. It is used
// both for initial fastest-node selection and for detecting a gateway/tunnel
// that became unavailable after an agent connection was established.
func Probe(ctx context.Context, rawURL string, timeout time.Duration) error {
    parsed, err := url.Parse(rawURL)
    if err != nil { return err }
    if parsed.Hostname() == "" { return fmt.Errorf("missing host") }
    if parsed.Scheme != "ws" && parsed.Scheme != "wss" { return fmt.Errorf("unsupported WebSocket scheme %q", parsed.Scheme) }

    probeCtx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()

    dialer := websocket.Dialer{HandshakeTimeout: timeout, Proxy: http.ProxyFromEnvironment}
    conn, response, err := dialer.DialContext(probeCtx, parsed.String(), nil)
    if err != nil {
        if response != nil { return fmt.Errorf("WebSocket handshake failed with HTTP %s: %w", response.Status, err) }
        return err
    }
    _ = conn.Close()
    return nil
}

func summarize(results []Candidate) string {
    values := make([]string, 0, len(results))
    for _, result := range results {
        if result.Err == nil { values = append(values, result.URL+"="+result.Latency.String()) } else { values = append(values, result.URL+"="+result.Err.Error()) }
    }
    return strings.Join(values, "; ")
}
