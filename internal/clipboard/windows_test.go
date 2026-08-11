//go:build windows

package clipboard

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestDIBPNGRoundTrip(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	img.SetNRGBA(2, 1, color.NRGBA{B: 255, A: 255})
	b, err := dibToPNG(imageToDIB(img))
	if err != nil {
		t.Fatal(err)
	}
	got, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds() != img.Bounds() {
		t.Fatalf("bounds=%v", got.Bounds())
	}
	r, _, b0, _ := got.At(0, 0).RGBA()
	if r < 60000 || b0 > 1000 {
		t.Fatalf("top-left color incorrect: %v", got.At(0, 0))
	}
	_, _, b1, _ := got.At(2, 1).RGBA()
	if b1 < 60000 {
		t.Fatalf("bottom-right color incorrect: %v", got.At(2, 1))
	}
}
