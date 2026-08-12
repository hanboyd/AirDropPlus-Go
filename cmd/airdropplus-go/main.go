package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hanboyd/AirDropPlus-Go/internal/archive"
	"github.com/hanboyd/AirDropPlus-Go/internal/bridge"
	"github.com/hanboyd/AirDropPlus-Go/internal/clipboard"
	"github.com/hanboyd/AirDropPlus-Go/internal/config"
	"github.com/hanboyd/AirDropPlus-Go/internal/device"
	"github.com/hanboyd/AirDropPlus-Go/internal/history"
	"github.com/hanboyd/AirDropPlus-Go/internal/server"
	"github.com/hanboyd/AirDropPlus-Go/internal/singleinstance"
	"github.com/hanboyd/AirDropPlus-Go/internal/ui"
)

var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	root, err := executableDir()
	if err != nil {
		return fail(err)
	}
	configPath := flag.String("config", filepath.Join(root, "data", "config.json"), "configuration file")
	showVersion := flag.Bool("version", false, "print version")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return 0
	}

	cfg, created, err := config.LoadOrCreate(*configPath)
	if err != nil {
		return fail(err)
	}
	releaseInstance, err := singleinstance.Acquire("AirDropPlus-Go")
	if err != nil {
		return fail(err)
	}
	defer releaseInstance()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	archiveWriter := archive.NewMarkdown(filepath.Join(filepath.Dir(*configPath), "clipboard-archive"))
	visibleLimit := min(cfg.HistoryLimit, 6)
	store := history.New(visibleLimit, history.WithEvicted(func(item history.Item) error {
		if err := archiveWriter.Write(item); err != nil {
			logger.Warn("clipboard archive failed", "error", err)
			return err
		}
		return nil
	}))
	clip := bridge.New(clipboard.New(), store, cfg.SharePCClipboard)
	tracker := device.New()
	app, err := server.New(cfg, clip, logger, server.WithAuthenticatedObserver(tracker.Seen), server.WithVersion(version))
	if err != nil {
		return fail(err)
	}
	httpServer := app.HTTPServer()

	pidPath := filepath.Join(filepath.Dir(*configPath), "airdropplus-go.pid")
	if err := os.WriteFile(pidPath, []byte(fmt.Sprint(os.Getpid())), 0o600); err != nil {
		return fail(fmt.Errorf("write PID file: %w", err))
	}
	defer os.Remove(pidPath)
	if created {
		fmt.Printf("Created private config: %s\n", *configPath)
		fmt.Printf("iPhone token: %s\n", cfg.Token)
	}
	fmt.Printf("AirDropPlus-Go %s listening on %s (iPhone: %s)\n", version, cfg.Listen, cfg.PublicURL)

	errCh := make(chan error, 1)
	go func() { errCh <- httpServer.ListenAndServe() }()
	uiContext, stopUI := context.WithCancel(context.Background())
	defer stopUI()
	uiErrCh := make(chan error, 1)
	go func() { uiErrCh <- ui.Run(uiContext, store, clip, tracker) }()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return fail(err)
		}
	case err := <-uiErrCh:
		if err != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = httpServer.Shutdown(ctx)
			return fail(err)
		}
	case <-stop:
		stopUI()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(ctx); err != nil {
			return fail(err)
		}
	}
	return 0
}

func executableDir() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(p), nil
}
func fail(err error) int { fmt.Fprintln(os.Stderr, "AirDropPlus-Go:", err); return 1 }
