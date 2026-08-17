package service

import (
    "errors"
    "runtime"
)

const Name = "FortmontAgent"

var ErrUnsupported = errors.New("service management is not supported on this operating system")

func ConfigDir() string {
    switch runtime.GOOS {
    case "windows":
        return `C:\ProgramData\Fortmont\Agent`
    case "linux":
        return "/etc/fortmont-agent"
    default:
        return ""
    }
}

func Install(executable, token string) error { return install(executable, token) }
func Uninstall() error { return uninstall() }
func Status() error { return status() }
