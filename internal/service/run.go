package service

import "context"

// Run starts the agent under the operating system's service manager when
// appropriate. On non-service invocations it simply runs the supplied worker.
func Run(worker func(context.Context) error) error {
    return run(worker)
}
