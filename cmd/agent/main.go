package main

import (
    "context"
    "flag"
    "fmt"
    "log/slog"
    "os"
    "os/signal"
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

    logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel(os.Getenv("FORTMONT_LOG_LEVEL"))}))
    cfg, err := config.Load()
    if err != nil {
        logger.Error("invalid configuration", "error", err)
        os.Exit(1)
    }

    enrollmentToken := strings.TrimSpace(*tokenFlag)
    if enrollmentToken == "" {
        if token, readErr := config.ReadEnrollmentToken(cfg.EnrollmentTokenPath); readErr == nil {
            enrollmentToken = token
        }
    }

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
        executable, err := os.Executable()
        if err != nil { return fmt.Errorf("resolve agent executable: %w", err) }
        return service.Install(executable, *token)

    case "status":
        return service.Status()

    case "uninstall":
        return service.Uninstall()

    case "run":
        // Managed services use an OS-level service config directory rather than
        // the interactive user's profile. The bootstrap token is stored there
        // by `service install` and is deleted after successful enrollment.
        if dir := service.ConfigDir(); dir != "" {
            if err := os.Setenv("FORTMONT_CONFIG_DIR", dir); err != nil { return err }
        }
        return runAgent()

    default:
        return fmt.Errorf("unknown service command %q; use install, status, uninstall, or run", args[0])
    }
}

func runAgent() error {
    tokenFlag := flag.NewFlagSet("service run", flag.ContinueOnError)
    token := tokenFlag.String("token", "", "one-time Fortmont enrollment token")
    if err := tokenFlag.Parse(os.Args[3:]); err != nil { return err }

    logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel(os.Getenv("FORTMONT_LOG_LEVEL"))}))
    cfg, err := config.Load()
    if err != nil { return fmt.Errorf("invalid configuration: %w", err) }

    enrollmentToken := strings.TrimSpace(*token)
    if enrollmentToken == "" {
        enrollmentToken, _ = config.ReadEnrollmentToken(cfg.EnrollmentTokenPath)
    }

    instance, err := agent.New(cfg, logger)
    if err != nil { return fmt.Errorf("failed to initialize agent: %w", err) }

    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()
    return instance.Run(ctx, enrollmentToken)
}

func logLevel(value string) slog.Level {
    switch strings.ToLower(strings.TrimSpace(value)) {
    case "debug": return slog.LevelDebug
    case "warn", "warning": return slog.LevelWarn
    case "error": return slog.LevelError
    default: return slog.LevelInfo
    }
}
