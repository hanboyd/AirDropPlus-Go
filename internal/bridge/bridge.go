package bridge

import (
	"errors"
	"sync/atomic"

	"github.com/hanboyd/AirDropPlus-Go/internal/clipboard"
	"github.com/hanboyd/AirDropPlus-Go/internal/history"
)

var ErrPCSharingDisabled = errors.New("PC clipboard sharing is disabled")

type Service struct {
	inner   clipboard.Service
	store   *history.Store
	sharePC atomic.Bool
}

func New(inner clipboard.Service, store *history.Store, sharePC bool) *Service {
	s := &Service{inner: inner, store: store}
	s.sharePC.Store(sharePC)
	return s
}

func (s *Service) Read() (clipboard.Content, error) {
	if !s.sharePC.Load() {
		return clipboard.Content{}, ErrPCSharingDisabled
	}
	return s.inner.Read()
}

func (s *Service) ReadLocal() (clipboard.Content, error) { return s.inner.Read() }
func (s *Service) SharingPC() bool                       { return s.sharePC.Load() }
func (s *Service) SetSharingPC(v bool)                   { s.sharePC.Store(v) }
func (s *Service) ToggleSharingPC() bool {
	next := !s.sharePC.Load()
	s.sharePC.Store(next)
	return next
}

func (s *Service) RecordLocal(c clipboard.Content) { s.store.Add(c, history.FromPC) }

func (s *Service) CopyToPC(c clipboard.Content) error {
	var err error
	switch c.Kind {
	case clipboard.Text:
		err = s.inner.WriteText(c.Text)
	case clipboard.Image:
		err = s.inner.WritePNG(c.PNG)
	default:
		err = errors.New("only text and images can be copied")
	}
	if err == nil {
		s.store.Add(c, history.FromPC)
	}
	return err
}

func (s *Service) WriteText(v string) error {
	if err := s.inner.WriteText(v); err != nil {
		return err
	}
	s.store.Add(clipboard.Content{Kind: clipboard.Text, Text: v}, history.FromIPhone)
	return nil
}

func (s *Service) WritePNG(v []byte) error {
	if err := s.inner.WritePNG(v); err != nil {
		return err
	}
	s.store.Add(clipboard.Content{Kind: clipboard.Image, PNG: append([]byte(nil), v...)}, history.FromIPhone)
	return nil
}
