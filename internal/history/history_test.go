package history

import (
	"github.com/hanboyd/AirDropPlus-Go/internal/clipboard"
	"testing"
)

func TestLimitPinDeleteAndDeduplicate(t *testing.T) {
	s := New(2)
	a := s.Add(clipboard.Content{Kind: clipboard.Text, Text: "a"}, FromPC)
	if got := s.Add(clipboard.Content{Kind: clipboard.Text, Text: "a"}, FromIPhone); got.ID != a.ID {
		t.Fatal("duplicate was added")
	}
	s.Pin(a.ID)
	s.Add(clipboard.Content{Kind: clipboard.Text, Text: "b"}, FromPC)
	s.Add(clipboard.Content{Kind: clipboard.Text, Text: "c"}, FromPC)
	items := s.List()
	if len(items) != 2 || items[1].ID != a.ID || !items[1].Pinned {
		t.Fatalf("unexpected items: %#v", items)
	}
	if !s.Delete(a.ID) || len(s.List()) != 1 {
		t.Fatal("delete failed")
	}
}

func TestImageHistoryMemoryIsBounded(t *testing.T) {
	s := New(10)
	s.maxBytes = 10
	s.Add(clipboard.Content{Kind: clipboard.Image, PNG: []byte("12345678")}, FromPC)
	s.Add(clipboard.Content{Kind: clipboard.Image, PNG: []byte("abcdefgh")}, FromIPhone)
	if got := len(s.List()); got != 1 {
		t.Fatalf("expected newest image only, got %d", got)
	}
}
