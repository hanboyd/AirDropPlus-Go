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

func TestEvictedItemsAreReported(t *testing.T) {
	var evicted []Item
	s := New(2, WithEvicted(func(item Item) error { evicted = append(evicted, item); return nil }))
	s.Add(clipboard.Content{Kind: clipboard.Text, Text: "a"}, FromPC)
	s.Add(clipboard.Content{Kind: clipboard.Text, Text: "b"}, FromPC)
	s.Add(clipboard.Content{Kind: clipboard.Text, Text: "c"}, FromPC)
	if len(evicted) != 1 || evicted[0].Content.Text != "a" {
		t.Fatalf("unexpected eviction: %#v", evicted)
	}
}

func TestArchiveFailurePreservesItem(t *testing.T) {
	s := New(1, WithEvicted(func(Item) error { return errArchiveTest }))
	s.Add(clipboard.Content{Kind: clipboard.Text, Text: "a"}, FromPC)
	s.Add(clipboard.Content{Kind: clipboard.Text, Text: "b"}, FromPC)
	if got := len(s.List()); got != 2 {
		t.Fatalf("archive failure dropped content, got %d items", got)
	}
}

type archiveTestError struct{}

func (archiveTestError) Error() string { return "archive failed" }

var errArchiveTest archiveTestError
