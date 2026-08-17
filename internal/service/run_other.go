//go:build !windows

package service

import "context"

func run(worker func(context.Context) error) error {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    return worker(ctx)
}
