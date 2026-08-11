package clipboard

import "errors"

var ErrEmpty = errors.New("Windows clipboard is empty or unsupported")

type Kind string

const (
	Text  Kind = "text"
	Image Kind = "img"
	Files Kind = "file"
)

type Content struct {
	Kind  Kind
	Text  string
	PNG   []byte
	Files []string
}

type Service interface {
	Read() (Content, error)
	WriteText(string) error
	WritePNG([]byte) error
}
