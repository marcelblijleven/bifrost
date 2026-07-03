package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/marcelblijleven/bifrost/internal/api"
	"github.com/marcelblijleven/bifrost/internal/config"
	"github.com/marcelblijleven/bifrost/internal/pipeline"
	"github.com/marcelblijleven/bifrost/internal/pipeline/steps"
	"github.com/marcelblijleven/bifrost/internal/provider"
	giteaprovider "github.com/marcelblijleven/bifrost/internal/provider/gitea"
	ghprovider "github.com/marcelblijleven/bifrost/internal/provider/github"
	"github.com/marcelblijleven/bifrost/internal/sse"
	"github.com/marcelblijleven/bifrost/internal/store"
	pgstore "github.com/marcelblijleven/bifrost/internal/store/postgres"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st, err := pgstore.New(ctx, cfg.DatabaseURL, store.Migrations)
	if err != nil {
		slog.Error("connect to database", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	providers := map[string]provider.Provider{}

	ghCfg := cfg.GitHub
	switch {
	case ghCfg.AppID != 0 && ghCfg.InstallationID != 0 && ghCfg.PrivateKey != "":
		p, err := ghprovider.NewFromApp(ghCfg.AppID, ghCfg.InstallationID, []byte(ghCfg.PrivateKey), ghCfg.BaseURL, ghCfg.UploadURL)
		if err != nil {
			slog.Error("init github app provider", "err", err)
			os.Exit(1)
		}
		providers["github"] = p
		slog.Info("registered provider", "provider", "github", "auth", "app", "base_url", ghCfg.BaseURL)
	case ghCfg.Token != "":
		p, err := ghprovider.NewFromToken(ghCfg.Token, ghCfg.BaseURL, ghCfg.UploadURL)
		if err != nil {
			slog.Error("init github token provider", "err", err)
			os.Exit(1)
		}
		providers["github"] = p
		slog.Info("registered provider", "provider", "github", "auth", "token", "base_url", ghCfg.BaseURL)
	}

	if cfg.Gitea.URL != "" && cfg.Gitea.Token != "" {
		providers["gitea"] = giteaprovider.New("gitea", cfg.Gitea.URL, cfg.Gitea.Token)
		slog.Info("registered provider", "provider", "gitea", "url", cfg.Gitea.URL)
	}

	if cfg.Forgejo.URL != "" && cfg.Forgejo.Token != "" {
		providers["forgejo"] = giteaprovider.New("forgejo", cfg.Forgejo.URL, cfg.Forgejo.Token)
		slog.Info("registered provider", "provider", "forgejo", "url", cfg.Forgejo.URL)
	}

	reg := pipeline.NewRegistry()
	reg.Register("semver", steps.NewSemverStep)
	reg.Register("tag", func(cfg map[string]any) (pipeline.Step, error) {
		return &steps.TagStep{}, nil
	})
	reg.Register("changelog", func(cfg map[string]any) (pipeline.Step, error) {
		return &steps.ChangelogStep{}, nil
	})
	reg.Register("dispatch_workflow", steps.NewDispatchStep)
	reg.Register("approval", steps.NewApprovalStep)
	reg.Register("create_release", steps.NewCreateReleaseStep)
	reg.Register("notify", steps.NewNotifyStep)

	broker := sse.New()
	h := api.NewHandler(st, providers, reg, cfg.JWTSecret, cfg.PublicURL, broker)
	h.Start(ctx)

	router := api.NewRouter(h, cfg.APIKey, cfg.JWTSecret)

	srv := &http.Server{
		Addr:        cfg.HTTPAddr,
		Handler:     router,
		ReadTimeout: 15 * time.Second,
		// WriteTimeout is intentionally omitted: SSE connections are long-lived
		// and manage their own write deadlines via http.ResponseController.
		IdleTimeout: 60 * time.Second,
	}

	slog.Info("bifrost starting", "addr", cfg.HTTPAddr, "version", "dev")

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed", "err", err)
	}

	slog.Info("bifrost stopped")
}
