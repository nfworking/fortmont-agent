//go:build windows

package service

import (
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
)

func install(executable, token string) error {
    if strings.TrimSpace(token) == "" { return fmt.Errorf("an enrollment token is required: use --token") }
    if strings.TrimSpace(os.Getenv("FORTMONT_WS_NODES")) == "" { return fmt.Errorf("FORTMONT_WS_NODES must be set when installing the service") }
    if err := os.MkdirAll(ConfigDir(), 0700); err != nil { return fmt.Errorf("create service config directory: %w", err) }
    if err := os.WriteFile(filepath.Join(ConfigDir(), "enrollment-token"), []byte(strings.TrimSpace(token)+"\n"), 0600); err != nil { return fmt.Errorf("write enrollment token: %w", err) }
    if err := writeServiceEnv(); err != nil { return err }

    _ = exec.Command("sc.exe", "stop", Name).Run()
    _ = exec.Command("sc.exe", "delete", Name).Run()
    binPath := fmt.Sprintf(`"%s" service run`, executable)
    if out, err := exec.Command("sc.exe", "create", Name, "binPath=", binPath, "start=", "auto", "DisplayName=", "Fortmont Agent").CombinedOutput(); err != nil { return fmt.Errorf("create Windows service: %w: %s", err, strings.TrimSpace(string(out))) }
    _ = exec.Command("sc.exe", "description", Name, "Fortmont infrastructure monitoring agent").Run()
    if out, err := exec.Command("sc.exe", "start", Name).CombinedOutput(); err != nil { return fmt.Errorf("start Windows service: %w: %s", err, strings.TrimSpace(string(out))) }
    fmt.Printf("Fortmont Agent service installed successfully windowsServiceName=%s\n", Name)
    return nil
}

func writeServiceEnv() error {
    keys := []string{"FORTMONT_WS_NODES", "FORTMONT_VERSION", "FORTMONT_PUBLIC_IP", "FORTMONT_LOG_LEVEL", "FORTMONT_CONNECT_TIMEOUT_SEC", "FORTMONT_RECONNECT_MAX_SEC", "FORTMONT_HEARTBEAT_SEC", "FORTMONT_PING_INTERVAL_SEC", "FORTMONT_GATEWAY_HEALTH_CHECK_SEC"}
    var lines []string
    for _, key := range keys { if value := os.Getenv(key); value != "" { lines = append(lines, key+"="+value) } }
    if err := os.WriteFile(filepath.Join(ConfigDir(), "service.env"), []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil { return fmt.Errorf("write service environment: %w", err) }
    return nil
}

func uninstall() error {
    _ = exec.Command("sc.exe", "stop", Name).Run()
    if out, err := exec.Command("sc.exe", "delete", Name).CombinedOutput(); err != nil { return fmt.Errorf("uninstall Windows service: %w: %s", err, strings.TrimSpace(string(out))) }
    _ = os.RemoveAll(ConfigDir())
    fmt.Printf("Fortmont Agent service uninstalled successfully windowsServiceName=%s\n", Name)
    return nil
}

func status() error {
    out, err := exec.Command("sc.exe", "query", Name).CombinedOutput()
    if len(out) > 0 { fmt.Print(string(out)) }
    if err != nil { return fmt.Errorf("Fortmont Agent service is not installed or could not be queried: %w", err) }
    return nil
}
