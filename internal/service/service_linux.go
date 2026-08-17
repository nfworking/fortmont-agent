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

func install(executable, token string) error {
    if strings.TrimSpace(token) == "" { return fmt.Errorf("an enrollment token is required: use --token") }
    if err := os.MkdirAll(ConfigDir(), 0700); err != nil { return fmt.Errorf("create service config directory: %w", err) }
    tokenPath := filepath.Join(ConfigDir(), "enrollment-token")
    if err := os.WriteFile(tokenPath, []byte(strings.TrimSpace(token)+"\n"), 0600); err != nil { return fmt.Errorf("write enrollment token: %w", err) }

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
    if out, err := exec.Command("systemctl", "enable", "--now", Name).CombinedOutput(); err != nil { return fmt.Errorf("start Fortmont Agent service: %w: %s", err, strings.TrimSpace(string(out))) }
    fmt.Printf("Fortmont Agent service installed successfully serviceName=%s\n", Name)
    return nil
}

func uninstall() error {
    _ = exec.Command("systemctl", "disable", "--now", Name).Run()
    if out, err := exec.Command("systemctl", "daemon-reload").CombinedOutput(); err != nil { return fmt.Errorf("systemd daemon-reload: %w: %s", err, strings.TrimSpace(string(out))) }
    if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) { return fmt.Errorf("remove systemd unit: %w", err) }
    _ = exec.Command("systemctl", "daemon-reload").Run()
    _ = os.RemoveAll(ConfigDir())
    fmt.Printf("Fortmont Agent service uninstalled successfully serviceName=%s\n", Name)
    return nil
}

func status() error {
    cmd := exec.Command("systemctl", "status", Name, "--no-pager")
    out, err := cmd.CombinedOutput()
    fmt.Print(string(out))
    if err != nil { return fmt.Errorf("Fortmont Agent service is not active or could not be queried: %w", err) }
    return nil
}
