package gateway

import (
    "context"
    "crypto/tls"
    "fmt"
    "net"
    "net/url"
    "sort"
    "strings"
    "sync"
    "time"
)

type Candidate struct {
    URL     string
    Latency time.Duration
    Err     error
}

func SelectFastest(ctx context.Context, nodes []string, timeout time.Duration) (string, []Candidate, error) {
    results := make([]Candidate, len(nodes))
    var wg sync.WaitGroup
    for i, node := range nodes {
        wg.Add(1)
        go func(i int, node string) {
            defer wg.Done()
            start := time.Now()
            err := probe(ctx, node, timeout)
            results[i] = Candidate{URL: node, Latency: time.Since(start), Err: err}
        }(i, node)
    }
    wg.Wait()

    reachable := make([]Candidate, 0, len(results))
    for _, result := range results {
        if result.Err == nil { reachable = append(reachable, result) }
    }
    sort.Slice(reachable, func(i, j int) bool { return reachable[i].Latency < reachable[j].Latency })
    if len(reachable) == 0 {
        return "", results, fmt.Errorf("no WebSocket gateways are reachable: %s", summarize(results))
    }
    return reachable[0].URL, results, nil
}

func probe(ctx context.Context, rawURL string, timeout time.Duration) error {
    parsed, err := url.Parse(rawURL)
    if err != nil { return err }
    if parsed.Hostname() == "" { return fmt.Errorf("missing host") }
    port := parsed.Port()
    if port == "" {
        if parsed.Scheme == "wss" { port = "443" } else { port = "80" }
    }
    address := net.JoinHostPort(parsed.Hostname(), port)
    dialer := net.Dialer{Timeout: timeout}
    conn, err := dialer.DialContext(ctx, "tcp", address)
    if err != nil { return err }
    defer conn.Close()
    if parsed.Scheme == "wss" {
        tlsConn := tls.Client(conn, &tls.Config{ServerName: parsed.Hostname(), MinVersion: tls.VersionTLS12})
        deadline := time.Now().Add(timeout)
        _ = tlsConn.SetDeadline(deadline)
        if err := tlsConn.HandshakeContext(ctx); err != nil { return err }
    }
    return nil
}

func summarize(results []Candidate) string {
    values := make([]string, 0, len(results))
    for _, result := range results {
        if result.Err == nil { values = append(values, result.URL+"="+result.Latency.String()) } else { values = append(values, result.URL+"="+result.Err.Error()) }
    }
    return strings.Join(values, "; ")
}
