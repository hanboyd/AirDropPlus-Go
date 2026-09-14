package device

import (
	"sync"
	"time"
)

type State struct {
	IP       string
	LastSeen time.Time
}

type Tracker struct {
	mu      sync.RWMutex
	state   State
	changed chan struct{}
}

func New() *Tracker { return &Tracker{changed: make(chan struct{}, 1)} }
func (t *Tracker) Seen(ip string) {
	t.mu.Lock()
	t.state = State{IP: ip, LastSeen: time.Now()}
	t.mu.Unlock()
	select {
	case t.changed <- struct{}{}:
	default:
	}
}
func (t *Tracker) State() State { t.mu.RLock(); defer t.mu.RUnlock(); return t.state }
func (t *Tracker) Online() bool {
	s := t.State()
	return !s.LastSeen.IsZero() && time.Since(s.LastSeen) < 2*time.Minute
}
func (t *Tracker) Changed() <-chan struct{} { return t.changed }
