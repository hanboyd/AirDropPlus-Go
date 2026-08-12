package archive

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanboyd/AirDropPlus-Go/internal/clipboard"
	"github.com/hanboyd/AirDropPlus-Go/internal/history"
)

func TestMarkdownArchivesTextImageAndTimestamp(t *testing.T) {
	dir := t.TempDir()
	w := NewMarkdown(dir)
	when := time.Date(2026, 8, 12, 18, 30, 4, 123, time.FixedZone("CST", 8*60*60))
	if err := w.Write(history.Item{ID: 1, Created: when, Source: history.FromIPhone, Content: clipboard.Content{Kind: clipboard.Text, Text: "full text\n```"}}); err != nil {
		t.Fatal(err)
	}
	if err := w.Write(history.Item{ID: 2, Created: when, Source: history.FromPC, Content: clipboard.Content{Kind: clipboard.Image, PNG: []byte("png")}}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "2026-08-12.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "2026-08-12T18:30:04.000000123+08:00") || !strings.Contains(s, "full text") || !strings.Contains(s, "assets/") {
		t.Fatalf("archive missing content: %s", s)
	}
	assets, err := filepath.Glob(filepath.Join(dir, "assets", "*.png"))
	if err != nil || len(assets) != 1 {
		t.Fatalf("image was not archived: %v %v", assets, err)
	}
}
