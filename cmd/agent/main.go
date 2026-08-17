package main

import (
    "context"
    "flag"
    "log/slog"
    "os"
    "os/signal"
    "strings"
    "syscall"

    "github.com/nfworking/fortmont-agent/internal/agent"
    "github.com/nfworking/fortmont-agent/internal/config"
)

func main() {
    tokenFlag := flag.String("token", "", "one-time Fortmont enrollment token for first registration")
    flag.Parse()

    logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel(os.Getenv("FORTMONT_LOG_LEVEL"))}))
    cfg, err := config.Load()
    if err != nil {
        logger.Error("invalid configuration", "error", err)
        os.Exit(1)
    }

    enrollmentToken := strings.TrimSpace(*tokenFlag)

    instance, err := agent.New(cfg, logger)
    if err != nil {
        logger.Error("failed to initialize agent", "error", err)
        os.Exit(1)
    }

    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    if err := instance.Run(ctx, enrollmentToken); err != nil {
        logger.Error("agent stopped", "error", err)
        os.Exit(1)
    }
}

func logLevel(value string) slog.Level {
    switch strings.ToLower(strings.TrimSpace(value)) {
    case "debug": return slog.LevelDebug
    case "warn", "warning": return slog.LevelWarn
    case "error": return slog.LevelError
    default: return slog.LevelInfo
    }
}
