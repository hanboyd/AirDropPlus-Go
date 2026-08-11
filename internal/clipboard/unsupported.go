//go:build !windows

package clipboard

import "errors"

type Windows struct{}

func New() *Windows { return &Windows{} }
func (w *Windows) Read() (Content, error) {
	return Content{}, errors.New("Windows clipboard is only available on Windows")
}
func (w *Windows) WriteText(string) error {
	return errors.New("Windows clipboard is only available on Windows")
}
func (w *Windows) WritePNG([]byte) error {
	return errors.New("Windows clipboard is only available on Windows")
}
