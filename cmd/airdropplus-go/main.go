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

	"github.com/hanboyd/AirDropPlus-Go/internal/clipboard"
	"github.com/hanboyd/AirDropPlus-Go/internal/config"
	"github.com/hanboyd/AirDropPlus-Go/internal/server"
)

var version = "dev"

func main() {
	root, err := executableDir()
	if err != nil {
		fatal(err)
	}
	configPath := flag.String("config", filepath.Join(root, "data", "config.json"), "configuration file")
	showVersion := flag.Bool("version", false, "print version")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}

	cfg, created, err := config.LoadOrCreate(*configPath)
	if err != nil {
		fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	app, err := server.New(cfg, clipboard.New(), logger)
	if err != nil {
		fatal(err)
	}
	httpServer := app.HTTPServer()

	pidPath := filepath.Join(filepath.Dir(*configPath), "airdropplus-go.pid")
	_ = os.WriteFile(pidPath, []byte(fmt.Sprint(os.Getpid())), 0o600)
	defer os.Remove(pidPath)
	if created {
		fmt.Printf("Created private config: %s\n", *configPath)
		fmt.Printf("iPhone token: %s\n", cfg.Token)
	}
	fmt.Printf("AirDropPlus-Go %s listening on %s (iPhone: %s)\n", version, cfg.Listen, cfg.PublicURL)

	errCh := make(chan error, 1)
	go func() { errCh <- httpServer.ListenAndServe() }()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			fatal(err)
		}
	case <-stop:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	}
}

func executableDir() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(p), nil
}
func fatal(err error) { fmt.Fprintln(os.Stderr, "AirDropPlus-Go:", err); os.Exit(1) }
