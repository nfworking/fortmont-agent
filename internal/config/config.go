package config

import (
    "bufio"
    "errors"
    "fmt"
    "os"
    "path/filepath"
    "strconv"
    "strings"
    "time"
)

type Config struct {
    WSNodes           []string
    ConfigDir         string
    CredentialsPath   string
    Version           string
    ConnectTimeout    time.Duration
    ReconnectMax      time.Duration
    HeartbeatInterval time.Duration
    PingInterval      time.Duration
}

func Load() (Config, error) {
    _ = loadDotEnv(".env")

    nodes := splitCSV(os.Getenv("FORTMONT_WS_NODES"))
    if len(nodes) == 0 {
        return Config{}, errors.New("FORTMONT_WS_NODES is required")
    }
    for _, node := range nodes {
        if !strings.HasPrefix(node, "ws://") && !strings.HasPrefix(node, "wss://") {
            return Config{}, fmt.Errorf("invalid WebSocket node %q: must use ws:// or wss://", node)
        }
    }

    dir := os.Getenv("FORTMONT_CONFIG_DIR")
    if dir == "" {
        dir = defaultConfigDir()
    }
    version := os.Getenv("FORTMONT_VERSION")
    if version == "" {
        version = "dev"
    }

    connectTimeout, err := seconds("FORTMONT_CONNECT_TIMEOUT_SEC", 10)
    if err != nil { return Config{}, err }
    reconnectMax, err := seconds("FORTMONT_RECONNECT_MAX_SEC", 30)
    if err != nil { return Config{}, err }
    heartbeat, err := seconds("FORTMONT_HEARTBEAT_SEC", 30)
    if err != nil { return Config{}, err }
    ping, err := seconds("FORTMONT_PING_INTERVAL_SEC", 30)
    if err != nil { return Config{}, err }

    return Config{
        WSNodes: nodes,
        ConfigDir: dir,
        CredentialsPath: filepath.Join(dir, "credentials.json"),
        Version: version,
        ConnectTimeout: connectTimeout,
        ReconnectMax: reconnectMax,
        HeartbeatInterval: heartbeat,
        PingInterval: ping,
    }, nil
}

func seconds(key string, fallback int) (time.Duration, error) {
    raw := os.Getenv(key)
    if raw == "" { return time.Duration(fallback) * time.Second, nil }
    value, err := strconv.Atoi(raw)
    if err != nil || value <= 0 {
        return 0, fmt.Errorf("%s must be a positive integer", key)
    }
    return time.Duration(value) * time.Second, nil
}

func splitCSV(raw string) []string {
    var result []string
    for _, value := range strings.Split(raw, ",") {
        value = strings.TrimSpace(value)
        if value != "" { result = append(result, strings.TrimRight(value, "/")) }
    }
    return result
}

func defaultConfigDir() string {
    if dir, err := os.UserConfigDir(); err == nil {
        return filepath.Join(dir, "Fortmont", "Agent")
    }
    return "./data"
}

// loadDotEnv intentionally supports the small .env format needed by the agent
// without adding a runtime dependency. Existing process environment variables
// always win over values from .env.
func loadDotEnv(path string) error {
    file, err := os.Open(path)
    if err != nil { return err }
    defer file.Close()

    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        line := strings.TrimSpace(scanner.Text())
        if line == "" || strings.HasPrefix(line, "#") { continue }
        if strings.HasPrefix(line, "export ") { line = strings.TrimSpace(strings.TrimPrefix(line, "export ")) }
        key, value, ok := strings.Cut(line, "=")
        if !ok { continue }
        key = strings.TrimSpace(key)
        value = strings.TrimSpace(value)
        if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
            value = value[1 : len(value)-1]
        }
        if key != "" {
            if _, exists := os.LookupEnv(key); !exists { _ = os.Setenv(key, value) }
        }
    }
    return scanner.Err()
}
