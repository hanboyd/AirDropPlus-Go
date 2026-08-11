package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreate(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "data", "config.json")
	cfg, created, err := LoadOrCreate(path)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected config to be created")
	}
	if len(cfg.Token) < 16 {
		t.Fatalf("weak token: %q", cfg.Token)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	cfg2, created2, err := LoadOrCreate(path)
	if err != nil {
		t.Fatal(err)
	}
	if created2 {
		t.Fatal("existing config should not be rewritten")
	}
	if cfg2.Token != cfg.Token {
		t.Fatal("token changed")
	}
}

func TestRejectsShortToken(t *testing.T) {
	c := Default(t.TempDir())
	c.Token = "short"
	if err := c.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}
