//go:build !windows && !linux

package service

import "fmt"

func install(executable, token string) error { return ErrUnsupported }
func uninstall() error { return ErrUnsupported }
func status() error { return fmt.Errorf("%w: %s", ErrUnsupported, "service install/status/uninstall") }
