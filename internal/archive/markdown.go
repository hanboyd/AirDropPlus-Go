package archive

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hanboyd/AirDropPlus-Go/internal/clipboard"
	"github.com/hanboyd/AirDropPlus-Go/internal/history"
)

type Markdown struct {
	dir string
	mu  sync.Mutex
}

func NewMarkdown(dir string) *Markdown { return &Markdown{dir: dir} }

func (m *Markdown) Write(item history.Item) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := os.MkdirAll(m.dir, 0o700); err != nil {
		return fmt.Errorf("create clipboard archive: %w", err)
	}
	stamp := item.Created.Format(time.RFC3339Nano)
	source := "PC"
	if item.Source == history.FromIPhone {
		source = "iPhone"
	}
	var body string
	switch item.Content.Kind {
	case clipboard.Text:
		body = fenced(item.Content.Text)
	case clipboard.Image:
		assets := filepath.Join(m.dir, "assets")
		if err := os.MkdirAll(assets, 0o700); err != nil {
			return fmt.Errorf("create clipboard archive assets: %w", err)
		}
		name := fmt.Sprintf("%s-%d.png", item.Created.Format("20060102-150405.000000000"), item.ID)
		if err := os.WriteFile(filepath.Join(assets, name), item.Content.PNG, 0o600); err != nil {
			return fmt.Errorf("write archived clipboard image: %w", err)
		}
		body = fmt.Sprintf("![剪贴板图片](assets/%s)\n", name)
	case clipboard.Files:
		var b strings.Builder
		for _, path := range item.Content.Files {
			fmt.Fprintf(&b, "- `%s`\n", strings.ReplaceAll(path, "`", "\\`"))
		}
		body = b.String()
	default:
		body = "（无法识别的剪贴板格式）\n"
	}
	entry := fmt.Sprintf("## %s · %s · %s\n\n%s\n", stamp, source, item.Content.Kind, body)
	path := filepath.Join(m.dir, item.Created.Format("2006-01-02")+".md")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open clipboard archive document: %w", err)
	}
	defer f.Close()
	if info, err := f.Stat(); err == nil && info.Size() == 0 {
		if _, err := fmt.Fprintf(f, "# AirDropPlus-Go 剪贴板归档 · %s\n\n", item.Created.Format("2006-01-02")); err != nil {
			return err
		}
	}
	_, err = f.WriteString(entry)
	return err
}

func fenced(text string) string {
	fence := "```"
	for strings.Contains(text, fence) {
		fence += "`"
	}
	return fence + "text\n" + text + "\n" + fence + "\n"
}
