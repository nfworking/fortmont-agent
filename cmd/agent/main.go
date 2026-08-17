package main

import (
    "context"
    "flag"
    "fmt"
    "io"
    "log/slog"
    "os"
    "os/signal"
    "path/filepath"
    "strings"
    "syscall"

    "github.com/nfworking/fortmont-agent/internal/agent"
    "github.com/nfworking/fortmont-agent/internal/config"
    "github.com/nfworking/fortmont-agent/internal/service"
)

func main() {
    if len(os.Args) >= 2 && os.Args[1] == "service" {
        if err := handleServiceCommand(os.Args[2:]); err != nil {
            fmt.Fprintln(os.Stderr, err)
            os.Exit(1)
        }
        return
    }

    tokenFlag := flag.String("token", "", "one-time Fortmont enrollment token for first registration")
    flag.Parse()

    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    if err := runAgent(ctx, strings.TrimSpace(*tokenFlag)); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}

func handleServiceCommand(args []string) error {
    if len(args) == 0 {
        return fmt.Errorf("usage: fortmont-agent service <install|status|uninstall|run>")
    }

    switch args[0] {
    case "install":
        flags := flag.NewFlagSet("service install", flag.ContinueOnError)
        token := flags.String("token", "", "one-time Fortmont enrollment token")
        if err := flags.Parse(args[1:]); err != nil { return err }
        if strings.TrimSpace(*token) == "" { return fmt.Errorf("service install requires --token") }

        // Load .env before writing the service environment. This makes
        // `service install` behave consistently with an interactive run.
        if _, err := config.Load(); err != nil { return fmt.Errorf("invalid configuration: %w", err) }

        executable, err := os.Executable()
        if err != nil { return fmt.Errorf("resolve agent executable: %w", err) }
        return service.Install(executable, *token)

    case "status":
        return service.Status()

    case "uninstall":
        return service.Uninstall()

    case "run":
        if dir := service.ConfigDir(); dir != "" {
            if err := os.Setenv("FORTMONT_CONFIG_DIR", dir); err != nil { return err }
        }

        tokenFlag := flag.NewFlagSet("service run", flag.ContinueOnError)
        token := tokenFlag.String("token", "", "one-time Fortmont enrollment token")
        if err := tokenFlag.Parse(args[1:]); err != nil { return err }

        return service.Run(func(ctx context.Context) error {
            return runAgent(ctx, strings.TrimSpace(*token))
        })

    default:
        return fmt.Errorf("unknown service command %q; use install, status, uninstall, or run", args[0])
    }
}

func runAgent(ctx context.Context, token string) error {
    logger, closeLog, err := newLogger()
    if err != nil { return err }
    defer closeLog()

    cfg, err := config.Load()
    if err != nil {
        logger.Error("invalid configuration", "error", err)
        return err
    }

    enrollmentToken := token
    if enrollmentToken == "" {
        if stored, readErr := config.ReadEnrollmentToken(cfg.EnrollmentTokenPath); readErr == nil {
            enrollmentToken = stored
        }
    }

    instance, err := agent.New(cfg, logger)
    if err != nil {
        logger.Error("failed to initialize agent", "error", err)
        return err
    }

    logger.Info("starting Fortmont Agent", "version", cfg.Version, "gateway_count", len(cfg.WSNodes))
    if err := instance.Run(ctx, enrollmentToken); err != nil {
        logger.Error("agent stopped", "error", err)
        return err
    }
    logger.Info("agent stopped cleanly")
    return nil
}

func newLogger() (*slog.Logger, func(), error) {
    level := logLevel(os.Getenv("FORTMONT_LOG_LEVEL"))
    handlerOptions := &slog.HandlerOptions{Level: level}

    // Windows/Linux services do not have a useful interactive stdout. Keep
    // stdout for normal CLI runs and persist service diagnostics beside the
    // service credentials so authentication/reconnect failures are inspectable.
    dir := service.ConfigDir()
    if os.Getenv("FORTMONT_CONFIG_DIR") != "" {
        dir = os.Getenv("FORTMONT_CONFIG_DIR")
    }
    if dir == "" {
        return slog.New(slog.NewTextHandler(os.Stdout, handlerOptions)), func() {}, nil
    }

    if err := os.MkdirAll(dir, 0700); err != nil {
        return nil, nil, fmt.Errorf("create agent log directory: %w", err)
    }

    logPath := filepath.Join(dir, "agent.log")
    file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
    if err != nil { return nil, nil, fmt.Errorf("open agent log: %w", err) }

    writer := io.MultiWriter(os.Stdout, file)
    logger := slog.New(slog.NewTextHandler(writer, handlerOptions))
    return logger, func() { _ = file.Close() }, nil
}

func logLevel(value string) slog.Level {
    switch strings.ToLower(strings.TrimSpace(value)) {
    case "debug": return slog.LevelDebug
    case "warn", "warning": return slog.LevelWarn
    case "error": return slog.LevelError
    default: return slog.LevelInfo
    }
}
