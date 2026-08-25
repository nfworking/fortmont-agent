//go:build linux

package service

import (
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
)

const unitPath = "/etc/systemd/system/fortmont-agent.service"
const systemdName = "fortmont-agent"

func install(executable, token string) error {
    if strings.TrimSpace(token) == "" { return fmt.Errorf("an enrollment token is required: use --token") }
    if err := os.MkdirAll(ConfigDir(), 0700); err != nil { return fmt.Errorf("create service config directory: %w", err) }
    if err := os.WriteFile(filepath.Join(ConfigDir(), "enrollment-token"), []byte(strings.TrimSpace(token)+"\n"), 0600); err != nil { return fmt.Errorf("write enrollment token: %w", err) }
    if err := writeServiceEnv(); err != nil { return err }

    unit := fmt.Sprintf(`[Unit]
Description=Fortmont Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s service run
Restart=always
RestartSec=5
Environment=FORTMONT_CONFIG_DIR=%s

[Install]
WantedBy=multi-user.target
`, executable, ConfigDir())
    if err := os.WriteFile(unitPath, []byte(unit), 0644); err != nil { return fmt.Errorf("write systemd unit: %w", err) }
    if out, err := exec.Command("systemctl", "daemon-reload").CombinedOutput(); err != nil { return fmt.Errorf("systemd daemon-reload: %w: %s", err, strings.TrimSpace(string(out))) }
    if out, err := exec.Command("systemctl", "enable", "--now", systemdName).CombinedOutput(); err != nil { return fmt.Errorf("start Fortmont Agent service: %w: %s", err, strings.TrimSpace(string(out))) }
    fmt.Printf("Fortmont Agent service installed successfully serviceName=%s\n", systemdName)
    return nil
}

func writeServiceEnv() error {
    keys := []string{ "FORTMONT_VERSION", "FORTMONT_PUBLIC_IP", "FORTMONT_LOG_LEVEL", "FORTMONT_CONNECT_TIMEOUT_SEC", "FORTMONT_RECONNECT_MAX_SEC", "FORTMONT_HEARTBEAT_SEC", "FORTMONT_PING_INTERVAL_SEC", "FORTMONT_GATEWAY_HEALTH_CHECK_SEC"}
    var lines []string
    for _, key := range keys { if value := os.Getenv(key); value != "" { lines = append(lines, key+"="+value) } }
    if err := os.WriteFile(filepath.Join(ConfigDir(), "service.env"), []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil { return fmt.Errorf("write service environment: %w", err) }
    return nil
}

func uninstall() error {
    _ = exec.Command("systemctl", "disable", "--now", systemdName).Run()
    if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) { return fmt.Errorf("remove systemd unit: %w", err) }
    if out, err := exec.Command("systemctl", "daemon-reload").CombinedOutput(); err != nil { return fmt.Errorf("systemd daemon-reload: %w: %s", err, strings.TrimSpace(string(out))) }
    _ = os.RemoveAll(ConfigDir())
    fmt.Printf("Fortmont Agent service uninstalled successfully serviceName=%s\n", systemdName)
    return nil
}

func status() error {
    out, err := exec.Command("systemctl", "status", systemdName, "--no-pager").CombinedOutput()
    fmt.Print(string(out))
    if err != nil { return fmt.Errorf("Fortmont Agent service is not active or could not be queried: %w", err) }
    return nil
}
