package bridge

import (
	"errors"
	"testing"

	"github.com/hanboyd/AirDropPlus-Go/internal/clipboard"
	"github.com/hanboyd/AirDropPlus-Go/internal/history"
)

type fakeClipboard struct{ content clipboard.Content }

func (f *fakeClipboard) Read() (clipboard.Content, error) {
	if f.content.Kind == "" {
		return clipboard.Content{}, clipboard.ErrEmpty
	}
	return f.content, nil
}
func (f *fakeClipboard) WriteText(v string) error {
	if v == "" {
		return errors.New("empty")
	}
	f.content = clipboard.Content{Kind: clipboard.Text, Text: v}
	return nil
}
func (f *fakeClipboard) WritePNG(v []byte) error {
	f.content = clipboard.Content{Kind: clipboard.Image, PNG: append([]byte(nil), v...)}
	return nil
}

func TestSharingGateAndRemoteHistory(t *testing.T) {
	f := &fakeClipboard{content: clipboard.Content{Kind: clipboard.Text, Text: "pc"}}
	store := history.New(10)
	service := New(f, store, false)
	if _, err := service.Read(); !errors.Is(err, ErrPCSharingDisabled) {
		t.Fatalf("unexpected error: %v", err)
	}
	service.SetSharingPC(true)
	if c, err := service.Read(); err != nil || c.Text != "pc" {
		t.Fatalf("read failed: %#v %v", c, err)
	}
	if err := service.WriteText("iphone"); err != nil {
		t.Fatal(err)
	}
	items := store.List()
	if len(items) != 1 || items[0].Source != history.FromIPhone || items[0].Content.Text != "iphone" {
		t.Fatalf("unexpected history: %#v", items)
	}
}
