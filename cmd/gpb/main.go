package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/a69/gpb/internal/authz"
	"github.com/a69/gpb/internal/bale"
	"github.com/a69/gpb/internal/command"
	"github.com/a69/gpb/internal/config"
	"github.com/a69/gpb/internal/github"
	"github.com/a69/gpb/internal/orchestrator"
	"github.com/a69/gpb/internal/reporter"
	"github.com/a69/gpb/internal/scheduler"
)

func main() {
	cfgPath := os.Getenv("GPB_CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "config.yaml"
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	level := slog.LevelInfo
	if cfg.Logging.Level == "debug" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	registry := authz.NewRegistry()
	for _, t := range cfg.Tenants {
		registry.Register(authz.Tenant{
			Name:            t.Name,
			GitHubToken:     t.GitHubToken,
			GitHubProjectID: t.GitHubProjectID,
			BaleToken:       t.BaleToken,
			GroupChatID:     t.GroupChatID,
			CronSpec:        t.CronSpec,
			UrgencyDays:     t.UrgencyDays,
		})
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	_ = orchestrator.New(registry, func(t authz.Tenant) reporter.GitHubClient {
		return github.NewClient(t.GitHubToken)
	}, func(t authz.Tenant) reporter.BaleClient {
		return bale.NewClient(t.BaleToken)
	})

	cmdRouter := command.NewRouter()
	cmdRouter.Register("status", func(ctx context.Context, cmd command.Command) (string, error) {
		tenant, ok := registry.ByGroup(cmd.ChatID)
		if !ok {
			return "", fmt.Errorf("unknown group")
		}
		gh := github.NewClient(tenant.GitHubToken)
		bl := bale.NewClient(tenant.BaleToken)
		r := reporter.New(gh, bl, tenant.UrgencyDays)
		go func() {
			if err := r.SendReport(context.Background(), tenant.GroupChatID, tenant.GitHubProjectID); err != nil {
				slog.Error("command report failed", "tenant", tenant.Name, "err", err)
			}
		}()
		return "Fetching report...", nil
	})

	guard := authz.NewGroupGuard(registry, cfg.Server.WebhookSecret)

	sched := scheduler.New()
	for _, t := range registry.All() {
		t := t
		sched.Add(t.CronSpec, func() {
			slog.Info("cron triggered", "tenant", t.Name)
			gh := github.NewClient(t.GitHubToken)
			bl := bale.NewClient(t.BaleToken)
			r := reporter.New(gh, bl, t.UrgencyDays)
			if err := r.SendReport(context.Background(), t.GroupChatID, t.GitHubProjectID); err != nil {
				slog.Error("cron report failed", "tenant", t.Name, "err", err)
			}
		})
	}
	sched.Start()
	defer sched.Stop()

	mux := http.NewServeMux()
	mux.HandleFunc("POST "+cfg.Server.WebhookPath, bale.WebhookHandler(guard, cmdRouter))
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := &http.Server{Addr: cfg.Server.Listen, Handler: mux}
	go func() {
		slog.Info("listening", "addr", cfg.Server.Listen)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	srv.Shutdown(shutdownCtx)
}
