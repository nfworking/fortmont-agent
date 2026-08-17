//go:build windows

package service

import (
    "context"
    "errors"
    "fmt"
    "sync"

    "golang.org/x/sys/windows/svc"
)

func run(worker func(context.Context) error) error {
    if !svc.IsWindowsService() {
        ctx, cancel := context.WithCancel(context.Background())
        defer cancel()
        return worker(ctx)
    }

    return svc.Run(Name, &windowsService{worker: worker})
}

type windowsService struct {
    worker func(context.Context) error
}

func (s *windowsService) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
    status <- svc.Status{State: svc.StartPending, WaitHint: 5000}

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    done := make(chan error, 1)
    var once sync.Once
    go func() {
        done <- s.worker(ctx)
    }()

    status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

    for {
        select {
        case err := <-done:
            once.Do(cancel)
            if err != nil && !errors.Is(err, context.Canceled) {
                return false, 1
            }
            return false, 0

        case request := <-requests:
            switch request.Cmd {
            case svc.Interrogate:
                status <- requestCurrentStatus()
            case svc.Stop, svc.Shutdown:
                status <- svc.Status{State: svc.StopPending, WaitHint: 5000}
                cancel()
                err := <-done
                if err != nil && !errors.Is(err, context.Canceled) {
                    return false, 1
                }
                return false, 0
            default:
                // Ignore unsupported service controls.
            }
        }
    }
}

func requestCurrentStatus() svc.Status {
    return svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
}

var _ = fmt.Sprintf
