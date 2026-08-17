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

// SelectFastest probes every configured gateway concurrently and selects the
// gateway with the lowest successful WebSocket handshake latency. Measuring a
// real WebSocket handshake (rather than only TCP/TLS) makes the result reflect
// the connection the agent will actually use.
func SelectFastest(ctx context.Context, nodes []string, timeout time.Duration) (string, []Candidate, error) {
    results := make([]Candidate, len(nodes))
    var wg sync.WaitGroup

    for i, node := range nodes {
        wg.Add(1)
        go func(i int, node string) {
            defer wg.Done()
            start := time.Now()
            err := probe(ctx, node, timeout)
            results[i] = Candidate{
                URL:     node,
                Latency: time.Since(start),
                Err:     err,
            }
        }(i, node)
    }

    wg.Wait()

    reachable := make([]Candidate, 0, len(results))
    for _, result := range results {
        if result.Err == nil {
            reachable = append(reachable, result)
        }
    }

    sort.SliceStable(reachable, func(i, j int) bool {
        return reachable[i].Latency < reachable[j].Latency
    })

    if len(reachable) == 0 {
        return "", results, fmt.Errorf("no WebSocket gateways are reachable: %s", summarize(results))
    }

    return reachable[0].URL, results, nil
}

func probe(ctx context.Context, rawURL string, timeout time.Duration) error {
    parsed, err := url.Parse(rawURL)
    if err != nil {
        return err
    }
    if parsed.Hostname() == "" {
        return fmt.Errorf("missing host")
    }
    if parsed.Scheme != "ws" && parsed.Scheme != "wss" {
        return fmt.Errorf("unsupported WebSocket scheme %q", parsed.Scheme)
    }

    probeCtx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()

    dialer := websocket.Dialer{
        HandshakeTimeout: timeout,
        Proxy:            http.ProxyFromEnvironment,
    }

    conn, response, err := dialer.DialContext(probeCtx, parsed.String(), nil)
    if err != nil {
        if response != nil {
            return fmt.Errorf("WebSocket handshake failed with HTTP %s: %w", response.Status, err)
        }
        return err
    }
    _ = conn.Close()
    return nil
}

func summarize(results []Candidate) string {
    values := make([]string, 0, len(results))
    for _, result := range results {
        if result.Err == nil {
            values = append(values, result.URL+"="+result.Latency.String())
        } else {
            values = append(values, result.URL+"="+result.Err.Error())
        }
    }
    return strings.Join(values, "; ")
}
