package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"linknest/server/internal/auth"
	"linknest/server/internal/config"
	"linknest/server/internal/database"
	"linknest/server/internal/device"
	"linknest/server/internal/file"
	"linknest/server/internal/httpapi"
	"linknest/server/internal/storage"
	"linknest/server/internal/task"
	lnwebsocket "linknest/server/internal/websocket"
)

func main() {
	if err := run(); err != nil {
		log.Printf("linknest server stopped with error: %v", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", defaultConfigPath(), "path to YAML config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if err := ensureStorageDirs(cfg); err != nil {
		return err
	}

	db, err := database.Open(cfg.Database)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if err := database.RunMigrations(db, filepath.Join("server", "migrations")); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	localStorage := storage.Local{
		RootDir:  cfg.Storage.RootDir,
		ChunkDir: cfg.Storage.ChunkDir,
	}

	authService := auth.NewService(db, cfg.Auth, localStorage)
	deviceService := device.NewService(db)
	fileService := file.NewService(db, localStorage)
	taskService := task.NewService(db)
	wsHandler := lnwebsocket.NewHandler(deviceService)

	handler := httpapi.NewRouter(httpapi.Dependencies{
		Auth:      authService,
		Device:    deviceService,
		File:      fileService,
		Task:      taskService,
		WebSocket: wsHandler,
		StaticDir: filepath.Join("server", "web", "static"),
	})

	server := &http.Server{
		Addr:              cfg.Server.Addr(),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	backgroundCtx, backgroundCancel := context.WithCancel(context.Background())
	defer backgroundCancel()

	go runOfflineScanner(backgroundCtx, deviceService, cfg.Transfer.OfflineTimeout())

	errCh := make(chan error, 1)
	go func() {
		log.Printf("linknest server listening on %s", server.Addr)
		errCh <- server.ListenAndServe()
	}()

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-stopCh:
		log.Printf("shutdown signal received: %s", sig.String())
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}

	backgroundCancel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown server: %w", err)
	}
	return nil
}

func defaultConfigPath() string {
	if path := os.Getenv("LINKNEST_CONFIG"); path != "" {
		return path
	}
	return filepath.Join("deploy", "config.example.yaml")
}

func ensureStorageDirs(cfg config.Config) error {
	dirs := []string{
		cfg.Storage.RootDir,
		cfg.Storage.ChunkDir,
		filepath.Dir(cfg.Database.DSN),
	}

	for _, dir := range dirs {
		if dir == "." || dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create dir %q: %w", dir, err)
		}
	}
	return nil
}

func runOfflineScanner(ctx context.Context, service *device.Service, timeout time.Duration) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := service.MarkExpiredOffline(ctx, time.Now().Add(-timeout)); err != nil {
				log.Printf("mark expired devices offline: %v", err)
			}
		}
	}
}
