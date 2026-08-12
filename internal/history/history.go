package history

import (
	"crypto/sha256"
	"path/filepath"
	"sync"
	"time"

	"github.com/hanboyd/AirDropPlus-Go/internal/clipboard"
)

type Source string

const (
	FromPC     Source = "pc"
	FromIPhone Source = "iphone"
)

type Item struct {
	ID      uint64
	Content clipboard.Content
	Source  Source
	Created time.Time
	Pinned  bool
	hash    [32]byte
}

type Event struct {
	Item Item
}

type Store struct {
	mu          sync.RWMutex
	limit       int
	nextID      uint64
	items       []Item
	bytes       int64
	maxBytes    int64
	subscribers map[chan Event]struct{}
}

func New(limit int) *Store {
	if limit < 1 {
		limit = 10
	}
	return &Store{limit: limit, maxBytes: 64 << 20, subscribers: make(map[chan Event]struct{})}
}

func (s *Store) Add(content clipboard.Content, source Source) Item {
	now := time.Now()
	h := contentHash(content)
	s.mu.Lock()
	if len(s.items) > 0 && s.items[0].hash == h && now.Sub(s.items[0].Created) < 2*time.Second {
		item := s.items[0]
		s.mu.Unlock()
		return item
	}
	s.nextID++
	item := Item{ID: s.nextID, Content: clone(content), Source: source, Created: now, hash: h}
	s.items = append([]Item{item}, s.items...)
	s.bytes += contentSize(item.Content)
	s.trimLocked()
	subs := make([]chan Event, 0, len(s.subscribers))
	for ch := range s.subscribers {
		subs = append(subs, ch)
	}
	s.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- Event{Item: item}:
		default:
		}
	}
	return item
}

func (s *Store) List() []Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Item, len(s.items))
	for i := range s.items {
		out[i] = s.items[i]
		out[i].Content = clone(s.items[i].Content)
	}
	return out
}

func (s *Store) Pin(id uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.items {
		if s.items[i].ID == id {
			s.items[i].Pinned = !s.items[i].Pinned
			return s.items[i].Pinned
		}
	}
	return false
}

func (s *Store) Delete(id uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.items {
		if s.items[i].ID == id {
			s.bytes -= contentSize(s.items[i].Content)
			s.items = append(s.items[:i], s.items[i+1:]...)
			return true
		}
	}
	return false
}

func (s *Store) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 8)
	s.mu.Lock()
	s.subscribers[ch] = struct{}{}
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		if _, ok := s.subscribers[ch]; ok {
			delete(s.subscribers, ch)
			close(ch)
		}
		s.mu.Unlock()
	}
}

func (s *Store) trimLocked() {
	for len(s.items) > s.limit || (s.bytes > s.maxBytes && len(s.items) > 1) {
		idx := len(s.items) - 1
		for i := len(s.items) - 1; i >= 0; i-- {
			if !s.items[i].Pinned {
				idx = i
				break
			}
		}
		s.bytes -= contentSize(s.items[idx].Content)
		s.items = append(s.items[:idx], s.items[idx+1:]...)
	}
}

func contentSize(c clipboard.Content) int64 {
	size := int64(len(c.Text) + len(c.PNG))
	for _, f := range c.Files {
		size += int64(len(f))
	}
	return size
}

func contentHash(c clipboard.Content) [32]byte {
	b := []byte(c.Kind)
	switch c.Kind {
	case clipboard.Text:
		b = append(b, c.Text...)
	case clipboard.Image:
		b = append(b, c.PNG...)
	case clipboard.Files:
		for _, f := range c.Files {
			b = append(b, filepath.Clean(f)...)
			b = append(b, 0)
		}
	}
	return sha256.Sum256(b)
}

func clone(c clipboard.Content) clipboard.Content {
	c.PNG = append([]byte(nil), c.PNG...)
	c.Files = append([]string(nil), c.Files...)
	return c
}
