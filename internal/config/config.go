package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const DefaultMaxUploadBytes int64 = 1 << 30

type Config struct {
	Listen                string `json:"listen"`
	PublicURL             string `json:"public_url"`
	Token                 string `json:"token"`
	DownloadDir           string `json:"download_dir"`
	MaxUploadBytes        int64  `json:"max_upload_bytes"`
	ImageUploadClipboard  bool   `json:"image_upload_to_clipboard"`
	LegacyShortcutVersion string `json:"legacy_shortcut_version"`
	HistoryLimit          int    `json:"history_limit"`
	SharePCClipboard      bool   `json:"share_pc_clipboard"`
}

func Default(baseDir string) Config {
	return Config{
		Listen:                "0.0.0.0:53317",
		PublicURL:             "http://192.168.1.50:53317",
		DownloadDir:           filepath.Join(baseDir, "received"),
		MaxUploadBytes:        DefaultMaxUploadBytes,
		ImageUploadClipboard:  true,
		LegacyShortcutVersion: "1.5",
		HistoryLimit:          10,
		SharePCClipboard:      true,
	}
}

func LoadOrCreate(path string) (Config, bool, error) {
	baseDir := filepath.Dir(filepath.Clean(path))
	cfg := Default(filepath.Dir(baseDir))
	created := false
	b, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(b, &cfg); err != nil {
			return Config{}, false, fmt.Errorf("parse config: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Config{}, false, fmt.Errorf("read config: %w", err)
	} else {
		created = true
	}
	if cfg.Token == "" {
		token := make([]byte, 24)
		if _, err := rand.Read(token); err != nil {
			return Config{}, false, fmt.Errorf("generate token: %w", err)
		}
		cfg.Token = base64.RawURLEncoding.EncodeToString(token)
		created = true
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, false, err
	}
	if !filepath.IsAbs(cfg.DownloadDir) {
		cfg.DownloadDir = filepath.Join(filepath.Dir(baseDir), cfg.DownloadDir)
	}
	if created {
		if err := Save(path, cfg); err != nil {
			return Config{}, false, err
		}
	}
	return cfg, created, nil
}

func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.Listen) == "" {
		return errors.New("listen address is required")
	}
	if len(c.Token) < 16 {
		return errors.New("token must contain at least 16 characters")
	}
	if strings.TrimSpace(c.DownloadDir) == "" {
		return errors.New("download_dir is required")
	}
	if c.MaxUploadBytes <= 0 {
		return errors.New("max_upload_bytes must be positive")
	}
	if c.HistoryLimit < 1 || c.HistoryLimit > 100 {
		return errors.New("history_limit must be between 1 and 100")
	}
	return nil
}
