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
    if strings.TrimSpace(token) == "" {
        return fmt.Errorf("an enrollment token is required: use --token")
    }
    if err := os.MkdirAll(ConfigDir(), 0700); err != nil { return fmt.Errorf("create service config directory: %w", err) }
    tokenPath := filepath.Join(ConfigDir(), "enrollment-token")
    if err := os.WriteFile(tokenPath, []byte(strings.TrimSpace(token)+"\n"), 0600); err != nil { return fmt.Errorf("write enrollment token: %w", err) }

    _ = exec.Command("sc.exe", "stop", Name).Run()
    _ = exec.Command("sc.exe", "delete", Name).Run()

    binPath := fmt.Sprintf(`"%s" service run`, executable)
    if out, err := exec.Command("sc.exe", "create", Name, "binPath=", binPath, "start=", "auto", "DisplayName=", "Fortmont Agent").CombinedOutput(); err != nil {
        return fmt.Errorf("create Windows service: %w: %s", err, strings.TrimSpace(string(out)))
    }
    _ = exec.Command("sc.exe", "description", Name, "Fortmont infrastructure monitoring agent").Run()
    if out, err := exec.Command("sc.exe", "start", Name).CombinedOutput(); err != nil {
        return fmt.Errorf("start Windows service: %w: %s", err, strings.TrimSpace(string(out)))
    }
    fmt.Printf("Fortmont Agent service installed successfully windowsServiceName=%s\n", Name)
    return nil
}

func uninstall() error {
    _ = exec.Command("sc.exe", "stop", Name).Run()
    if out, err := exec.Command("sc.exe", "delete", Name).CombinedOutput(); err != nil {
        return fmt.Errorf("uninstall Windows service: %w: %s", err, strings.TrimSpace(string(out)))
    }
    _ = os.RemoveAll(ConfigDir())
    fmt.Printf("Fortmont Agent service uninstalled successfully windowsServiceName=%s\n", Name)
    return nil
}

func status() error {
    cmd := exec.Command("sc.exe", "query", Name)
    out, err := cmd.CombinedOutput()
    if len(out) > 0 { fmt.Print(string(out)) }
    if err != nil { return fmt.Errorf("Fortmont Agent service is not installed or could not be queried: %w", err) }
    return nil
}
