//go:build windows

package service

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func install(executable, token string) error {
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("an enrollment token is required: use --token")
	}
	
	if err := os.MkdirAll(ConfigDir(), 0700); err != nil {
		return fmt.Errorf("create service config directory: %w", err)
	}

	// A token supplied to service install represents a new enrollment. Remove
	// an older identity so it cannot take precedence over the new token.
	if err := os.Remove(filepath.Join(ConfigDir(), "credentials.json")); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reset existing agent credentials: %w", err)
	}
	if err := os.WriteFile(filepath.Join(ConfigDir(), "enrollment-token"), []byte(strings.TrimSpace(token)+"\n"), 0600); err != nil {
		return fmt.Errorf("write enrollment token: %w", err)
	}
	if err := writeServiceEnv(); err != nil {
		return err
	}

	_ = exec.Command("sc.exe", "stop", Name).Run()
	_ = exec.Command("sc.exe", "delete", Name).Run()

	// Install a stable executable copy. This also makes `go run ./cmd/agent
	// service install` work: the temporary go-run executable is otherwise
	// deleted when the go command exits.
	installedExecutable := filepath.Join(ConfigDir(), "fortmont-agent.exe")
	if err := copyFile(executable, installedExecutable); err != nil {
		return fmt.Errorf("install agent executable: %w", err)
	}

	binPath := fmt.Sprintf(`"%s" service run`, installedExecutable)
	if out, err := exec.Command("sc.exe", "create", Name, "binPath=", binPath, "start=", "auto", "DisplayName=", "Fortmont Agent").CombinedOutput(); err != nil {
		return fmt.Errorf("create Windows service: %w: %s", err, strings.TrimSpace(string(out)))
	}
	_ = exec.Command("sc.exe", "description", Name, "Fortmont infrastructure monitoring agent").Run()
	_ = exec.Command("sc.exe", "failure", Name, "reset=", "86400", "actions=", "restart/5000/restart/10000/restart/30000").Run()

	if out, err := exec.Command("sc.exe", "start", Name).CombinedOutput(); err != nil {
		return fmt.Errorf("start Windows service: %w: %s", err, strings.TrimSpace(string(out)))
	}
	fmt.Printf("Fortmont Agent service installed successfully windowsServiceName=%s\n", Name)
	fmt.Printf("Fortmont Agent executable: %s\n", installedExecutable)
	fmt.Printf("Fortmont Agent runtime logs: %s\n", filepath.Join(ConfigDir(), "agent.log"))
	return nil
}

func copyFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()

	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0700)
	if err != nil {
		return err
	}
	defer output.Close()

	if _, err := io.Copy(output, input); err != nil {
		return err
	}
	return output.Sync()
}

func writeServiceEnv() error {
	keys := []string{
		
		"FORTMONT_VERSION",
		"FORTMONT_PUBLIC_IP",
		"FORTMONT_LOG_LEVEL",
		"FORTMONT_CONNECT_TIMEOUT_SEC",
		"FORTMONT_RECONNECT_MAX_SEC",
		"FORTMONT_HEARTBEAT_SEC",
		"FORTMONT_PING_INTERVAL_SEC",
		"FORTMONT_GATEWAY_HEALTH_CHECK_SEC",
	}
	var lines = []string{"FORTMONT_CONFIG_DIR=" + ConfigDir()}
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			lines = append(lines, key+"="+value)
		}
	}
	if err := os.WriteFile(filepath.Join(ConfigDir(), "service.env"), []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		return fmt.Errorf("write service environment: %w", err)
	}
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
	out, err := exec.Command("sc.exe", "query", Name).CombinedOutput()
	if len(out) > 0 {
		fmt.Print(string(out))
	}
	if err != nil {
		return fmt.Errorf("Fortmont Agent service is not installed or could not be queried: %w", err)
	}
	fmt.Printf("Runtime logs: %s\n", filepath.Join(ConfigDir(), "agent.log"))
	return nil
}
